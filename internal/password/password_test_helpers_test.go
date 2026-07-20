package password

import (
	"io"
	"os"
	"testing"
)

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func temporaryInput(t *testing.T, value string) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "password-input-*")
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	if _, err := io.WriteString(file, value); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("rewind input: %v", err)
	}
	return file
}
