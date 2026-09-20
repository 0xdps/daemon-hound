// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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

func writeGitRepo(t *testing.T, root, origin string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "[remote \"origin\"]\n\turl = " + origin + "\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestShouldSkipScanDir(t *testing.T) {
	if !ShouldSkipScanDir("node_modules") {
		t.Error("expected node_modules to be skipped")
	}
	if !ShouldSkipScanDir("Library") {
		t.Error("expected Library to be skipped")
	}
	if !ShouldSkipScanDir(".git") {
		t.Error("expected .git to be skipped")
	}
	if ShouldSkipScanDir("src") {
		t.Error("did not expect src to be skipped")
	}
	if ShouldSkipScanDir(".dotfiles") {
		t.Error("did not expect .dotfiles to be skipped")
	}
}

func TestFindGitRepos(t *testing.T) {
	root := t.TempDir()
	writeGitRepo(t, filepath.Join(root, "a"), "https://github.com/ex/a.git")
	writeGitRepo(t, filepath.Join(root, "group", "b"), "https://github.com/ex/b.git")
	writeGitRepo(t, filepath.Join(root, "group", "deep", "c"), "https://github.com/ex/c.git")
	writeGitRepo(t, filepath.Join(root, "group", "node_modules", "pkg"), "https://github.com/ex/pkg.git")
	writeGitRepo(t, filepath.Join(root, ".dotfiles"), "https://github.com/ex/dotfiles.git")

	// Symlink cycle must not hang the walk.
	cycle := filepath.Join(root, "cycle")
	if err := os.MkdirAll(cycle, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(cycle, "back")); err != nil {
		t.Fatal(err)
	}

	progressCalls := 0
	repos, err := FindGitRepos(root, 2, func(ScanProgress) { progressCalls++ })
	if err != nil {
		t.Fatalf("FindGitRepos: %v", err)
	}
	if progressCalls == 0 {
		t.Fatal("expected progress callback to run")
	}

	found := map[string]string{}
	for _, r := range repos {
		found[r.Namespace] = r.Root
	}
	if _, ok := found["github.com/ex/a"]; !ok {
		t.Error("missing repo a")
	}
	if _, ok := found["github.com/ex/b"]; !ok {
		t.Error("missing repo b")
	}
	if _, ok := found["github.com/ex/c"]; !ok {
		t.Error("missing repo c at depth 2")
	}
	if _, ok := found["github.com/ex/pkg"]; ok {
		t.Error("should skip git repos under node_modules")
	}
	if _, ok := found["github.com/ex/dotfiles"]; !ok {
		t.Error("missing hidden .dotfiles repo")
	}

	shallow, err := FindGitRepos(root, 0, nil)
	if err != nil {
		t.Fatalf("FindGitRepos depth 0: %v", err)
	}
	for _, r := range shallow {
		if r.Namespace == "github.com/ex/b" || r.Namespace == "github.com/ex/c" {
			t.Errorf("depth 0 should not include %s", r.Namespace)
		}
	}
}

func TestFindRepoRootForNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ResetRepoIndexForTest()

	writeGitRepo(t, filepath.Join(home, "src", "ex", "a"), "https://github.com/ex/a.git")
	writeGitRepo(t, filepath.Join(home, "src", "ex", "b"), "https://github.com/ex/b.git")

	root, err := FindRepoRootForNamespace("github.com/ex/a")
	if err != nil {
		t.Fatalf("FindRepoRootForNamespace: %v", err)
	}
	if filepath.Base(root) != "a" {
		t.Errorf("got root %q", root)
	}

	rootB, err := FindRepoRootForNamespace("github.com/ex/b")
	if err != nil {
		t.Fatalf("cached lookup for b: %v", err)
	}
	if filepath.Base(rootB) != "b" {
		t.Errorf("got root %q", rootB)
	}

	if _, err := FindRepoRootForNamespace("github.com/ex/missing"); err == nil {
		t.Fatal("expected missing namespace to fail")
	}
}
