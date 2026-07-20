package site

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HTTPS://Example.COM/", "https://example.com"},
		{"http://localhost:8080", "http://localhost:8080"},
	}
	for _, test := range tests {
		got, err := NormalizeOrigin(test.input)
		if err != nil {
			t.Errorf("NormalizeOrigin(%q) error = %v", test.input, err)
			continue
		}
		if got != test.want {
			t.Errorf("NormalizeOrigin(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestNormalizeOriginRejectsPath(t *testing.T) {
	if _, err := NormalizeOrigin("https://example.com/blog"); err == nil {
		t.Fatal("NormalizeOrigin accepted an origin with a path")
	}
}
