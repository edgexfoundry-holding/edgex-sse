//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

/*
Package token generates URI-safe random string tokens suitable for
use as authorization tokens.

Implementation: base64-URI encoded string of TokenLength bytes read
from cyrpto/rand.Read().
*/
package token

import (
	"crypto/rand"
	"encoding/base64"
)

/*
TokenLength is the number of random bytes used.
Keep it as a multiple of 3 - the random bytes will be base64'ed.
*/
const TokenLength = 18

// GenerateToken returns a new random token string, and error indication if any.
func GenerateToken() (string, error) {
	bytes := make([]byte, TokenLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
