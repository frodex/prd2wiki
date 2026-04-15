package libclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMemoryStoreParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tools/call" {
			http.NotFound(w, r)
			return
		}
		resp := toolCallResponse{
			OK: true,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: `{"record_id":"mem_0000000000000000000001","version":2,"created":false}`},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
	}
	id, err := c.MemoryStore(context.Background(), "wiki:u", "page-uuid", "# hi", map[string]any{"author": "t"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "mem_0000000000000000000001" {
		t.Fatalf("record_id: %q", id)
	}
}

func TestMemorySearchParsesMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tools/call" {
			http.NotFound(w, r)
			return
		}
		resp := toolCallResponse{
			OK: true,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: `{"matches":[{"page_uuid":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","record_id":"mem_1","title":"T","snippet":"s","score":0.9,"history_count":0}]}`},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
	}
	hits, err := c.MemorySearch(context.Background(), "wiki:proj-uuid", "hello", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].PageUUID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || hits[0].Score != 0.9 {
		t.Fatalf("hits: %+v", hits)
	}
}

func TestNew_EmptySocket(t *testing.T) {
	c, err := New("  ", "")
	if c != nil || err != nil {
		t.Fatal("expected nil client, nil error for empty socket")
	}
	c, err = New("", "")
	if c != nil || err != nil {
		t.Fatal("expected nil client, nil error for empty socket")
	}
}

func TestNew_BadSocket(t *testing.T) {
	c, err := New("/tmp/nonexistent-test-socket-12345.sock", "")
	if err == nil {
		t.Fatal("expected error for unreachable socket")
	}
	if c == nil {
		t.Fatal("should return client even on connection error (for later retry)")
	}
}

// Integration tests — run against the live librarian socket.
// Skipped if the socket is not available (CI-safe).

func TestTicketAuth_LiveSocket_Search(t *testing.T) {
	const sock = "/var/run/pippi-librarian.sock"
	if _, err := os.Stat(sock); err != nil {
		t.Skipf("librarian socket not available: %v", err)
	}
	c, err := New(sock, "")
	if err != nil {
		t.Skipf("cannot connect to librarian: %v", err)
	}
	c.EnableTicketAuth([]string{"memory_store", "memory_search", "memory_delete", "memory_get"})

	_, err = c.MemorySearch(context.Background(), "wiki:test", "hello", 1, false)
	if err != nil {
		t.Fatalf("MemorySearch with ticket auth failed: %v", err)
	}
}

func TestTicketAuth_LiveSocket_Store(t *testing.T) {
	const sock = "/var/run/pippi-librarian.sock"
	if _, err := os.Stat(sock); err != nil {
		t.Skipf("librarian socket not available: %v", err)
	}
	c, err := New(sock, "")
	if err != nil {
		t.Skipf("cannot connect to librarian: %v", err)
	}
	c.EnableTicketAuth([]string{"memory_store", "memory_search"})

	id, err := c.MemoryStore(context.Background(), "wiki:test-integration", "test-page-auth", "# Test\n\nTicket auth test.", nil)
	if err != nil {
		t.Fatalf("MemoryStore with ticket auth failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty record ID")
	}
	t.Logf("stored record: %s", id)
}
