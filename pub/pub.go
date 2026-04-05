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

// Package pub implements an event publication descriptor with support for
// awaiting a response message from the broker.
package pub

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/myplatforme/glider-client-go/message"
	"github.com/myplatforme/glider-client-go/metadata"
	pb "github.com/myplatforme/glider-client-go/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SendFunc is the function that sends an event through the transport layer.
type SendFunc[CTX proto.Message] func(pub *Pub[CTX]) error

// Pub holds an event prepared for sending.
type Pub[CTX proto.Message] struct {
	// ID is the unique event identifier.
	ID string
	// PID is the identifier of the sending process.
	PID string
	// Name is the event name.
	Name string
	// Project is the project identifier.
	Project string
	// Payload is the event payload.
	Payload []byte
	// Context is the event context carrying metadata and cancellation.
	Context context.Context
	// Timestamp is the time the event was created.
	Timestamp time.Time

	// SendFunc performs the actual event transmission through the broker.
	SendFunc SendFunc[CTX]
	// RegisterCbFunc registers a channel to wait for the response message.
	RegisterCbFunc func(event, id string, cb chan *pb.Message)
	// UnregisterCbFunc removes the response channel registration after delivery.
	UnregisterCbFunc func(event, id string)

	// subscribed is true when Sub was called, indicating the broker must return a response.
	subscribed bool
}

// Sub sends the event and blocks until a response message named event is received.
// The callback is registered before sending to avoid missing a response that
// arrives before SendFunc returns.
//
// Returns an enriched context, the response payload, and any error.
// Terminates with an error if the context is cancelled.
func (p *Pub[CTX]) Sub(event string) (context.Context, []byte, error) {
	p.subscribed = true
	ctx := p.Context
	msgChan := make(chan *pb.Message)
	// Register the callback before sending so a fast response cannot be missed.
	p.RegisterCbFunc(event, p.ID, msgChan)
	defer p.UnregisterCbFunc(event, p.ID)

	if err := p.SendFunc(p); err != nil {
		return nil, nil, err
	}

	select {
	case msg := <-msgChan:
		// Enrich the context with metadata before checking the error so callers
		// can inspect the error source (project, segment, SubId) even on failure.
		ctx = metadata.FromRecv(ctx, msg)

		if msg.Error != "" {
			return ctx, nil, errors.New(msg.Error)
		}

		if msg.Context != nil {
			var msgContext CTX
			if err := json.Unmarshal(msg.Context, &msgContext); err != nil {
				return ctx, nil, err
			}
			ctx = message.WithContext(ctx, msgContext)
		}

		return ctx, msg.Payload, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

// Do sends the event without waiting for a response.
func (p *Pub[CTX]) Do() error {
	return p.SendFunc(p)
}

// GetEvent serialises the Pub into a proto message for sending over the gRPC stream.
// Routing fields from the context metadata are populated when Sub mode is active.
func (p *Pub[CTX]) GetEvent() (*pb.Message, error) {
	msg := &pb.Message{
		Id:        p.ID,
		Name:      p.Name,
		Project:   p.Project,
		Payload:   p.Payload,
		Sub:       p.subscribed,
		Timestamp: timestamppb.New(p.Timestamp),
	}

	md := metadata.MetaFromContext(p.Context)
	// Routing fields are set only when all of them are present — a partial set
	// would prevent the broker from delivering the response correctly.
	if md.Sub && md.SubPod != "" && md.SubModule != "" && md.SubBroker != "" && md.SubProject != "" {
		msg.SubId = md.SubId
		msg.Sub = true
		msg.SubPod = md.SubPod
		msg.SubModule = md.SubModule
		msg.SubBroker = md.SubBroker
		msg.SubProject = md.SubProject
	}

	// If typed metadata is present in the context, serialise and attach it to the message.
	if msgContext, err := message.Context[CTX](p.Context); err == nil {
		marshal, err := json.Marshal(msgContext)
		if err != nil {
			return nil, err
		}
		msg.Context = marshal
	}

	return msg, nil
}
