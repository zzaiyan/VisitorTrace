package geoip

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
)

// ip2region v2/v3 (xdb) offline database reader for the IPv4 data file. The
// whole file (a few MB) is held in memory and the structure is immutable
// after load, so concurrent lookups need no locking.
//
// Layout: a 256-byte header, a 256x256 vector index of 8-byte cells, the
// region payload, and 14-byte index rows. All multibyte fields are
// little-endian, including the stored IPv4 bounds, which are byte-swapped
// for comparison.
const (
	xdbHeaderBytes     = 256
	xdbVectorRows      = 256
	xdbVectorColumns   = 256
	xdbVectorSpacing   = 8
	xdbVectorSize      = xdbVectorRows * xdbVectorColumns * xdbVectorSpacing
	xdbIndexBytes      = 14
	xdbStructureLegacy = 2
	xdbStructureIPv6   = 3
)

type xdbReader struct {
	data []byte
}

type xdbBackend struct {
	reader *xdbReader
}

func openXDB(path string) (localDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	reader := &xdbReader{data: data}
	if err := reader.validate(); err != nil {
		return nil, err
	}
	return &xdbBackend{reader: reader}, nil
}

func (b *xdbBackend) lookup(address netip.Addr) Location {
	if address.Is6() && !address.Is4In6() {
		return Location{}
	}
	value, ok := b.reader.lookup(xdbIPv4(address))
	if !ok {
		return Location{}
	}
	return locationFromIP2Region(value)
}

func (b *xdbBackend) close() error { return nil }

func (b *xdbBackend) verify() error {
	if err := b.reader.validate(); err != nil {
		return err
	}
	// Confirm that the data pointers of the first and last index rows stay
	// inside the file.
	start := xdbStartIndex(b.reader.data)
	end := xdbEndIndex(b.reader.data)
	for _, pointer := range []uint32{start, end} {
		info := b.reader.data[pointer+8 : pointer+xdbIndexBytes]
		length := binary.LittleEndian.Uint16(info[0:2])
		offset := binary.LittleEndian.Uint32(info[2:6])
		if uint64(offset)+uint64(length) > uint64(len(b.reader.data)) {
			return fmt.Errorf("ip2region database data pointer is out of range")
		}
	}
	return nil
}

func (r *xdbReader) validate() error {
	if len(r.data) < xdbHeaderBytes+xdbVectorSize {
		return fmt.Errorf("ip2region database is truncated")
	}
	structure := binary.LittleEndian.Uint16(r.data[0:2])
	switch structure {
	case xdbStructureLegacy:
		// v2 files are IPv4 only.
	case xdbStructureIPv6:
		if version := binary.LittleEndian.Uint16(r.data[16:18]); version != 4 {
			return fmt.Errorf("unsupported ip2region database IP version %d (want the IPv4 data file)", version)
		}
	default:
		return fmt.Errorf("unsupported ip2region database structure %d", structure)
	}
	start := xdbStartIndex(r.data)
	end := xdbEndIndex(r.data)
	if start < xdbHeaderBytes+xdbVectorSize || end < start || uint64(end)+xdbIndexBytes > uint64(len(r.data)) {
		return fmt.Errorf("ip2region database index is invalid")
	}
	return nil
}

func (r *xdbReader) lookup(address uint32) (string, bool) {
	row := address >> 24
	column := (address >> 16) & 0xFF
	offset := uint32(xdbHeaderBytes) + (row*xdbVectorColumns+column)*xdbVectorSpacing
	start := binary.LittleEndian.Uint32(r.data[offset : offset+4])
	end := binary.LittleEndian.Uint32(r.data[offset+4 : offset+8])
	if start == 0 || end == 0 {
		return "", false
	}
	low, high := 0, int((end-start)/xdbIndexBytes)
	for low <= high {
		middleIndex := (low + high) >> 1
		middle := start + uint32(middleIndex)*xdbIndexBytes
		row := r.data[middle : middle+xdbIndexBytes]
		from := xdbStoredIPv4(row[0:4])
		to := xdbStoredIPv4(row[4:8])
		switch {
		case address < from:
			high = middleIndex - 1
		case address > to:
			low = middleIndex + 1
		default:
			length := binary.LittleEndian.Uint16(row[8:10])
			offset := binary.LittleEndian.Uint32(row[10:14])
			if uint64(offset)+uint64(length) > uint64(len(r.data)) {
				return "", false
			}
			return string(r.data[offset : offset+uint32(length)]), true
		}
	}
	return "", false
}

func xdbIPv4(address netip.Addr) uint32 {
	value := address.Unmap()
	if !value.Is4() {
		return 0
	}
	raw := value.As4()
	return binary.BigEndian.Uint32(raw[:])
}

// xdbStoredIPv4 decodes the little-endian stored bounds into the network
// order used for comparisons.
func xdbStoredIPv4(raw []byte) uint32 {
	return binary.LittleEndian.Uint32(raw)
}

func xdbStartIndex(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data[8:12])
}

func xdbEndIndex(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data[12:16])
}
