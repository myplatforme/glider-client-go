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

// Package authhmac provides HMAC request authentication with replay attack protection.
package authhmac

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// Hmac represents a signed request envelope with replay protection fields.
type Hmac struct {
	// Signature contains the hex-encoded HMAC signature of the payload.
	Signature string
	// Nonce contains a per-request unique value.
	Nonce string
	// Timestamp contains the Unix timestamp of signature creation as a string.
	Timestamp string
}

// Options configures time-based validation constraints for signature verification.
type Options struct {
	// Skew is the maximum allowed clock drift between parties.
	// Enforced in both directions: past and future timestamps.
	Skew time.Duration
	// TTL is the maximum allowed request age.
	// Zero disables the check.
	TTL time.Duration
	// Secret is the shared secret key for computing and verifying HMAC.
	Secret string
}

// NewAuth creates an Authhmac instance from the provided options.
// Zero values for Skew and TTL disable the corresponding time constraints.
func NewAuth(opt Options) *Authhmac {
	return &Authhmac{
		skew:   opt.Skew,
		ttl:    opt.TTL,
		secret: opt.Secret,
	}
}

// Authhmac validates and generates HMAC signatures for request payloads.
type Authhmac struct {
	// skew is the allowed clock drift during validation.
	skew time.Duration
	// ttl is the maximum request age during validation.
	ttl time.Duration
	// secret is the shared secret key for HMAC.
	secret string
}

// Validate verifies signature integrity and time constraints for the request.
//
// Parameters:
//   - data: payload bytes that were signed during generation
//   - req: request envelope with signature, nonce, and timestamp
//
// Returns an error on invalid signature or time constraint violation.
func (a *Authhmac) Validate(data []byte, req *Hmac) error {
	ts, err := strconv.ParseInt(req.Timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp")
	}

	if ts <= 0 {
		return errors.New("invalid timestamp")
	}

	now := time.Now()
	createdAt := time.Unix(ts, 0)

	if a.skew > 0 {
		// NOTE: drift is taken as absolute value to reject requests
		// with timestamps too far in the past or in the future.
		d := now.Sub(createdAt)
		if d < 0 {
			d = -d
		}
		if d > a.skew {
			return errors.New("timestamp skew too large")
		}
	} else {
		if createdAt.After(now) {
			return errors.New("timestamp in the future")
		}
	}

	if a.ttl > 0 {
		// NOTE: limiting request age narrows the replay attack window.
		age := now.Sub(createdAt)
		if age < 0 {
			age = 0
		}
		if age > a.ttl {
			return errors.New("request too old")
		}
	}

	sigRaw, err := hex.DecodeString(req.Signature)
	if err != nil {
		return errors.New("invalid signature")
	}

	payload := a.buildPayload(data, ts, req.Nonce)

	m := hmac.New(sha256.New, []byte(a.secret))
	_, _ = m.Write(payload)
	expectedRaw := m.Sum(nil)

	// WARNING: hmac.Equal uses constant-time comparison to prevent timing attacks.
	if !hmac.Equal(expectedRaw, sigRaw) {
		return errors.New("invalid signature")
	}
	return nil
}

// Generate creates a signed request envelope for the provided payload.
//
// Parameters:
//   - data: payload bytes to sign
//
// Returns:
//   - *Hmac: signed envelope with nonce and timestamp
//   - error: nonce generation or signature computation failure
func (a *Authhmac) Generate(data []byte) (*Hmac, error) {
	ts := time.Now().Unix()
	nonce, err := a.generateNonce(16)
	if err != nil {
		return nil, err
	}

	payload := a.buildPayload(data, ts, nonce)
	signature, err := a.generateSignature(a.secret, payload)
	if err != nil {
		return nil, err
	}

	return &Hmac{
		Signature: signature,
		Nonce:     nonce,
		Timestamp: strconv.FormatInt(ts, 10),
	}, nil
}

// generateSignature computes an HMAC-SHA256 signature for the payload and returns it as a hex string.
func (a *Authhmac) generateSignature(secret string, payload []byte) (string, error) {
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write(payload)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// generateNonce returns a cryptographically secure random nonce in hex encoding.
// size specifies the number of bytes; defaults to 16 for invalid values.
func (a *Authhmac) generateNonce(size int) (string, error) {
	if size <= 0 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// buildPayload serializes data, ts, and nonce into a deterministic byte slice.
// Format: uint32(len(data)) | data | uint64(ts) | uint16(len(nonce)) | nonce.
// The fixed format guarantees identical payloads on both the generation and verification sides.
func (a *Authhmac) buildPayload(data []byte, ts int64, nonce string) []byte {
	nonceBytes := []byte(nonce)

	total := 4 + len(data) + 8 + 2 + len(nonceBytes)
	out := make([]byte, total)

	i := 0
	binary.BigEndian.PutUint32(out[i:i+4], uint32(len(data)))
	i += 4

	copy(out[i:i+len(data)], data)
	i += len(data)

	binary.BigEndian.PutUint64(out[i:i+8], uint64(ts))
	i += 8

	binary.BigEndian.PutUint16(out[i:i+2], uint16(len(nonceBytes)))
	i += 2

	copy(out[i:i+len(nonceBytes)], nonceBytes)
	return out
}
