package librarian

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestExtractBase64Images(t *testing.T) {
	// Use valid base64 that decodes successfully
	pngData := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))

	body := "# Test\n\nHere is an image: ![screenshot](data:image/png;base64," + pngData + ")\n\nAnd some text after."

	cleaned, images := ExtractBase64Images(body, "test-page", "default")

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	if strings.Contains(cleaned, "base64") {
		t.Error("cleaned body still contains base64")
	}

	if !strings.Contains(cleaned, "/api/projects/default/pages/test-page/attachments/") {
		t.Error("cleaned body doesn't contain attachment URL")
	}

	if !strings.Contains(cleaned, "![screenshot]") {
		t.Error("alt text not preserved")
	}

	if string(images[0].Data) != "fake-png-bytes" {
		t.Errorf("decoded data mismatch: got %q", string(images[0].Data))
	}

	if !strings.HasPrefix(images[0].Path, "pages/test-page/_attachments/image-") {
		t.Errorf("unexpected git path: %s", images[0].Path)
	}

	if !strings.HasSuffix(images[0].Path, ".png") {
		t.Errorf("expected .png extension in path: %s", images[0].Path)
	}
}

func TestExtractBase64MultipleImages(t *testing.T) {
	pngData := base64.StdEncoding.EncodeToString([]byte("png"))
	jpegData := base64.StdEncoding.EncodeToString([]byte("jpeg"))

	body := "![a](data:image/png;base64," + pngData + ") text ![b](data:image/jpeg;base64," + jpegData + ")"
	cleaned, images := ExtractBase64Images(body, "test", "default")
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if strings.Contains(cleaned, "base64") {
		t.Error("still contains base64")
	}
	// Verify different extensions
	if !strings.HasSuffix(images[0].Filename, ".png") {
		t.Errorf("first image should be .png, got %s", images[0].Filename)
	}
	if !strings.HasSuffix(images[1].Filename, ".jpg") {
		t.Errorf("second image should be .jpg, got %s", images[1].Filename)
	}
}

func TestExtractNoImages(t *testing.T) {
	body := "# No images here\n\nJust text."
	cleaned, images := ExtractBase64Images(body, "test", "default")
	if len(images) != 0 {
		t.Error("expected no images")
	}
	if cleaned != body {
		t.Error("body should be unchanged")
	}
}

func TestExtractEmptyAltText(t *testing.T) {
	pngData := base64.StdEncoding.EncodeToString([]byte("data"))
	body := "![](data:image/png;base64," + pngData + ")"
	cleaned, images := ExtractBase64Images(body, "pg", "proj")
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	// Empty alt should become "screenshot"
	if !strings.Contains(cleaned, "![screenshot]") {
		t.Errorf("expected default alt text 'screenshot', got: %s", cleaned)
	}
}

func TestExtractPreservesNonImageContent(t *testing.T) {
	pngData := base64.StdEncoding.EncodeToString([]byte("img"))
	body := "Before\n\n![pic](data:image/png;base64," + pngData + ")\n\nAfter with `code` and [link](https://example.com)"
	cleaned, _ := ExtractBase64Images(body, "pg", "proj")
	if !strings.Contains(cleaned, "Before") {
		t.Error("lost content before image")
	}
	if !strings.Contains(cleaned, "After with `code` and [link](https://example.com)") {
		t.Error("lost content after image")
	}
}

func TestExtractAllMimeTypes(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("x"))
	tests := []struct {
		mime string
		ext  string
	}{
		{"png", ".png"},
		{"jpeg", ".jpg"},
		{"jpg", ".jpg"},
		{"gif", ".gif"},
		{"webp", ".webp"},
		{"svg+xml", ".svg"},
	}
	for _, tt := range tests {
		body := "![t](data:image/" + tt.mime + ";base64," + data + ")"
		_, images := ExtractBase64Images(body, "pg", "proj")
		if len(images) != 1 {
			t.Errorf("mime %s: expected 1 image, got %d", tt.mime, len(images))
			continue
		}
		if !strings.HasSuffix(images[0].Filename, tt.ext) {
			t.Errorf("mime %s: expected ext %s, got filename %s", tt.mime, tt.ext, images[0].Filename)
		}
	}
}
