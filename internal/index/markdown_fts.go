package index

import (
	"regexp"
	"strings"
)

var (
	// Markdown [label](url) — index anchor text only (URLs add noise / duplicate tokens).
	mdLinkRE = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// ![alt](url)
	mdImageRE = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	// <https://...>
	angleURLRE = regexp.MustCompile(`<https?://[^>\s]+>`)
	// Wiki internal page IDs in paths (/projects/.../pages/<id>).
	wikiPagePathRE = regexp.MustCompile(`(?i)/pages/([0-9a-f]{5,40}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
)

// StripMarkdownForFTS replaces markdown links and image refs with their visible text only
// so FTS BM25 is driven by readable words, not repeated /projects/.../paths and hex IDs.
func StripMarkdownForFTS(s string) string {
	s = mdImageRE.ReplaceAllString(s, "$1")
	s = mdLinkRE.ReplaceAllString(s, "$1")
	s = angleURLRE.ReplaceAllString(s, " ")
	return s
}

// WikiPageIDsInMarkdown returns distinct page IDs referenced by /pages/<id> paths in markdown.
func WikiPageIDsInMarkdown(s string) []string {
	found := wikiPagePathRE.FindAllStringSubmatch(s, -1)
	if len(found) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]struct{}, len(found))
	for _, m := range found {
		if len(m) < 2 {
			continue
		}
		id := strings.ToLower(m[1])
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, m[1])
	}
	return out
}
