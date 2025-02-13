package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func registerListFilesTool(a *Agent) {
	a.tools["list_files"] = Tool{
		Name:        "list_files",
		Description: "Recursively list all files in the current directory",
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

			var files []string
			err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && isPathSafe(path) {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			result, err := json.Marshal(files)
			return string(result), err
		},
	}
}