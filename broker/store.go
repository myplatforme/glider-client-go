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

package broker

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Store holds all known brokers and provides thread-safe access to them.
type Store[CTX proto.Message] struct {
	// rw protects the broker map from concurrent read and write races.
	rw sync.RWMutex
	// brokers contains all brokers indexed by ID.
	brokers map[string]*Broker[CTX]
}

// New creates an empty broker store.
func New[CTX proto.Message]() *Store[CTX] {
	return &Store[CTX]{
		brokers: make(map[string]*Broker[CTX]),
	}
}

// String returns a string representation of the store for debugging.
func (s *Store[CTX]) String() string {
	s.rw.RLock()
	defer s.rw.RUnlock()
	return fmt.Sprintf("%v", s.brokers)
}

// Done shuts down all brokers in the store and clears it.
//
// NOTE: bk.Done() is called before removing each broker from the map to ensure
// its context is cancelled before the state is reset.
func (s *Store[CTX]) Done() {
	s.rw.Lock()
	defer s.rw.Unlock()
	for _, bk := range s.brokers {
		bk.Done()
		delete(s.brokers, bk.ID)
	}
}

// Get returns the broker with the given ID.
// Returns false if no broker with that ID exists.
func (s *Store[CTX]) Get(id string) (*Broker[CTX], bool) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	bk, ok := s.brokers[id]
	return bk, ok
}

// Set adds or replaces a broker in the store.
func (s *Store[CTX]) Set(broker *Broker[CTX]) {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.brokers[broker.ID] = broker
}
