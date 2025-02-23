package main

import (
	"fmt"
	"os"
	"strings"
)

type SearchNotUniqueError struct {
	Count int
}

func (e *SearchNotUniqueError) Error() string {
	return fmt.Sprintf("search text matches %d locations - must match exactly once", e.Count)
}

func countMatches(content, search string) int {
	count := 0
	pos := 0
	for {
		i := strings.Index(content[pos:], search)
		if i == -1 {
			break
		}
		count++
		pos += i + 1
	}
	return count
}

// tryRelativeIndent attempts to do search/replace while handling indentation differences
func tryRelativeIndent(content, search, replace string) (string, bool) {
	lines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")
	replaceLines := strings.Split(replace, "\n")

	if len(searchLines) <= 1 || len(replaceLines) <= 1 {
		return "", false // Only handle multi-line blocks
	}

	// Try to find the search block with flexible indentation
	for i := 0; i <= len(lines)-len(searchLines); i++ {
		matched := true
		baseIndent := ""

		// Get base indentation from first line
		if sl := strings.TrimSpace(searchLines[0]); strings.TrimSpace(lines[i]) == sl {
			baseIndent = strings.Repeat(" ", len(lines[i])-len(strings.TrimLeft(lines[i], " ")))
		} else {
			continue
		}

		// Check if all lines match with consistent indentation
		for j := 1; j < len(searchLines); j++ {
			sl := strings.TrimSpace(searchLines[j])
			ll := strings.TrimSpace(lines[i+j])
			if sl != ll {
				matched = false
				break
			}
		}

		if matched {
			// Found a match - replace while preserving base indentation
			result := make([]string, len(lines))
			copy(result, lines)
			
			// Apply replacement with preserved indentation
			for j, rline := range replaceLines {
				if j < len(searchLines) {
					result[i+j] = baseIndent + strings.TrimSpace(rline)
				}
			}
			return strings.Join(result, "\n"), true
		}
	}

	return "", false
}

func registerSearchReplaceTool(a *Agent) {
	a.tools["search_replace"] = Tool{
		Name:        "search_replace",
		Description: "Search and replace text in a file. The search text must match exactly one location in the file.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit",
				},
				"search": map[string]interface{}{
					"type":        "string", 
					"description": "Text to search for - must match exactly one location in the file",
				},
				"replace": map[string]interface{}{
					"type":        "string",
					"description": "Text to replace with",
				},
			},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			path := input["path"].(string)
			searchText := input["search"].(string)
			replaceText := input["replace"].(string)

			if !isPathSafe(path) {
				return "", os.ErrPermission
			}

			// Read original file
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("error reading file: %v", err)
			}

			// Check for unique match
			matches := countMatches(string(content), searchText)
			if matches == 0 {
				return "No matches found", nil
			}
			if matches > 1 {
				return "", &SearchNotUniqueError{Count: matches}
			}

			var newContent string
			var matched bool

			// Try various search/replace strategies
			
			// 1. Try exact match first
			if strings.Contains(string(content), searchText) {
				newContent = strings.Replace(string(content), searchText, replaceText, 1)
				matched = true
			}

			// 2. Try with relative indentation if exact match failed
			if !matched {
				newContent, matched = tryRelativeIndent(string(content), searchText, replaceText)
			}

			if !matched {
				return "No matches found after trying various strategies", nil
			}

			err = writeWithConfirmation(path, []byte(newContent), a.yolo)
			if err != nil {
				return "", err
			}

			return "Changes applied successfully", nil
		},
	}
}