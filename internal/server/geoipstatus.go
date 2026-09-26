package server

import (
	"os"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/operations"
)

type geoIPDatasetStatus struct {
	config.GeoIPDataset
	Label       string
	RoleLabel   string
	OfficialURL string
	SourceMode  string
	File        operations.FileStatus
	Task        *operations.TaskStatus
	Configured  bool
	Loaded      bool
}

func (s *Server) geoIPAttributions() []geoip.Attribution {
	seen := make(map[string]bool)
	var attributions []geoip.Attribution
	for _, dataset := range s.Config.SelectedGeoIPDatasets() {
		if dataset.Online || seen[dataset.Provider] {
			continue
		}
		seen[dataset.Provider] = true
		attributions = append(attributions, geoip.AttributionForProvider(dataset.Provider))
	}
	return attributions
}

func (s *Server) geoIPDatasetStatuses(tasks []operations.TaskStatus, now time.Time, lang string) []geoIPDatasetStatus {
	taskByName := make(map[string]operations.TaskStatus, len(tasks))
	for _, task := range tasks {
		taskByName[task.Operation] = task
	}
	labels := map[string]string{
		"dbip": "DB-IP City Lite", "ip2location": "IP2Location LITE", "maxmind": "MaxMind GeoLite2",
		"ip2region": "ip2region", "tencent": translate(lang, "geoip_provider_tencent"), "amap": translate(lang, "geoip_provider_amap"), "ipinfo": "IPinfo", "bigdatacloud": "BigDataCloud",
	}
	roles := map[string]map[string]string{
		"zh-CN": {"basic": "主库", "domestic_a": "国内主库 A", "domestic_b": "国内主库 B", "foreign_a": "国外主库 A", "foreign_b": "国外主库 B", "backup": "备用库"},
		"en":    {"basic": "Primary", "domestic_a": "Domestic primary A", "domestic_b": "Domestic primary B", "foreign_a": "Foreign primary A", "foreign_b": "Foreign primary B", "backup": "Backup"},
		"ja":    {"basic": "主データ", "domestic_a": "国内主データ A", "domestic_b": "国内主データ B", "foreign_a": "国外主データ A", "foreign_b": "国外主データ B", "backup": "予備データ"},
	}
	roleNames := roles[lang]
	if roleNames == nil {
		roleNames = roles["zh-CN"]
	}
	selected := s.Config.SelectedGeoIPDatasets()
	result := make([]geoIPDatasetStatus, 0, len(selected))
	s.geoMu.RLock()
	defer s.geoMu.RUnlock()
	for _, dataset := range selected {
		item := geoIPDatasetStatus{GeoIPDataset: dataset, Label: labels[dataset.Provider]}
		if !dataset.Online {
			profile, _ := geoip.UpdateProfileForProvider(dataset.Provider)
			item.OfficialURL = profile.URL
			item.SourceMode = "official"
			if dataset.UpdateURL != profile.URL {
				item.SourceMode = "custom"
			}
		}
		var displayRoles []string
		for _, role := range dataset.Roles {
			displayRoles = append(displayRoles, roleNames[role])
		}
		item.RoleLabel = strings.Join(displayRoles, " · ")
		if task, exists := taskByName[dataset.Operation]; exists {
			copy := task
			item.Task = &copy
		}
		if dataset.Online {
			item.Configured = s.Config.OnlineServices[dataset.Provider].Key != ""
		} else {
			if info, err := os.Stat(dataset.Path); err == nil && !info.IsDir() {
				item.File = operations.FileStatus{Exists: true, Name: info.Name(), Size: info.Size(), ModifiedAt: info.ModTime().UTC()}
				profile, _ := geoip.UpdateProfileForProvider(dataset.Provider)
				if profile.CalendarMonthly {
					item.File.Stale = info.ModTime().UTC().Year() != now.UTC().Year() || info.ModTime().UTC().Month() != now.UTC().Month()
				} else if profile.FreshFor > 0 {
					item.File.Stale = now.Sub(info.ModTime().UTC()) > profile.FreshFor
				}
			}
			switch dataset.ID {
			case "primary":
				item.Loaded = s.geoIP != nil
			case "foreign":
				item.Loaded = s.geoForeign != nil
			default:
				item.Loaded = s.geoBackups[dataset.Provider] != nil
			}
		}
		result = append(result, item)
	}
	return result
}
