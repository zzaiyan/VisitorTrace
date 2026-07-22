package geoip

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

type maxMindProvider struct{}

func (maxMindProvider) attribution() Attribution {
	return Attribution{URL: "https://www.maxmind.com", Label: "IP geolocation by MaxMind"}
}

func (maxMindProvider) updateProfile() UpdateProfile {
	return UpdateProfile{
		URL:          "https://download.maxmind.com/geoip/databases/GeoLite2-City/download?suffix=tar.gz",
		OfficialHost: "download.maxmind.com",
		FreshFor:     72 * time.Hour,
	}
}

func (maxMindProvider) validate(reader *maxminddb.Reader) error {
	databaseType := strings.ToLower(reader.Metadata.DatabaseType)
	if (!strings.Contains(databaseType, "geoip2") && !strings.Contains(databaseType, "geolite2")) || !strings.Contains(databaseType, "city") {
		return fmt.Errorf("unsupported GeoIP database type %q for provider %s", reader.Metadata.DatabaseType, ProviderMaxMind)
	}
	return nil
}

func (maxMindProvider) lookup(reader *maxminddb.Reader, address net.IP) Location {
	return lookupNested(reader, address)
}
