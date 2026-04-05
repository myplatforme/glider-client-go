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

package pubsub

import "context"

// Project retrieves the project identifier from the context.
// Returns an empty string and false if no value has been set.
func Project(ctx context.Context) (string, bool) {
	project, ok := ctx.Value("project").(string)
	return project, ok
}

// WithProject returns a new context with the given project identifier stored in it.
func WithProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, "project", project)
}
