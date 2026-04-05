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

// Package broker manages the gRPC connection to an individual message broker.
package broker

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/myplatforme/glider-client-go/message"
	"github.com/myplatforme/glider-client-go/metadata"
	pb "github.com/myplatforme/glider-client-go/proto"
	"github.com/myplatforme/glider-client-go/pub"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// ErrInvalidResponseBody is returned when the broker response body is invalid.
	ErrInvalidResponseBody = errors.New("invalid response body")
	// ErrBrokerDisconnected is returned when attempting to send through a broken connection.
	ErrBrokerDisconnected = errors.New("broker is disconnected")
	// ErrBrokerDialFailed is returned when a gRPC connection cannot be established.
	ErrBrokerDialFailed = errors.New("broker dial failed")
	// ErrBrokerCertFailed is returned when the TLS certificate cannot be loaded.
	ErrBrokerCertFailed = errors.New("broker cert failed")

	// durationMax is the maximum reconnect delay in seconds (Fibonacci-bounded).
	durationMax = 24
	// overstayAttempts is the number of retries at maximum delay before
	// delegating control to the OwerstayingFunc handler.
	overstayAttempts = 2
)

// CallbackFunc is a callback that receives the broker ID as its argument.
type CallbackFunc func(id string)

// RecvFunc is a handler for incoming messages from the broker.
type RecvFunc func(ctx context.Context, evt *pb.Message)

// Broker manages the gRPC connection to a single broker and its lifecycle.
type Broker[CTX proto.Message] struct {
	// ID is the unique identifier of the broker.
	ID string `json:"id"`
	// Host is the broker's hostname or IP address.
	Host string `json:"host"`
	// Port is the broker's port number.
	Port int `json:"port"`
	// Insecure controls whether the connection uses a CA certificate instead of system TLS.
	Insecure bool `json:"insecure"`

	// rw protects connection state fields from concurrent access.
	rw sync.RWMutex
	// duration is the current reconnect delay computed by the Fibonacci backoff.
	duration time.Duration
	// Interceptor is the gRPC stream interceptor for HMAC authentication.
	Interceptor InterceptorFunc
	// context is the context of the current connection; cancelled on disconnect.
	context context.Context
	// cancel cancels the current connection context.
	cancel context.CancelFunc
	// connected is true once the broker confirms readiness via "_.ready".
	connected bool
	// done is true after the broker has initiated a reset via "_.reset".
	done bool
	// stream is the active bidirectional gRPC stream with the broker.
	stream pb.PubSub_ChannelClient
	// overstayCount tracks reconnect attempts at maximum delay.
	overstayCount int
	// certPath is the path to the CA certificate file for TLS.
	certPath string

	// ConnectFunc is called when the broker connection is established.
	ConnectFunc CallbackFunc
	// DisconnectFunc is called when the broker connection is lost.
	DisconnectFunc CallbackFunc
	// RecvFunc is called when an application-level message is received.
	RecvFunc RecvFunc
	// ResetFunc is called when the broker sends a "_.reset" command.
	ResetFunc func()
	// OwerstayingFunc is called when the reconnect attempt limit is exceeded.
	// Returns true if reconnection was handled at a higher level.
	OwerstayingFunc func() bool
}

// Done cancels the current connection context, initiating broker shutdown.
func (b *Broker[CTX]) Done() {
	b.rw.RLock()
	defer b.rw.RUnlock()
	if b.cancel != nil {
		b.cancel()
	}
}

// Send sends an event through the active gRPC stream.
// Returns ErrBrokerDisconnected if the connection is not established.
func (b *Broker[CTX]) Send(pub *pub.Pub[CTX]) error {
	b.rw.RLock()
	defer b.rw.RUnlock()
	if !b.connected {
		return ErrBrokerDisconnected
	}
	msg, err := pub.GetEvent()
	if err != nil {
		return err
	}
	return b.stream.Send(msg)
}

