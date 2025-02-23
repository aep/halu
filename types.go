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

	// Only check components under cwd for dots
	relPath, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return false
	}
	
	// Allow if path is exactly cwd
	if relPath == "." {
		return true
	}
	
	// Check if any component under cwd starts with a dot
	pathParts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, part := range pathParts {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}

	// Check if path is within cwd
	return strings.HasPrefix(absPath, cwd)
}