package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major uint64
	minor uint64
	patch uint64
	pre   []string
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if build := strings.IndexByte(value, '+'); build >= 0 {
		value = value[:build]
	}
	var prerelease string
	if separator := strings.IndexByte(value, '-'); separator >= 0 {
		prerelease = value[separator+1:]
		value = value[:separator]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("version must contain major, minor, and patch numbers")
	}
	numbers := make([]uint64, 3)
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("invalid numeric identifier %q", part)
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("invalid numeric identifier %q", part)
		}
		numbers[index] = parsed
	}
	result := semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}
	if prerelease == "" {
		return result, nil
	}
	result.pre = strings.Split(prerelease, ".")
	for _, identifier := range result.pre {
		if identifier == "" {
			return semanticVersion{}, fmt.Errorf("empty prerelease identifier")
		}
		for _, character := range identifier {
			if (character < '0' || character > '9') && (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && character != '-' {
				return semanticVersion{}, fmt.Errorf("invalid prerelease identifier %q", identifier)
			}
		}
		if numeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return semanticVersion{}, fmt.Errorf("numeric prerelease identifier has leading zero")
		}
	}
	return result, nil
}

func compareSemanticVersions(left, right string) (int, error) {
	a, err := parseSemanticVersion(left)
	if err != nil {
		return 0, err
	}
	b, err := parseSemanticVersion(right)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1, nil
		}
		if pair[0] > pair[1] {
			return 1, nil
		}
	}
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return 0, nil
	}
	if len(a.pre) == 0 {
		return 1, nil
	}
	if len(b.pre) == 0 {
		return -1, nil
	}
	for index := 0; index < len(a.pre) && index < len(b.pre); index++ {
		if a.pre[index] == b.pre[index] {
			continue
		}
		aNumeric, bNumeric := numeric(a.pre[index]), numeric(b.pre[index])
		if aNumeric && bNumeric {
			aValue, _ := strconv.ParseUint(a.pre[index], 10, 64)
			bValue, _ := strconv.ParseUint(b.pre[index], 10, 64)
			if aValue < bValue {
				return -1, nil
			}
			return 1, nil
		}
		if aNumeric {
			return -1, nil
		}
		if bNumeric {
			return 1, nil
		}
		if a.pre[index] < b.pre[index] {
			return -1, nil
		}
		return 1, nil
	}
	if len(a.pre) < len(b.pre) {
		return -1, nil
	}
	if len(a.pre) > len(b.pre) {
		return 1, nil
	}
	return 0, nil
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
