package librarian

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"time"
)

// ExtractedImage represents a base64 image extracted from markdown.
type ExtractedImage struct {
	Filename string
	Data     []byte
	Path     string // git path: pages/{id}/_attachments/{filename}
}

// base64ImageRe matches ![alt](data:image/{type};base64,{data})
var base64ImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(data:image/(png|jpeg|jpg|gif|webp|svg\+xml);base64,([A-Za-z0-9+/=]+)\)`)

// ExtractBase64Images finds all base64 data URLs in markdown, decodes them,
// and returns the cleaned markdown + list of extracted images.
func ExtractBase64Images(body string, pageID, project string) (string, []ExtractedImage) {
	var images []ExtractedImage
	counter := 0

	cleaned := base64ImageRe.ReplaceAllStringFunc(body, func(match string) string {
		parts := base64ImageRe.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match // shouldn't happen, keep original
		}

		alt := parts[1]
		mimeType := parts[2]
		b64data := parts[3]

		// Decode base64
		decoded, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return match // can't decode, keep original
		}

		// Determine file extension from MIME type
		ext := mimeToExt(mimeType)

		// Generate filename
		counter++
		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("image-%s-%d%s", timestamp, counter, ext)

		if alt == "" {
			alt = "screenshot"
		}

		// Build the attachment URL (matches api/attachments.go pattern)
		url := fmt.Sprintf("/api/projects/%s/pages/%s/attachments/%s", project, pageID, filename)

		// Build the git path (matches api/attachments.go pattern)
		gitPath := fmt.Sprintf("pages/%s/_attachments/%s", pageID, filename)

		images = append(images, ExtractedImage{
			Filename: filename,
			Data:     decoded,
			Path:     gitPath,
		})

		return fmt.Sprintf("![%s](%s)", alt, url)
	})

	return cleaned, images
}

func mimeToExt(mime string) string {
	switch mime {
	case "png":
		return ".png"
	case "jpeg", "jpg":
		return ".jpg"
	case "gif":
		return ".gif"
	case "webp":
		return ".webp"
	case "svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}
