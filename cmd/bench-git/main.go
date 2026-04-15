// bench-git: compare go-git vs git CLI vs direct SQLite for page reads.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func main() {
	repoPath := "data/default.wiki.git"
	branch := "draft/incoming"
	// A page with deep history (slow)
	pagePath := "pages/da/cea4b.md"
	if len(os.Args) > 1 {
		pagePath = os.Args[1]
	}

	fmt.Printf("Repo:   %s\nBranch: %s\nPage:   %s\n\n", repoPath, branch, pagePath)

	// 1. go-git: read file from branch tip
	fmt.Println("=== go-git: read file from branch tip ===")
	start := time.Now()
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		fmt.Println("open error:", err)
		os.Exit(1)
	}
	openDur := time.Since(start)

	start = time.Now()
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		fmt.Println("ref error:", err)
		os.Exit(1)
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		fmt.Println("commit error:", err)
		os.Exit(1)
	}
	tree, err := commit.Tree()
	if err != nil {
		fmt.Println("tree error:", err)
		os.Exit(1)
	}
	file, err := tree.File(pagePath)
	if err != nil {
		fmt.Println("file error:", err)
		os.Exit(1)
	}
	content, err := file.Contents()
	if err != nil {
		fmt.Println("contents error:", err)
		os.Exit(1)
	}
	readDur := time.Since(start)
	fmt.Printf("  Open repo:  %v\n", openDur)
	fmt.Printf("  Read file:  %v\n", readDur)
	fmt.Printf("  Total:      %v\n", openDur+readDur)
	fmt.Printf("  Body size:  %d bytes\n\n", len(content))

	// 2. go-git: FindBranchForPage equivalent (try all branches)
	fmt.Println("=== go-git: FindBranchForPage (scan branches) ===")
	start = time.Now()
	branches := []string{"truth", "draft/incoming", "draft/agent", "draft/test"}
	var foundBranch string
	for _, b := range branches {
		r, err := repo.Reference(plumbing.NewBranchReferenceName(b), true)
		if err != nil {
			continue
		}
		c, err := repo.CommitObject(r.Hash())
		if err != nil {
			continue
		}
		t, err := c.Tree()
		if err != nil {
			continue
		}
		_, err = t.File(pagePath)
		if err == nil {
			foundBranch = b
			break
		}
	}
	branchDur := time.Since(start)
	fmt.Printf("  Found on:   %s\n", foundBranch)
	fmt.Printf("  Time:       %v\n\n", branchDur)

	// 3. go-git: PageHistoryAllBranches equivalent (walk commits)
	fmt.Println("=== go-git: walk commit history for file (1 result) ===")
	start = time.Now()
	logIter, err := repo.Log(&gogit.LogOptions{
		From: ref.Hash(),
	})
	if err != nil {
		fmt.Println("log error:", err)
	} else {
		found := 0
		logIter.ForEach(func(c *object.Commit) error {
			if found >= 1 {
				return fmt.Errorf("done")
			}
			// Check if this commit touches our file
			if c.NumParents() == 0 {
				found++
				return nil
			}
			parent, err := c.Parent(0)
			if err != nil {
				return nil
			}
			patch, err := parent.Patch(c)
			if err != nil {
				return nil
			}
			for _, fp := range patch.FilePatches() {
				from, to := fp.Files()
				if (from != nil && from.Path() == pagePath) || (to != nil && to.Path() == pagePath) {
					found++
					return nil
				}
			}
			return nil
		})
	}
	historyDur := time.Since(start)
	fmt.Printf("  Time:       %v\n\n", historyDur)

	// 4. git CLI: read file
	fmt.Println("=== git CLI: read file ===")
	start = time.Now()
	out, err := exec.Command("git", "-C", repoPath, "show", branch+":"+pagePath).Output()
	cliDur := time.Since(start)
	if err != nil {
		fmt.Println("  error:", err)
	} else {
		fmt.Printf("  Time:       %v\n", cliDur)
		fmt.Printf("  Body size:  %d bytes\n\n", len(out))
	}

	// 5. git CLI: last commit for file
	fmt.Println("=== git CLI: last commit for file ===")
	start = time.Now()
	out, err = exec.Command("git", "-C", repoPath, "log", "-1", "--format=%an|%ai", branch, "--", pagePath).Output()
	cliHistDur := time.Since(start)
	if err != nil {
		fmt.Println("  error:", err)
	} else {
		fmt.Printf("  Time:       %v\n", cliHistDur)
		fmt.Printf("  Result:     %s\n", string(out))
	}

	// Summary
	fmt.Println("=== SUMMARY ===")
	fmt.Printf("  go-git read file:       %v\n", readDur)
	fmt.Printf("  go-git branch scan:     %v\n", branchDur)
	fmt.Printf("  go-git history walk:    %v\n", historyDur)
	fmt.Printf("  git CLI read file:      %v\n", cliDur)
	fmt.Printf("  git CLI last commit:    %v\n", cliHistDur)
	fmt.Printf("  go-git / CLI ratio:     %.1fx\n", float64(readDur)/float64(cliDur))
}
