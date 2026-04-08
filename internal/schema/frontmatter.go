package schema

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Date wraps time.Time with custom YAML marshal/unmarshal.
// It supports both "2006-01-02" and RFC3339 formats.
// A zero Date marshals to nil (omitted from YAML output).
type Date struct {
	time.Time
}

const dateLayout = "2006-01-02"

// UnmarshalYAML decodes a YAML scalar into a Date.
// Accepts "2006-01-02" and RFC3339 formats.
func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	s := value.Value
	if s == "" || s == "null" {
		d.Time = time.Time{}
		return nil
	}
	// Try date-only format first
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		// Fall back to RFC3339
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("cannot parse date %q: must be YYYY-MM-DD or RFC3339", s)
		}
	}
	d.Time = t.UTC()
	return nil
}

// MarshalYAML encodes a Date as "2006-01-02". Zero dates are omitted.
func (d Date) MarshalYAML() (interface{}, error) {
	if d.Time.IsZero() {
		return nil, nil
	}
	return d.Time.UTC().Format(dateLayout), nil
}

// Frontmatter represents the YAML frontmatter of a wiki page.
type Frontmatter struct {
	ID           string     `yaml:"id"`
	Title        string     `yaml:"title"`
	Type         string     `yaml:"type"`
	Status       string     `yaml:"status"`
	DCCreator    string     `yaml:"dc.creator,omitempty"`
	DCCreated    Date       `yaml:"dc.created,omitempty"`
	DCModified   Date       `yaml:"dc.modified,omitempty"`
	DCRights     string     `yaml:"dc.rights,omitempty"`
	TrustLevel   int        `yaml:"trust_level,omitempty"`
	Conformance  string     `yaml:"conformance,omitempty"`
	Tags         []string   `yaml:"tags,omitempty"`
	Provenance   Provenance `yaml:"provenance,omitempty"`
	Supersedes   string     `yaml:"supersedes,omitempty"`
	SupersededBy string     `yaml:"superseded_by,omitempty"`
	Updates      []string   `yaml:"updates,omitempty"`
	SourceMeta   *SourceMeta `yaml:"source_meta,omitempty"`
	Access       *Access     `yaml:"access,omitempty"`
	ContestedBy  string     `yaml:"contested_by,omitempty"`
	Module       string     `yaml:"module,omitempty"`
	Category     string     `yaml:"category,omitempty"`
	ProjectRef   string     `yaml:"project_ref,omitempty"`
}

// Provenance captures the origin and contributor history of a page.
type Provenance struct {
	Sources      []Source      `yaml:"sources,omitempty"`
	Contributors []Contributor `yaml:"contributors,omitempty"`
}

// Source references an upstream document.
type Source struct {
	Ref       string `yaml:"ref"`
	Title     string `yaml:"title,omitempty"`
	Version   int    `yaml:"version,omitempty"`
	Checksum  string `yaml:"checksum,omitempty"`
	Retrieved Date   `yaml:"retrieved,omitempty"`
	Status    string `yaml:"status,omitempty"`
}

// Contributor records a human contributor and their role/decision.
type Contributor struct {
	Identity string `yaml:"identity"`
	Role     string `yaml:"role"`
	Decision string `yaml:"decision,omitempty"`
	Date     Date   `yaml:"date,omitempty"`
}

// SourceMeta records metadata about the primary source document.
type SourceMeta struct {
	URL       string `yaml:"url,omitempty"`
	Kind      string `yaml:"kind,omitempty"`
	Authority string `yaml:"authority,omitempty"`
	Retrieved Date   `yaml:"retrieved,omitempty"`
}

// Access controls visibility of a page.
type Access struct {
	RestrictTo []string `yaml:"restrict_to,omitempty"`
}

var delimiter = []byte("---")

// Parse splits a markdown document into frontmatter and body.
// If the document does not start with "---\n", frontmatter is nil and the
// entire content is returned as body.
func Parse(data []byte) (*Frontmatter, []byte, error) {
	// Must start with "---" followed by a newline
	if !bytes.HasPrefix(data, append(delimiter, '\n')) {
		return nil, data, nil
	}

	// Find the closing "---"
	rest := data[4:] // skip opening "---\n"
	end := bytes.Index(rest, append(delimiter, '\n'))
	var yamlBytes []byte
	var body []byte
	if end == -1 {
		// Check for "---" at end of file without trailing newline
		if bytes.HasSuffix(rest, delimiter) {
			yamlBytes = rest[:len(rest)-3]
			body = []byte{}
		} else {
			// No closing delimiter — treat whole file as body
			return nil, data, nil
		}
	} else {
		yamlBytes = rest[:end]
		body = rest[end+4:] // skip closing "---\n"
	}

	fm := &Frontmatter{}
	if err := yaml.Unmarshal(yamlBytes, fm); err != nil {
		return nil, nil, fmt.Errorf("frontmatter: YAML parse error: %w", err)
	}
	return fm, body, nil
}

// Serialize combines frontmatter and body into a markdown document with
// YAML frontmatter delimiters: "---\n<yaml>\n---\n<body>".
func Serialize(fm *Frontmatter, body []byte) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: YAML marshal error: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(delimiter)
	buf.WriteByte('\n')
	buf.Write(yamlBytes)
	buf.Write(delimiter)
	buf.WriteByte('\n')
	buf.Write(body)
	return buf.Bytes(), nil
}
