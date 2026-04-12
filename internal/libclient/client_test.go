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
	if New("  ", "") != nil {
		t.Fatal("expected nil client")
	}
	if New("", "") != nil {
		t.Fatal("expected nil client")
	}
}
