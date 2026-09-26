package config

import (
	"path/filepath"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

// GeoIPDataset identifies one selected data source and its independent
// maintenance target. Repeated online services share one dataset and key.
type GeoIPDataset struct {
	ID          string
	Provider    string
	Roles       []string
	Online      bool
	Path        string
	Operation   string
	UpdateURL   string
	ChecksumURL string
}

func (c Config) SelectedGeoIPDatasets() []GeoIPDataset {
	var result []GeoIPDataset
	onlineIndex := make(map[string]int)
	add := func(role, id, provider, path, operation string) {
		if provider == "" {
			return
		}
		if _, err := geoip.NormalizeProvider(provider); err != nil {
			onlineID := "online_" + provider
			if index, exists := onlineIndex[onlineID]; exists {
				result[index].Roles = append(result[index].Roles, role)
				return
			}
			onlineIndex[onlineID] = len(result)
			result = append(result, GeoIPDataset{ID: onlineID, Provider: provider, Roles: []string{role}, Online: true, Operation: "geoip_online_" + provider})
			return
		}
		profile, _ := geoip.UpdateProfileForProvider(provider)
		item := GeoIPDataset{ID: id, Provider: provider, Roles: []string{role}, Path: path, Operation: operation, UpdateURL: profile.URL}
		if id == "primary" {
			item.UpdateURL = c.GeoIPUpdateURL
			item.ChecksumURL = c.GeoIPChecksumURL
		} else if source, ok := c.GeoIPDatasetSources[id]; ok && source.Provider == provider {
			if source.URL != "" {
				item.UpdateURL = source.URL
			}
			item.ChecksumURL = source.ChecksumURL
		}
		result = append(result, item)
	}
	if c.GeoIPPreset == "basic" || c.GeoIPPreset == "" {
		basic := c.GeoIPBasicBackend
		if basic == "" {
			basic = c.GeoIPProvider
		}
		add("basic", "primary", basic, c.GeoIPPath, "geoip_update")
		return result
	}
	domestic := c.GeoIPDomesticOffline
	if domestic == "" {
		if c.GeoIPPreset == "recommended" {
			domestic = "ipinfo"
		} else {
			domestic = "ip2region"
		}
	}
	foreign := c.GeoIPForeignOffline
	if foreign == "" {
		foreign = "ip2location"
	}
	foreignPath := c.GeoIPForeignPath
	if foreignPath == "" {
		foreignPath = filepath.Join(c.DataDir, "geoip-foreign.mmdb")
	}
	if foreign == "ip2region" && foreignPath == filepath.Join(c.DataDir, "geoip-foreign.mmdb") {
		foreignPath = filepath.Join(c.DataDir, "geoip-foreign.xdb")
	}
	add("domestic_a", "primary", domestic, c.GeoIPPath, "geoip_update")
	if c.GeoIPPreset == "precise" {
		suffix := ".mmdb"
		if c.GeoIPDomesticOnline == "ip2region" {
			suffix = ".xdb"
		}
		add("domestic_b", "domestic_b", c.GeoIPDomesticOnline, filepath.Join(c.DataDir, "geoip-domestic-b"+suffix), "geoip_domestic_b_"+c.GeoIPDomesticOnline)
	}
	add("foreign_a", "foreign", foreign, foreignPath, "geoip_foreign_"+foreign)
	if c.GeoIPPreset == "precise" {
		suffix := ".mmdb"
		if c.GeoIPForeignOnline == "ip2region" {
			suffix = ".xdb"
		}
		add("foreign_b", "foreign_b", c.GeoIPForeignOnline, filepath.Join(c.DataDir, "geoip-foreign-b"+suffix), "geoip_foreign_b_"+c.GeoIPForeignOnline)
		for _, provider := range c.GeoIPBackups {
			path := filepath.Join(c.DataDir, "geoip-backup-"+provider+".mmdb")
			if provider == "ip2region" {
				path = filepath.Join(c.DataDir, "geoip-backup-ip2region.xdb")
			}
			add("backup", "backup_"+provider, provider, path, "geoip_backup_"+provider)
		}
	}
	return result
}
