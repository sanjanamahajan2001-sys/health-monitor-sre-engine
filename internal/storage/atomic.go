package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a temporary file and then renames it to the target path.
// This ensures that the write is atomic and the target file is never in a partially written state.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create a temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpName)
		}
	}()

	// Apply permissions before writing (optional, but good for security)
	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	// Close before renaming
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Rename to the final path
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename temporary file to %s: %w", path, err)
	}

	return nil
}
