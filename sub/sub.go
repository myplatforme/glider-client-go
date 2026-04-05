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

package sub

import (
	"context"
	"sync"

	pb "github.com/myplatforme/glider-client-go/proto"
)

// handler associates a unique subscription identifier with its callback function.
type handler struct {
	// id is the unique identifier of this particular subscription within a Sub.
	id string
	// cb is the function invoked when a message is received.
	cb ReceiveFunc
}

// Sub holds the set of handlers registered for a single named event.
type Sub struct {
	// ID is the unique identifier of the subscription.
	ID string
	// Name is the event name this subscription is registered for.
	Name string

	// rw protects handlers from races during concurrent registration and delivery.
	rw sync.RWMutex
	// handlers is the list of active handlers for this event.
	handlers []handler
}

// Register adds handler cb with the given id to the subscription.
func (s *Sub) Register(id string, cb ReceiveFunc) {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.handlers = append(s.handlers, handler{
		id: id,
		cb: cb,
	})
}

// Unregister removes the handler with the given id.
// Returns true if no handlers remain after removal.
func (s *Sub) Unregister(id string) bool {
	s.rw.Lock()
	defer s.rw.Unlock()
	var handlers []handler
	for _, handler := range s.handlers {
		if handler.id == id {
			continue
		}
		handlers = append(handlers, handler)
	}
	s.handlers = handlers
	return len(s.handlers) == 0
}

// Recv delivers the message to all registered handlers.
//
// NOTE: each handler is called in its own goroutine so a slow consumer
// does not block delivery to the others.
func (s *Sub) Recv(ctx context.Context, msg *pb.Message) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	for _, handler := range s.handlers {
		go handler.cb(ctx, msg.Payload)
	}
}
