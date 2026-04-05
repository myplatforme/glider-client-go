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

// Package metadata provides the message metadata structure and functions
// for storing and retrieving it from a context.
package metadata

import (
	"context"
	"errors"

	pb "github.com/myplatforme/glider-client-go/proto"
)

// metadataKey is the private key type used to store metadata in a context.
// A dedicated type prevents collisions with keys from other packages.
type metadataKey string

// MD holds message metadata extracted from an incoming proto message.
type MD struct {
	// ID is the unique message identifier.
	ID string
	// Project is the identifier of the project that owns the message.
	Project string
	// Segment is the segment identifier of the message source.
	Segment string
	// Sub is true when the message is a response to a previously sent request.
	Sub bool
	// SubId is the identifier of the original request this message responds to.
	SubId string
	// SubPod is the identifier of the pod that sent the original request.
	SubPod string
	// SubModule is the module that sent the original request.
	SubModule string
	// SubBroker is the broker through which the original request was routed.
	SubBroker string
	// SubProject is the project on whose behalf the original request was sent.
	SubProject string
	// Error holds the error from the message when the server returned a non-empty Error field.
	Error error
}

// FromRecv populates metadata from an incoming proto message and returns a
// new context with the metadata stored in it.
func FromRecv(ctx context.Context, msg *pb.Message) context.Context {
	md := MetaFromContext(ctx)
	md.ID = msg.Id
	md.Project = msg.Project
	md.Segment = msg.Segment

	// Sub* fields are populated only for response messages — they are needed
	// to route the reply back to the correct caller.
	if msg.Sub {
		md.Sub = msg.Sub
		md.SubId = msg.Id
		md.SubPod = msg.SubPod
		md.SubModule = msg.SubModule
		md.SubBroker = msg.SubBroker
		md.SubProject = msg.SubProject
	}

	if msg.Error != "" {
		md.Error = errors.New(msg.Error)
	}

	return md.WithContext(ctx)
}

// WithContext returns a new context with the metadata stored in it.
func (m *MD) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, metadataKey("metadata"), m)
}

// MetaFromContext retrieves metadata from the context.
// Returns an empty MD if no metadata has been set.
func MetaFromContext(ctx context.Context) *MD {
	md, ok := ctx.Value(metadataKey("metadata")).(*MD)
	if !ok {
		return &MD{}
	}
	return md
}
