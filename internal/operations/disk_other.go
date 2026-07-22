//go:build !linux

package operations

import "fmt"

func diskSpace(string) (available, total uint64, err error) {
	return 0, 0, fmt.Errorf("disk statistics are unavailable on this platform")
}
