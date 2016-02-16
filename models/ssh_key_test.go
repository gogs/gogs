package models

import (
	"testing"
)

func TestSSHKeyVerification(t *testing.T) {
	keys := map[string][]byte{
		"dsa-1024":    []byte("ssh-dss AAAAB3NzaC1kc3MAAACBAOChCC7lf6Uo9n7BmZ6M8St19PZf4Tn59NriyboW2x/DZuYAz3ibZ2OkQ3S0SqDIa0HXSEJ1zaExQdmbO+Ux/wsytWZmCczWOVsaszBZSl90q8UnWlSH6P+/YA+RWJm5SFtuV9PtGIhyZgoNuz5kBQ7K139wuQsecdKktISwTakzAAAAFQCzKsO2JhNKlL+wwwLGOcLffoAmkwAAAIBpK7/3xvduajLBD/9vASqBQIHrgK2J+wiQnIb/Wzy0UsVmvfn8A+udRbBo+csM8xrSnlnlJnjkJS3qiM5g+eTwsLIV1IdKPEwmwB+VcP53Cw6lSyWyJcvhFb0N6s08NZysLzvj0N+ZC/FnhKTLzIyMtkHf/IrPCwlM+pV/M/96YgAAAIEAqQcGn9CKgzgPaguIZooTAOQdvBLMI5y0bQjOW6734XOpqQGf/Kra90wpoasLKZjSYKNPjE+FRUOrStLrxcNs4BeVKhy2PYTRnybfYVk1/dmKgH6P1YSRONsGKvTsH6c5IyCRG0ncCgYeF8tXppyd642982daopE7zQ/NPAnJfag= nocomment"),
		"rsa-1024":    []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQDAu7tvIvX6ZHrRXuZNfkR3XLHSsuCK9Zn3X58lxBcQzuo5xZgB6vRwwm/QtJuF+zZPtY5hsQILBLmF+BZ5WpKZp1jBeSjH2G7lxet9kbcH+kIVj0tPFEoyKI9wvWqIwC4prx/WVk2wLTJjzBAhyNxfEq7C9CeiX9pQEbEqJfkKCQ== nocomment\n"),
		"rsa-2048":    []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDMZXh+1OBUwSH9D45wTaxErQIN9IoC9xl7MKJkqvTvv6O5RR9YW/IK9FbfjXgXsppYGhsCZo1hFOOsXHMnfOORqu/xMDx4yPuyvKpw4LePEcg4TDipaDFuxbWOqc/BUZRZcXu41QAWfDLrInwsltWZHSeG7hjhpacl4FrVv9V1pS6Oc5Q1NxxEzTzuNLS/8diZrTm/YAQQ/+B+mzWI3zEtF4miZjjAljWd1LTBPvU23d29DcBmmFahcZ441XZsTeAwGxG/Q6j8NgNXj9WxMeWwxXV2jeAX/EBSpZrCVlCQ1yJswT6xCp8TuBnTiGWYMBNTbOZvPC4e0WI2/yZW/s5F nocomment"),
		"ecdsa-256":   []byte("ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFQacN3PrOll7PXmN5B/ZNVahiUIqI05nbBlZk1KXsO3d06ktAWqbNflv2vEmA38bTFTfJ2sbn2B5ksT52cDDbA= nocomment"),
		"ecdsa-384":   []byte("ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBINmioV+XRX1Fm9Qk2ehHXJ2tfVxW30ypUWZw670Zyq5GQfBAH6xjygRsJ5wWsHXBsGYgFUXIHvMKVAG1tpw7s6ax9oA+dJOJ7tj+vhn8joFqT+sg3LYHgZkHrfqryRasQ== nocomment"),
		"ecdsa-512":   []byte("ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBACGt3UG3EzRwNOI17QR84l6PgiAcvCE7v6aXPj/SC6UWKg4EL8vW9ZBcdYL9wzs4FZXh4MOV8jAzu3KRWNTwb4k2wFNUpGOt7l28MztFFEtH5BDDrtAJSPENPy8pvPLMfnPg5NhvWycqIBzNcHipem5wSJFN5PdpNOC2xMrPWKNqj+ZjQ== nocomment"),
		"ed25519-256": []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC+8wwPU2VCo6pA+s9eOSqtDdFYA83/w+fpuSJLHTahU nocomment"),
	}

	for name, pubkey := range keys {
		keyTypeN, lengthN, errN := SSHNativeParsePublicKey(pubkey)
		if errN != nil {
			if errN != SSH_UNKNOWN_KEY_TYPE {
				t.Errorf("error parsing public key '%s': %s", name, errN)
				continue
			}
		}
		keyTypeK, lengthK, errK := SSHKeyGenParsePublicKey(pubkey)
		if errK != nil {
			t.Errorf("error parsing public key '%s': %s", name, errK)
			continue
		}
		// we know that ed25519 is currently not supported by native and returns SSH_UNKNOWN_KEY_TYPE
		if (keyTypeN != keyTypeK || lengthN != lengthK) && errN != SSH_UNKNOWN_KEY_TYPE {
			t.Errorf("key mismatch for '%s': native: %s(%d), ssh-keygen: %s(%d)", name, keyTypeN, lengthN, keyTypeK, lengthK)
		}
	}
}
