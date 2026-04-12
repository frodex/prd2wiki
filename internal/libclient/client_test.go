package libclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
