package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"halu/glad"
	"halu/glad/qwen"
)

var styleToolCall = lipgloss.NewStyle().
	Bold(true).
	Padding(1).
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63"))

var styleHeader = lipgloss.NewStyle().
	Underline(true)

func main() {
	baseURL := flag.String("url", "http://localhost:8000", "vLLM server base URL")
	prompt := flag.String("prompt", "", "Text prompt for completion")
	flag.Parse()

	if *prompt == "" {
		fmt.Println("Error: prompt is required")
		flag.Usage()
		os.Exit(1)
	}

	llm := qwen.NewLLM(*baseURL)

	chat := llm.NewSession(glad.SessionSetup{
		System: "you are GLaDOS, a coding assistant",
		Tools: []glad.Tool{
			{
				Name:        "getCurrentTime",
				Description: "Get the current time in RFC3339 format",
			},
			{
				Name:        "listFiles",
				Description: "list files in current directory",
			},
			{
				Name:        "readFile",
				Description: "read a file",
				Args: map[string]glad.Arg{
					"path": {
						Type: "string",
					},
				},
			},
		},
	})

	fmt.Println(styleHeader.Render("hi!"))

	chat.User(*prompt)

	fmt.Println()
	err := chat.Complete(context.TODO(), glad.Callbacks{
		Text: func(content string) {
			fmt.Print(content)
		},
		Tool: func(name string, arg map[string]any) string {
			fmt.Println()
			fmt.Println(styleToolCall.Render(fmt.Sprintf("%s %s >", name, arg)))
			fmt.Println()
			if name == "readFile" {
				return "package main\n func main() {panic(1)}"
			}
			return time.Now().Format("2006-01-02 15:04:05 MST")
		},
	})
	if err != nil {
		panic(err)
	}
}
