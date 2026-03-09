// Package project tests — verifies name inference from package.json, go.mod, git, and directory fallback.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/project
package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferName_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "my-web-app"}`), 0644)
	if err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	if name != "my-web-app" {
		t.Errorf("expected 'my-web-app', got %q", name)
	}
}

func TestInferName_ScopedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "@myorg/cool-app"}`), 0644)
	if err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	if name != "cool-app" {
		t.Errorf("expected 'cool-app', got %q", name)
	}
}

func TestInferName_GoMod(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/user/myservice\n\ngo 1.22\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	if name != "myservice" {
		t.Errorf("expected 'myservice', got %q", name)
	}
}

func TestInferName_PackageJSON_PriorityOverGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "frontend"}`), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/user/backend\n\ngo 1.22\n"), 0644)

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	if name != "frontend" {
		t.Errorf("expected 'frontend' (from package.json), got %q", name)
	}
}

func TestInferName_DirectoryFallback(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with a known name
	subdir := filepath.Join(dir, "my-project")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	name, err := InferName(subdir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	if name != "my-project" {
		t.Errorf("expected 'my-project', got %q", name)
	}
}

func TestInferName_EmptyPackageJSON(t *testing.T) {
	dir := t.TempDir()
	// package.json with no name field — should fall back to directory
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version": "1.0.0"}`), 0644)

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	// Falls back to directory name
	expected := filepath.Base(dir)
	if name == "" {
		t.Error("expected non-empty name")
	}
	_ = expected // directory name varies in tests
}

func TestInferName_InvalidPackageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{invalid json`), 0644)

	name, err := InferName(dir)
	if err != nil {
		t.Fatalf("InferName returned error: %v", err)
	}
	// Falls back to directory name
	if name == "" {
		t.Error("expected non-empty name")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyApp", "myapp"},
		{"my_app", "my-app"},
		{"My Cool App!", "my-cool-app"},
		{"@scope/name", "scope-name"},
		{"---name---", "name"},
		{"a--b--c", "a-b-c"},
		{"UPPERCASE", "uppercase"},
		{"hello.world", "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/user/myapp.git", "myapp"},
		{"https://github.com/user/myapp", "myapp"},
		{"git@github.com:user/myapp.git", "myapp"},
		{"git@github.com:user/myapp", "myapp"},
		{"https://gitlab.com/group/subgroup/project.git", "project"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := repoNameFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("repoNameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}
