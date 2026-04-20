package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanMinimalTree(t *testing.T) {
	tmp := t.TempDir()
	treeDir := filepath.Join(tmp, "wiki")
	dataDir := filepath.Join(tmp, "data")
	reposDir := filepath.Join(dataDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// UUID with first segment test0000 — repo name proj_test0000.git
	const projUUID = "test0000-0000-4000-8000-000000000001"
	bare := filepath.Join(reposDir, "proj_test0000.git")
	if err := initBareRepo(bare); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dataDir, "acme.wiki.git")
	if err := os.Symlink(filepath.Join("repos", "proj_test0000.git"), linkPath); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(treeDir, "acme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(treeDir, "acme", ".uuid"), []byte(projUUID+"\nAcme Wiki\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const pageUUID = "aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee"
	if err := os.WriteFile(filepath.Join(treeDir, "acme", "hello.link"), []byte(pageUUID+"\n\nHello Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Scan(treeDir, dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(idx.Projects) != 1 {
		t.Fatalf("projects: %d", len(idx.Projects))
	}
	if idx.Projects[0].RepoKey != "acme" {
		t.Errorf("RepoKey = %q", idx.Projects[0].RepoKey)
	}
	ent, ok := idx.PageByURLPath("acme/hello")
	if !ok {
		t.Fatal("page lookup: not found")
	}
	if ent.Page.UUID != pageUUID {
		t.Fatalf("uuid = %q", ent.Page.UUID)
	}
}

func initBareRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
}
