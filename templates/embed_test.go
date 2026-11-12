package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryCreateDropdownAccessibility(t *testing.T) {
	content, err := files.ReadFile("repo/create.tmpl")
	require.NoError(t, err)
	markup := string(content)

	assert.Equal(t, 4, strings.Count(markup, "data-a11y-dropdown"))
	for _, dropdown := range []struct {
		name      string
		labelID   string
		optionsID string
	}{
		{name: "owner", labelID: "repo-owner-label", optionsID: "repo-owner-options"},
		{name: "gitignore", labelID: "repo-gitignore-label", optionsID: "repo-gitignore-options"},
		{name: "license", labelID: "repo-license-label", optionsID: "repo-license-options"},
		{name: "readme", labelID: "repo-readme-label", optionsID: "repo-readme-options"},
	} {
		t.Run(dropdown.name, func(t *testing.T) {
			assert.Contains(t, markup, `id="`+dropdown.labelID+`"`)
			assert.Contains(t, markup, `data-a11y-label="`+dropdown.labelID+`"`)
			assert.Contains(t, markup, `data-a11y-options="`+dropdown.optionsID+`"`)
			assert.Contains(t, markup, `id="`+dropdown.optionsID+`" role="listbox"`)
		})
	}

	assert.Contains(t, markup, `id="repo-gitignore-options" role="listbox" aria-multiselectable="true"`)
	assert.GreaterOrEqual(t, strings.Count(markup, `role="option"`), 5)
}
