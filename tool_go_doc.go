package main

import (
	"os/exec"
)

func registerGoDocTool(a *Agent) {
	a.tools["go_doc"] = Tool{
		Name:        "go_doc",
		Description: "Get documentation for Go packages, types, functions, methods, etc.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The Go package, function, method, or type to get documentation for (e.g., 'io/ioutil', 'json.Marshal', 'os.File')",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(input map[string]interface{}) (string, error) {
			query := input["query"].(string)

			// Execute the go doc command
			cmd := exec.Command("go", "doc", "-all", query)
			output, err := cmd.CombinedOutput()
			if err != nil {
				// If go doc returns an error, include both the error and any output
				// as the command might have returned partial documentation or helpful error messages
				return string(output) + "\nError: " + err.Error(), nil
			}

			return string(output), nil
		},
	}
}
