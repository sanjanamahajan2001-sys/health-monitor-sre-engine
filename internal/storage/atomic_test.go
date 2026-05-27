package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "atomic-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetPath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello atomic")

	// 1. Initial write
	err = AtomicWriteFile(targetPath, content, 0644)
	if err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}

	// 2. Overwrite
	newContent := []byte("new content")
	err = AtomicWriteFile(targetPath, newContent, 0644)
	if err != nil {
		t.Fatalf("AtomicWriteFile overwrite failed: %v", err)
	}

	data, err = os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read file after overwrite: %v", err)
	}
	if string(data) != string(newContent) {
		t.Errorf("expected %q, got %q", string(newContent), string(data))
	}

	// 3. Verify permissions
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	// Check lowest 9 bits
	if info.Mode().Perm() != 0644 {
		t.Errorf("expected 0644, got %o", info.Mode().Perm())
	}
}
