package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToWikiPageName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Home", want: "Home"},
		{input: "../../../../tmp/target_file", want: "tmp target_file"},
		{input: "..\\..\\..\\..\\tmp\\target_file", want: "tmp target_file"},
		{input: "A/B", want: "A B"},
		{input: "../pwn", want: "pwn"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := ToWikiPageName(test.input)
			require.Equal(t, test.want, got)
		})
	}
}
