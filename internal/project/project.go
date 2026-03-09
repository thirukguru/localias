// Package project provides utilities for inferring project names from the current
// working directory. It checks package.json, go.mod, git remote origin, and falls
// back to the directory name. It also detects git worktrees for subdomain prefixing.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/project
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// sanitizeRegex allows only lowercase alphanumeric characters and hyphens.
var sanitizeRegex = regexp.MustCompile(`[^a-z0-9-]`)

// InferName determines the project name from the given directory.
// Priority order:
//  1. package.json "name" field
//  2. go.mod module path (last segment)
//  3. git remote origin repo name
//  4. directory basename
//
// If the directory is a git linked worktree, the branch name is prepended
// as a subdomain (e.g. "fix-ui.myapp").
func InferName(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving directory: %w", err)
	}

	name := ""

	// 1. Try package.json
	if n := nameFromPackageJSON(absDir); n != "" {
		name = n
	}

	// 2. Try go.mod
	if name == "" {
		if n := nameFromGoMod(absDir); n != "" {
			name = n
		}
	}

	// 3. Try git remote origin
	if name == "" {
		if n := nameFromGitRemote(absDir); n != "" {
			name = n
		}
	}

	// 4. Fall back to directory name
	if name == "" {
		name = filepath.Base(absDir)
	}

	name = sanitizeName(name)
	if name == "" {
		return "", fmt.Errorf("could not infer project name from %s", absDir)
	}

	// Check for git worktree and prepend branch as subdomain
	if branch := worktreeBranch(absDir); branch != "" {
		name = sanitizeName(branch) + "." + name
	}

	return name, nil
}

// nameFromPackageJSON reads package.json and extracts the "name" field.
func nameFromPackageJSON(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	// Handle scoped packages: @scope/name → name
	name := pkg.Name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

// nameFromGoMod reads go.mod and extracts the last segment of the module path.
func nameFromGoMod(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimPrefix(line, "module ")
			mod = strings.TrimSpace(mod)
			// Take the last path segment
			if idx := strings.LastIndex(mod, "/"); idx >= 0 {
				return mod[idx+1:]
			}
			return mod
		}
	}
	return ""
}

// nameFromGitRemote gets the repo name from git remote origin URL.
func nameFromGitRemote(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	return repoNameFromURL(url)
}

// repoNameFromURL extracts the repository name from a git URL.
// Handles both HTTPS and SSH format URLs.
func repoNameFromURL(url string) string {
	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")
	// Handle SSH: git@github.com:user/repo
	if idx := strings.LastIndex(url, ":"); idx > 0 && !strings.Contains(url, "://") {
		url = url[idx+1:]
	}
	// Take the last path segment
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		return url[idx+1:]
	}
	return url
}

// worktreeBranch returns the branch name if the directory is a git linked worktree.
// Returns empty string if it's a regular repo or the main worktree.
func worktreeBranch(dir string) string {
	// Check if .git is a file (linked worktree) rather than a directory
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}

	// If .git is a directory, it's the main worktree — no branch prefix needed
	if info.IsDir() {
		return ""
	}

	// .git is a file — this is a linked worktree
	// Get the current branch name
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" || branch == "main" || branch == "master" {
		return ""
	}
	return branch
}

// sanitizeName converts a name to a valid .localhost subdomain.
// Lowercases, replaces invalid chars with hyphens, trims hyphens.
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = sanitizeRegex.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	return name
}
