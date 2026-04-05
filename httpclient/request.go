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

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/myplatforme/glider-client-go/authhmac"
)

// request holds a prepared HTTP request with its HMAC signature.
type request struct {
	// Project is the project identifier sent in the X-Project header.
	Project string
	// Module is the module identifier sent in the X-Module header.
	Module string
	// Url is the parsed request address.
	Url *url.URL
	// Method is the HTTP method.
	Method string
	// body is the serialised request body.
	body []byte
	// hmac is the HMAC signature of the request body.
	hmac *authhmac.Hmac
	// secret is reserved and not used directly.
	secret string
}

// GetNonce returns the nonce used when signing the request.
// It is compared against the nonce reflected in the server response.
func (r *request) GetNonce() string {
	return r.hmac.Nonce
}

// Do executes the HTTP request and returns the response.
// It attaches HMAC headers and the project/module identifiers.
// Any non-2xx status code is mapped to a typed error.
func (r *request) Do(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, r.Url.String(), bytes.NewBuffer(r.body))
	if err != nil {
		return nil, ErrInternalServerError
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", r.hmac.Signature)
	req.Header.Set("X-Timestamp", r.hmac.Timestamp)
	req.Header.Set("X-Nonce", r.hmac.Nonce)
	req.Header.Set("X-Project", r.Project)
	req.Header.Set("X-Module", r.Module)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, ErrRequestFailed
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, ErrUnauthorized
		case http.StatusForbidden:
			return nil, ErrPermissionDenied
		case http.StatusNotFound:
			return nil, ErrNotFound
		default:
			return nil, ErrInternalServerError
		}
	}

	// TODO: this check is unreachable — the switch above returns for every
	// status other than 200, and at 200 the condition below is always false.
	// Left as a placeholder in case the logic above is extended.
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrRequestFailed
	}

	return resp, nil
}
