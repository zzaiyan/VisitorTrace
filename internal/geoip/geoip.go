package geoip

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

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

type Provider string

const (
	ProviderDBIP        Provider = "dbip"
	ProviderMaxMind     Provider = "maxmind"
	ProviderIP2Location Provider = "ip2location"
)

type Attribution struct {
	URL   string
	Label string
}

type UpdateProfile struct {
	URL             string
	OfficialHost    string
	FreshFor        time.Duration
	CalendarMonthly bool
}

type providerAdapter interface {
	attribution() Attribution
	updateProfile() UpdateProfile
	validate(*maxminddb.Reader) error
	lookup(*maxminddb.Reader, net.IP) Location
}

func NormalizeProvider(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = string(ProviderDBIP)
	}
	switch Provider(value) {
	case ProviderDBIP, ProviderMaxMind, ProviderIP2Location:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported GeoIP provider %q (want dbip, maxmind, or ip2location)", value)
	}
}

func AttributionForProvider(value string) Attribution {
	provider, err := adapterFor(value)
	if err != nil {
		provider = dbipProvider{}
	}
	return provider.attribution()
}

func UpdateProfileForProvider(value string) (UpdateProfile, error) {
	provider, err := adapterFor(value)
	if err != nil {
		return UpdateProfile{}, err
	}
	return provider.updateProfile(), nil
}

func IsDefaultUpdateURL(value string) bool {
	value = strings.TrimSpace(value)
	for _, provider := range []string{string(ProviderDBIP), string(ProviderMaxMind), string(ProviderIP2Location)} {
		profile, _ := UpdateProfileForProvider(provider)
		if value == profile.URL {
			return true
		}
	}
	return false
}

func adapterFor(value string) (providerAdapter, error) {
	normalized, err := NormalizeProvider(value)
	if err != nil {
		return nil, err
	}
	switch Provider(normalized) {
	case ProviderDBIP:
		return dbipProvider{}, nil
	case ProviderMaxMind:
		return maxMindProvider{}, nil
	case ProviderIP2Location:
		return ip2LocationProvider{}, nil
	default:
		return nil, fmt.Errorf("unsupported GeoIP provider %q", value)
	}
}

type Resolver struct {
	reader   *maxminddb.Reader
	provider providerAdapter
}

type nestedRecord struct {
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
	return OpenWithProvider(string(ProviderDBIP), path)
}

func OpenWithProvider(provider, path string) (*Resolver, error) {
	adapter, err := adapterFor(provider)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("GeoIP path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	if err := adapter.validate(reader); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return &Resolver{reader: reader, provider: adapter}, nil
}

func Validate(path string) error {
	return ValidateWithProvider(string(ProviderDBIP), path)
}

func ValidateWithProvider(provider, path string) error {
	adapter, err := adapterFor(provider)
	if err != nil {
		return err
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("open GeoIP database: %w", err)
	}
	defer reader.Close()
	if err := adapter.validate(reader); err != nil {
		return err
	}
	if err := reader.Verify(); err != nil {
		return fmt.Errorf("verify GeoIP database: %w", err)
	}
	return nil
}

func (r *Resolver) Lookup(address netip.Addr) Location {
	if r == nil || r.reader == nil || !address.IsValid() {
		return Location{}
	}
	return r.provider.lookup(r.reader, net.IP(address.AsSlice()))
}

func lookupNested(reader *maxminddb.Reader, address net.IP) Location {
	var record nestedRecord
	if err := reader.Lookup(address, &record); err != nil {
		return Location{}
	}
	return locationFromNestedRecord(record)
}

func locationFromNestedRecord(record nestedRecord) Location {
	result := Location{
		CountryCode: record.Country.ISOCode,
		CountryName: localizedName(record.Country.Names),
	}
	if len(record.Subdivisions) > 0 {
		result.RegionCode = record.Subdivisions[0].ISOCode
		result.RegionName = localizedName(record.Subdivisions[0].Names)
	}
	result.City = localizedName(record.City.Names)
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
