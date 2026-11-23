package web

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/database"
)

func TestIsForcedPrivate(t *testing.T) {
	tests := []struct {
		name         string
		forcePrivate bool
		isAdmin      bool
		isPrivate    bool
		want         bool
	}{
		{name: "private repository of a user", forcePrivate: true, isPrivate: true, want: true},
		{name: "private repository of a site admin", forcePrivate: true, isAdmin: true, isPrivate: true, want: false},
		{name: "public repository stays editable", forcePrivate: true, isPrivate: false, want: false},
		{name: "not forced", forcePrivate: false, isPrivate: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isForcedPrivate(test.forcePrivate, &database.User{IsAdmin: test.isAdmin}, &database.Repository{IsPrivate: test.isPrivate})
			assert.Equal(t, test.want, got)
		})
	}
}