// Start establishes the gRPC connection and begins processing messages.
// Blocks until the connection context is cancelled.
func (b *Broker[CTX]) Start() {
	b.rw.Lock()
	// Cancel the previous context if Start is called again after a disconnect.
	if b.cancel != nil {
		b.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.context = ctx
	b.cancel = cancel
	b.rw.Unlock()

	conn, err := b.client()
	if err != nil {
		fmt.Println("Client error", err)
		return
	}
	defer conn.Close()

	cli := pb.NewPubSubClient(conn)
	stream, err := cli.Channel(ctx)
	if err != nil {
		fmt.Println("Channel error:", status.Convert(err).Code())
		b.restart()
		return
	}

	// Read goroutine: handles incoming messages and broker system events.
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				b.handleError(err)
				cancel()
				return
			}

			if msg != nil {
				switch msg.Name {
				case "_.ready":
					// Broker confirmed readiness: reset counters and mark connection active.
					b.rw.Lock()
					b.duration = 0
					b.overstayCount = 0
					b.connected = true
					b.stream = stream
					b.ConnectFunc(b.ID)
					b.rw.Unlock()

				case "_.reset":
					// Broker requested reset: acknowledge and initiate reconnection.
					b.initResetProcess()

				default:
					b.handleRecv(msg)
				}
			}
		}
	}()

	// Shutdown goroutine: closes the stream when the context is cancelled.
	// NOTE: two identical cases are intentional; the second never fires and
	// serves as a placeholder for a potential second signal.
	go func() {
		select {
		case <-ctx.Done():
			stream.CloseSend()
			cancel()
		case <-ctx.Done():
		}
	}()

	fmt.Println("Connected to shard:", b.ID)
	<-ctx.Done()

	b.rw.Lock()
	b.connected = false
	b.stream = nil
	b.rw.Unlock()

	b.DisconnectFunc(b.ID)
}

// SetCertPath sets the path to the CA certificate used for TLS connections.
func (b *Broker[CTX]) SetCertPath(crt string) {
	b.certPath = crt
}

// handleRecv enriches the context with message metadata and forwards it to RecvFunc.
func (b *Broker[CTX]) handleRecv(msg *pb.Message) {
	ctx := metadata.FromRecv(context.Background(), msg)
	ctx = message.FromRecv[CTX](ctx, msg)
	b.RecvFunc(ctx, msg)
}

// initResetProcess acknowledges the reset command to the broker and calls ResetFunc.
func (b *Broker[CTX]) initResetProcess() {
	b.rw.Lock()
	b.done = true
	b.rw.Unlock()
	// Send acknowledgement before calling ResetFunc so the broker does not
	// wait for a response after the connection is torn down.
	b.stream.Send(&pb.Message{
		Id:        uuid.New().String(),
		Name:      "_.reset.success",
		Timestamp: timestamppb.New(time.Now()),
	})
	b.ResetFunc()
}

// handleError inspects the gRPC stream error and decides whether to reconnect.
func (b *Broker[CTX]) handleError(err error) {
	switch status.Code(err) {
	case codes.InvalidArgument:
		fmt.Println("Invalid argument:", status.Convert(err).Message())
	case codes.Unauthenticated:
		fmt.Println("Unauthenticated:", status.Convert(err).Message())
	case codes.Unavailable:
		fmt.Println("The connection was broken")
		b.restart()
	default:
		fmt.Println("Connect stream error:", status.Convert(err).Message())
		b.restart()
	}
}

// restart implements reconnection with Fibonacci backoff delay.
// Delegates to OwerstayingFunc when the maximum delay is reached.
func (b *Broker[CTX]) restart() {
	if b.duration == time.Duration(durationMax) {
		b.overstayCount++
		// Stop retrying if the attempt limit is exceeded and the upper level
		// did not take over reconnection.
		if b.overstayCount > overstayAttempts {
			return
		}
		if b.OwerstayingFunc() {
			return
		}
	}

	b.rw.RLock()
	ctx := b.context
	// Skip reconnect if the context is already done or the broker is marked done.
	if ctx == nil || ctx.Err() != nil || b.done {
		b.rw.RUnlock()
		return
	}
	b.rw.RUnlock()

	b.duration = time.Duration(fibo(int(b.duration)))
	wait := b.duration * time.Second

	fmt.Println("Wait for reconnect:", wait)
	timer := time.NewTimer(wait)
	defer timer.Stop()

	// Wait for the timer or context cancellation, whichever comes first.
	select {
	case <-timer.C:
		b.Start()
	case <-ctx.Done():
		return
	}
}

// client creates a gRPC connection with the appropriate transport credentials.
func (b *Broker[CTX]) client() (*grpc.ClientConn, error) {
	var creds grpc.DialOption
	if b.Insecure {
		caFile := Path(b.certPath)
		var err error
		tlsCreds, err := credentials.NewClientTLSFromFile(caFile, b.Host)
		if err != nil {
			return nil, err
		}
		creds = grpc.WithTransportCredentials(tlsCreds)
	} else {
		// WARNING: TLS 1.2 minimum version is required for a secure connection.
		tlsCreds := credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: b.Host,
		})
		creds = grpc.WithTransportCredentials(tlsCreds)
	}

	authOpt := grpc.WithStreamInterceptor(
		grpc.StreamClientInterceptor(b.Interceptor),
	)

	url := fmt.Sprintf("%s:%d", b.Host, b.Port)
	conn, err := grpc.NewClient(url, creds, authOpt)
	if err != nil {
		return nil, ErrBrokerDialFailed
	}
	return conn, nil
}
