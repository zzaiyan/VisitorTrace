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
	ProviderIP2Region   Provider = "ip2region"
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
	open(path string) (localDatabase, error)
}

// localDatabase is one provider's loaded database, independent of the
// on-disk format behind it.
type localDatabase interface {
	lookup(address netip.Addr) Location
	close() error
	verify() error
}

// mmdbSchema is the MaxMind-style lookup subset shared by MMDB providers.
type mmdbSchema interface {
	validate(*maxminddb.Reader) error
	lookup(*maxminddb.Reader, net.IP) Location
}

type mmdbBackend struct {
	reader *maxminddb.Reader
	schema mmdbSchema
}

func openMMDB(schema mmdbSchema, path string) (localDatabase, error) {
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	if err := schema.validate(reader); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return &mmdbBackend{reader: reader, schema: schema}, nil
}

func (b *mmdbBackend) lookup(address netip.Addr) Location {
	return b.schema.lookup(b.reader, net.IP(address.AsSlice()))
}

func (b *mmdbBackend) close() error { return b.reader.Close() }

func (b *mmdbBackend) verify() error { return b.reader.Verify() }

func NormalizeProvider(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = string(ProviderDBIP)
	}
	switch Provider(value) {
	case ProviderDBIP, ProviderMaxMind, ProviderIP2Location, ProviderIP2Region:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported GeoIP provider %q (want dbip, maxmind, ip2location, or ip2region)", value)
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
	case ProviderIP2Region:
		return ip2regionProvider{}, nil
	default:
		return nil, fmt.Errorf("unsupported GeoIP provider %q", value)
	}
}

type Resolver struct {
	backend localDatabase
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
	backend, err := adapter.open(path)
	if err != nil {
		return nil, err
	}
	return &Resolver{backend: backend}, nil
}

func Validate(path string) error {
	return ValidateWithProvider(string(ProviderDBIP), path)
}

func ValidateWithProvider(provider, path string) error {
	adapter, err := adapterFor(provider)
	if err != nil {
		return err
	}
	backend, err := adapter.open(path)
	if err != nil {
		return err
	}
	defer backend.close()
	if err := backend.verify(); err != nil {
		return fmt.Errorf("verify GeoIP database: %w", err)
	}
	return nil
}

func (r *Resolver) Lookup(address netip.Addr) Location {
	if r == nil || r.backend == nil || !address.IsValid() {
		return Location{}
	}
	return r.backend.lookup(address)
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
	if r == nil || r.backend == nil {
		return nil
	}
	return r.backend.close()
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
