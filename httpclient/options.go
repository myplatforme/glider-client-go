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

import "github.com/myplatforme/glider-client-go/authhmac"

// Options holds the parameters for creating an HTTP client.
type Options struct {
	// BaseURL is the pubsub server base address.
	BaseURL string
	// Module is the module identifier sent with every request.
	Module string
	// Project is the project identifier sent with every request.
	Project string
	// Auth is the HMAC authenticator used to sign requests.
	Auth *authhmac.Authhmac
}

// validate checks required fields and panics if any are missing.
//
// NOTE: panic is used instead of an error because missing fields represent a
// configuration mistake, not a runtime condition.
func (o Options) validate() {
	if o.BaseURL == "" {
		panic(ErrBaseURLRequired)
	}
	if o.Auth == nil {
		panic(ErrAuthhmacRequired)
	}
	if o.Module == "" {
		panic(ErrModuleRequired)
	}
	if o.Project == "" {
		panic(ErrProjectRequired)
	}
}
