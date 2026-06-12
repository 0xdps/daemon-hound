package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveNamespace(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "SSH format with .git",
			input: "git@github.com:0xdps/daemon-hound.git",
			want:  "github.com/0xdps/daemon-hound",
		},
		{
			name:  "SSH format without .git",
			input: "git@github.com:0xdps/daemon-hound",
			want:  "github.com/0xdps/daemon-hound",
		},
		{
			name:  "HTTPS format with .git",
			input: "https://github.com/0xdps/daemon-hound.git",
			want:  "github.com/0xdps/daemon-hound",
		},
		{
			name:  "HTTPS format without .git",
			input: "https://github.com/0xdps/daemon-hound",
			want:  "github.com/0xdps/daemon-hound",
		},
		{
			name:  "GitLab SSH",
			input: "git@gitlab.com:user/project.git",
			want:  "gitlab.com/user/project",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unsupported format",
			input:   "ftp://example.com/repo.git",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveNamespace(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("DeriveNamespace(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("DeriveNamespace(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("DeriveNamespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFileChecksum(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.txt"
	content := []byte("hello world")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	checksum1, err := FileChecksum(tmpFile)
	if err != nil {
		t.Fatalf("FileChecksum failed: %v", err)
	}
	if checksum1 == "" {
		t.Error("FileChecksum returned empty string")
	}

	// Same content should produce same checksum
	checksum2, err := FileChecksum(tmpFile)
	if err != nil {
		t.Fatalf("FileChecksum failed: %v", err)
	}
	if checksum1 != checksum2 {
		t.Error("FileChecksum not deterministic")
	}

	// Different content should produce different checksum
	if err := os.WriteFile(tmpFile, []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	checksum3, err := FileChecksum(tmpFile)
	if err != nil {
		t.Fatalf("FileChecksum failed: %v", err)
	}
	if checksum1 == checksum3 {
		t.Error("FileChecksum not unique for different content")
	}
}

func TestNormalizePath(t *testing.T) {
	// Just test it doesn't error and returns absolute
	path, err := NormalizePath(".")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("NormalizePath(.) = %q, expected absolute path", path)
	}
}

func TestIsInsideGitRepo(t *testing.T) {
	// Create a temp git repo
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	if !IsInsideGitRepo(tmpDir) {
		t.Error("IsInsideGitRepo(tmpDir) = false, expected true")
	}

	// Subdirectory should also be inside
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if !IsInsideGitRepo(subDir) {
		t.Error("IsInsideGitRepo(subDir) = false, expected true")
	}

	// Non-git temp directory should not be inside a git repo
	otherDir := t.TempDir()
	if IsInsideGitRepo(otherDir) {
		t.Error("IsInsideGitRepo(otherDir) = true, expected false")
	}
}

func TestFindGitRoot(t *testing.T) {
	// Create a temp git repo
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := FindGitRoot(tmpDir)
	if err != nil {
		t.Fatalf("FindGitRoot failed: %v", err)
	}
	if root != tmpDir {
		t.Errorf("FindGitRoot = %q, want %q", root, tmpDir)
	}

	// From subdirectory
	subDir := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	root, err = FindGitRoot(subDir)
	if err != nil {
		t.Fatalf("FindGitRoot failed: %v", err)
	}
	if root != tmpDir {
		t.Errorf("FindGitRoot = %q, want %q", root, tmpDir)
	}
}
