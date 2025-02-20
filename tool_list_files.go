package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type FileInfo struct {
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	ModTime   string `json:"mod_time"`
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

			entries, err := os.ReadDir(path)
			if err != nil {
				return "", err
			}

			var filesInfo []FileInfo
			for _, entry := range entries {
				name := entry.Name()
				// Skip dotfiles
				if strings.HasPrefix(name, ".") {
					continue
				}

				if !isPathSafe(filepath.Join(path, name)) {
					continue
				}
				
				info, err := entry.Info()
				if err != nil {
					continue
				}

				fileInfo := FileInfo{
					Path:      filepath.Join(path, name),
					IsDir:     entry.IsDir(),
					Size:      info.Size(),
					ModTime:   info.ModTime().String(),
				}
				filesInfo = append(filesInfo, fileInfo)
			}

			result, err := json.Marshal(filesInfo)
			return string(result), err
		},
	}
}