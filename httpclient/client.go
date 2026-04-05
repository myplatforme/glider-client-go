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

// Package httpclient implements an HTTP client with HMAC authentication
// for communicating with the pubsub control server.
package httpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/myplatforme/glider-client-go/authhmac"
)

var (
	// ErrBaseURLRequired is returned when the server base URL is missing from Options.
	ErrBaseURLRequired = errors.New("base URL is required")
	// ErrAuthhmacRequired is returned when the HMAC authenticator is missing from Options.
	ErrAuthhmacRequired = errors.New("authhmac is required")
	// ErrBodyRequired is returned when the request body is absent.
	ErrBodyRequired = errors.New("body is required")
	// ErrFailedToParseURL is returned when the request URL cannot be parsed.
	ErrFailedToParseURL = errors.New("failed to parse URL")
	// ErrFailedToMarshalBody is returned when the request body cannot be marshalled to JSON.
	ErrFailedToMarshalBody = errors.New("failed to marshal body")
	// ErrInternalServerError is returned on an unexpected HTTP status from the server.
	ErrInternalServerError = errors.New("internal server error")
	// ErrRequestFailed is returned on a network error during request execution.
	ErrRequestFailed = errors.New("request failed")
	// ErrUnauthorized is returned on HTTP 401 — missing or invalid credentials.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrPermissionDenied is returned on HTTP 403 — access denied.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrNotFound is returned on HTTP 404 — the requested resource does not exist.
	ErrNotFound = errors.New("request not found")
	// ErrModuleRequired is returned when the module identifier is missing from Options.
	ErrModuleRequired = errors.New("module is required")
	// ErrProjectRequired is returned when the project identifier is missing from Options.
	ErrProjectRequired = errors.New("project is required")
	// ErrInvalidResponseBody is returned when the response body cannot be deserialised.
	ErrInvalidResponseBody = errors.New("invalid response body")
	// ErrInvalidRequestNonce is returned when the response nonce does not match the request nonce.
	ErrInvalidRequestNonce = errors.New("invalid request nonce")
)

// Client executes HMAC-signed HTTP requests to the pubsub server.
type Client struct {
	// baseURL is the server base address.
	baseURL string
	// module is the module identifier sent with every request.
	module string
	// project is the project identifier sent with every request.
	project string
	// auth is used to generate and validate HMAC signatures.
	auth *authhmac.Authhmac
}

// New creates an HTTP client from the provided options.
// Panics on invalid options via opts.validate().
func New(opts Options) *Client {
	opts.validate()
	return &Client{
		baseURL: opts.BaseURL,
		module:  opts.Module,
		project: opts.Project,
		auth:    opts.Auth,
	}
}

// Create builds a signed HTTP request for the given method and path.
// The request body is marshalled to JSON and signed with HMAC before sending.
func (c *Client) Create(method, uri string, data any) (*request, error) {
	uri = fmt.Sprintf("%s%s", c.baseURL, uri)
	u, err := url.Parse(uri)
	if err != nil {
		return nil, ErrFailedToParseURL
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, ErrFailedToMarshalBody
	}

	// Sign the request body so the server can verify its authenticity.
	hmac, err := c.auth.Generate(body)
	if err != nil {
		return nil, err
	}

	return &request{
		Url:     u,
		Method:  method,
		Project: c.project,
		Module:  c.module,
		body:    body,
		hmac:    hmac,
	}, nil
}

// ValidateHmac verifies the HMAC signature of the server response body.
func (c *Client) ValidateHmac(body []byte, hmac *authhmac.Hmac) error {
	return c.auth.Validate(body, hmac)
}
