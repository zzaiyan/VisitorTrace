package visitor

import (
	"bytes"
	"testing"
)

func TestDigestUsesBrowserIDWhenAvailable(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	first, err := Digest(key, "00112233445566778899aabbccddeeff", "192.0.2.1", "User Agent")
	if err != nil {
		t.Fatalf("Digest() error = %v", err)
	}
	second, err := Digest(key, "00112233445566778899aabbccddeeff", "198.51.100.1", "Different Agent")
	if err != nil {
		t.Fatalf("Digest() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Browser Visitor ID digest changed with fallback observations")
	}
}

func TestDigestFallback(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	first, err := Digest(key, "", "192.0.2.1", "Example   Browser")
	if err != nil {
		t.Fatalf("Digest() error = %v", err)
	}
	second, err := Digest(key, "", "192.0.2.1", "Example Browser")
	if err != nil {
		t.Fatalf("Digest() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("fallback User-Agent normalization is unstable")
	}
}
