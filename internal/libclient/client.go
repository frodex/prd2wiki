// Package libclient calls pippi-librarian over a unix HTTP socket (POST /tools/call).
package libclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is a minimal HTTP client for the librarian tool endpoint.
type Client struct {
	http    *http.Client
	socket  string
	apiKey  string
	baseURL string // host part for http.Request (unix transport ignores host)
	ticket  ticketState
}

// ticketState caches a ticket issued via auth_ticket_issue (ADR-0007).
// Tickets are in-memory on the server — no DB. PID-bound via SO_PEERCRED.
type ticketState struct {
	sync.Mutex
	enabled  bool
	steps    []string
	ticketID string
	chainID  string
	expiry   time.Time
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

// NewHTTP returns a client that calls the librarian over plain HTTP (e.g. loopback SSE endpoint).
// Use this for admin tools like backfill where the socket's ticket auth is not available.
func NewHTTP(baseURL, apiKey string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
	}
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

// EnableTicketAuth activates the ADR-0007 ticket protocol. Call once after creating the client.
// Steps lists the tool names this client will call (e.g. "memory_store", "memory_search").
func (c *Client) EnableTicketAuth(steps []string) {
	c.ticket.Lock()
	defer c.ticket.Unlock()
	c.ticket.enabled = true
	c.ticket.steps = steps
}

func (c *Client) ensureTicket(ctx context.Context) error {
	c.ticket.Lock()
	defer c.ticket.Unlock()
	if !c.ticket.enabled {
		return nil
	}
	if c.ticket.ticketID != "" && time.Now().Before(c.ticket.expiry.Add(-30*time.Second)) {
		return nil
	}
	return c.issueTicketLocked(ctx)
}

func (c *Client) issueTicketLocked(ctx context.Context) error {
	chainID := fmt.Sprintf("wiki-%d", time.Now().UnixNano())
	args := map[string]any{
		"chain_id": chainID,
		"steps":    c.ticket.steps,
		"scopes":   []string{"disclose:content", "disclose:metadata"},
	}
	body, err := json.Marshal(toolCallRequest{Name: "auth_ticket_issue", Args: args})
	if err != nil {
		return fmt.Errorf("libclient: ticket issue marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools/call", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("libclient: ticket issue request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// No ticket headers on auth_ticket_issue — only peer creds (automatic on socket)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("libclient: ticket issue: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("libclient: ticket issue read: %w", err)
	}
	var tcr toolCallResponse
	if err := json.Unmarshal(raw, &tcr); err != nil {
		return fmt.Errorf("libclient: ticket issue decode: %w; body=%s", err, truncate(string(raw), 500))
	}
	if !tcr.OK || len(tcr.Content) == 0 {
		return fmt.Errorf("libclient: ticket issue failed: %s", tcr.Error)
	}
	var payload struct {
		TicketID  string `json:"ticket_id"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal([]byte(tcr.Content[0].Text), &payload); err != nil {
		return fmt.Errorf("libclient: ticket issue parse: %w", err)
	}
	if payload.TicketID == "" {
		return fmt.Errorf("libclient: ticket issue returned empty ticket_id")
	}
	c.ticket.ticketID = payload.TicketID
	c.ticket.chainID = chainID
	c.ticket.expiry, _ = time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	return nil
}

func (c *Client) applyTicketHeaders(req *http.Request, toolName string) {
	c.ticket.Lock()
	defer c.ticket.Unlock()
	if !c.ticket.enabled || c.ticket.ticketID == "" {
		return
	}
	nonce := make([]byte, 12)
	rand.Read(nonce)
	req.Header.Set("X-Pippi-Ticket", c.ticket.ticketID)
	req.Header.Set("X-Pippi-Chain-Id", c.ticket.chainID)
	req.Header.Set("X-Pippi-Step-Id", toolName)
	req.Header.Set("X-Pippi-Nonce", hex.EncodeToString(nonce))
}

func (c *Client) invalidateTicket() {
	c.ticket.Lock()
	defer c.ticket.Unlock()
	c.ticket.ticketID = ""
}

// isTicketError returns true if the response indicates a ticket auth failure that should be retried.
func isTicketError(tcr toolCallResponse) bool {
	if tcr.OK {
		return false
	}
	e := strings.ToLower(tcr.Error)
	return strings.Contains(e, "ticket") || strings.Contains(e, "nonce") || strings.Contains(e, "expired")
}

// doCall executes a tool call with ticket auth, retrying once on ticket errors.
func (c *Client) doCall(ctx context.Context, toolName string, args map[string]any) (*toolCallResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := c.ensureTicket(ctx); err != nil {
			return nil, err
		}
		body, err := json.Marshal(toolCallRequest{Name: toolName, Args: args})
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
		c.applyTicketHeaders(req, toolName)

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var tcr toolCallResponse
		if err := json.Unmarshal(raw, &tcr); err != nil {
			return nil, fmt.Errorf("libclient: decode response: %w; body=%s", err, truncate(string(raw), 500))
		}
		if isTicketError(tcr) && attempt == 0 {
			c.invalidateTicket()
			continue
		}
		return &tcr, nil
	}
	return nil, fmt.Errorf("libclient: %s: ticket auth failed after retry", toolName)
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
	tcr, err := c.doCall(ctx, "memory_store", args)
	if err != nil {
		return "", err
	}
	if !tcr.OK || len(tcr.Content) == 0 {
		if tcr.Error != "" {
			return "", fmt.Errorf("libclient: memory_store: %s", tcr.Error)
		}
		return "", fmt.Errorf("libclient: memory_store failed")
	}
	var payload struct {
		RecordID string `json:"record_id"`
		Version  int    `json:"version"`
		Created  bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(tcr.Content[0].Text), &payload); err != nil {
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
	tcr, err := c.doCall(ctx, "memory_search", args)
	if err != nil {
		return nil, err
	}
	if !tcr.OK || len(tcr.Content) == 0 {
		if tcr.Error != "" {
			return nil, fmt.Errorf("libclient: memory_search: %s", tcr.Error)
		}
		return nil, fmt.Errorf("libclient: memory_search failed")
	}
	var payload struct {
		Matches []MemorySearchHit `json:"matches"`
	}
	if err := json.Unmarshal([]byte(tcr.Content[0].Text), &payload); err != nil {
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
	tcr, err := c.doCall(ctx, "memory_delete", map[string]any{"id": id})
	if err != nil {
		return err
	}
	if !tcr.OK {
		if tcr.Error != "" {
			return fmt.Errorf("libclient: memory_delete: %s", tcr.Error)
		}
		return fmt.Errorf("libclient: memory_delete failed")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
