package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

// Agent represents our AI agent with its tools and client
type Agent struct {
	client *anthropic.Client
	tools  map[string]Tool
	yolo   bool
}

// TokenUsage tracks token usage statistics
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

// Logger colors
var (
	stepColor    = color.New(color.FgCyan)
	toolColor    = color.New(color.FgGreen)
	promptColor  = color.New(color.FgYellow)
	resultColor  = color.New(color.FgMagenta)
	errorColor   = color.New(color.FgRed)
	messageColor = color.New(color.FgBlue)
	tokenColor   = color.New(color.FgHiBlue)
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

	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	// Create Anthropic client
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	agent := &Agent{
		client: client,
		tools:  make(map[string]Tool),
		yolo:   yolo,
	}

	// Register tools
	agent.registerTools()

	return agent, nil
}

// Run starts the interaction with the given prompt
func (a *Agent) Run(ctx context.Context, prompt string, messages []anthropic.MessageParam) (string, []anthropic.MessageParam, TokenUsage, error) {
	// Initialize token usage
	tokenUsage := TokenUsage{}

	// Convert tools to the format expected by the Anthropic API
	var toolParams []anthropic.ToolUnionUnionParam
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

	// Prepare parameters for streaming message
	streamParams := anthropic.MessageNewParams{
		Model:     anthropic.F("claude-3-7-sonnet-latest"),
		MaxTokens: anthropic.F(int64(4096)),
		Messages:  anthropic.F(messages),
		Tools:     anthropic.F(toolParams),
	}

	// Convert tools to MessageCountTokensToolUnionParam type for token counting
	var tokenCountToolParams []anthropic.MessageCountTokensToolUnionParam
	for _, tool := range toolParams {
		if tp, ok := tool.(anthropic.ToolParam); ok {
			tokenCountToolParams = append(tokenCountToolParams, tp)
		}
	}

	// Get input token count first
	tokensCountResult, err := a.client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{
		Model:    streamParams.Model,
		Messages: streamParams.Messages,
		Tools:    anthropic.F(tokenCountToolParams),
	})
	if err != nil {
		log.Printf("Warning: Failed to count input tokens: %v", err)
	} else {
		tokenUsage.InputTokens = tokensCountResult.InputTokens
	}

	// Retry logic for streaming errors
	maxRetries := 10
	var message anthropic.Message

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create the streaming message
		stream := a.client.Messages.NewStreaming(ctx, streamParams)
		message = anthropic.Message{}

		// Process the stream
		for stream.Next() {
			event := stream.Current()
			message.Accumulate(event)

			// Track token usage from message delta events
			if event.Type == anthropic.MessageStreamEventTypeMessageDelta {
				if messageEvent, ok := event.AsUnion().(anthropic.MessageDeltaEvent); ok {
					tokenUsage.OutputTokens = messageEvent.Usage.OutputTokens
				}
			}

			// Handle content blocks deltas for streaming output
			if event.Type == anthropic.MessageStreamEventTypeContentBlockDelta {
				delta := event.Delta.(anthropic.ContentBlockDeltaEventDelta)
				if delta.Type == anthropic.ContentBlockDeltaEventDeltaTypeTextDelta {
					fmt.Print(delta.Text)
				}
			}
		}

		// Check for errors
		if stream.Err() != nil {
			errMsg := stream.Err().Error()
			if attempt < maxRetries {
				fmt.Printf("\n[Retrying due to streaming error %s... Attempt %d/%d]\n", errMsg, attempt+1, maxRetries)
				continue // Retry
			}

			// If we've reached max retries or it's a different error, return the error
			return "", messages, tokenUsage, fmt.Errorf("streaming error: %v", stream.Err())
		}

		// If we got here, streaming completed successfully
		break
	}

	fmt.Println() // Add newline after streaming

	// Get final token usage from the complete message
	if message.Usage.InputTokens > 0 {
		tokenUsage.InputTokens = message.Usage.InputTokens
	}
	if message.Usage.OutputTokens > 0 {
		tokenUsage.OutputTokens = message.Usage.OutputTokens
	}

	// Add assistant's message to history
	messages = append(messages, message.ToParam())

	// Process any tool calls
	needsToolExecution := false
	for _, block := range message.Content {
		if block.Type == "tool_use" {
			needsToolExecution = true

			// Execute the tool
			tool, ok := a.tools[block.Name]
			if !ok {
				return "", messages, tokenUsage, fmt.Errorf("unknown tool: %s", block.Name)
			}

			var input map[string]interface{}
			inputBytes, _ := json.Marshal(block.Input)
			if err := json.Unmarshal(inputBytes, &input); err != nil {
				return "", messages, tokenUsage, fmt.Errorf("failed to parse tool input: %v", err)
			}

			// Print tool call with input parameters
			inputStr := prettyPrint(input)

			// For write_file, ensure the path is always shown in the debug output
			if block.Name == "write_file" && input["path"] != nil {
				path := input["path"].(string)
				if len(inputStr) > 100 {
					toolColor.Printf("\n➤ tool: %s(path: %s, content: [truncated])\n", block.Name, path)
				} else {
					toolColor.Printf("\n➤ tool: %s(%s)\n", block.Name, inputStr)
				}
			} else {
				// Default behavior for other tools
				if len(inputStr) > 100 {
					inputStr = inputStr[:97] + "..."
				}
				toolColor.Printf("\n➤ tool: %s(%s)\n", block.Name, inputStr)
			}

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

			// Print token usage for the current step
			tokenColor.Printf("\n⚙ used %d input, %d output tokens\n", tokenUsage.InputTokens, tokenUsage.OutputTokens)

			// Get the next message with the tool result
			finalResponse, newMessages, newTokenUsage, err := a.Run(ctx, "", messages)

			// Accumulate the token usage from recursive calls
			tokenUsage.InputTokens += newTokenUsage.InputTokens
			tokenUsage.OutputTokens += newTokenUsage.OutputTokens

			return finalResponse, newMessages, tokenUsage, err
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
		return finalResponse, messages, tokenUsage, nil
	}

	return "", messages, tokenUsage, nil
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
	var totalInputTokens, totalOutputTokens int64

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

		// Run with the input
		_, newMessages, tokenUsage, err := agent.Run(ctx, input, messages)
		if err != nil {
			errorColor.Printf("%s\n", err)
			continue
		}

		// Update conversation history
		messages = newMessages

		// Update and display total token usage
		totalInputTokens += tokenUsage.InputTokens
		totalOutputTokens += tokenUsage.OutputTokens
		
		// Calculate costs (Claude pricing: $3/M for input, $15/M for output)
		inputCost := float64(tokenUsage.InputTokens) * 0.000003
		outputCost := float64(tokenUsage.OutputTokens) * 0.000015
		totalCost := inputCost + outputCost
		
		totalInputCost := float64(totalInputTokens) * 0.000003
		totalOutputCost := float64(totalOutputTokens) * 0.000015
		totalSessionCost := totalInputCost + totalOutputCost
		
		tokenColor.Printf("\n⚙ Token usage summary:\n")
		tokenColor.Printf("   - This interaction: %d input ($%.4f), %d output ($%.4f) tokens, total cost: $%.4f\n", 
			tokenUsage.InputTokens, inputCost, tokenUsage.OutputTokens, outputCost, totalCost)
		tokenColor.Printf("   - Total session: %d input ($%.4f), %d output ($%.4f) tokens, total cost: $%.4f\n", 
			totalInputTokens, totalInputCost, totalOutputTokens, totalOutputCost, totalSessionCost)

		fmt.Println()
	}
}
