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

// Package message provides functions for storing and retrieving
// typed message metadata from a context.
package message

import (
	"context"
	"encoding/json"
	"errors"

	pb "github.com/myplatforme/glider-client-go/proto"
	"google.golang.org/protobuf/proto"
)

// ErrMessageContextNotFound is returned when message metadata is absent from the context.
var ErrMessageContextNotFound = errors.New("message context not found")

// contextKey is the private key type used to store metadata in a context.
// A dedicated type prevents collisions with keys from other packages.
type contextKey string

// Context extracts typed message metadata from the context.
// Returns ErrMessageContextNotFound if no metadata was set.
func Context[CTX proto.Message](ctx context.Context) (*CTX, error) {
	metadata, ok := ctx.Value(contextKey("message")).(CTX)
	if !ok {
		return nil, ErrMessageContextNotFound
	}
	return &metadata, nil
}

// WithContext returns a new context with the given message metadata stored in it.
func WithContext[CTX proto.Message](ctx context.Context, metadata CTX) context.Context {
	return context.WithValue(ctx, contextKey("message"), metadata)
}

// FromRecv deserialises the Context field of an incoming message and stores the
// metadata in the context. If the field is absent or cannot be deserialised,
// the context is returned unchanged.
func FromRecv[CTX proto.Message](ctx context.Context, msg *pb.Message) context.Context {
	if msg.Context != nil {
		var metadata CTX
		if err := json.Unmarshal(msg.Context, &metadata); err != nil {
			// Deserialisation failure does not stop message processing —
			// metadata is optional for application logic.
			return ctx
		}
		return WithContext(ctx, metadata)
	}
	return ctx
}
