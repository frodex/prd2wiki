package index

import "testing"

func TestStripMarkdownForFTS(t *testing.T) {
	in := "See [Composable Pages](/projects/default/pages/2373479) and [Other](x)."
	got := StripMarkdownForFTS(in)
	if want := "See Composable Pages and Other."; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWikiPageIDsInMarkdown(t *testing.T) {
	s := "Link to [/projects/default/pages/2373479](x) and [/projects/p/pages/db139ca](./y)."
	ids := WikiPageIDsInMarkdown(s)
	if len(ids) != 2 {
		t.Fatalf("ids: %v", ids)
	}
}
