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

// Package pubsub provides a client for publishing and subscribing to events
// through a message broker.
package pubsub

import (
	"context"
	"time"

	"github.com/myplatforme/glider-client-go/authhmac"
	"github.com/myplatforme/glider-client-go/callback"
	"github.com/myplatforme/glider-client-go/client"
	"github.com/myplatforme/glider-client-go/sub"
	"google.golang.org/protobuf/proto"
)

// Options holds the parameters for creating a pubsub client.
type Options[CTX proto.Message] struct {
	// Project is the project identifier. Defaults to "root" when empty.
	Project string
	// Module is the module identifier within the project.
	Module string
	// Host is the broker server address.
	Host string
	// Secret is the shared secret for HMAC request signing.
	Secret string
	// Signal is a channel used to signal client shutdown.
	Signal chan struct{}
	// Subs is the list of events the client subscribes to.
	Subs []string
	// Pubs is the list of events the client publishes.
	Pubs []string
	// AuthOptions holds HMAC authentication parameters.
	// When nil, safe defaults are applied: Skew 30s, TTL 60s.
	AuthOptions *authhmac.Options
	// Certs maps broker IDs to their CA certificate file paths.
	Certs map[string]string
}

// Client is the interface of the pubsub client.
type Client[CTX proto.Message] interface {
	// Pub publishes payload for the named event and returns an event descriptor.
	Pub(ctx context.Context, eventName string, payload []byte) client.Pub[CTX]
	// Sub registers handler fn for the named event.
	Sub(ctx context.Context, eventName string, fn func(ctx context.Context, payload []byte))
}

// New creates and starts a pubsub client with the provided options.
//
// NOTE: when AuthOptions is nil, safe defaults (Skew 30s, TTL 60s) are applied
// to avoid accidentally disabling timestamp validation.
func New[CTX proto.Message](opts Options[CTX]) Client[CTX] {
	if opts.AuthOptions == nil {
		opts.AuthOptions = &authhmac.Options{
			Skew: 30 * time.Second,
			TTL:  60 * time.Second,
		}
	}

	if opts.Project == "" {
		opts.Project = "root"
	}

	cli := &client.Client[CTX]{
		Project:   opts.Project,
		Module:    opts.Module,
		Subs:      opts.Subs,
		Pubs:      opts.Pubs,
		AuthSkew:  opts.AuthOptions.Skew,
		AuthTTL:   opts.AuthOptions.TTL,
		Secret:    opts.Secret,
		Host:      opts.Host,
		Certs:     opts.Certs,
		Callbacks: callback.New(),
		SubsStore: sub.New(),
	}

	go cli.Start()
	return cli
}
