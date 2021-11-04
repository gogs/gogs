// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package strutil

import (
	"crypto/rand"
	"math/big"
	"unicode"
)

// ToUpperFirst returns s with only the first Unicode letter mapped to its upper case.
func ToUpperFirst(s string) string {
	for i, v := range s {
		return string(unicode.ToUpper(v)) + s[i+1:]
	}
	return ""
}

// RandomChars returns a generated string in given number of random characters.
func RandomChars(n int) (string, error) {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	randomInt := func(max *big.Int) (int, error) {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return 0, err
		}

		return int(r.Int64()), nil
	}

	buffer := make([]byte, n)
	max := big.NewInt(int64(len(alphanum)))
	for i := 0; i < n; i++ {
		index, err := randomInt(max)
		if err != nil {
			return "", err
		}

		buffer[i] = alphanum[index]
	}

	return string(buffer), nil
}
