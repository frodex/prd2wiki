package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/frodex/prd2wiki/internal/blob"
)

const maxBlobUploadBytes = 50 << 20 // 50MB

func (s *Server) postBlob(w http.ResponseWriter, r *http.Request) {
	if !s.requireWriteScope(w, r) {
		return
	}
	if s.blobStore == nil {
		http.Error(w, "blob store unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(maxBlobUploadBytes); err != nil {
		http.Error(w, "multipart parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing form field \"file\"", http.StatusBadRequest)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBlobUploadBytes+1))
	if err != nil {
		http.Error(w, "read upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) > maxBlobUploadBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	hash, err := s.blobStore.PutBytes(data)
	if err != nil {
		http.Error(w, "store blob: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":  "/blobs/" + hash,
		"hash": hash,
	})
}

// GetBlob serves GET /blobs/{hash} with content-type sniffing.
func GetBlob(store *blob.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		hash := strings.TrimSpace(r.PathValue("hash"))
		if !blob.ValidHash(hash) {
			http.Error(w, "invalid hash", http.StatusBadRequest)
			return
		}
		f, err := store.Open(hash)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxBlobUploadBytes))
		if err != nil {
			http.Error(w, "read blob", http.StatusInternalServerError)
			return
		}
		ct := http.DetectContentType(data)
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(data)
	}
}
