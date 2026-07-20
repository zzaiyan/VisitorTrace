package visitor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func Digest(siteKey []byte, browserID, clientIP, userAgent string) ([]byte, error) {
	if len(siteKey) != 32 {
		return nil, fmt.Errorf("Site HMAC key must contain 32 bytes")
	}
	mac := hmac.New(sha256.New, siteKey)
	if browserID != "" {
		value, err := decodeBrowserID(browserID)
		if err != nil {
			return nil, err
		}
		_, _ = mac.Write([]byte("browser\x00"))
		_, _ = mac.Write(value)
		return mac.Sum(nil), nil
	}
	if clientIP == "" || userAgent == "" {
		return nil, fmt.Errorf("client IP and User-Agent are required when Browser Visitor ID is unavailable")
	}
	_, _ = mac.Write([]byte("fallback\x00"))
	_, _ = mac.Write([]byte(clientIP))
	_, _ = mac.Write([]byte{'\x00'})
	_, _ = mac.Write([]byte(strings.Join(strings.Fields(userAgent), " ")))
	return mac.Sum(nil), nil
}

func decodeBrowserID(value string) ([]byte, error) {
	if len(value) != 32 {
		return nil, fmt.Errorf("Browser Visitor ID must be 32 hexadecimal characters")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("Browser Visitor ID must be 32 hexadecimal characters")
	}
	return decoded, nil
}
