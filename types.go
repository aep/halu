package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Tool represents a function that can be called by the AI
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Execute     func(input map[string]interface{}) (string, error)
}

// isPathSafe checks if a path is within the current working directory and not a dotfile
func isPathSafe(path string) bool {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	// Clean and normalize paths
	absPath = filepath.Clean(absPath)
	cwd = filepath.Clean(cwd)

	// Check if any component of the path starts with a dot
	pathParts := strings.Split(filepath.ToSlash(absPath), "/")
	for _, part := range pathParts {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}

	// Check if path is within cwd
	return strings.HasPrefix(absPath, cwd)
}