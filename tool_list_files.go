package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"bufio"
)

type FileInfo struct {
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	ModTime   string `json:"mod_time"`
}

// gitignoreMatch checks if a path matches a gitignore pattern
func gitignoreMatch(pattern, path, basePath string) bool {
	if pattern == "" {
		return false
	}
	
	// Remove leading and trailing whitespace
	pattern = strings.TrimSpace(pattern)
	
	// Skip comment lines
	if strings.HasPrefix(pattern, "#") {
		return false
	}
	
	// Handle negation
	isNegation := strings.HasPrefix(pattern, "!")
	if isNegation {
		pattern = pattern[1:]
	}

	// Clean up pattern
	pattern = strings.TrimSpace(pattern)
	if strings.HasPrefix(pattern, "./") {
		pattern = pattern[2:]
	}
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}

	// Get relative path from the .gitignore location
	relPath, err := filepath.Rel(basePath, path)
	if err != nil {
		return false
	}

	// Handle directory-only patterns
	if strings.HasSuffix(pattern, "/") {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return false
		}
		pattern = strings.TrimSuffix(pattern, "/")
	}

	// Convert pattern to filepath matching pattern
	convertedPattern := pattern
	// Handle common glob patterns
	convertedPattern = strings.Replace(convertedPattern, ".", "\\.", -1)
	convertedPattern = strings.Replace(convertedPattern, "**", "{*,*/*,*/*/*,*/*/*/*}", -1)
	convertedPattern = strings.Replace(convertedPattern, "*", "[^/]*", -1)
	convertedPattern = strings.Replace(convertedPattern, "?", "[^/]", -1)
	
	// Match against both full path and relative path
	matched, err := filepath.Match(convertedPattern, relPath)
	if err != nil {
		matched = false
	}
	if !matched {
		// Try matching just the base name for patterns without slashes
		if !strings.Contains(pattern, "/") {
			matched, _ = filepath.Match(convertedPattern, filepath.Base(path))
		}
	}

	if isNegation {
		return !matched
	}
	return matched
}

// readGitignore reads .gitignore file and returns patterns
func readGitignore(dir string) []string {
	ignoreFile := filepath.Join(dir, ".gitignore")
	patterns := []string{}
	
	file, err := os.Open(ignoreFile)
	if err != nil {
		return patterns
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pattern := strings.TrimSpace(scanner.Text())
		if pattern != "" && !strings.HasPrefix(pattern, "#") {
			patterns = append(patterns, pattern)
		}
	}
	
	return patterns
}

// shouldIgnore checks if a file should be ignored based on .gitignore patterns
func shouldIgnore(path string, ignorePatterns map[string][]string) bool {
	// Check patterns from current directory up to root
	dir := filepath.Dir(path)
	for checkDir := dir; ; checkDir = filepath.Dir(checkDir) {
		if patterns, exists := ignorePatterns[checkDir]; exists {
			for _, pattern := range patterns {
				if gitignoreMatch(pattern, path, checkDir) {
					return true
				}
			}
		}
		// Stop when we reach the root
		if checkDir == "." || checkDir == "/" || filepath.Dir(checkDir) == checkDir {
			break
		}
	}
	return false
}

func registerListFilesTool(a *Agent) {
	a.tools["list_files"] = Tool{
		Name:        "list_files",
		Description: "List files and directories in the current directory",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory path to list files from",
				},
			},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			path := input["path"].(string)

			if !isPathSafe(path) {
				return "", os.ErrPermission
			}

			// Store ignore patterns for each directory
			ignorePatterns := make(map[string][]string)
			
			// First pass: collect all .gitignore patterns
			filepath.Walk(path, func(currentPath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() && isPathSafe(currentPath) {
					patterns := readGitignore(currentPath)
					if len(patterns) > 0 {
						ignorePatterns[currentPath] = patterns
					}
				}
				return nil
			})

			var filesInfo []FileInfo
			err := filepath.Walk(path, func(currentPath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Skip dotfiles, but allow "." as the root path
				if strings.HasPrefix(filepath.Base(currentPath), ".") && currentPath != path {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				// Check if path should be ignored
				if shouldIgnore(currentPath, ignorePatterns) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				if isPathSafe(currentPath) {
					fileInfo := FileInfo{
						Path:      currentPath,
						IsDir:     info.IsDir(),
						Size:      info.Size(),
						ModTime:   info.ModTime().String(),
					}
					filesInfo = append(filesInfo, fileInfo)
				}
				return nil
			})
			
			if err != nil {
				return "", err
			}
			
			result, err := json.Marshal(filesInfo)
			return string(result), err
		},
	}
}