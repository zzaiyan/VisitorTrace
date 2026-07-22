package geoip

import "testing"

func TestNormalizeDBIPCity(t *testing.T) {
	tests := []struct {
		country string
		city    string
		region  string
		want    string
	}{
		{"CN", "Shenzhen (Bantian Residential District)", "Guangdong", "Shenzhen"},
		{"CN", "Qinghe (Qinghe Subdistrict, Beijing)", "Beijing", "Beijing"},
		{"CN", "Jinrongjie (Xicheng District)", "Beijing", "Beijing"},
		{"CN", "Longgang District", "Guangdong", ""},
		{"CN", "Dali Old Town", "Yunnan", "Dali"},
		{"CN", "Guangzhou", "Guangdong", "Guangzhou"},
		{"US", "Washington (District)", "District of Columbia", "Washington (District)"},
	}
	for _, test := range tests {
		if got := normalizeDBIPCity(test.country, test.city, test.region); got != test.want {
			t.Errorf("normalizeDBIPCity(%q, %q, %q) = %q, want %q", test.country, test.city, test.region, got, test.want)
		}
	}
}
