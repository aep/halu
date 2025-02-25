package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

// Agent represents our AI agent with its tools and client
type Agent struct {
	client *anthropic.Client
	vllm   *VLLMClient
	tools  map[string]Tool
	yolo   bool
	local  bool
}

// VLLMClient handles interactions with local LLM endpoint
type VLLMClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewVLLMClient(baseURL string) *VLLMClient {
	return &VLLMClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: time.Second * 300,
		},
	}
}

// Logger colors
var (
	stepColor    = color.New(color.FgCyan)
	toolColor    = color.New(color.FgGreen)
	promptColor  = color.New(color.FgYellow)
	resultColor  = color.New(color.FgMagenta)
	errorColor   = color.New(color.FgRed)
	messageColor = color.New(color.FgBlue)
)

// prettyPrint formats and prints JSON-like data
func prettyPrint(data interface{}) string {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(bytes)
}

// NewAgent creates a new AI agent with the given API key
func NewAgent(yolo bool, local bool) (*Agent, error) {
	// Load environment variables
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get home directory: %v", err)
	} else {
		envPath := filepath.Join(homeDir, ".halu.env")
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("Warning: .halu.env file not found in home directory")
		}
	}

	var client *anthropic.Client
	var vllm *VLLMClient

	if local {
		// Get local endpoint from environment
		endpoint := os.Getenv("HALU_LOCAL_LLM_ENDPOINT")
		if endpoint == "" {
			return nil, fmt.Errorf("HALU_LOCAL_LLM_ENDPOINT environment variable not set")
		}

		// Create VLLM client
		vllm = NewVLLMClient(endpoint)
	} else {
		// Get API key from environment
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
		}

		// Create Anthropic client
		client = anthropic.NewClient(
			option.WithAPIKey(apiKey),
		)
	}

	agent := &Agent{
		client: client,
		vllm:   vllm,
		tools:  make(map[string]Tool),
		yolo:   yolo,
		local:  local,
	}

	// Register tools
	agent.registerTools()

	return agent, nil
}

// Analyze starts the code analysis with the given prompt
func (a *Agent) Run(ctx context.Context, prompt string, messages []anthropic.MessageParam) (string, []anthropic.MessageParam, error) {
	if a.local {
		// Convert Anthropic messages to VLLM format
		vllmMessages := convertToVLLMMessages(messages)
		response, newVLLMMessages, err := a.runLocal(ctx, prompt, vllmMessages)
		if err != nil {
			return "", messages, err
		}
		// Convert VLLM messages back to Anthropic format
		newMessages := convertToAnthropicMessages(newVLLMMessages)
		return response, newMessages, nil
	}
	// Convert tools to the format expected by the Anthropic API
	var toolParams []anthropic.ToolParam
	for _, tool := range a.tools {
		toolParams = append(toolParams, anthropic.ToolParam{
			Name:        anthropic.F(tool.Name),
			Description: anthropic.F(tool.Description),
			InputSchema: anthropic.F(interface{}(tool.InputSchema)),
		})
	}

	// Only add new message if prompt is not empty
	if strings.TrimSpace(prompt) != "" {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)))
	}

	// Create the streaming message
	stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F("claude-3-7-sonnet-latest"),
		MaxTokens: anthropic.F(int64(4096)),
		Messages:  anthropic.F(messages),
		Tools:     anthropic.F(toolParams),
	})

	message := anthropic.Message{}
	needsToolExecution := false

	// Process the stream
	for stream.Next() {
		event := stream.Current()
		message.Accumulate(event)

		// Handle content blocks deltas for streaming output
		if event.Type == anthropic.MessageStreamEventTypeContentBlockDelta {
			delta := event.Delta.(anthropic.ContentBlockDeltaEventDelta)
			if delta.Type == anthropic.ContentBlockDeltaEventDeltaTypeTextDelta {
				fmt.Print(delta.Text)
			}
		}
	}

	if stream.Err() != nil {
		return "", messages, fmt.Errorf("streaming error: %v", stream.Err())
	}

	fmt.Println() // Add newline after streaming

	// Add assistant's message to history
	messages = append(messages, message.ToParam())

	// Process any tool calls
	for _, block := range message.Content {
		if block.Type == "tool_use" {
			needsToolExecution = true

			// Execute the tool
			tool, ok := a.tools[block.Name]
			if !ok {
				return "", messages, fmt.Errorf("unknown tool: %s", block.Name)
			}

			var input map[string]interface{}
			inputBytes, _ := json.Marshal(block.Input)
			if err := json.Unmarshal(inputBytes, &input); err != nil {
				return "", messages, fmt.Errorf("failed to parse tool input: %v", err)
			}

			// Print tool call with input parameters
			inputStr := prettyPrint(input)
			if len(inputStr) > 100 {
				inputStr = inputStr[:97] + "..."
			}
			toolColor.Printf("\n➤ tool: %s(%s)\n", block.Name, inputStr)

			result, err := tool.Execute(input)
			errorStr := ""
			if err != nil {
				errorStr = fmt.Sprintf("Error: %v", err)
				errorColor.Printf("➤ Tool execution failed: %v\n", err)
				result = fmt.Sprintf("tool execution failed: %s", errorStr)
			}

			// Add the tool result to the conversation
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(block.ID, result, false),
			))

			// Get the next message with the tool result
			return a.Run(ctx, "", messages)
		}
	}

	if !needsToolExecution {
		// Build final response from message content
		var finalResponse string
		for _, block := range message.Content {
			if block.Type == "text" {
				finalResponse += block.Text
			}
		}

		stepColor.Println("\n➤ done")
		return finalResponse, messages, nil
	}

	return "", messages, nil
}

// prettyTruncate truncates long results for display
func prettyTruncate(result string) string {
	maxLen := 1000
	if len(result) > maxLen {
		return result[:maxLen] + "... [truncated]"
	}
	return result
}

func main() {
	// Add flags
	yolo := flag.Bool("yolo", false, "Skip confirmation when writing files")
	local := flag.Bool("local", false, "Use local LLM endpoint instead of Anthropic API")
	flag.Parse()

	agent, err := NewAgent(*yolo, *local)
	if err != nil {
		errorColor.Printf("Failed to create agent: %v\n", err)
		os.Exit(1)
	}

	p, err := NewPrompt(DefaultHistoryFile())
	if err != nil {
		errorColor.Printf("Failed to create prompt: %v\n", err)
		os.Exit(1)
	}
	defer p.Close()

	ctx := context.Background()
	var messages []anthropic.MessageParam

	// Main conversation loop
	for {
		// Get user input
		input, err := p.GetMultiLineInput()
		if err != nil {
			panic(err)
		}
		fmt.Println()
		if input == "" {
			return
		}

		// Save to history
		if err := p.AddToHistory(input); err != nil {
			errorColor.Printf("Failed to save history: %v\n", err)
		}

		_, newMessages, err := agent.Run(ctx, input, messages)
		if err != nil {
			errorColor.Printf("Analysis failed: %v\n", err)
			continue
		}

		// Update conversation history
		messages = newMessages

		fmt.Println()
	}
}
