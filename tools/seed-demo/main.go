package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

type demoCity struct {
	country string
	region  string
	city    string
	lat     float64
	lon     float64
	os      string
	browser string
}

var demoCities = []demoCity{
	{country: "CN", region: "BJ", city: "Beijing", lat: 39.9042, lon: 116.4074, os: "Windows", browser: "Chrome"},
	{country: "CN", region: "SH", city: "Shanghai", lat: 31.2304, lon: 121.4737, os: "macOS", browser: "Safari"},
	{country: "CN", region: "GD", city: "Guangzhou", lat: 23.1291, lon: 113.2644, os: "Android", browser: "Chrome Mobile"},
	{country: "JP", region: "13", city: "Tokyo", lat: 35.6762, lon: 139.6503, os: "iOS", browser: "Safari Mobile"},
	{country: "KR", region: "11", city: "Seoul", lat: 37.5665, lon: 126.9780, os: "Windows", browser: "Edge"},
	{country: "SG", region: "SG", city: "Singapore", lat: 1.3521, lon: 103.8198, os: "Linux", browser: "Firefox"},
	{country: "GB", region: "ENG", city: "London", lat: 51.5074, lon: -0.1278, os: "macOS", browser: "Safari"},
	{country: "FR", region: "IDF", city: "Paris", lat: 48.8566, lon: 2.3522, os: "Windows", browser: "Chrome"},
	{country: "US", region: "NY", city: "New York", lat: 40.7128, lon: -74.0060, os: "Windows", browser: "Chrome"},
	{country: "US", region: "CA", city: "San Francisco", lat: 37.7749, lon: -122.4194, os: "macOS", browser: "Firefox"},
	{country: "CA", region: "ON", city: "Toronto", lat: 43.6532, lon: -79.3832, os: "Windows", browser: "Edge"},
	{country: "AU", region: "NSW", city: "Sydney", lat: -33.8688, lon: 151.2093, os: "iOS", browser: "Safari Mobile"},
}

func main() {
	fs := flag.NewFlagSet("seed-demo", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	siteID := fs.String("site-id", "", "Site ID to seed")
	fs.Parse(os.Args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	ctx := context.Background()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fail(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fail(err)
	}
	id := *siteID
	if id == "" {
		sites, err := st.ListSites(ctx)
		if err != nil {
			fail(err)
		}
		if len(sites) != 1 {
			fail(fmt.Errorf("--site-id is required when the database contains %d Sites", len(sites)))
		}
		id = sites[0].ID
	}
	if _, err := st.GetSite(ctx, id); err != nil {
		fail(err)
	}

	now := time.Now().UTC()
	seeded := 0
	for index, city := range demoCities {
		repeats := 1 + index%4
		for repeat := 0; repeat < repeats; repeat++ {
			digest := sha256.Sum256([]byte(fmt.Sprintf("visitortrace-demo-%s-%d-%d", id, index, repeat)))
			occurredAt := now.Add(-time.Duration((index*2+repeat)%28) * 24 * time.Hour).Add(-time.Duration(index*repeat+repeat) * time.Hour)
			observation := store.PageviewObservation{
				SiteID:          id,
				OccurredAt:      occurredAt,
				Path:            demoPath(index, repeat),
				CountryCode:     city.country,
				RegionCode:      city.region,
				City:            city.city,
				Latitude:        floatPointer(city.lat),
				Longitude:       floatPointer(city.lon),
				VisitorDigest:   digest[:],
				OriginalIP:      demoIP(index, repeat),
				OperatingSystem: city.os,
				Browser:         city.browser,
			}
			if _, err := st.RecordPageview(ctx, observation); err != nil {
				fail(err)
			}
			seeded++
		}
	}
	fmt.Printf("seeded %d demo Pageviews into Site %s\n", seeded, id)
}

func demoPath(cityIndex, repeat int) string {
	paths := []string{"/", "/research", "/blog/visitortrace", "/about"}
	return paths[(cityIndex+repeat)%len(paths)]
}

func demoIP(cityIndex, repeat int) string {
	return fmt.Sprintf("192.0.2.%d", (cityIndex*4+repeat)%240+1)
}

func floatPointer(value float64) *float64 {
	return &value
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "seed-demo: %v\n", err)
	os.Exit(1)
}
