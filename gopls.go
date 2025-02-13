package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/jsonrpc2"
)

// DocumentSymbol represents a programming construct like functions
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children"`
}

// Range represents a text range in a document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a line/column position in a document
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// FunctionLocation contains information about a function's location and content
type FunctionLocation struct {
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column"`
}

// TypeLocation contains information about a type's location and content
type TypeLocation struct {
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column"`
}

// streamReadWriteCloser implements io.ReadWriteCloser for jsonrpc2 communication
type streamReadWriteCloser struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (s *streamReadWriteCloser) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *streamReadWriteCloser) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *streamReadWriteCloser) Close() error {
	if err := s.stdin.Close(); err != nil {
		return err
	}
	return s.stdout.Close()
}

func findType(filePath, typeName string) (*TypeLocation, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Read the file content
	fileContent, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Start gopls
	cmd := exec.Command("gopls", "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start gopls: %v", err)
	}
	defer cmd.Process.Kill()

	rwc := &streamReadWriteCloser{stdin: stdin, stdout: stdout}
	stream := jsonrpc2.NewBufferedStream(rwc, jsonrpc2.VSCodeObjectCodec{})
	conn := jsonrpc2.NewConn(context.Background(), stream, jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (interface{}, error) {
		return nil, nil
	}))

	workspaceDir := filepath.Dir(absPath)
	fileURI := "file://" + absPath

	// Initialize gopls
	var initResult interface{}
	err = conn.Call(context.Background(), "initialize", map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   "file://" + workspaceDir,
		"workspaceFolders": []map[string]interface{}{
			{"uri": "file://" + workspaceDir, "name": filepath.Base(workspaceDir)},
		},
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"documentSymbol": map[string]interface{}{
					"hierarchicalDocumentSymbolSupport": true,
				},
			},
			"workspace": map[string]interface{}{
				"workspaceFolders": true,
			},
		},
	}, &initResult)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %v", err)
	}

	// Send required notifications
	err = conn.Notify(context.Background(), "initialized", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to send initialized notification: %v", err)
	}

	err = conn.Notify(context.Background(), "textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        fileURI,
			"languageId": "go",
			"version":    1,
			"text":       string(fileContent),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send didOpen notification: %v", err)
	}

	// Get document symbols
	var symbols []DocumentSymbol
	err = conn.Call(context.Background(), "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI,
		},
	}, &symbols)
	if err != nil {
		return nil, fmt.Errorf("failed to get document symbols: %v", err)
	}

	// Find the type
	var location *TypeLocation
	var findType func([]DocumentSymbol) bool
	findType = func(syms []DocumentSymbol) bool {
		for _, symbol := range syms {
			if symbol.Kind == 23 && symbol.Name == typeName { // 23 is Type kind in LSP
				lines := strings.Split(string(fileContent), "\n")
				content := strings.Join(lines[symbol.Range.Start.Line:symbol.Range.End.Line+1], "\n")
				location = &TypeLocation{
					StartLine:   symbol.Range.Start.Line + 1,
					EndLine:     symbol.Range.End.Line + 1,
					Name:        typeName,
					Content:     content,
					StartColumn: symbol.Range.Start.Character + 1,
					EndColumn:   symbol.Range.End.Character + 1,
				}
				return true
			}
			if len(symbol.Children) > 0 && findType(symbol.Children) {
				return true
			}
		}
		return false
	}
	findType(symbols)

	// Cleanup
	var shutdownResult interface{}
	_ = conn.Call(context.Background(), "shutdown", nil, &shutdownResult)
	_ = conn.Notify(context.Background(), "exit", nil)

	if location == nil {
		return nil, fmt.Errorf("type %s not found in %s", typeName, filePath)
	}

	return location, nil
}

func findFunction(filePath, funcName string) (*FunctionLocation, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Read the file content
	fileContent, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Start gopls
	cmd := exec.Command("gopls", "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start gopls: %v", err)
	}
	defer cmd.Process.Kill()

	rwc := &streamReadWriteCloser{stdin: stdin, stdout: stdout}
	stream := jsonrpc2.NewBufferedStream(rwc, jsonrpc2.VSCodeObjectCodec{})
	conn := jsonrpc2.NewConn(context.Background(), stream, jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (interface{}, error) {
		return nil, nil
	}))

	workspaceDir := filepath.Dir(absPath)
	fileURI := "file://" + absPath

	// Initialize gopls
	var initResult interface{}
	err = conn.Call(context.Background(), "initialize", map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   "file://" + workspaceDir,
		"workspaceFolders": []map[string]interface{}{
			{"uri": "file://" + workspaceDir, "name": filepath.Base(workspaceDir)},
		},
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"documentSymbol": map[string]interface{}{
					"hierarchicalDocumentSymbolSupport": true,
				},
			},
			"workspace": map[string]interface{}{
				"workspaceFolders": true,
			},
		},
	}, &initResult)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %v", err)
	}

	// Send required notifications
	err = conn.Notify(context.Background(), "initialized", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to send initialized notification: %v", err)
	}

	err = conn.Notify(context.Background(), "textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        fileURI,
			"languageId": "go",
			"version":    1,
			"text":       string(fileContent),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send didOpen notification: %v", err)
	}

	// Get document symbols
	var symbols []DocumentSymbol
	err = conn.Call(context.Background(), "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI,
		},
	}, &symbols)
	if err != nil {
		return nil, fmt.Errorf("failed to get document symbols: %v", err)
	}

	// Find the function
	var location *FunctionLocation
	var findFunc func([]DocumentSymbol) bool
	findFunc = func(syms []DocumentSymbol) bool {
		for _, symbol := range syms {
			if symbol.Kind == 12 && symbol.Name == funcName { // 12 is Function kind in LSP
				lines := strings.Split(string(fileContent), "\n")
				content := strings.Join(lines[symbol.Range.Start.Line:symbol.Range.End.Line+1], "\n")
				location = &FunctionLocation{
					StartLine:   symbol.Range.Start.Line + 1,
					EndLine:     symbol.Range.End.Line + 1,
					Name:        funcName,
					Content:     content,
					StartColumn: symbol.Range.Start.Character + 1,
					EndColumn:   symbol.Range.End.Character + 1,
				}
				return true
			}
			if len(symbol.Children) > 0 && findFunc(symbol.Children) {
				return true
			}
		}
		return false
	}
	findFunc(symbols)

	// Cleanup
	var shutdownResult interface{}
	_ = conn.Call(context.Background(), "shutdown", nil, &shutdownResult)
	_ = conn.Notify(context.Background(), "exit", nil)

	if location == nil {
		return nil, fmt.Errorf("function %s not found in %s", funcName, filePath)
	}

	return location, nil
}