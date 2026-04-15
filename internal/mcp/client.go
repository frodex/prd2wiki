package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// WikiClient is an HTTP client that wraps calls to the wiki core REST API.
type WikiClient struct {
	baseURL string
	token   string // Bearer token for write operations; empty = no auth header
	http    *http.Client
}

// NewWikiClient creates a WikiClient that targets the given base URL.
func NewWikiClient(baseURL string) *WikiClient {
	return &WikiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// SetToken configures a Bearer token for write operations.
// When set, mutating requests (POST, PUT, DELETE) include an Authorization header.
func (c *WikiClient) SetToken(token string) {
	c.token = token
}

// PageResponse matches the API's GET page response.
type PageResponse struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Type       string      `json:"type"`
	Status     string      `json:"status"`
	TrustLevel int         `json:"trust_level"`
	Tags       []string    `json:"tags"`
	Body       string      `json:"body"`
	Provenance interface{} `json:"provenance"`
}

// CreatePageRequest is the JSON body for creating a page.
type CreatePageRequest struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Status string   `json:"status,omitempty"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags,omitempty"`
	Branch string   `json:"branch,omitempty"`
	Intent string   `json:"intent,omitempty"`
	Author string   `json:"author,omitempty"`
}

// CreatePageResponse is the JSON response from a successful page creation.
type CreatePageResponse struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Status   string      `json:"status"`
	Path     string      `json:"path"`
	Issues   interface{} `json:"issues"`
	Warnings []string    `json:"warnings"`
	Valid    *bool       `json:"valid,omitempty"`
}

// PageResult holds summary fields returned by list/search queries.
type PageResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Path       string `json:"path"`
	Project    string `json:"project"`
	TrustLevel int    `json:"trust_level"`
	Tags       string `json:"tags"`
}

// RefNode represents a node in the provenance reference tree.
type RefNode struct {
	Ref        string    `json:"ref"`
	Title      string    `json:"title,omitempty"`
	Version    int       `json:"version,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	Status     string    `json:"status"`
	TrustLevel int       `json:"trust_level,omitempty"`
	Children   []RefNode `json:"children"`
}

// ReferencesResponse wraps the reference tree returned by GetReferences.
// PageID is the root page, Hard contains its direct children.
type ReferencesResponse struct {
	PageID string    `json:"page_id"`
	Hard   []RefNode `json:"hard"`
}

// GetPage fetches a single page by project and ID.
// An optional branch can be supplied; pass "" to use the server default.
func (c *WikiClient) GetPage(project, id, branch string) (*PageResponse, error) {
	path := fmt.Sprintf("/api/projects/%s/pages/%s", project, id)
	params := url.Values{}
	if branch != "" {
		params.Set("branch", branch)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.get(path)
	if err != nil {
		return nil, err
	}

	var resp PageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode page response: %w", err)
	}
	return &resp, nil
}

// CreatePage submits a new page to the given project.
func (c *WikiClient) CreatePage(project string, req CreatePageRequest) (*CreatePageResponse, error) {
	path := fmt.Sprintf("/api/projects/%s/pages", project)

	body, err := c.post(path, req)
	if err != nil {
		return nil, err
	}

	var resp CreatePageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create response: %w", err)
	}
	return &resp, nil
}

// ListPages returns pages in a project, optionally filtered by key/value pairs
// such as "type", "status", or "tag".
func (c *WikiClient) ListPages(project string, filters map[string]string) ([]PageResult, error) {
	params := url.Values{}
	for k, v := range filters {
		params.Set(k, v)
	}
	path := fmt.Sprintf("/api/projects/%s/pages", project)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.get(path)
	if err != nil {
		return nil, err
	}

	var results []PageResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode list response: %w", err)
	}
	return results, nil
}

// DeletePage deletes a page by project, ID, and branch.
// An optional branch can be supplied; pass "" to use the server default.
func (c *WikiClient) DeletePage(project, id, branch string) error {
	path := fmt.Sprintf("/api/projects/%s/pages/%s", project, id)
	params := url.Values{}
	if branch != "" {
		params.Set("branch", branch)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	return c.delete(path)
}

// Search queries the search endpoint with the given parameters.
// Common keys include "q", "type", "status", "tag".
func (c *WikiClient) Search(project string, params map[string]string) ([]PageResult, error) {
	qv := url.Values{}
	for k, v := range params {
		qv.Set(k, v)
	}
	path := fmt.Sprintf("/api/projects/%s/search", project)
	if len(qv) > 0 {
		path += "?" + qv.Encode()
	}

	body, err := c.get(path)
	if err != nil {
		return nil, err
	}

	var results []PageResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	return results, nil
}

// GetReferences fetches the provenance reference tree for a page.
// depth controls how many levels are expanded (clamped to 5 by the server).
func (c *WikiClient) GetReferences(project, id string, depth int) (*ReferencesResponse, error) {
	path := fmt.Sprintf("/api/projects/%s/pages/%s/references?depth=%s",
		project, id, strconv.Itoa(depth))

	body, err := c.get(path)
	if err != nil {
		return nil, err
	}

	// The server returns a single RefNode root object.
	var root RefNode
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode references response: %w", err)
	}

	return &ReferencesResponse{
		PageID: root.Ref,
		Hard:   root.Children,
	}, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// get performs a GET request and returns the response body.
func (c *WikiClient) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	return readBody(resp)
}

// post performs a POST request with a JSON-encoded body and returns the response body.
func (c *WikiClient) post(path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	return readBody(resp)
}

// delete performs a DELETE request and returns any error.
func (c *WikiClient) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build DELETE request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("DELETE %s: status %d: %s", path, resp.StatusCode, string(body))
}

// readBody reads the response body and returns an error for non-2xx status codes.
func readBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
