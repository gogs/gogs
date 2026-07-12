package web

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/context"
)

func TestRenderIndex_PrefixesRelativeEntrypoints(t *testing.T) {
	index := []byte(`<script src="./assets/app.js"></script><link href="./assets/app.css">`)

	got, err := renderIndex(index, context.WebContext{SubURL: "/gogs"})
	require.NoError(t, err)
	require.Contains(t, string(got), `src="/gogs/assets/app.js"`)
	require.Contains(t, string(got), `href="/gogs/assets/app.css"`)
}
