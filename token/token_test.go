//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package token

import (
	"regexp"
	"testing"
)

/*
TestTokenFormat generates a new random string, verifying it has the
correct length and encoding.
*/
func TestTokenFormat(t *testing.T) {
	str, err := GenerateToken()
	if err != nil {
		t.Fatalf("Error generating token: %v", err)
	}
	if len(str) != (TokenLength * 4 / 3) {
		t.Fatalf("Token generated (%s) was the wrong length", str)
	}
	match, _ := regexp.MatchString("[^A-Za-z0-9_=-]", str)
	if match {
		t.Fatalf("Token generated (%s) contained invalid characters", str)
	}
}

// numToTest tells TestTokenRandomness() how many strings to generate.
const numToTest = 10

/*
TestTokenRandomness generates multiple (numToTest) tokens and verifies
that no two are the same.
*/
func TestTokenRandomness(t *testing.T) {
	strs := make([]string, 0, numToTest)
	for i := 0; i < numToTest; i++ {
		str, err := GenerateToken()
		if err != nil {
			t.Fatalf("Error generating token: %v", err)
		}
		strs = append(strs, str)
	}
	// No two should be the same
	for i := 0; i < (numToTest - 1); i++ {
		for j := i + 1; j < numToTest; j++ {
			if strs[i] == strs[j] {
				t.Fatalf("Generated %d tokens and some were the same - indices %d and %d were both %s", numToTest, i, j, strs[i])
			}
		}
	}
}
