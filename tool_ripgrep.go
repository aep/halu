package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func registerRipgrepTool(a *Agent) {
	a.tools["ripgrep"] = Tool{
		Name:        "ripgrep",
		Description: "Search file contents using ripgrep (rg)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The pattern to search for",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to search in (directory or file)",
				},
				"case_sensitive": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to use case-sensitive matching (default: false)",
				},
				"literal": map[string]interface{}{
					"type":        "boolean",
					"description": "Treat the pattern as a literal string, not a regex (default: false)",
				},
				"context_lines": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context lines to show before and after match (default: 0)",
				},
				"word_regexp": map[string]interface{}{
					"type":        "boolean",
					"description": "Only show matches surrounded by word boundaries (default: false)",
				},
				"files_with_matches": map[string]interface{}{
					"type":        "boolean",
					"description": "Only show filenames containing matches, not the matching lines (default: false)",
				},
				"max_depth": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum search depth for directories (default: no limit)",
				},
				"line_number": map[string]interface{}{
					"type":        "boolean",
					"description": "Show line numbers (default: true)",
				},
			},
			"required": []string{"pattern", "path"},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			pattern := input["pattern"].(string)
			path := input["path"].(string)
			
			if !isPathSafe(path) {
				return "", os.ErrPermission
			}
			
			// Build command with safe options
			args := []string{"--color", "never"}
			
			// Process safe options
			if caseSensitive, ok := input["case_sensitive"].(bool); ok && caseSensitive {
				args = append(args, "-s")
			} else {
				args = append(args, "-i") // Default to case-insensitive
			}
			
			if literal, ok := input["literal"].(bool); ok && literal {
				args = append(args, "-F")
			}
			
			if contextLines, ok := input["context_lines"].(float64); ok && contextLines > 0 {
				args = append(args, fmt.Sprintf("-C%d", int(contextLines)))
			}
			
			if wordRegexp, ok := input["word_regexp"].(bool); ok && wordRegexp {
				args = append(args, "-w")
			}
			
			if filesWithMatches, ok := input["files_with_matches"].(bool); ok && filesWithMatches {
				args = append(args, "-l")
			}
			
			if maxDepth, ok := input["max_depth"].(float64); ok && maxDepth >= 0 {
				args = append(args, fmt.Sprintf("--max-depth=%d", int(maxDepth)))
			}
			
			// Line numbers are shown by default, unless explicitly disabled
			lineNumber := true
			if ln, ok := input["line_number"].(bool); ok {
				lineNumber = ln
			}
			if !lineNumber {
				args = append(args, "-N")
			}
			
			// Add pattern and path as the last arguments
			args = append(args, pattern, path)
			
			// Execute ripgrep command
			cmd := exec.Command("rg", args...)
			
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			
			err := cmd.Run()
			if err != nil {
				// If no matches found, ripgrep exits with code 1, which is not a real error
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
					return "No matches found.", nil
				}
				
				if stderr.Len() > 0 {
					return "", fmt.Errorf("ripgrep error: %s - %s", err, stderr.String())
				}
				return "", err
			}
			
			result := stdout.String()
			if strings.TrimSpace(result) == "" {
				return "No matches found.", nil
			}
			
			return result, nil
		},
	}
}