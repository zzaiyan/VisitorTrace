package server

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const svgXMLDeclaration = `<?xml version="1.0" encoding="UTF-8"?>`

var widgetFrameTemplate = template.Must(template.ParseFS(pageAssets, "templates/widget.html"))

type widgetFrameData struct {
	Language       string
	Title          string
	AnalyticsURL   string
	Attribution    string
	PageviewsLabel string
	VisitorsLabel  string
	Width          int
	Height         int
	SVG            template.HTML
}

func (s *Server) widgetFrame(w http.ResponseWriter, r *http.Request) {
	query := cloneQuery(r.URL.Query())
	siteValues := query["site_id"]
	if len(siteValues) != 1 || strings.TrimSpace(siteValues[0]) == "" {
		http.Error(w, "site_id is required and must occur once", http.StatusBadRequest)
		return
	}
	siteID := strings.TrimSpace(siteValues[0])
	delete(query, "site_id")

	configuredSite, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil || !configuredSite.PublishPublic {
		http.Error(w, "unknown Site", http.StatusNotFound)
		return
	}
	language, err := widgetLanguage(r, configuredSite.PublicLanguage, query["lang"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	delete(query, "lang")
	options, err := parseSiteMapOptions(configuredSite, query)
	if err != nil {
		if errors.Is(err, errMapPreset) {
			s.logger.Error("load Interactive Widget Map Preset failed", "site_id", siteID, "error", err)
			http.Error(w, errMapPreset.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cached, err := s.renderSiteMap(r.Context(), configuredSite, options)
	if errors.Is(err, store.ErrPublicationDisabled) {
		http.Error(w, "public views are disabled", http.StatusNotFound)
		return
	}
	if err != nil {
		s.logger.Error("render Interactive Widget failed", "site_id", siteID, "error", err)
		http.Error(w, "could not render Interactive Widget", http.StatusInternalServerError)
		return
	}

	body, err := s.renderWidgetFrame(configuredSite, options.Width, options.Height, cached.body, language)
	if err != nil {
		s.logger.Error("render Interactive Widget page failed", "site_id", siteID, "error", err)
		http.Error(w, "could not render Interactive Widget", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(body)
	etag := fmt.Sprintf("\"%x\"", sum[:16])
	w.Header().Set("Cache-Control", "public, max-age=300")
	setWidgetFrameHeaders(w)
	w.Header().Set("ETag", etag)
	w.Header().Set("Vary", "Accept-Language")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *Server) renderWidgetFrame(configuredSite store.Site, width, height int, svgBody []byte, language string) ([]byte, error) {
	attribution := geoip.AttributionForProvider(s.Config.GeoIPProvider)
	// maprender escapes every dynamic label before producing the SVG.
	inlineSVG := template.HTML(strings.TrimPrefix(string(svgBody), svgXMLDeclaration))
	data := widgetFrameData{
		Language:       language,
		Title:          configuredSite.Name + " " + translate(language, "visitor_map"),
		AnalyticsURL:   s.appPath("/public/" + configuredSite.ID + "/analytics"),
		Attribution:    attribution.Label,
		PageviewsLabel: translate(language, "pageviews"),
		VisitorsLabel:  translate(language, "unique_visitors"),
		Width:          width,
		Height:         height,
		SVG:            inlineSVG,
	}
	var body bytes.Buffer
	if err := widgetFrameTemplate.ExecuteTemplate(&body, "widget.html", data); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func setWidgetFrameHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors *")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func widgetLanguage(r *http.Request, siteDefault string, values []string) (string, error) {
	if len(values) > 1 {
		return "", fmt.Errorf("lang must occur at most once")
	}
	if len(values) == 1 {
		if !validLanguage(values[0]) {
			return "", fmt.Errorf("lang must be zh-CN, ja, or en")
		}
		return values[0], nil
	}
	if validLanguage(siteDefault) {
		return siteDefault, nil
	}
	if accepted := preferredLanguage(r.Header.Get("Accept-Language")); accepted != "" {
		return accepted, nil
	}
	return "zh-CN", nil
}
