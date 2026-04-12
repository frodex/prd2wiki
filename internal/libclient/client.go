// Package libclient calls pippi-librarian over a unix HTTP socket (POST /tools/call).
package libclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal HTTP client for the librarian tool endpoint.
type Client struct {
	http    *http.Client
	socket  string
	apiKey  string
	baseURL string // host part for http.Request (unix transport ignores host)
}

// toolCallRequest matches pippi-librarian internal/librarian.ToolCallRequest.
type toolCallRequest struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// toolCallResponse matches pippi-librarian ToolCallResponse (partial).
type toolCallResponse struct {
	OK      bool `json:"ok"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
	Error string `json:"error,omitempty"`
}

// New returns a client for the given unix socket path, or nil if socketPath is empty.
// It checks connectivity at creation time and returns an error if the socket is not reachable.
func New(socketPath, apiKey string) (*Client, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return nil, nil
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	c := &Client{
		http: &http.Client{
			Transport: tr,
			Timeout:   60 * time.Second,
		},
		socket:  socketPath,
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: "http://unix",
	}
	// BUG-008: Check connectivity at creation time so startup logs an error
	// instead of silently failing on first sync attempt.
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return c, fmt.Errorf("libclient: socket %s not reachable: %w (sync will fail until librarian starts)", socketPath, err)
	}
	conn.Close()
	return c, nil
}

// MemoryStore calls memory_store and returns the new head record_id (mem_…).
func (c *Client) MemoryStore(ctx context.Context, namespace, pageUUID, content string, metadata map[string]any) (string, error) {
	if c == nil || c.http == nil {
		return "", fmt.Errorf("libclient: nil client")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(pageUUID) == "" {
		return "", fmt.Errorf("libclient: namespace and page_uuid required")
	}
	args := map[string]any{
		"namespace": namespace,
		"page_uuid": pageUUID,
		"content":   content,
	}
	if metadata != nil {
		args["metadata"] = metadata
	}
	body, err := json.Marshal(toolCallRequest{Name: "memory_store", Args: args})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools/call", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tcr toolCallResponse
	if err := json.Unmarshal(raw, &tcr); err != nil {
		return "", fmt.Errorf("libclient: decode response: %w; body=%s", err, truncate(string(raw), 500))
	}
	if !tcr.OK || len(tcr.Content) == 0 {
		if tcr.Error != "" {
			return "", fmt.Errorf("libclient: memory_store: %s", tcr.Error)
		}
		return "", fmt.Errorf("libclient: memory_store failed (HTTP %d)", resp.StatusCode)
	}
	text := tcr.Content[0].Text
	var payload struct {
		RecordID string `json:"record_id"`
		Version  int    `json:"version"`
		Created  bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return "", fmt.Errorf("libclient: parse tool result JSON: %w", err)
	}
	if strings.TrimSpace(payload.RecordID) == "" {
		return "", fmt.Errorf("libclient: empty record_id in response")
	}
	return payload.RecordID, nil
}

// MemorySearchHit is one row from memory_search (matches pippi-librarian JSON).
type MemorySearchHit struct {
	PageUUID     string  `json:"page_uuid"`
	RecordID     string  `json:"record_id"`
	Title        string  `json:"title"`
	Snippet      string  `json:"snippet"`
	Score        float64 `json:"score"`
	HistoryCount int     `json:"history_count"`
}

// MemorySearch calls memory_search and returns decoded matches.
func (c *Client) MemorySearch(ctx context.Context, namespace, query string, limit int, deep bool) ([]MemorySearchHit, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("libclient: nil client")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("libclient: namespace and query required")
	}
	if limit <= 0 {
		limit = 10
	}
	args := map[string]any{
		"namespace": namespace,
		"query":     query,
		"limit":     limit,
		"deep":      deep,
	}
	body, err := json.Marshal(toolCallRequest{Name: "memory_search", Args: args})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools/call", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var tcr toolCallResponse
	if err := json.Unmarshal(raw, &tcr); err != nil {
		return nil, fmt.Errorf("libclient: decode response: %w; body=%s", err, truncate(string(raw), 500))
	}
	if !tcr.OK || len(tcr.Content) == 0 {
		if tcr.Error != "" {
			return nil, fmt.Errorf("libclient: memory_search: %s", tcr.Error)
		}
		return nil, fmt.Errorf("libclient: memory_search failed (HTTP %d)", resp.StatusCode)
	}
	text := tcr.Content[0].Text
	var payload struct {
		Matches []MemorySearchHit `json:"matches"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, fmt.Errorf("libclient: parse memory_search JSON: %w", err)
	}
	return payload.Matches, nil
}

// MemoryDelete calls memory_delete with the given record ID (mem_…).
func (c *Client) MemoryDelete(ctx context.Context, id string) error {
	if c == nil || c.http == nil {
		return fmt.Errorf("libclient: nil client")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("libclient: id required")
	}
	args := map[string]any{
		"id": id,
	}
	body, err := json.Marshal(toolCallRequest{Name: "memory_delete", Args: args})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools/call", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var tcr toolCallResponse
	if err := json.Unmarshal(raw, &tcr); err != nil {
		return fmt.Errorf("libclient: decode response: %w; body=%s", err, truncate(string(raw), 500))
	}
	if !tcr.OK {
		if tcr.Error != "" {
			return fmt.Errorf("libclient: memory_delete: %s", tcr.Error)
		}
		return fmt.Errorf("libclient: memory_delete failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
