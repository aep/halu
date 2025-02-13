package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
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

			// Create temp file
			tempFile, err := os.CreateTemp("", "ai-edit-*")
			if err != nil {
				return "", fmt.Errorf("error creating temp file: %v", err)
			}
			tempFilePath := tempFile.Name()
			defer os.Remove(tempFilePath)

			// Write new content to temp file
			if _, err := tempFile.Write([]byte(content)); err != nil {
				return "", fmt.Errorf("error writing to temp file: %v", err)
			}
			tempFile.Close()

			// Show diff and get confirmation
			fmt.Println("\nShowing diff between original and proposed changes...")
			cmd := exec.Command("git", "diff", "--no-index", path, tempFilePath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()

			fmt.Print("\nPress Enter to apply changes, Ctrl+C to cancel: ")
			reader := bufio.NewReader(os.Stdin)
			reader.ReadString('\n')

			// Apply changes by copying file contents
			source, err := os.Open(tempFilePath)
			if err != nil {
				return "", fmt.Errorf("error opening temp file: %v", err)
			}
			defer source.Close()

			dest, err := os.Create(path)
			if err != nil {
				return "", fmt.Errorf("error creating destination file: %v", err)
			}
			defer dest.Close()

			if _, err = io.Copy(dest, source); err != nil {
				return "", fmt.Errorf("error copying file: %v", err)
			}

			return "Changes applied successfully", nil
		},
	}
}