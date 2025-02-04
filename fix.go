package main

import (
	"bufio"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"strings"
)

var fixCmd = &cobra.Command{
	Use:   "fix [file] [prompt...]",
	Short: "Fix a file using Claude",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cc *cobra.Command, args []string) {
		filepath := args[0]
		prompt := strings.Join(args[1:], " ")

		content, err := os.ReadFile(filepath)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		if len(content) > 500 {
			fmt.Println("input file is larger than 500 lines. refusing to waste that much money.")
			os.Exit(1)
		}

		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			fmt.Println("Please set ANTHROPIC_API_KEY in .env file")
			os.Exit(1)
		}

		message := prompt + "\n\n ```" + string(content) + "```"

		response, err := chatWithClaude(message, apiKey, `
			you are coding assistant. every user message is code to be changed.
			respond with only the complete artifact including your changes.`+
			prompt)
		if err != nil {
			fmt.Printf("Error getting response from Claude: %v\n", err)
			os.Exit(1)
		}

		// Extract the artifact from Claude's response
		artifact := extractArtifact(response)

		// If no artifact found, print response and exit
		if artifact == "" {
			fmt.Println(response)
			os.Exit(0)
		}

		// Create temp file
		tempFile, err := os.CreateTemp("", "claude-fix-*")
		if err != nil {
			fmt.Printf("Error creating temp file: %v\n", err)
			os.Exit(1)
		}
		tempFilePath := tempFile.Name()
		defer os.Remove(tempFilePath)

		// Write artifact to temp file
		if _, err := tempFile.Write([]byte(artifact)); err != nil {
			fmt.Printf("Error writing to temp file: %v\n", err)
			os.Exit(1)
		}
		tempFile.Close()

		// Show diff and get confirmation
		fmt.Println("\nShowing diff between original and proposed changes...")
		cmd := exec.Command("git", "diff", "--", filepath, tempFilePath)
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
			fmt.Printf("Error opening temp file: %v\n", err)
			os.Exit(1)
		}
		defer source.Close()

		dest, err := os.Create(filepath)
		if err != nil {
			fmt.Printf("Error creating destination file: %v\n", err)
			os.Exit(1)
		}
		defer dest.Close()

		_, err = io.Copy(dest, source)
		if err != nil {
			fmt.Printf("Error copying file: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Changes applied successfully")
	},
}

func extractArtifact(response string) string {
	// Split the response into lines
	lines := strings.Split(response, "\n")

	var codeLines []string
	inCodeBlock := false
	fenceCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect code fence boundaries
		if strings.HasPrefix(trimmed, "```") {
			fenceCount++
			inCodeBlock = fenceCount%2 == 1

			// Skip the fence line itself
			continue
		}

		// Collect lines between fences
		if inCodeBlock {
			codeLines = append(codeLines, line)
		}
	}

	// If no fences found or incomplete fence pair, treat entire response as code
	if fenceCount == 0 || fenceCount%2 == 1 {
		return strings.TrimSpace(response)
	}

	// Join the code lines and trim any leading/trailing whitespace
	return strings.TrimSpace(strings.Join(codeLines, "\n"))
}

