package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}
type Client struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *bufio.Scanner
	next  int
	mu    sync.Mutex
}

// Start launches a configured stdio MCP server and performs the protocol handshake.
func Start(ctx context.Context, server Server) (*Client, error) {
	if server.Command == "" {
		return nil, fmt.Errorf("MCP server %q has no command", server.Name)
	}
	cmd := exec.CommandContext(ctx, server.Command, server.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{cmd: cmd, stdin: stdin, out: bufio.NewScanner(stdout)}
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "mihani", "version": "0.1.0"}}); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n"))
	return c, nil
}
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	id := c.next
	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	data, _ := json.Marshal(request)
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}
	for c.out.Scan() {
		var response struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(c.out.Bytes(), &response) != nil {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("MCP %d: %s", response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	}
	return nil, fmt.Errorf("MCP server closed its output")
}
func (c *Client) Tools() ([]Tool, error) {
	result, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []Tool `json:"tools"`
	}
	return payload.Tools, json.Unmarshal(result, &payload)
}

func (c *Client) ToolSchemas() ([]map[string]any, error) {
	list, err := c.Tools()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(list))
	for _, tool := range list {
		result = append(result, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
	}
	return result, nil
}
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	type result struct {
		Content []map[string]any `json:"content"`
	}
	response, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var parsed result
	if err := json.Unmarshal(response, &parsed); err != nil {
		return nil, err
	}
	return json.Marshal(parsed.Content)
}
func (c *Client) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
