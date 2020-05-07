// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
)

func Test_SSHParsePublicKey(t *testing.T) {
	// TODO: Refactor SSHKeyGenParsePublicKey to accept a tempPath and remove this init.
	conf.MustInit("")
	tests := []struct {
		name      string
		content   string
		expType   string
		expLength int
	}{
		{
			name:      "dsa-1024",
			content:   "ssh-dss AAAAB3NzaC1kc3MAAACBAOChCC7lf6Uo9n7BmZ6M8St19PZf4Tn59NriyboW2x/DZuYAz3ibZ2OkQ3S0SqDIa0HXSEJ1zaExQdmbO+Ux/wsytWZmCczWOVsaszBZSl90q8UnWlSH6P+/YA+RWJm5SFtuV9PtGIhyZgoNuz5kBQ7K139wuQsecdKktISwTakzAAAAFQCzKsO2JhNKlL+wwwLGOcLffoAmkwAAAIBpK7/3xvduajLBD/9vASqBQIHrgK2J+wiQnIb/Wzy0UsVmvfn8A+udRbBo+csM8xrSnlnlJnjkJS3qiM5g+eTwsLIV1IdKPEwmwB+VcP53Cw6lSyWyJcvhFb0N6s08NZysLzvj0N+ZC/FnhKTLzIyMtkHf/IrPCwlM+pV/M/96YgAAAIEAqQcGn9CKgzgPaguIZooTAOQdvBLMI5y0bQjOW6734XOpqQGf/Kra90wpoasLKZjSYKNPjE+FRUOrStLrxcNs4BeVKhy2PYTRnybfYVk1/dmKgH6P1YSRONsGKvTsH6c5IyCRG0ncCgYeF8tXppyd642982daopE7zQ/NPAnJfag= nocomment",
			expType:   "dsa",
			expLength: 1024,
		},
		{
			name:      "rsa-1024",
			content:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQDAu7tvIvX6ZHrRXuZNfkR3XLHSsuCK9Zn3X58lxBcQzuo5xZgB6vRwwm/QtJuF+zZPtY5hsQILBLmF+BZ5WpKZp1jBeSjH2G7lxet9kbcH+kIVj0tPFEoyKI9wvWqIwC4prx/WVk2wLTJjzBAhyNxfEq7C9CeiX9pQEbEqJfkKCQ== nocomment",
			expType:   "rsa",
			expLength: 1024,
		},
		{
			name:      "rsa-2048",
			content:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDMZXh+1OBUwSH9D45wTaxErQIN9IoC9xl7MKJkqvTvv6O5RR9YW/IK9FbfjXgXsppYGhsCZo1hFOOsXHMnfOORqu/xMDx4yPuyvKpw4LePEcg4TDipaDFuxbWOqc/BUZRZcXu41QAWfDLrInwsltWZHSeG7hjhpacl4FrVv9V1pS6Oc5Q1NxxEzTzuNLS/8diZrTm/YAQQ/+B+mzWI3zEtF4miZjjAljWd1LTBPvU23d29DcBmmFahcZ441XZsTeAwGxG/Q6j8NgNXj9WxMeWwxXV2jeAX/EBSpZrCVlCQ1yJswT6xCp8TuBnTiGWYMBNTbOZvPC4e0WI2/yZW/s5F nocomment",
			expType:   "rsa",
			expLength: 2048,
		},
		{
			name:      "ecdsa-256",
			content:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFQacN3PrOll7PXmN5B/ZNVahiUIqI05nbBlZk1KXsO3d06ktAWqbNflv2vEmA38bTFTfJ2sbn2B5ksT52cDDbA= nocomment",
			expType:   "ecdsa",
			expLength: 256,
		},
		{
			name:      "ecdsa-384",
			content:   "ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBINmioV+XRX1Fm9Qk2ehHXJ2tfVxW30ypUWZw670Zyq5GQfBAH6xjygRsJ5wWsHXBsGYgFUXIHvMKVAG1tpw7s6ax9oA+dJOJ7tj+vhn8joFqT+sg3LYHgZkHrfqryRasQ== nocomment",
			expType:   "ecdsa",
			expLength: 384,
		},
		{
			name:      "ecdsa-521",
			content:   "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBACGt3UG3EzRwNOI17QR84l6PgiAcvCE7v6aXPj/SC6UWKg4EL8vW9ZBcdYL9wzs4FZXh4MOV8jAzu3KRWNTwb4k2wFNUpGOt7l28MztFFEtH5BDDrtAJSPENPy8pvPLMfnPg5NhvWycqIBzNcHipem5wSJFN5PdpNOC2xMrPWKNqj+ZjQ== nocomment",
			expType:   "ecdsa",
			expLength: 521,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typ, length, err := SSHNativeParsePublicKey(test.content)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expType, typ)
			assert.Equal(t, test.expLength, length)

			typ, length, err = SSHKeyGenParsePublicKey(test.content)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expType, typ)
			assert.Equal(t, test.expLength, length)
		})
	}
}
