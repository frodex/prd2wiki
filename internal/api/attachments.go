package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const maxUploadSize = 50 << 20 // 50 MB

// allowedTypes maps file extensions to MIME types for permitted attachments.
var allowedTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".pdf":  "application/pdf",
}

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")

	// Read file data and filename from either multipart or raw body.
	var data []byte
	var filename string
	var err error

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		data, filename, err = readMultipart(r)
	} else {
		data, filename, err = readRawBody(r)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate size.
	if len(data) > maxUploadSize {
		http.Error(w, fmt.Sprintf("file too large: %d bytes (max %d)", len(data), maxUploadSize), http.StatusRequestEntityTooLarge)
		return
	}

	// Validate file type by extension.
	ext := strings.ToLower(filepath.Ext(filename))
	if _, ok := allowedTypes[ext]; !ok {
		http.Error(w, fmt.Sprintf("file type %q not allowed", ext), http.StatusUnsupportedMediaType)
		return
	}

	// Sanitize filename: keep only the base name.
	filename = filepath.Base(filename)

	// Store in git.
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "truth"
	}

	gitPath := fmt.Sprintf("pages/%s/_attachments/%s", id, filename)
	message := fmt.Sprintf("attach %s to %s", filename, id)
	author := r.Header.Get("X-Author")
	if author == "" {
		author = "anonymous"
	}

	if err := repo.WritePage(branch, gitPath, data, message, author); err != nil {
		http.Error(w, "store attachment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("/api/projects/%s/pages/%s/attachments/%s", project, id, filename)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"url":      url,
		"filename": filename,
	})
}

func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	filename := r.PathValue("filename")

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "truth"
	}

	gitPath := fmt.Sprintf("pages/%s/_attachments/%s", id, filename)
	data, err := repo.ReadPage(branch, gitPath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "attachment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "read attachment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine Content-Type from extension.
	ext := strings.ToLower(filepath.Ext(filename))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "truth"
	}

	prefix := fmt.Sprintf("pages/%s/_attachments/", id)
	allPaths, err := repo.ListPages(branch)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Branch doesn't exist yet — return empty list.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		http.Error(w, "list attachments: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type attachment struct {
		Filename string `json:"filename"`
		URL      string `json:"url"`
	}

	var results []attachment
	for _, p := range allPaths {
		if strings.HasPrefix(p, prefix) {
			name := strings.TrimPrefix(p, prefix)
			if name == "" || strings.Contains(name, "/") {
				continue
			}
			results = append(results, attachment{
				Filename: name,
				URL:      fmt.Sprintf("/api/projects/%s/pages/%s/attachments/%s", project, id, name),
			})
		}
	}

	if results == nil {
		results = []attachment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// readMultipart reads the file from a multipart/form-data request.
func readMultipart(r *http.Request) ([]byte, string, error) {
	// Limit parsed body size to maxUploadSize + 1MB for headers/metadata.
	if err := r.ParseMultipartForm(maxUploadSize + (1 << 20)); err != nil {
		return nil, "", fmt.Errorf("parse multipart: %w", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("missing 'file' field: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	if len(data) > maxUploadSize {
		return nil, "", fmt.Errorf("file too large (max %d bytes)", maxUploadSize)
	}

	return data, header.Filename, nil
}

// readRawBody reads the file from a raw request body (clipboard paste).
func readRawBody(r *http.Request) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxUploadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty request body")
	}
	if len(data) > maxUploadSize {
		return nil, "", fmt.Errorf("file too large (max %d bytes)", maxUploadSize)
	}

	filename := r.Header.Get("X-Filename")
	if filename == "" {
		// Generate a name for clipboard pastes.
		filename = fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli())
	}

	return data, filename, nil
}
