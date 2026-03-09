// Package main is the entry point for localias — a local reverse proxy CLI tool
// that replaces port numbers with stable named .localhost URLs for local development.
//
// Author: Thiru K
// Repository: github.com/thirukguru/localias
package main

import "github.com/thirukguru/localias/cmd"

func main() {
	cmd.Execute()
}
