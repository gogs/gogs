package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverviewKindOf(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "README.md", want: "readme"},
		{name: "readme", want: "readme"},
		{name: "CODE_OF_CONDUCT.md", want: "code_of_conduct"},
		{name: "code-of-conduct.txt", want: "code_of_conduct"},
		{name: "CONTRIBUTING.md", want: "contributing"},
		{name: "LICENSE", want: "license"},
		{name: "LICENCE.txt", want: "license"},
		{name: "LICENSE-MIT", want: "license"},
		{name: "COPYING", want: "license"},
		{name: "UNLICENSE", want: "license"},
		{name: "SECURITY.md", want: "security"},
		{name: "licenses.go", want: ""},
		{name: "contributing_test.go", want: ""},
		{name: "security.go.md", want: ""},
		{name: "main.go", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, overviewKindOf(test.name))
		})
	}
}
