// Copyright 2025 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathSecurity_BasicTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		path       string
		allowedDir string
		wantErr    bool
	}{
		{"normal path", "file.txt", tmpDir, false},
		{"dot-dot traversal", "../etc/passwd", tmpDir, true},
		{"nested traversal", "foo/../../etc/passwd", tmpDir, true},
		{"deep traversal", "a/b/c/../../../../../../../etc/passwd", tmpDir, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathSecurity(tt.path, tt.allowedDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePathSecurity() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePathSecurity_SymlinkTraversal(t *testing.T) {
	repoDir := t.TempDir()
	outsideDir := t.TempDir()

	outsideFile := filepath.Join(outsideDir, "sensitive.txt")
	if err := os.WriteFile(outsideFile, []byte("sensitive data"), 0644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(repoDir, "malicious_link")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	err := ValidatePathSecurity("malicious_link", repoDir)
	if err == nil {
		t.Error("ValidatePathSecurity() should detect symlink traversal")
	}

	if !IsErrSymlinkTraversal(err) {
		t.Errorf("Expected ErrSymlinkTraversal, got %T: %v", err, err)
	}
}

func TestValidatePathSecurity_ChainedSymlinks(t *testing.T) {
	repoDir := t.TempDir()
	outsideDir := t.TempDir()

	outsideFile := filepath.Join(outsideDir, "target.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}

	link2 := filepath.Join(repoDir, "link2")
	if err := os.Symlink(outsideFile, link2); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	link1 := filepath.Join(repoDir, "link1")
	if err := os.Symlink(link2, link1); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	err := ValidatePathSecurity("link1", repoDir)
	if err == nil {
		t.Error("ValidatePathSecurity() should detect chained symlink traversal")
	}
}

func TestValidatePathSecurity_ValidSymlink(t *testing.T) {
	repoDir := t.TempDir()

	targetFile := filepath.Join(repoDir, "real_file.txt")
	if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	validLink := filepath.Join(repoDir, "valid_link")
	if err := os.Symlink(targetFile, validLink); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	err := ValidatePathSecurity("valid_link", repoDir)
	if err != nil {
		t.Errorf("ValidatePathSecurity() should allow valid symlink: %v", err)
	}
}

func TestIsSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	regularFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	isLink, _ := IsSymlink(regularFile)
	if isLink {
		t.Error("IsSymlink() should return false for regular file")
	}

	symlinkFile := filepath.Join(tmpDir, "link")
	if err := os.Symlink(regularFile, symlinkFile); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	isLink, _ = IsSymlink(symlinkFile)
	if !isLink {
		t.Error("IsSymlink() should return true for symlink")
	}
}

func TestSafeWriteFile(t *testing.T) {
	repoDir := t.TempDir()
	outsideDir := t.TempDir()

	err := SafeWriteFile("test.txt", []byte("content"), 0644, repoDir)
	if err != nil {
		t.Errorf("SafeWriteFile() failed for normal path: %v", err)
	}

	outsideFile := filepath.Join(outsideDir, "target.txt")
	symlinkPath := filepath.Join(repoDir, "malicious")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	err = SafeWriteFile("malicious", []byte("pwned"), 0644, repoDir)
	if err == nil {
		t.Error("SafeWriteFile() should fail for symlink pointing outside")
	}
}
