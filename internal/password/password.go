package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

const (
	minLength   = 8
	maxLength   = 128
	memory      = 19 * 1024
	iterations  = 2
	parallelism = 1
	keyLength   = 32
	saltLength  = 16
)

func Read(passwordFile string, in *os.File, out io.Writer) ([]byte, error) {
	if passwordFile != "" {
		info, err := os.Stat(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("stat password file: %w", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("password file permissions %o are too broad; want 600", info.Mode().Perm())
		}
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("read password file: %w", err)
		}
		return Validate(strings.TrimRight(string(data), "\r\n"))
	}
	if term.IsTerminal(int(in.Fd())) {
		_, _ = fmt.Fprint(out, "Administrator password: ")
		first, err := term.ReadPassword(int(in.Fd()))
		if err != nil {
			return nil, fmt.Errorf("read password: %w", err)
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, "Confirm administrator password: ")
		second, err := term.ReadPassword(int(in.Fd()))
		if err != nil {
			return nil, fmt.Errorf("read password confirmation: %w", err)
		}
		_, _ = fmt.Fprintln(out)
		if subtle.ConstantTimeCompare(first, second) != 1 {
			return nil, errors.New("password confirmation does not match")
		}
		return Validate(string(first))
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("read password input: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) != 2 {
		return nil, errors.New("non-interactive password input must contain password and confirmation lines")
	}
	first := strings.TrimSuffix(lines[0], "\r")
	second := strings.TrimSuffix(lines[1], "\r")
	if first != second {
		return nil, errors.New("password confirmation does not match")
	}
	return Validate(first)
}

func Hash(value []byte) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey(value, salt, iterations, memory, parallelism, keyLength)
	encode := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, encode.EncodeToString(salt), encode.EncodeToString(key)), nil
}

func Verify(value []byte, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var m, t, p uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey(value, salt, t, m, uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func Validate(value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, errors.New("password must be valid UTF-8")
	}
	length := utf8.RuneCountInString(value)
	if length < minLength || length > maxLength {
		return nil, fmt.Errorf("password length must be between %d and %d characters", minLength, maxLength)
	}
	return []byte(value), nil
}
