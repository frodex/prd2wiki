package mcp

import (
	"fmt"
	"strings"
)

// PromptArgDef describes a single argument accepted by a prompt.
type PromptArgDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// PromptDef describes a prompt exposed via MCP.
type PromptDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Arguments   []PromptArgDef `json:"arguments,omitempty"`
}

// registeredPrompt pairs a prompt definition with its renderer.
type registeredPrompt struct {
	Def    PromptDef
	Render func(args map[string]string) (string, error)
}

func (s *MCPServer) registerPrompts() {
	s.prompts = make(map[string]registeredPrompt)

	s.prompts["review_page"] = registeredPrompt{
		Def: PromptDef{
			Name:        "review_page",
			Description: "Guides an agent through reviewing a wiki page for accuracy, provenance, and consistency.",
			Arguments: []PromptArgDef{
				{Name: "project", Description: "The project slug containing the page.", Required: true},
				{Name: "page_id", Description: "The page ID to review.", Required: true},
			},
		},
		Render: renderReviewPage,
	}

	s.prompts["ingest_source"] = registeredPrompt{
		Def: PromptDef{
			Name:        "ingest_source",
			Description: "Guides an agent through registering a new source and linking it to existing wiki pages.",
			Arguments: []PromptArgDef{
				{Name: "project", Description: "The project slug to ingest the source into.", Required: true},
				{Name: "url", Description: "The URL of the source to ingest.", Required: true},
				{Name: "title", Description: "A human-readable title for the source.", Required: true},
			},
		},
		Render: renderIngestSource,
	}
}

func renderReviewPage(args map[string]string) (string, error) {
	project := args["project"]
	pageID := args["page_id"]
	if project == "" || pageID == "" {
		return "", fmt.Errorf("review_page requires 'project' and 'page_id' arguments")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Review the wiki page %s in project %s.\n", pageID, project)
	b.WriteString("\nSteps:\n")
	b.WriteString("1. Read the page using wiki_read\n")
	b.WriteString("2. Check provenance chain using wiki_lint\n")
	b.WriteString("3. Verify each source is still valid and current\n")
	b.WriteString("4. Check for contradictions with other pages (use wiki_search)\n")
	b.WriteString("5. If issues found, create a challenge using wiki_challenge\n")
	fmt.Fprintf(&b, "6. If the page is sound, report: \"Page %s reviewed — no issues found\"\n", pageID)
	b.WriteString("\nFocus on:\n")
	b.WriteString("- Are all cited sources still active and accurate?\n")
	b.WriteString("- Does the content contradict any other page in the project?\n")
	b.WriteString("- Is the reasoning in any [!decision] blocks still valid?\n")
	b.WriteString("- Are there gaps in the provenance chain?\n")
	return b.String(), nil
}

func renderIngestSource(args map[string]string) (string, error) {
	project := args["project"]
	url := args["url"]
	title := args["title"]
	if project == "" || url == "" || title == "" {
		return "", fmt.Errorf("ingest_source requires 'project', 'url', and 'title' arguments")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Register a new source in project %s.\n", project)
	fmt.Fprintf(&b, "\nSource: %s\n", title)
	fmt.Fprintf(&b, "URL: %s\n", url)
	b.WriteString("\nSteps:\n")
	b.WriteString("1. Review the source content\n")
	b.WriteString("2. Determine the source kind (standard, paper, documentation, observation, agent-research)\n")
	b.WriteString("3. Identify key claims and information to extract\n")
	b.WriteString("4. Create a source page using wiki_ingest with the title, URL, and a summary\n")
	b.WriteString("5. Search for existing pages that should reference this source using wiki_search\n")
	b.WriteString("6. For each related page, propose an update using wiki_propose (intent: integrate) to add the new source as a provenance reference\n")
	return b.String(), nil
}
