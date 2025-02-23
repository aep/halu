package main

import (
	"os"
)

func registerWriteFileTool(a *Agent) {
	a.tools["write_file"] = Tool{
		Name:        "write_file",
		Description: "Replace a files contents",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "New content for the file",
				},
			},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			path := input["path"].(string)
			content := input["content"].(string)

			if !isPathSafe(path) {
				return "", os.ErrPermission
			}

			err := writeWithConfirmation(path, []byte(content), a.yolo)
			if err != nil {
				return "", err
			}

			return "Changes applied successfully", nil
		},
	}
}