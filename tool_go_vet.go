package main

import (
	"os/exec"
)

func registerGoVetTool(a *Agent) {
	a.tools["go_vet"] = Tool{
		Name:        "go_vet",
		Description: "Run static analysis on Go code using go vet to detect potential errors",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the Go file or directory to analyze or ./... for the entire project",
				},
			},
			"required": []string{"path"},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			path := input["path"].(string)

			// Execute the go vet command
			cmd := exec.Command("go", "vet", path)
			output, err := cmd.CombinedOutput()

			// We don't return the error because go vet will exit with non-zero
			// status when it finds issues, but we still want to see those issues
			if err != nil {
				if len(output) == 0 {
					return "Error running go vet: " + err.Error(), nil
				}
				return string(output), nil
			}

			// If no issues were found
			if len(output) == 0 {
				return "No issues found by go vet.", nil
			}

			return string(output), nil
		},
	}
}

