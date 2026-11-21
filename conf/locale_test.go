package conf

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/ini/v2"
)

var (
	localePlaceholder = regexp.MustCompile(`\{[A-Za-z0-9_]+\}`)
	localeGoVerb      = regexp.MustCompile(`%(\[\d+\])?[a-z]`)
)

// TestLocales checks that every translation has only keys of en-US, and the
// same "{name}" placeholders as the en-US message of its key.
func TestLocales(t *testing.T) {
	load := func(t *testing.T, name string) map[string]string {
		data, err := Files.ReadFile("locale/" + name)
		require.NoError(t, err)
		f, err := ini.LoadSources([]any{data}, ini.IgnoreInlineComment(true), ini.UnescapeValueCommentSymbols(true))
		require.NoError(t, err)

		messages := make(map[string]string)
		for _, section := range f.Sections() {
			for _, key := range section.Keys() {
				messages[section.Name()+"."+key.Name()] = key.Value()
			}
		}
		return messages
	}
	placeholders := func(s string) []string {
		found := localePlaceholder.FindAllString(s, -1)
		set := make(map[string]bool, len(found))
		var out []string
		for _, p := range found {
			if !set[p] {
				set[p] = true
				out = append(out, p)
			}
		}
		return out
	}

	english := load(t, "locale_en-US.ini")
	for key, message := range english {
		// The %s of the bind DN helper is the LDAP syntax shown to admins.
		if key != "admin.auths.bind_dn_helper" {
			assert.False(t, localeGoVerb.MatchString(message), "%s: use {name} placeholders instead of %%s", key)
		}
	}

	names, err := FileNames("locale")
	require.NoError(t, err)
	for _, name := range names {
		if name == "locale_en-US.ini" {
			continue
		}
		t.Run(strings.TrimSuffix(strings.TrimPrefix(name, "locale_"), ".ini"), func(t *testing.T) {
			for key, message := range load(t, name) {
				want, ok := english[key]
				if !assert.True(t, ok, "%s does not exist in en-US", key) {
					continue
				}
				assert.ElementsMatch(t, placeholders(want), placeholders(message), key)
			}
		})
	}
}
