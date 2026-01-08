// Copyright 2025 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gogs.io/gogs/internal/osutil"
)

// ErrSymlinkTraversal is returned when the final path resolves outside the allowed directory by following through symlink(s).
type ErrSymlinkTraversal struct {
	Path       string
	ResolvedTo string
	AllowedDir string
}

func (e ErrSymlinkTraversal) Error() string {
	return fmt.Sprintf("symlink traversal detected: %s resolves to %s which is outside %s",
		e.Path, e.ResolvedTo, e.AllowedDir)
}

// IsErrSymlinkTraversal checks if the error is a symlink traversal error
func IsErrSymlinkTraversal(err error) bool {
	_, ok := err.(ErrSymlinkTraversal)
	return ok
}

// resolveRealPath resolves a path to its real absolute path, following all symlinks
// This handles cases like macOS where /var is a symlink to /private/var
func resolveRealPath(path string) (string, error) {
	// First get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Try to evaluate symlinks (resolve the full path)
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If path doesn't exist yet, try to resolve parent
		if os.IsNotExist(err) {
			parent := filepath.Dir(absPath)
			realParent, parentErr := filepath.EvalSymlinks(parent)
			if parentErr != nil {
				// Parent also doesn't exist, just return cleaned abs path
				return filepath.Clean(absPath), nil
			}
			return filepath.Join(realParent, filepath.Base(absPath)), nil
		}
		return absPath, nil
	}

	return realPath, nil
}

// ValidatePathSecurity performs comprehensive path security validation
// including symlink resolution checks.
//
// 🚨 SECURITY: Ensures the path resolves within the allowed directory by following through symlink(s) (if any).
// This function ensures that:
// 1. The path does not contain traversal sequences (../)
// 2. If the path is a symlink, it resolves to a location within allowedDir
// 3. The final resolved path is within the allowed directory
func ValidatePathSecurity(path, allowedDir string) error {
	cleanPath := filepath.Clean(path)
	cleanAllowedDir := filepath.Clean(allowedDir)

	if strings.Contains(cleanPath, "..") {
		return ErrSymlinkTraversal{
			Path:       path,
			ResolvedTo: cleanPath,
			AllowedDir: cleanAllowedDir,
		}
	}

	fullPath := filepath.Join(cleanAllowedDir, cleanPath)

	info, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return validateParentPath(fullPath, cleanAllowedDir)
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return validateSymlink(fullPath, cleanAllowedDir)
	}

	return validateResolvedPath(fullPath, cleanAllowedDir)
}

// validateSymlink resolves a symlink and ensures it points within the allowed directory
func validateSymlink(symlinkPath, allowedDir string) error {
	// Resolve the symlink to its final destination
	finalPath, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		// If we can't resolve, try reading the link directly
		target, readErr := os.Readlink(symlinkPath)
		if readErr != nil {
			return fmt.Errorf("failed to read symlink: %w", readErr)
		}

		if filepath.IsAbs(target) {
			finalPath = filepath.Clean(target)
		} else {
			symlinkDir := filepath.Dir(symlinkPath)
			finalPath = filepath.Clean(filepath.Join(symlinkDir, target))
		}
	}

	// Resolve both paths to their real paths (handles /var -> /private/var on macOS)
	realFinalPath, err := resolveRealPath(finalPath)
	if err != nil {
		return fmt.Errorf("failed to resolve final path: %w", err)
	}

	realAllowedDir, err := resolveRealPath(allowedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve allowed directory: %w", err)
	}

	// Check if the resolved path is within the allowed directory
	if !strings.HasPrefix(realFinalPath, realAllowedDir+string(os.PathSeparator)) &&
		realFinalPath != realAllowedDir {
		return ErrSymlinkTraversal{
			Path:       symlinkPath,
			ResolvedTo: realFinalPath,
			AllowedDir: realAllowedDir,
		}
	}

	return nil
}

// validateParentPath ensures the parent directory is within the allowed directory
func validateParentPath(path, allowedDir string) error {
	parentDir := filepath.Dir(path)

	info, err := os.Lstat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return validateParentPath(parentDir, allowedDir)
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return validateSymlink(parentDir, allowedDir)
	}

	return validateResolvedPath(parentDir, allowedDir)
}

// validateResolvedPath ensures a resolved path is within the allowed directory
func validateResolvedPath(path, allowedDir string) error {
	// Resolve both paths to their real paths
	realPath, err := resolveRealPath(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	realAllowedDir, err := resolveRealPath(allowedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve allowed directory: %w", err)
	}

	if !strings.HasPrefix(realPath, realAllowedDir+string(os.PathSeparator)) &&
		realPath != realAllowedDir {
		return ErrSymlinkTraversal{
			Path:       path,
			ResolvedTo: realPath,
			AllowedDir: realAllowedDir,
		}
	}

	return nil
}

// SafeWriteFile writes content to a file after validating the path is safe
func SafeWriteFile(path string, content []byte, perm os.FileMode, allowedDir string) error {
	if err := ValidatePathSecurity(path, allowedDir); err != nil {
		return err
	}

	fullPath := filepath.Join(allowedDir, path)
	if osutil.IsSymlink(fullPath) {
		if err := validateSymlink(fullPath, allowedDir); err != nil {
			return err
		}
	}

	return os.WriteFile(fullPath, content, perm)
}
