package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// writeWithConfirmation handles the common pattern of writing content to a file with diff preview
// and user confirmation. If yolo is true, it writes directly without confirmation.
func writeWithConfirmation(path string, content []byte, yolo bool) error {
	if yolo {
		// Ensure directory exists before writing
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("error creating directory: %v", err)
		}
		return os.WriteFile(path, content, 0o644)
	}

	// Create temp file with new content
	tempFile, err := os.CreateTemp("", "ai-edit-*")
	if err != nil {
		return fmt.Errorf("error creating temp file: %v", err)
	}
	tempFilePath := tempFile.Name()
	defer os.Remove(tempFilePath)

	// Write new content to temp file
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("error writing to temp file: %v", err)
	}
	tempFile.Close()

	// Create empty file for diff if original doesn't exist
	originalPath := path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		emptyFile, err := os.CreateTemp("", "ai-edit-empty-*")
		if err != nil {
			return fmt.Errorf("error creating empty temp file: %v", err)
		}
		emptyFile.Close()
		originalPath = emptyFile.Name()
		defer os.Remove(originalPath)
	}

	// Show diff and get confirmation
	fmt.Println("\nShowing diff between original and proposed changes...")
	cmd := exec.Command("git", "diff", "--no-index", originalPath, tempFilePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	fmt.Print("\nPress Enter to apply changes, Ctrl+C to cancel: ")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	// Ensure directory exists before creating the destination file
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	// Apply changes by copying file contents
	source, err := os.Open(tempFilePath)
	if err != nil {
		return fmt.Errorf("error opening temp file: %v", err)
	}
	defer source.Close()

	dest, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer dest.Close()

	if _, err = io.Copy(dest, source); err != nil {
		return fmt.Errorf("error copying file: %v", err)
	}

	return nil
}