package geoiponline

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
	"time"
)

func TestNormalizeProviderAndKeyRequirement(t *testing.T) {
	if provider, err := NormalizeProvider(" Tencent "); err != nil || provider != "tencent" {
		t.Fatalf("NormalizeProvider = %q, %v", provider, err)
	}
	if _, err := NormalizeProvider("maxmind"); err == nil {
		t.Fatal("same-origin online providers are rejected")
	}
	if _, err := New("ipinfo", "", time.Second); err == nil {
		t.Fatal("New without a key should fail")
	}
}

func TestLookupTencentCachesResult(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/ws/location/v1/ip" || r.URL.Query().Get("ip") != "114.114.114.114" {
			t.Errorf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"result":{"location":{"lat":32.06,"lng":118.78},"ad_info":{"nation":"中国","nation_code":156,"province":"江苏省","city":"南京市"}}}`))
	}))
	defer server.Close()
	client, err := New("tencent", "test-key", time.Second)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	client.setBaseURL(server.URL)
	address := netip.MustParseAddr("114.114.114.114")
	location, err := client.Lookup(context.Background(), address)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if location.CountryCode != "CN" || location.RegionName != "江苏省" || location.City != "南京" || location.Latitude == nil || *location.Longitude != 118.78 {
		t.Fatalf("Lookup = %+v", location)
	}
	if _, err := client.Lookup(context.Background(), address); err != nil {
		t.Fatalf("cached Lookup error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("server called %d times, want 1 (cached)", calls)
	}
}

func TestLookupAMapEmptyCityArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","province":"青海省","city":[]}`))
	}))
	defer server.Close()
	client, _ := New("amap", "test-key", time.Second)
	client.setBaseURL(server.URL)
	location, err := client.Lookup(context.Background(), netip.MustParseAddr("36.17.0.1"))
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if location.CountryCode != "CN" || location.RegionName != "青海省" || location.City != "青海省" {
		t.Fatalf("Lookup = %+v", location)
	}
}

func TestLookupIPinfoParsesCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/8.8.8.8/json" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"8.8.8.8","city":"Mountain View","region":"California","country":"US","loc":"37.4056,-122.0775"}`))
	}))
	defer server.Close()
	client, _ := New("ipinfo", "test-token", time.Second)
	client.setBaseURL(server.URL)
	location, err := client.Lookup(context.Background(), netip.MustParseAddr("8.8.8.8"))
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if location.CountryCode != "US" || location.City != "Mountain View" || location.Latitude == nil || *location.Latitude != 37.4056 {
		t.Fatalf("Lookup = %+v", location)
	}
}

func TestLookupBigDataCloudParsesFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"country":{"isoAlpha2":"DE","name":"Germany"},"location":{"principalSubdivision":"Hesse","isoPrincipalSubdivisionCode":"DE-HE","city":"Frankfurt","latitude":50.11,"longitude":8.68}}`))
	}))
	defer server.Close()
	client, _ := New("bigdatacloud", "test-key", time.Second)
	client.setBaseURL(server.URL)
	location, err := client.Lookup(context.Background(), netip.MustParseAddr("145.100.0.1"))
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if location.CountryCode != "DE" || location.RegionCode != "HE" || location.RegionName != "Hesse" || location.City != "Frankfurt" || location.Longitude == nil || *location.Longitude != 8.68 {
		t.Fatalf("Lookup = %+v", location)
	}
}

func TestLookupSurfacesProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client, _ := New("ipinfo", "test-token", time.Second)
	client.setBaseURL(server.URL)
	if _, err := client.Lookup(context.Background(), netip.MustParseAddr("8.8.8.8")); err == nil {
		t.Fatal("Lookup should surface the HTTP error")
	}
}

func TestTencentSignatureIsAppended(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"result":{"ad_info":{"nation":"中国","nation_code":"CN","province":"北京市","city":"北京市"}}}`))
	}))
	defer server.Close()
	// Key with an SK: the credential carries "KEY:SK".
	client, err := New("tencent", "ABCD-KEY:my-secret-sk", time.Second)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	client.setBaseURL(server.URL)
	if _, err := client.Lookup(context.Background(), netip.MustParseAddr("114.114.114.114")); err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	// Independent construction: path + "?" + sorted params + SK, MD5 hex.
	want := md5.Sum([]byte("/ws/location/v1/ip?ip=114.114.114.114&key=ABCD-KEYmy-secret-sk"))
	if got := gotQuery.Get("sig"); got != hex.EncodeToString(want[:]) {
		t.Fatalf("sig = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}

func TestAMapSignatureIsAppended(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","province":"北京市","city":"北京市"}`))
	}))
	defer server.Close()
	client, err := New("amap", "0123456789abcdef0123456789abcdef:sk-value", time.Second)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	client.setBaseURL(server.URL)
	if _, err := client.Lookup(context.Background(), netip.MustParseAddr("114.114.114.114")); err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	want := md5.Sum([]byte("ip=114.114.114.114&key=0123456789abcdef0123456789abcdefsk-value"))
	if got := gotQuery.Get("sig"); got != hex.EncodeToString(want[:]) {
		t.Fatalf("sig = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}

func TestPlainKeysDoNotSign(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"result":{"ad_info":{"nation":"中国","nation_code":156}}}`))
	}))
	defer server.Close()
	client, _ := New("tencent", "plain-key-no-sk", time.Second)
	client.setBaseURL(server.URL)
	if _, err := client.Lookup(context.Background(), netip.MustParseAddr("114.114.114.114")); err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if _, ok := gotQuery["sig"]; ok {
		t.Fatal("plain key must not sign the request")
	}
}
