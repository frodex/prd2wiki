package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxTreeMultipartMemory bounds multipart Tree API bodies (meta JSON + markdown file).
const maxTreeMultipartMemory = 32 << 20 // 32 MiB

func isMultipartForm(ct string) bool {
	mediatype, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediatype, "multipart/form-data")
}

func readMultipartFilePart(r *http.Request, field string) ([]byte, error) {
	fhs := r.MultipartForm.File[field]
	if len(fhs) == 0 {
		return nil, fmt.Errorf("multipart: missing file part %q", field)
	}
	f, err := fhs[0].Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxTreeMultipartMemory))
}

func readOptionalMultipartFilePart(r *http.Request, field string) ([]byte, bool, error) {
	fhs := r.MultipartForm.File[field]
	if len(fhs) == 0 {
		return nil, false, nil
	}
	b, err := readMultipartFilePart(r, field)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// decodeTreeUpdateRequest reads a TreeUpdateRequest from JSON or multipart/form-data.
//
// Multipart mode (shell-safe for large markdown): two file parts
//   - "meta": application/json (TreeUpdateRequest; body may be empty or a stub)
//   - "content": optional raw page body; when present, overrides meta.body
func decodeTreeUpdateRequest(r *http.Request) (TreeUpdateRequest, error) {
	ct := r.Header.Get("Content-Type")
	if isMultipartForm(ct) {
		if err := r.ParseMultipartForm(maxTreeMultipartMemory); err != nil {
			return TreeUpdateRequest{}, err
		}
		metaBytes, err := readMultipartFilePart(r, "meta")
		if err != nil {
			return TreeUpdateRequest{}, err
		}
		var req TreeUpdateRequest
		if err := json.Unmarshal(metaBytes, &req); err != nil {
			return TreeUpdateRequest{}, fmt.Errorf("multipart meta JSON: %w", err)
		}
		if content, ok, err := readOptionalMultipartFilePart(r, "content"); err != nil {
			return TreeUpdateRequest{}, err
		} else if ok {
			req.Body = string(content)
		}
		return req, nil
	}
	var req TreeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return TreeUpdateRequest{}, err
	}
	return req, nil
}

// decodeTreeCreateRequest reads a TreeCreateRequest from JSON or multipart/form-data.
// Same multipart contract as decodeTreeUpdateRequest.
func decodeTreeCreateRequest(r *http.Request) (TreeCreateRequest, error) {
	ct := r.Header.Get("Content-Type")
	if isMultipartForm(ct) {
		if err := r.ParseMultipartForm(maxTreeMultipartMemory); err != nil {
			return TreeCreateRequest{}, err
		}
		metaBytes, err := readMultipartFilePart(r, "meta")
		if err != nil {
			return TreeCreateRequest{}, err
		}
		var req TreeCreateRequest
		if err := json.Unmarshal(metaBytes, &req); err != nil {
			return TreeCreateRequest{}, fmt.Errorf("multipart meta JSON: %w", err)
		}
		if content, ok, err := readOptionalMultipartFilePart(r, "content"); err != nil {
			return TreeCreateRequest{}, err
		} else if ok {
			req.Body = string(content)
		}
		return req, nil
	}
	var req TreeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return TreeCreateRequest{}, err
	}
	return req, nil
}
