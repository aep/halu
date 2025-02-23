package qwen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"halu/glad"
)

type LLM struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewLLM(baseURL string) *LLM {
	return &LLM{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: time.Second * 300,
		},
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	llm      *LLM
	messages []message
	tools    []tool
}

func (l *LLM) NewSession(sa glad.SessionSetup) *Session {
	c := &Session{llm: l}

	for _, t := range sa.Tools {
		// Create JSON schema for parameters
		params := map[string]interface{}{
			"type":       "object",
			"properties": t.Args,
		}
		if len(t.Required) > 0 {
			params["required"] = t.Required
		}

		paramsJSON, _ := json.Marshal(params)

		c.tools = append(c.tools, tool{
			Type: "function",
			Function: function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  paramsJSON,
			},
		})
	}

	c.messages = append(c.messages, message{Role: "system", Content: buildSystemPrompt(sa.System, c.tools)})

	return c
}

func (c *Session) User(content string) {
	c.messages = append(c.messages, message{Role: "user", Content: content})
}

type function struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	Stream      bool      `json:"stream"`
	Tools       []tool    `json:"tools"`
}

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
		Delta        message `json:"delta"`
	} `json:"choices"`
}

func buildSystemPrompt(systemMsg string, tools []tool) string {
	var sb strings.Builder
	sb.WriteString(systemMsg)
	sb.WriteString("\n\n## Tools\n\nYou have access to the following tools:\n\n")

	for _, tool := range tools {
		fn := tool.Function
		sb.WriteString(fmt.Sprintf("### %s\n\n%s: %s Parameters: %s Format the arguments as a JSON object.\n\n",
			fn.Name, fn.Name, fn.Description, string(fn.Parameters)))
	}

	sb.WriteString("## When you need to call a tool, use <tool_call >\n")

	return sb.String()
}

// tokenBuffer helps accumulate and analyze incoming tokens
type tokenBuffer struct {
	content    strings.Builder
	pending    strings.Builder // for tokens we're not sure about yet
	toolCalls  []string        // collects tool calls as raw strings
	inToolCall bool            // whether we're currently in a tool call
}

// normalizeTag removes whitespace from a tag for comparison
func normalizeTag(tag string) string {
	return strings.Join(strings.Fields(tag), "")
}

// tryFlushText attempts to flush accumulated text if it's not part of a tool call
func tryFlushText(buf *tokenBuffer, cb glad.Callbacks) {
	if !buf.inToolCall && buf.content.Len() > 0 {
		if cb.Text != nil {
			cb.Text(buf.content.String())
		}
		buf.content.Reset()
	}
}

// tryFlushPending moves pending content to main content and flushes if not in tool call
func tryFlushPending(buf *tokenBuffer, cb glad.Callbacks) {
	if buf.pending.Len() > 0 {
		buf.content.WriteString(buf.pending.String())
		buf.pending.Reset()
		if !buf.inToolCall {
			tryFlushText(buf, cb)
		}
	}
}

func (c *Session) Complete(ctx context.Context, cb glad.Callbacks) error {
	req := chatCompletionRequest{
		Model:    "Qwen/Qwen2.5-Coder-32B-Instruct-AWQ",
		Stream:   true,
		Tools:    c.tools,
		Messages: c.messages,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.llm.BaseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.llm.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent string
	buf := &tokenBuffer{}

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		line = bytes.TrimPrefix(line, []byte("data: "))

		if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
			break
		}

		var streamResp chatCompletionResponse
		if err := json.Unmarshal(line, &streamResp); err != nil {
			return fmt.Errorf("error unmarshaling stream response: %w\nline: %s", err, string(line))
		}

		for _, choice := range streamResp.Choices {
			content := choice.Delta.Content
			if content == "" {
				content = choice.Message.Content
			}

			if content != "" {
				fullContent += content

				// Process content character by character
				for i := 0; i < len(content); i++ {
					ch := content[i]
					if ch == '<' {
						// Start buffering in pending
						buf.pending.WriteByte(ch)
					} else if buf.pending.Len() > 0 {
						// Already buffering, continue in pending
						buf.pending.WriteByte(ch)

						pendingStr := buf.pending.String()
						if strings.HasSuffix(pendingStr, ">") {
							normalized := normalizeTag(pendingStr)
							if normalized == "<tool_call>" {
								// Found start of tool call
								buf.inToolCall = true
								buf.content.WriteString(pendingStr)
								buf.pending.Reset()
							} else if normalized == "</tool_call>" {
								// Found end of tool call
								buf.content.WriteString(pendingStr)
								// Store the complete tool call
								buf.toolCalls = append(buf.toolCalls, buf.content.String())
								buf.content.Reset()
								buf.pending.Reset()
								buf.inToolCall = false
							} else if !strings.HasPrefix(normalized, "<tool_call") && !strings.HasPrefix(normalized, "</tool_call") {
								// Not a tool call tag, flush pending as regular text
								tryFlushPending(buf, cb)
							}
						} else if buf.pending.Len() > 20 {
							// Too long to be a tag, flush as regular text
							tryFlushPending(buf, cb)
						}
					} else {
						// Regular character outside of potential tag
						buf.content.WriteByte(ch)
						tryFlushText(buf, cb)
					}
				}
			}
		}
	}

	// Final flushes
	tryFlushPending(buf, cb)
	tryFlushText(buf, cb)

	c.messages = append(c.messages, message{
		Role:    "assistant",
		Content: fullContent,
	})

	// Process any collected tool calls
	for _, rawCall := range buf.toolCalls {
		// Extract JSON content between tags
		start := strings.Index(rawCall, ">") + 1
		end := strings.LastIndex(rawCall, "<")
		if start > 0 && end > start {
			jsonStr := strings.TrimSpace(rawCall[start:end])
			var call map[string]any
			if err := json.Unmarshal([]byte(jsonStr), &call); err == nil {
				if name, ok := call["name"].(string); ok {
					result := cb.Tool(name, call)
					c.messages = append(c.messages, message{
						Role:    "system",
						Content: fmt.Sprintf("%s\n", result),
					})
				}
			}
		}
	}

	if len(buf.toolCalls) > 0 {
		return c.Complete(ctx, cb)
	}

	fmt.Println()
	return nil
}
