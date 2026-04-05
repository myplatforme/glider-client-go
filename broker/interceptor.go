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
	"context"

	"github.com/myplatforme/glider-client-go/authhmac"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// InterceptorFunc is the type of a client-side gRPC stream interceptor.
type InterceptorFunc func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error)

// AuthInterceptor returns a gRPC stream interceptor that signs each request with
// an HMAC signature and injects identity headers into the outgoing context.
//
// NOTE: the signing payload is module+project concatenation to bind the
// signature to a specific module and project combination.
func AuthInterceptor(auth *authhmac.Authhmac, project, module, pid string) InterceptorFunc {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		hmac, err := auth.Generate([]byte(module + project))
		if err != nil {
			return nil, err
		}

		// Identity and HMAC fields are passed as gRPC metadata so the server
		// can verify authenticity and route the request correctly.
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			"X-Module", module,
			"X-Project", project,
			"X-Pod-ID", pid,
			"X-Signature", hmac.Signature,
			"X-Timestamp", hmac.Timestamp,
			"X-Nonce", hmac.Nonce,
		))

		return streamer(ctx, desc, cc, method, opts...)
	}
}
