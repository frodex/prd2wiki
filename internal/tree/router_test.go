package tree

import "testing"

func TestIsReservedRequestPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", false},
		{"/prd2wiki/foo", false},
		{"/api/projects/x/pages", true},
		{"/static/x", true},
		{"/blobs/ab/cdef", true},
		{"/admin/", true},
		{"/projects/default/pages", true},
		{"/health", true},
		{"/health/extra", false},
		{"/debug/pprof", true},
	}
	for _, tt := range tests {
		if got := IsReservedRequestPath(tt.path); got != tt.want {
			t.Errorf("IsReservedRequestPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
