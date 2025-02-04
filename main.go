package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get home directory: %v", err)
	} else {
		envPath := filepath.Join(homeDir, ".halu.env")
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("Warning: .halu.env file not found in home directory")
		}
	}
	
	if err := fixCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

const ANTHROPIC_API_URL = "https://api.anthropic.com/v1/messages"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
}

type Response struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func chatWithClaude(prompt, apiKey string, systemPrompt ...string) (string, error) {
	messages := []Message{}

	messages = append(messages, Message{
		Role:    "user",
		Content: prompt,
	})

	request := Request{
		Model:     "claude-3-5-sonnet-20241022",
		Messages:  messages,
		MaxTokens: 4024,
	}
	if len(systemPrompt) > 0 {
		request.System = systemPrompt[0]
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", ANTHROPIC_API_URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if response.Error.Message != "" {
		return "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Content) > 0 {
		return response.Content[0].Text, nil
	}

	return "", fmt.Errorf("no content in response")
}