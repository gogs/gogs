package git

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSHA1_Equal(t *testing.T) {
	tests := []struct {
		s1     *SHA1
		s2     interface{}
		expVal bool
	}{
		{
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     "fcf7087e732bfe3c25328248a9bf8c3ccd85bed4",
			expVal: true,
		}, {
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     EmptyID,
			expVal: false,
		},

		{
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4").bytes,
			expVal: true,
		}, {
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     MustIDFromString(EmptyID).bytes,
			expVal: false,
		},

		{
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			expVal: true,
		}, {
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     MustIDFromString(EmptyID),
			expVal: false,
		},

		{
			s1:     MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			s2:     []byte(EmptyID),
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, test.s1.Equal(test.s2))
		})
	}
}

func TestNewID(t *testing.T) {
	sha, err := NewID([]byte("000000"))
	assert.Equal(t, errors.New("length must be 20"), err)
	assert.Nil(t, sha)
}

func TestNewIDFromString(t *testing.T) {
	sha, err := NewIDFromString("000000")
	assert.Equal(t, errors.New("length must be 40"), err)
	assert.Nil(t, sha)
}
