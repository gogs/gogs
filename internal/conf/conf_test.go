package conf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"

	"gogs.io/gogs/internal/testx"
)

// initHelperOutputEnv names the file the helper subprocess writes its rendered
// configuration to. The parent reads it back for the golden comparison, which
// avoids routing the multi-line dump through testx.Exec's stdout heuristics.
const initHelperOutputEnv = "GOGS_TEST_INIT_OUTPUT"

// TestInitHelper runs the actual configuration initialization in a subprocess so
// that WorkDir resolves to a fixed GOGS_WORK_DIR. This keeps the golden output
// deterministic while still exercising the relative-to-absolute path resolution
// performed during Init.
func TestInitHelper(_ *testing.T) {
	if !testx.WantHelperProcess() {
		return
	}

	ini.PrettyFormat = false
	defer func() {
		ini.PrettyFormat = true
	}()

	if err := Init(filepath.Join("testdata", "custom.ini")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg := ini.Empty()
	cfg.NameMapper = ini.SnackCase

	for _, v := range []struct {
		section string
		config  any
	}{
		{"", &App},
		{"server", &Server},
		{"server", &SSH},
		{"repository", &Repository},
		{"database", &Database},
		{"security", &Security},
		{"email", &Email},
		{"auth", &Auth},
		{"user", &User},
		{"session", &Session},
		{"attachment", &Attachment},
		{"time", &Time},
		{"picture", &Picture},
		{"mirror", &Mirror},
		{"i18n", &I18n},
	} {
		err := cfg.Section(v.section).ReflectFrom(v.config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", v.section, err)
			os.Exit(1)
		}
	}

	buf := new(bytes.Buffer)
	if _, err := cfg.WriteTo(buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := os.WriteFile(os.Getenv(initHelperOutputEnv), buf.Bytes(), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func TestInit(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "rendered.ini")

	// Pin WorkDir to a fixed path in the subprocess so relative paths in the
	// fixture resolve deterministically. USER matches the fixture's RUN_USER
	// for CheckRunUser. The parent environment is inherited so
	// platform-specific variables (e.g., SystemRoot on Windows) remain
	// available to the subprocess.
	cmd := exec.Command(os.Args[0], "-test.run=TestInitHelper", "--")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"GOGS_WORK_DIR=/tmp",
		"USER=git",
		initHelperOutputEnv+"="+outputPath,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "run helper: %s", out)

	got, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	testx.AssertGolden(t, filepath.Join("testdata", "TestInit.golden.ini"), testx.Update("TestInit"), string(got))
}
