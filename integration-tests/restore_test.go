package integrationtests

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/userx"
)

// TestRestoreBackupOfGogs012 restores a backup that Gogs 0.12 made with the
// restore command into a database of its own, starts a server on it, and
// authenticates as a restored user with the password and the access token that
// they had.
func TestRestoreBackupOfGogs012(t *testing.T) {
	// Not parallel, because the server it starts shares the work directory of the
	// servers under test and empties their temporary directories on startup.

	// The backup has the rows of the fixtures that the restore scenarios of the
	// database package check, with a real hash of the password instead of a
	// placeholder.
	const password = "password-of-alice"
	fixtures := filepath.Join("..", "internal", "database", "testdata", "backup", "v0.12.11")
	files, err := filepath.Glob(filepath.Join(fixtures, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "backup.zip")
	archive, err := os.Create(archivePath)
	require.NoError(t, err)
	w := zip.NewWriter(archive)
	add := func(name string, content []byte) {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write(content)
		require.NoError(t, err)
	}
	add("gogs-backup/metadata.ini", []byte("VERSION = 1\nGOGS_VERSION = 0.12.11\n"))
	var token string
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)

		var lines [][]byte
		scanner := bufio.NewScanner(bytes.NewReader(content))
		for scanner.Scan() {
			var row map[string]any
			decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&row))
			switch filepath.Base(file) {
			case "User.json":
				if row["Name"] == "alice" {
					row["Passwd"] = userx.EncodePassword(password, row["Salt"].(string))
				}
			case "AccessToken.json":
				if token == "" {
					token = row["Sha1"].(string)
				}
			}
			line, err := json.Marshal(row)
			require.NoError(t, err)
			lines = append(lines, line)
		}
		require.NoError(t, scanner.Err())
		add("gogs-backup/db/"+filepath.Base(file), append(bytes.Join(lines, []byte("\n")), '\n'))
	}
	require.NoError(t, w.Close())
	require.NoError(t, archive.Close())
	require.NotEmpty(t, token)

	dbName, dropDatabase, err := createPostgresDatabase()
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Database %q is left intact for inspection", dbName)
			return
		}
		if err := dropDatabase(); err != nil {
			t.Logf("Failed to drop database %q: %v", dbName, err)
		}
	})
	port, err := freePort()
	require.NoError(t, err)
	sshPort, err := freePort()
	require.NoError(t, err)
	configPath := filepath.Join(dir, "app.ini")
	require.NoError(t, writeConfig(dir, configPath, port, sshPort, dbName, UniqueName(""), false, conf.AntiBotVerificationAltcha))

	// TestMain points the first argument at the server binary.
	restore := exec.CommandContext(t.Context(), os.Args[0], "restore", "--config", configPath, "--from", archivePath, "--tempdir", t.TempDir(), "--database-only")
	output, err := restore.CombinedOutput()
	require.NoError(t, err, "restore output:\n%s", output)

	url := fmt.Sprintf("http://localhost:%d", port)
	logPath := filepath.Join(dir, "server.log")
	t.Cleanup(func() {
		if t.Failed() {
			serverLog, _ := os.ReadFile(logPath)
			t.Logf("Log of the server on the restored database:\n%s", serverLog)
		}
	})
	stopServer, err := startServer(os.Args[0], configPath, url, logPath, "")
	require.NoError(t, err)
	t.Cleanup(stopServer)

	web := newClient(t)
	web.baseURL = url
	web.signIn(&User{Name: "alice", Password: password})
	assert.Equal(t, "alice", web.username())

	api := newClient(t)
	api.baseURL = url
	api.header.Set("Authorization", "token "+token)
	var me apiV1User
	api.doJSON(http.MethodGet, "/api/v1/user", nil, http.StatusOK, &me)
	assert.Equal(t, "alice", me.UserName)

	// Email addresses other than the primary one keep their owner.
	var emails []struct {
		Email string `json:"email"`
	}
	api.doJSON(http.MethodGet, "/api/v1/user/emails", nil, http.StatusOK, &emails)
	var addresses []string
	for _, e := range emails {
		addresses = append(addresses, e.Email)
	}
	assert.Contains(t, addresses, "alice@work.example.com")
}
