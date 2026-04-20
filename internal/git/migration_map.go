package git

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MigrationMapPath is the default filename for the migration tool output (under data dir).
const MigrationMapFile = "migration-map.json"

// LoadMigrationAliases reads dataDir/migration-map.json and returns a map from
// post-migration git paths (NewPath) to prior paths (OldPath) for history following.
// Missing or invalid file yields an empty map and nil error.
func LoadMigrationAliases(dataDir string) (map[string][]string, error) {
	p := filepath.Join(dataDir, MigrationMapFile)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]string{}, nil
		}
		return nil, err
	}
	var raw struct {
		Pages map[string]struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string][]string)
	for _, p := range raw.Pages {
		newP := filepath.ToSlash(p.NewPath)
		oldP := filepath.ToSlash(p.OldPath)
		if newP == "" || oldP == "" || newP == oldP {
			continue
		}
		// Dedupe old paths per new path.
		slice := out[newP]
		dup := false
		for _, x := range slice {
			if x == oldP {
				dup = true
				break
			}
		}
		if !dup {
			out[newP] = append(slice, oldP)
		}
	}
	return out, nil
}
