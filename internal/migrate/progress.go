package migrate

import (
	"fmt"
	"log/slog"
)

// Progress tracks migration progress for logging.
type Progress struct {
	Total   int
	Done    int
	Skipped int
	Failed  int
}

func (p *Progress) Log(oldID, title string) {
	p.Done++
	pct := 0
	if p.Total > 0 {
		pct = (p.Done * 100) / p.Total
	}
	slog.Info(fmt.Sprintf("[%d/%d %d%%] migrated", p.Done, p.Total, pct),
		"old_id", oldID, "title", truncate(title, 60))
}

func (p *Progress) Skip(oldID string) {
	p.Done++
	p.Skipped++
	slog.Info(fmt.Sprintf("[%d/%d] skipped (already migrated)", p.Done, p.Total), "old_id", oldID)
}

func (p *Progress) Fail(oldID string, err error) {
	p.Done++
	p.Failed++
	slog.Error(fmt.Sprintf("[%d/%d] FAILED", p.Done, p.Total), "old_id", oldID, "error", err)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
