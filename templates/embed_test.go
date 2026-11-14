package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/macaron.v1"
)

// The emails use functions that only internal/email defines, so the page
// renderer fails to start when it parses them.
func TestPageFileSystemsSkipEmails(t *testing.T) {
	for name, fs := range map[string]macaron.TemplateFileSystem{
		"embedded": NewTemplateFileSystem("", t.TempDir()),
		"disk":     NewDiskTemplateFileSystem(macaron.RenderOptions{Directory: "."}),
	} {
		t.Run(name, func(t *testing.T) {
			var names []string
			for _, f := range fs.ListFiles() {
				names = append(names, f.Name())
				assert.False(t, strings.HasPrefix(f.Name(), "mail/"), f.Name())
			}
			assert.Contains(t, names, "base/head")
		})
	}
}
