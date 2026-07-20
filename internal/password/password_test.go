package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	value := []byte("correct horse battery staple")
	hash, err := Hash(value)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if !Verify(value, hash) {
		t.Fatal("Verify() rejected the original password")
	}
	if Verify([]byte("wrong password"), hash) {
		t.Fatal("Verify() accepted a different password")
	}
}

func TestReadNonInteractive(t *testing.T) {
	input := temporaryInput(t, "password8\npassword8\n")
	value, err := Read("", input, discardWriter{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(value) != "password8" {
		t.Fatalf("Read() = %q", value)
	}
}
