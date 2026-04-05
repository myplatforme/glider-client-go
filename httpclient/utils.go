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

// Fibo returns the smallest Fibonacci number greater than attempt.
// Used to compute the delay between HTTP retries, capped at 16 seconds.
func Fibo(attempt int) int {
	if attempt < 0 {
		attempt = 0
	}
	a, b := 1, 1
	for b <= attempt {
		a, b = b, a+b
	}
	if b > 16 {
		return 16
	}
	return b
}
