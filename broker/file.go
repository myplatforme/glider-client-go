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
	"os"
	"path/filepath"
)

// Path returns the absolute path for the given relative path rel.
// If rel is already absolute, it is returned unchanged.
// Panics with ErrBrokerCertFailed if the working directory cannot be determined.
func Path(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}

	wd, err := os.Getwd()
	if err != nil {
		// Without a working directory the certificate path cannot be resolved,
		// making the connection impossible, so panicking is appropriate here.
		panic(ErrBrokerCertFailed)
	}

	return filepath.Join(wd, rel)
}
