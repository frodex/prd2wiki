package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeTreeUpdateRequest_JSON(t *testing.T) {
	req := TreeUpdateRequest{Title: "A", Type: "concept", Body: "x"}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PUT", "/", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	got, err := decodeTreeUpdateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "A" || got.Body != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeTreeUpdateRequest_multipartContentOverridesMetaBody(t *testing.T) {
	meta := TreeUpdateRequest{Title: "T", Type: "concept", Status: "draft", Body: "from-json"}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	partMeta, err := mw.CreateFormFile("meta", "meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partMeta.Write(metaBytes); err != nil {
		t.Fatal(err)
	}
	partContent, err := mw.CreateFormFile("content", "body.md")
	if err != nil {
		t.Fatal(err)
	}
	markdown := "has `backticks` and $VAR and \"quotes\"\n"
	if _, err := partContent.Write([]byte(markdown)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("PUT", "/", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	got, err := decodeTreeUpdateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != markdown {
		t.Fatalf("body mismatch: %q", got.Body)
	}
	if got.Title != "T" {
		t.Fatalf("title lost")
	}
}

func TestDecodeTreeUpdateRequest_multipartMissingMeta(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	partContent, err := mw.CreateFormFile("content", "body.md")
	if err != nil {
		t.Fatal(err)
	}
	partContent.Write([]byte("only content"))
	mw.Close()

	r := httptest.NewRequest("PUT", "/", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	_, err = decodeTreeUpdateRequest(r)
	if err == nil || !strings.Contains(err.Error(), `missing file part "meta"`) {
		t.Fatalf("expected missing meta error, got %v", err)
	}
}

func TestDecodeTreeCreateRequest_multipart(t *testing.T) {
	meta := TreeCreateRequest{Title: "New", Type: "reference", Slug: "x-slug", Body: ""}
	metaBytes, _ := json.Marshal(meta)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	wm, _ := mw.CreateFormFile("meta", "meta.json")
	wm.Write(metaBytes)
	wc, _ := mw.CreateFormFile("content", "x.md")
	wc.Write([]byte("# H\n"))
	mw.Close()

	r := httptest.NewRequest("POST", "/", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	got, err := decodeTreeCreateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New" || got.Body != "# H\n" {
		t.Fatalf("got %+v", got)
	}
}
