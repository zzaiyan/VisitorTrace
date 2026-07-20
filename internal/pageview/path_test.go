package pageview

import (
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/", "/"},
		{"/a/./b/../c/", "/a/c/"},
		{"/a//b/", "/a//b/"},
		{"/A/Index.html", "/A/Index.html"},
		{"/hello world", "/hello%20world"},
		{"/%E8%AE%BF%E8%BF%B9", "/%E8%AE%BF%E8%BF%B9"},
	}
	for _, test := range tests {
		got, err := NormalizePath(test.input)
		if err != nil {
			t.Errorf("NormalizePath(%q) error = %v", test.input, err)
			continue
		}
		if got != test.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestNormalizePathRejectsInvalidValues(t *testing.T) {
	values := []string{"relative", "/path?q=1", "/path#fragment", "/bad%0Apath", "/bad%zz", "/" + strings.Repeat("x", MaxPathBytes)}
	for _, value := range values {
		if _, err := NormalizePath(value); err == nil {
			t.Errorf("NormalizePath(%q) unexpectedly succeeded", value)
		}
	}
}
