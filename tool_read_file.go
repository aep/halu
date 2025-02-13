package main

import (
	"io/ioutil"
	"os"
)

func registerReadFileTool(a *Agent) {
	a.tools["read_file"] = Tool{
		Name:        "read_file",
		Description: "Read the contents of a file",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to read",
				},
			},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			path := input["path"].(string)

			if !isPathSafe(path) {
				return "", os.ErrPermission
			}

			content, err := ioutil.ReadFile(path)
			if err != nil {
				return "", err
			}
			return string(content), nil
		},
	}
}