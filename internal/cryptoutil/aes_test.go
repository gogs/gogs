// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cryptoutil

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAESGCM(t *testing.T) {
	key := make([]byte, 16) // AES-128
	_, err := rand.Read(key)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("this will be encrypted")

	encrypted, err := AESGCMEncrypt(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := AESGCMDecrypt(key, encrypted)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, plaintext, decrypted)
}
