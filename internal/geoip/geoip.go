package geoip

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type Location struct {
	CountryCode string
	CountryName string
	RegionCode  string
	RegionName  string
	City        string
	Latitude    *float64
	Longitude   *float64
}

type Resolver struct {
	reader *maxminddb.Reader
}

type mmdbRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

func Open(path string) (*Resolver, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("GeoIP path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	return &Resolver{reader: reader}, nil
}

func (r *Resolver) Lookup(address netip.Addr) Location {
	if r == nil || r.reader == nil || !address.IsValid() {
		return Location{}
	}
	var record mmdbRecord
	if err := r.reader.Lookup(net.IP(address.AsSlice()), &record); err != nil {
		return Location{}
	}
	result := Location{
		CountryCode: record.Country.ISOCode,
		CountryName: localizedName(record.Country.Names),
		City:        localizedName(record.City.Names),
	}
	if len(record.Subdivisions) > 0 {
		result.RegionCode = record.Subdivisions[0].ISOCode
		result.RegionName = localizedName(record.Subdivisions[0].Names)
	}
	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		latitude := record.Location.Latitude
		longitude := record.Location.Longitude
		result.Latitude = &latitude
		result.Longitude = &longitude
	}
	return result
}

func (r *Resolver) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

func localizedName(values map[string]string) string {
	for _, key := range []string{"en", "zh-CN", "zh"} {
		if value := values[key]; value != "" {
			return value
		}
	}
	for _, value := range values {
		return value
	}
	return ""
}
