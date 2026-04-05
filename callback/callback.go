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

// Package callback stores and routes broker response messages to their
// waiting callers, keyed by "event:id".
package callback

import (
	"fmt"
	"sync"

	pb "github.com/myplatforme/glider-client-go/proto"
)

// Callback holds registered channels waiting for response messages.
type Callback struct {
	// rw protects list from races during concurrent registration and delivery.
	rw sync.RWMutex
	// list contains channels indexed by the "event:id" key.
	list map[string]chan *pb.Message
}

// New creates an empty callback store.
func New() *Callback {
	return &Callback{
		list: make(map[string]chan *pb.Message),
	}
}

// Recv delivers an incoming message to the channel registered for its event and SubId.
// If no callback is found, the message is silently dropped.
//
// NOTE: the send is performed in a separate goroutine to avoid blocking the
// broker read loop when the consumer is slow.
func (c *Callback) Recv(msg *pb.Message) {
	cbId := fmt.Sprintf("%s:%s", msg.Name, msg.SubId)
	c.rw.RLock()
	cb, ok := c.list[cbId]
	c.rw.RUnlock()
	if !ok {
		return
	}
	go func() {
		cb <- msg
	}()
}

// Register registers channel cb to receive a response message with the given event name and id.
func (c *Callback) Register(event, id string, cb chan *pb.Message) {
	cbId := fmt.Sprintf("%s:%s", event, id)
	c.rw.Lock()
	defer c.rw.Unlock()
	c.list[cbId] = cb
}

// Unregister removes the callback for the given event name and id.
func (c *Callback) Unregister(event, id string) {
	cbId := fmt.Sprintf("%s:%s", event, id)
	c.rw.Lock()
	defer c.rw.Unlock()
	delete(c.list, cbId)
}
