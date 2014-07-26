// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package captcha

import "testing"

func TestSiphash(t *testing.T) {
	good := uint64(0xe849e8bb6ffe2567)
	cur := siphash(0, 0, 0)
	if cur != good {
		t.Fatalf("siphash: expected %x, got %x", good, cur)
	}
}

func BenchmarkSiprng(b *testing.B) {
	b.SetBytes(8)
	p := &siprng{}
	for i := 0; i < b.N; i++ {
		p.Uint64()
	}
}
