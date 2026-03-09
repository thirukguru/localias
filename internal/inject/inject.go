// Package inject provides framework detection and flag injection for known
// development frameworks. Detects frameworks like Vite, Astro, Angular, etc.
// and injects appropriate --port and --host flags.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/inject
package inject

import (
	"fmt"
	"strings"
)

// frameworkPatterns maps command patterns to their flag injection rules.
var frameworkPatterns = []struct {
	patterns []string
	portFlag string
	hostFlag string
}{
	{
		patterns: []string{"vite", "@vitejs", "vitest"},
		portFlag: "--port",
		hostFlag: "--host",
	},
	{
		patterns: []string{"astro"},
		portFlag: "--port",
		hostFlag: "--host",
	},
	{
		patterns: []string{"ng serve", "angular"},
		portFlag: "--port",
		hostFlag: "--host",
	},
	{
		patterns: []string{"expo"},
		portFlag: "--port",
		hostFlag: "",
	},
	{
		patterns: []string{"react-router"},
		portFlag: "--port",
		hostFlag: "",
	},
	{
		patterns: []string{"next dev", "next start"},
		portFlag: "-p",
		hostFlag: "-H",
	},
	{
		patterns: []string{"nuxt dev", "nuxi dev"},
		portFlag: "--port",
		hostFlag: "--host",
	},
	{
		patterns: []string{"remix dev"},
		portFlag: "--port",
		hostFlag: "",
	},
}

// InjectFlags detects the framework from the command and injects port/host flags.
// If the command already contains port/host flags, they are not overridden.
func InjectFlags(args []string, port int) []string {
	if len(args) == 0 {
		return args
	}

	cmdStr := strings.Join(args, " ")

	for _, fw := range frameworkPatterns {
		for _, pattern := range fw.patterns {
			if !strings.Contains(strings.ToLower(cmdStr), strings.ToLower(pattern)) {
				continue
			}

			result := make([]string, len(args))
			copy(result, args)

			// Inject port flag if not already present
			if fw.portFlag != "" && !containsFlag(args, fw.portFlag) {
				result = append(result, fw.portFlag, fmt.Sprintf("%d", port))
			}

			// Inject host flag if not already present
			if fw.hostFlag != "" && !containsFlag(args, fw.hostFlag) {
				result = append(result, fw.hostFlag, "127.0.0.1")
			}

			return result
		}
	}

	return args
}

// containsFlag checks if a flag is already in the args.
func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}
