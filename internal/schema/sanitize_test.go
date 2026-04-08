package schema

import "testing"

func TestSanitizePathSegment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"foo--bar", "foo-bar"},
		{"  spaces  ", "spaces"},
		{"", "untitled"},
		{"../../../etc/passwd", "etcpasswd"},
		{"UPPER_case", "upper_case"},
	}
	for _, tc := range tests {
		got := SanitizePathSegment(tc.input)
		if got != tc.want {
			t.Errorf("SanitizePathSegment(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeUnicode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"résumé", "resume"},
		{"naïve", "naive"},
		{"über", "uber"},
		{"café", "cafe"},
		{"São Paulo", "sao-paulo"},
		{"Ångström", "angstrom"},
	}
	for _, tc := range tests {
		got := SanitizePathSegment(tc.input)
		if got != tc.want {
			t.Errorf("SanitizePathSegment(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
