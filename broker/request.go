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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/myplatforme/glider-client-go/authhmac"
	"github.com/myplatforme/glider-client-go/httpclient"
	"google.golang.org/protobuf/proto"
)

// RequestFunc is a function that fetches the broker list from the server.
type RequestFunc[CTX proto.Message] func() ([]*Broker[CTX], error)

// RequestOptions holds the parameters sent when registering the client with the server.
type RequestOptions struct {
	// Project is the project identifier.
	Project string `json:"project"`
	// Module is the module identifier.
	Module string `json:"module"`
	// Subs is the list of events the client subscribes to.
	Subs []string `json:"subs"`
	// Pubs is the list of events the client publishes.
	Pubs []string `json:"pubs"`
}

// RequestRes is the server response body containing the assigned broker list.
type RequestRes[CTX proto.Message] struct {
	// Brokers contains the brokers assigned to this client.
	Brokers []*Broker[CTX] `json:"brokers"`
}

// Request returns a function that fetches the broker list for the given module
// and project, retrying with Fibonacci backoff until a successful response is received.
//
// NOTE: the function intentionally blocks in a loop until success — without brokers
// the client cannot operate, so infinite retry is preferable to returning an error
// at startup.
func Request[CTX proto.Message](client *httpclient.Client, opts RequestOptions) RequestFunc[CTX] {
	return func() ([]*Broker[CTX], error) {
		attemptSeconds := 0
		for {
			ctx := context.Background()
			req, err := client.Create(http.MethodPost, "/pubsub/client", opts)
			if err != nil {
				attemptSeconds++
				time.Sleep(time.Duration(httpclient.Fibo(attemptSeconds)) * time.Second)
				fmt.Printf("Error creating request: %s\n", err)
				fmt.Printf("Recovering in %d seconds\n", httpclient.Fibo(attemptSeconds))
				continue
			}

			resp, err := req.Do(ctx)
			if err != nil {
				attemptSeconds++
				time.Sleep(time.Duration(httpclient.Fibo(attemptSeconds)) * time.Second)
				fmt.Printf("Error doing request: %s\n", err)
				fmt.Printf("Recovering in %d seconds\n", httpclient.Fibo(attemptSeconds))
				continue
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			var res httpclient.Response[RequestRes[CTX]]
			err = json.Unmarshal(body, &res)
			if err != nil {
				return nil, httpclient.ErrInvalidResponseBody
			}

			if !res.Success && res.Error != nil {
				return nil, httpclient.ErrInternalServerError
			}

			// Verify the HMAC signature of the response to confirm server authenticity.
			responseSignature := resp.Header.Get("X-Signature")
			responseTimestamp := resp.Header.Get("X-Timestamp")
			responseNonce := resp.Header.Get("X-Nonce")

			if err := client.ValidateHmac(body, &authhmac.Hmac{
				Signature: responseSignature,
				Timestamp: responseTimestamp,
				Nonce:     responseNonce,
			}); err != nil {
				return nil, err
			}

			// Compare the response nonce with the request nonce to prevent replay attacks.
			if !res.CompareRequestNonce(req.GetNonce()) {
				return nil, httpclient.ErrInvalidRequestNonce
			}

			attemptSeconds = 0
			return res.Data.Brokers, nil
		}
	}
}
