package searchsnippet

import (
	"html"
	"html/template"
	"regexp"
	"sort"
	"strings"
)

// queryTermsForHighlight splits a user FTS query into tokens suitable for marking in a plain excerpt.
func queryTermsForHighlight(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, w := range strings.Fields(q) {
		w = strings.Trim(w, `"'`)
		w = strings.TrimSuffix(w, "*")
		if len(w) < 2 || len(w) > 64 {
			continue
		}
		lw := strings.ToLower(w)
		if lw == "or" || lw == "and" || lw == "not" {
			continue
		}
		if strings.HasPrefix(w, "-") {
			continue
		}
		if !seen[lw] {
			seen[lw] = true
			out = append(out, w)
		}
	}
	return out
}

// HighlightPlainAsHTML escapes plain text, then wraps query term hits in <mark class="search-hit">.
func HighlightPlainAsHTML(plain, query string) template.HTML {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return ""
	}
	terms := queryTermsForHighlight(query)
	if len(terms) == 0 {
		return template.HTML(html.EscapeString(plain))
	}
	sort.Slice(terms, func(i, j int) bool {
		return len([]rune(terms[i])) > len([]rune(terms[j]))
	})
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = regexp.QuoteMeta(t)
	}
	re, err := regexp.Compile("(?i)(" + strings.Join(parts, "|") + ")")
	if err != nil {
		return template.HTML(html.EscapeString(plain))
	}
	idx := re.FindAllStringIndex(plain, -1)
	if len(idx) == 0 {
		return template.HTML(html.EscapeString(plain))
	}
	var b strings.Builder
	last := 0
	for _, pair := range idx {
		if pair[0] < last {
			continue
		}
		if pair[0] > last {
			b.WriteString(html.EscapeString(plain[last:pair[0]]))
		}
		b.WriteString(`<mark class="search-hit">`)
		b.WriteString(html.EscapeString(plain[pair[0]:pair[1]]))
		b.WriteString(`</mark>`)
		last = pair[1]
	}
	if last < len(plain) {
		b.WriteString(html.EscapeString(plain[last:]))
	}
	return template.HTML(b.String())
}

// FormatSearchExcerpt clamps a plain FTS (or other) snippet, then adds hit marks for query terms.
func FormatSearchExcerpt(snip, query string) template.HTML {
	return HighlightPlainAsHTML(ClampExcerpt(strings.TrimSpace(snip), 300, 6), query)
}

// VectorExcerptHTML clamps a vector-store snippet and highlights query terms.
func VectorExcerptHTML(vectorSnippet, query string) template.HTML {
	return HighlightPlainAsHTML(ClampExcerpt(strings.TrimSpace(vectorSnippet), 200, 2), query)
}

// HistoryVectorExcerptHTML formats a history vector hit with a plain prefix and highlighted body.
func HistoryVectorExcerptHTML(shortCommit, vectorSnippet, query string) template.HTML {
	snip := ClampExcerpt(strings.TrimSpace(vectorSnippet), 200, 2)
	body := HighlightPlainAsHTML(snip, query)
	var pre string
	if shortCommit != "" {
		pre = "[history]" + shortCommit + " — "
	} else {
		pre = "[history] — "
	}
	return template.HTML(html.EscapeString(pre) + string(body))
}
