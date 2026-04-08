package web

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// allowedTags is the whitelist of safe HTML tags from goldmark output.
var allowedTags = map[atom.Atom]bool{
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.P: true, atom.Br: true, atom.Hr: true,
	atom.Strong: true, atom.Em: true, atom.Code: true, atom.Pre: true,
	atom.A: true, atom.Img: true,
	atom.Ul: true, atom.Ol: true, atom.Li: true,
	atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tr: true, atom.Th: true, atom.Td: true,
	atom.Blockquote: true, atom.Del: true, atom.Sup: true, atom.Sub: true,
	atom.Div: true, atom.Span: true,
	atom.Input: true, // for task list checkboxes
}

// allowedAttrs per tag.
var allowedAttrs = map[string][]string{
	"a":     {"href", "title"},
	"img":   {"src", "alt", "title"},
	"input": {"type", "checked", "disabled"},
	"td":    {"align"},
	"th":    {"align"},
}

func sanitizeHTML(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return html.EscapeString(raw) // fallback: escape everything
	}

	var buf bytes.Buffer
	sanitizeNode(&buf, doc)
	return buf.String()
}

func sanitizeNode(w io.Writer, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		io.WriteString(w, html.EscapeString(n.Data))
	case html.ElementNode:
		if allowedTags[n.DataAtom] {
			io.WriteString(w, "<"+n.Data)
			// Filter attributes.
			allowed := allowedAttrs[n.Data]
			for _, attr := range n.Attr {
				for _, a := range allowed {
					if attr.Key == a {
						// Extra check: href/src must not be javascript:
						if (attr.Key == "href" || attr.Key == "src") && strings.HasPrefix(strings.TrimSpace(strings.ToLower(attr.Val)), "javascript:") {
							continue
						}
						io.WriteString(w, " "+attr.Key+"=\""+html.EscapeString(attr.Val)+"\"")
					}
				}
			}
			io.WriteString(w, ">")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				sanitizeNode(w, c)
			}
			io.WriteString(w, "</"+n.Data+">")
		} else {
			// Skip the tag but render children.
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				sanitizeNode(w, c)
			}
		}
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			sanitizeNode(w, c)
		}
	}
}
