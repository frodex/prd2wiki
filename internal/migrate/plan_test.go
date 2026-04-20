package migrate

import "testing"

func TestPlanProjectForTreePath(t *testing.T) {
	plan := &Plan{
		Projects: map[string]*ProjectPlan{
			"g":   {TreePath: "games"},
			"gb":  {TreePath: "games/battletech"},
			"prd": {TreePath: "prd2wiki"},
		},
	}
	cases := []struct {
		path string
		want string
	}{
		{"prd2wiki/foo", "prd2wiki"},
		{"games/battletech/mech", "games/battletech"},
		{"games/other", "games"},
	}
	for _, tc := range cases {
		p := plan.ProjectForTreePath(tc.path)
		if p == nil {
			t.Fatalf("%q: got nil want %s", tc.path, tc.want)
		}
		if p.TreePath != tc.want {
			t.Errorf("%q: got %q want %q", tc.path, p.TreePath, tc.want)
		}
	}
}
