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

// Package sub manages subscriptions to named events and routes incoming
// messages to their registered handlers.
package sub

import (
	"context"
	"sync"

	"github.com/google/uuid"
	pb "github.com/myplatforme/glider-client-go/proto"
)

// ReceiveFunc is a handler for an incoming message.
type ReceiveFunc func(ctx context.Context, payload []byte)

// Store holds subscriptions indexed by event name and routes messages to them.
type Store struct {
	// rw protects list from races during concurrent registration and delivery.
	rw sync.RWMutex
	// list contains active subscriptions indexed by event name.
	list map[string]*Sub
}

// New creates an empty subscription store.
func New() *Store {
	return &Store{
		list: make(map[string]*Sub),
	}
}

// Recv delivers an incoming message to the subscription registered for evt.Name.
// If no subscription is found, the message is silently dropped.
func (s *Store) Recv(ctx context.Context, evt *pb.Message) {
	s.rw.RLock()
	ss, ok := s.list[evt.Name]
	s.rw.RUnlock()
	if !ok {
		return
	}
	ss.Recv(ctx, evt)
}

// Register adds handler fn for the event named name.
// If no subscription exists for that event, one is created.
// Returns a unique handler identifier for later removal.
func (s *Store) Register(name string, fn ReceiveFunc) string {
	s.rw.RLock()
	ss, ok := s.list[name]
	s.rw.RUnlock()
	if !ok {
		ss = &Sub{}
		s.rw.Lock()
		s.list[name] = ss
		s.rw.Unlock()
	}
	id := uuid.New().String()
	ss.Register(id, fn)
	return id
}

// Unregister removes the handler with the given id from the event named name.
// If no handlers remain after removal, the subscription entry is deleted entirely.
func (s *Store) Unregister(name string, id string) {
	s.rw.RLock()
	ss, ok := s.list[name]
	s.rw.RUnlock()
	if !ok {
		return
	}
	// Remove the event entry when no handlers are left to avoid a memory leak.
	ok = ss.Unregister(id)
	if ok {
		s.rw.Lock()
		delete(s.list, name)
		s.rw.Unlock()
	}
}
