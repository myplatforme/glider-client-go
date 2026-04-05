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

package httpclient

// Response is the standard envelope for pubsub server responses.
type Response[T any] struct {
	// Data contains the response payload.
	Data T `json:"data"`
	// RequestNonce is the original request nonce echoed by the server to prevent replay attacks.
	RequestNonce string `json:"request_nonce"`
	// Error contains the error message when Success is false.
	Error *string `json:"error"`
	// Success indicates whether the request was processed successfully.
	Success bool `json:"success"`
}

// CompareRequestNonce checks whether the response nonce matches the original request nonce.
// A mismatch indicates a replayed or substituted response.
func (r *Response[T]) CompareRequestNonce(nonce string) bool {
	return r.RequestNonce == nonce
}
