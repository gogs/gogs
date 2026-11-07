package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_parseSignature(t *testing.T) {
	tests := []struct {
		line   string
		expSig *Signature
	}{
		{
			line: "Patrick Gundlach <gundlach@speedata.de> 1378823654 +0200",
			expSig: &Signature{
				Name:  "Patrick Gundlach",
				Email: "gundlach@speedata.de",
				When:  time.Unix(1378823654, 0),
			},
		}, {
			line: "Patrick Gundlach <gundlach@speedata.de> Tue Sep 10 16:34:14 2013 +0200",
			expSig: &Signature{
				Name:  "Patrick Gundlach",
				Email: "gundlach@speedata.de",
				When:  time.Unix(1378823654, 0),
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			sig, err := parseSignature([]byte(test.line))
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expSig.Name, sig.Name)
			assert.Equal(t, test.expSig.Email, sig.Email)
			assert.Equal(t, test.expSig.When.Unix(), sig.When.Unix())
		})
	}
}
