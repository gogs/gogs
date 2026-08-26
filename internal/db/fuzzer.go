// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
// +build gofuzz

package db

func Fuzz(data []byte) int {
	_, err := CheckPublicKeyString(string(data))
	if err != nil {
		return 0
	}
	return 1
}
