/*
 *
 * Copyright 2025 Platfor.me (https://platfor.me)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package client implements a pubsub client with load balancing across brokers.
package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/myplatforme/glider-client-go/authhmac"
	"github.com/myplatforme/glider-client-go/broker"
	"github.com/myplatforme/glider-client-go/callback"
	"github.com/myplatforme/glider-client-go/httpclient"
	"github.com/myplatforme/glider-client-go/metadata"
	pb "github.com/myplatforme/glider-client-go/proto"
	"github.com/myplatforme/glider-client-go/pub"
	"github.com/myplatforme/glider-client-go/sub"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrClientNotConnected is returned when a message is sent with no broker connected.
	ErrClientNotConnected = errors.New("client is not connected")
	// resetDelay is the wait time before a full connection reset.
	// It gives brokers a chance to recover without forcing a restart.
	resetDelay = 13 * time.Second
)

// Pub is the interface for a published event descriptor.
type Pub[CTX proto.Message] interface {
	// Sub waits for the response event with the given name.
	Sub(event string) (context.Context, []byte, error)
	// Do sends the event without waiting for a response.
	Do() error
}

// Client implements a pubsub client with support for multiple brokers.
type Client[CTX proto.Message] struct {
	// Project is the project identifier.
	Project string
	// Module is the module identifier within the project.
	Module string
	// PID is the unique process identifier; taken from HOSTNAME or generated.
	PID string
	// Subs is the list of events the client subscribes to.
	Subs []string
	// Pubs is the list of events the client publishes.
	Pubs []string
	// AuthSkew is the allowed clock drift for HMAC authentication.
	AuthSkew time.Duration
	// AuthTTL is the maximum request age for HMAC authentication.
	AuthTTL time.Duration
	// Secret is the shared secret for HMAC authentication.
	Secret string
	// Host is the broker server address.
	Host string
	// Certs maps broker IDs to their CA certificate file paths.
	Certs map[string]string

	// Callbacks holds registered channels waiting for response messages.
	Callbacks *callback.Callback
	// SubsStore holds active subscriptions and routes incoming messages.
	SubsStore *sub.Store

	// brokers holds all known brokers, including offline ones.
	brokers *broker.Store[CTX]
	// currentSend is an atomic counter for round-robin send distribution.
	currentSend atomic.Uint64

	rw sync.RWMutex
	// onlineBrokers contains only the currently connected brokers.
	onlineBrokers []*broker.Broker[CTX]
	// online is true when at least one broker is connected.
	online atomic.Bool
	// resetProcess is true while a deferred connection reset is in progress.
	resetProcess atomic.Bool
	// overstayProcess is true while waiting before a forced reconnect.
	overstayProcess atomic.Bool
	// stopResetProcess signals the reset goroutine that the connection has recovered.
	stopResetProcess chan struct{}
}

// Sub registers handler fn for the event named name.
// The subscription is automatically removed when ctx is cancelled.
func (c *Client[CTX]) Sub(ctx context.Context, name string, fn func(ctx context.Context, payload []byte)) {
	id := c.SubsStore.Register(name, fn)
	go func() {
		<-ctx.Done()
		c.SubsStore.Unregister(name, id)
	}()
}

// Pub publishes payload for the event named name and returns an event descriptor.
func (c *Client[CTX]) Pub(ctx context.Context, name string, payload []byte) Pub[CTX] {
	id := uuid.New().String()
	md := metadata.MetaFromContext(ctx)
	md.ID = id

	return &pub.Pub[CTX]{
		ID:               id,
		PID:              c.PID,
		Name:             name,
		Payload:          payload,
		Context:          md.WithContext(ctx),
		Timestamp:        time.Now(),
		SendFunc:         c.send,
		RegisterCbFunc:   c.Callbacks.Register,
		UnregisterCbFunc: c.Callbacks.Unregister,
	}
}

// Start initialises HMAC authentication, fetches the broker list,
// and launches a connection to each broker.
//
// NOTE: if the HOSTNAME environment variable is absent, a random UUID is used
// as the PID to guarantee process uniqueness.
func (c *Client[CTX]) Start() {
	// Recreate the channel on every start so stale reset goroutines from a
	// previous cycle do not receive a signal meant for the new one.
	c.stopResetProcess = make(chan struct{})

	auth := authhmac.NewAuth(authhmac.Options{
		Skew:   c.AuthSkew,
		TTL:    c.AuthTTL,
		Secret: c.Secret,
	})

	// ch is used only for the initial broker list request; subsequent
	// communication goes directly through each broker's gRPC stream.
	ch := httpclient.New(httpclient.Options{
		BaseURL: c.Host,
		Module:  c.Module,
		Project: c.Project,
		Auth:    auth,
	})

	// broker.Request returns a closure rather than the result immediately,
	// so requestBrokers() can be called again on reconnect without rebuilding parameters.
	requestBrokers := broker.Request[CTX](ch, broker.RequestOptions{
		Project: c.Project,
		Module:  c.Module,
		Subs:    c.Subs,
		Pubs:    c.Pubs,
	})

	brokers, err := requestBrokers()
	if err != nil {
		// Without brokers the client is non-functional and recovery is impossible.
		panic(err)
	}

	pid := os.Getenv("HOSTNAME")
	if pid == "" {
		pid = uuid.New().String()
	}
	c.PID = pid

	if c.brokers == nil {
		// Initialise the store only on first start; on reconnect the existing
		// store is reused to preserve broker metadata.
		c.brokers = broker.New[CTX]()
	}

	fmt.Printf("Connected to %d brokers\n", len(brokers))

	for _, bk := range brokers {
		if crt, ok := c.Certs[bk.ID]; ok {
			bk.SetCertPath(crt)
		}
		bk.DisconnectFunc = c.disconnect
		bk.ResetFunc = c.reset
		bk.ConnectFunc = c.connect
		bk.RecvFunc = c.recv
		bk.OwerstayingFunc = c.owerstaying
		// PID is passed to the interceptor so the broker can identify
		// the specific client instance during authentication.
		bk.Interceptor = broker.AuthInterceptor(auth, c.Project, c.Module, pid)
		c.brokers.Set(bk)
		go bk.Start()
	}

	// Reset flags after all brokers are started, not before, to prevent
	// re-entry into reset/overstay before Start finishes.
	c.overstayProcess.Store(false)
	c.resetProcess.Store(false)
}

// recv routes an incoming message to the subscription store and registered callbacks.
func (c *Client[CTX]) recv(ctx context.Context, msg *pb.Message) {
	c.SubsStore.Recv(ctx, msg)
	c.Callbacks.Recv(msg)
}

// send delivers an event through the next available broker.
// On send failure it recursively tries the next broker.
//
// WARNING: recursion terminates with ErrClientNotConnected only when
// all brokers go offline.
func (c *Client[CTX]) send(pub *pub.Pub[CTX]) error {
	bk, err := c.nextBroker()
	if err != nil {
		return err
	}
	if err := bk.Send(pub); err != nil {
		return c.send(pub)
	}
	return nil
}

// disconnect removes the broker from the online list and, if a reset is in
// progress and the client has gone fully offline, signals the reset goroutine.
func (c *Client[CTX]) disconnect(id string) {
	c.delete(id)
	c.rw.RLock()
	defer c.rw.RUnlock()
	// Signal only when a reset is in progress AND the client is offline;
	// that means reconnection did not happen and the reset can be aborted.
	if c.resetProcess.Load() && c.online.Load() || !c.resetProcess.Load() {
		return
	}
	c.stopResetProcess <- struct{}{}
}

// connect moves the broker with the given id into the online list.
func (c *Client[CTX]) connect(id string) {
	bk, ok := c.brokers.Get(id)
	if !ok {
		return
	}
	c.set(bk)
}

// set adds a broker to the online list, replacing any existing entry with the same ID.
func (c *Client[CTX]) set(bk *broker.Broker[CTX]) {
	c.rw.Lock()
	defer c.rw.Unlock()
	// Rebuild the list excluding the old entry for the same broker
	// to avoid duplicates after a reconnect.
	var next []*broker.Broker[CTX]
	for _, set := range c.onlineBrokers {
		if set.ID != bk.ID {
			next = append(next, bk)
		}
	}
	next = append(next, bk)
	c.onlineBrokers = next
	c.online.Store(true)
	c.overstayProcess.Store(false)
	// Reset the round-robin counter so the new broker participates in rotation immediately.
	c.currentSend.Store(0)
}

// delete removes the broker with the given ID from the online list.
func (c *Client[CTX]) delete(id string) {
	c.rw.Lock()
	defer c.rw.Unlock()
	var next []*broker.Broker[CTX]
	for _, bk := range c.onlineBrokers {
		if bk.ID != id {
			next = append(next, bk)
		}
	}
	c.onlineBrokers = next
	c.online.Store(len(next) > 0)
	c.currentSend.Store(0)
}

// reset starts a deferred reconnect process. A second call while the first
// is still running is ignored.
//
// NOTE: the 13-second timer gives brokers time to recover before a full
// connection reset and another Start call.
func (c *Client[CTX]) reset() {
	c.rw.Lock()
	defer c.rw.Unlock()
	if c.resetProcess.Load() {
		return
	}
	c.resetProcess.Store(true)

	go func() {
		timer := time.NewTimer(resetDelay)
		// Wait for either a successful-reconnect signal or the timer expiry;
		// both cases exit the select and proceed to the reset.
		select {
		case <-c.stopResetProcess:
			break
		case <-timer.C:
			break
		}

		fmt.Println("Init reset process...")
		timer.Stop()
		c.brokers.Done()
		c.Start()
	}()
}

// owerstaying handles the situation where the client has been without a
// connection for too long. It starts a reconnect if no brokers are online
// and neither a reset nor an overstay process is already running.
func (c *Client[CTX]) owerstaying() bool {
	c.rw.Lock()
	defer c.rw.Unlock()
	if c.resetProcess.Load() || c.online.Load() || c.overstayProcess.Load() {
		return false
	}
	c.overstayProcess.Store(true)
	// NOTE: sleeping under the mutex intentionally blocks concurrent disconnect/connect
	// calls for the duration of the wait, ensuring the reset process can complete.
	time.Sleep(time.Second * 6)
	c.brokers.Done()
	c.Start()
	return true
}

// nextBroker returns the next online broker using round-robin selection.
func (c *Client[CTX]) nextBroker() (*broker.Broker[CTX], error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	if !c.online.Load() {
		return nil, ErrClientNotConnected
	}
	n := uint64(len(c.onlineBrokers))
	if n == 0 {
		return nil, ErrClientNotConnected
	}
	// NOTE: atomic increment with modulo provides even distribution without write locks.
	idx := (c.currentSend.Add(1) - 1) % n
	return c.onlineBrokers[idx], nil
}
