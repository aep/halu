package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
)

type Prompt struct {
	rl      *readline.Instance
	history string
}

func NewPrompt(historyFile string) (*Prompt, error) {
	// Ensure history directory exists
	historyDir := filepath.Dir(historyFile)
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %v", err)
	}

	// Create readline instance
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            color.GreenString("➤ "),
		HistoryFile:       historyFile,
		HistorySearchFold: true,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create readline instance: %v", err)
	}

	return &Prompt{
		rl:      rl,
		history: historyFile,
	}, nil
}

// GetMultiLineInput reads input from the user, treating backslash at the end
// of a line as a continuation marker
func (p *Prompt) GetMultiLineInput() (string, error) {
	var lines []string
	//fmt.Println(color.YellowString("Enter prompt (backslash \\ for continuation):"))

	firstLine := true
	for {
		// Use continuation prompt for subsequent lines
		if !firstLine {
			p.rl.SetPrompt(color.GreenString("... "))
		}

		line, err := p.rl.Readline()
		if err != nil {
			// Handle Ctrl+C
			if err == readline.ErrInterrupt {
				return "", fmt.Errorf("interrupted")
			}
			// Handle Ctrl+D
			if err == io.EOF {
				break
			}
			return "", err
		}

		firstLine = false

		// Check if line ends with backslash
		line = strings.TrimRight(line, " \t") // Remove trailing whitespace
		if strings.HasSuffix(line, "\\") {
			// Remove the backslash and continue reading
			line = strings.TrimSuffix(line, "\\")
			lines = append(lines, line)
			continue
		}

		lines = append(lines, line)
		break // No backslash at the end, we're done
	}

	// Reset prompt to original state
	p.rl.SetPrompt(color.GreenString("➤ "))

	return strings.Join(lines, "\n"), nil
}

// AddToHistory adds a line to the history file
func (p *Prompt) AddToHistory(input string) error {
	// Split multi-line input and add each line to history
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		p.rl.SaveHistory(line)
	}
	return nil
}

// LoadHistory loads the command history into memory
func (p *Prompt) LoadHistory() ([]string, error) {
	content, err := ioutil.ReadFile(p.history)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var history []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			history = append(history, line)
		}
	}
	return history, nil
}

// Close cleans up the readline instance
func (p *Prompt) Close() error {
	return p.rl.Close()
}

// DefaultHistoryFile returns the default history file location
func DefaultHistoryFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".halu_history")
}

