package geoip

import (
	"strings"
	"time"
)

type ip2regionProvider struct{}

func (ip2regionProvider) attribution() Attribution {
	return Attribution{URL: "https://github.com/lionsoul2014/ip2region", Label: "IP geolocation by ip2region"}
}

func (ip2regionProvider) updateProfile() UpdateProfile {
	return UpdateProfile{
		URL:          "https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region.xdb",
		OfficialHost: "raw.githubusercontent.com",
		FreshFor:     7 * 24 * time.Hour,
	}
}

func (p ip2regionProvider) open(path string) (localDatabase, error) {
	return openXDB(path)
}

// locationFromIP2Region maps a region row onto the unified Location fields.
// The current dataset uses "country|province|city|ISP|ISO-code"; the legacy
// "country|area|province|city|ISP" order is accepted as a fallback and its
// country name is mapped to an ISO code when recognized.
func locationFromIP2Region(value string) Location {
	parts := strings.Split(value, "|")
	get := func(index int) string {
		if index >= len(parts) {
			return ""
		}
		trimmed := strings.TrimSpace(parts[index])
		if trimmed == "0" {
			return ""
		}
		return trimmed
	}
	country := get(0)
	province := get(1)
	city := get(2)
	code := get(4)
	if len(parts) >= 5 && looksLikeISOCountryCode(code) {
		// Current format: country|province|city|ISP|ISO-code.
		return Location{
			CountryCode: strings.ToUpper(code),
			CountryName: country,
			RegionName:  province,
			City:        ip2RegionCity(city),
		}
	}
	// Legacy format: country|area|province|city|ISP.
	return Location{
		CountryCode: ip2RegionCountryCode(country),
		CountryName: country,
		RegionName:  get(2),
		City:        ip2RegionCity(get(3)),
	}
}

func looksLikeISOCountryCode(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func ip2RegionCountryCode(country string) string {
	if country == "" {
		return ""
	}
	if code, ok := ip2RegionCountryCodes[strings.ToLower(country)]; ok {
		return code
	}
	return ""
}

func ip2RegionCity(city string) string {
	if city == "" {
		return ""
	}
	return strings.TrimSuffix(city, "市")
}

// ip2RegionCountryCodes covers the country names that appear in the free
// ip2region dataset, keyed by lowercase name. Unlisted countries keep their
// name and an empty ISO code.
var ip2RegionCountryCodes = map[string]string{
	"中国": "CN", "china": "CN",
	"美国": "US", "united states": "US", "usa": "US",
	"日本": "JP", "japan": "JP",
	"韩国": "KR", "south korea": "KR", "republic of korea": "KR", "korea": "KR",
	"新加坡": "SG", "singapore": "SG",
	"德国": "DE", "germany": "DE",
	"英国": "GB", "united kingdom": "GB", "great britain": "GB",
	"法国": "FR", "france": "FR",
	"加拿大": "CA", "canada": "CA",
	"澳大利亚": "AU", "australia": "AU",
	"俄罗斯": "RU", "russia": "RU",
	"印度": "IN", "india": "IN",
	"巴西": "BR", "brazil": "BR",
	"意大利": "IT", "italy": "IT",
	"西班牙": "ES", "spain": "ES",
	"荷兰": "NL", "netherlands": "NL",
	"瑞典": "SE", "sweden": "SE",
	"瑞士": "CH", "switzerland": "CH",
	"爱尔兰": "IE", "ireland": "IE",
	"新西兰": "NZ", "new zealand": "NZ",
	"泰国": "TH", "thailand": "TH",
	"越南": "VN", "vietnam": "VN", "viet nam": "VN",
	"马来西亚": "MY", "malaysia": "MY",
	"印度尼西亚": "ID", "indonesia": "ID",
	"菲律宾": "PH", "philippines": "PH",
	"中国香港": "HK", "hong kong": "HK", "中国台湾": "TW", "taiwan": "TW", "中国澳门": "MO", "macao": "MO", "macau": "MO",
	"土耳其": "TR", "turkey": "TR", "turkiye": "TR",
	"以色列": "IL", "israel": "IL",
	"沙特阿拉伯": "SA", "saudi arabia": "SA",
	"阿联酋": "AE", "united arab emirates": "AE",
	"墨西哥": "MX", "mexico": "MX",
	"阿根廷": "AR", "argentina": "AR",
	"南非": "ZA", "south africa": "ZA",
	"埃及": "EG", "egypt": "EG",
	"尼日利亚": "NG", "nigeria": "NG",
	"肯尼亚": "KE", "kenya": "KE",
	"波兰": "PL", "poland": "PL",
	"乌克兰": "UA", "ukraine": "UA",
	"挪威": "NO", "norway": "NO",
	"丹麦": "DK", "denmark": "DK",
	"芬兰": "FI", "finland": "FI",
	"奥地利": "AT", "austria": "AT",
	"比利时": "BE", "belgium": "BE",
	"葡萄牙": "PT", "portugal": "PT",
	"希腊": "GR", "greece": "GR",
	"捷克": "CZ", "czech republic": "CZ",
	"罗马尼亚": "RO", "romania": "RO",
	"匈牙利": "HU", "hungary": "HU",
	"智利": "CL", "chile": "CL",
	"哥伦比亚": "CO", "colombia": "CO",
	"秘鲁": "PE", "peru": "PE",
	"巴基斯坦": "PK", "pakistan": "PK",
	"孟加拉国": "BD", "bangladesh": "BD",
	"斯里兰卡": "LK", "sri lanka": "LK",
	"尼泊尔": "NP", "nepal": "NP",
	"缅甸": "MM", "myanmar": "MM",
	"柬埔寨": "KH", "cambodia": "KH",
	"老挝": "LA", "laos": "LA",
	"蒙古": "MN", "mongolia": "MN",
	"哈萨克斯坦": "KZ", "kazakhstan": "KZ",
	"伊朗": "IR", "iran": "IR",
	"伊拉克": "IQ", "iraq": "IQ",
	"约旦": "JO", "jordan": "JO",
	"卡塔尔": "QA", "qatar": "QA",
	"科威特": "KW", "kuwait": "KW",
	"巴林": "BH", "bahrain": "BH",
	"阿曼": "OM", "oman": "OM",
	"黎巴嫩": "LB", "lebanon": "LB",
	"冰岛": "IS", "iceland": "IS",
	"卢森堡": "LU", "luxembourg": "LU",
	"保加利亚": "BG", "bulgaria": "BG",
	"克罗地亚": "HR", "croatia": "HR",
	"塞尔维亚": "RS", "serbia": "RS",
	"立陶宛": "LT", "lithuania": "LT",
	"拉脱维亚": "LV", "latvia": "LV",
	"爱沙尼亚": "EE", "estonia": "EE",
	"白俄罗斯": "BY", "belarus": "BY",
	"摩尔多瓦": "MD", "moldova": "MD",
	"斯洛伐克": "SK", "slovakia": "SK",
	"斯洛文尼亚": "SI", "slovenia": "SI",
	"阿尔及利亚": "DZ", "algeria": "DZ",
	"摩洛哥": "MA", "morocco": "MA",
	"突尼斯": "TN", "tunisia": "TN",
	"加纳": "GH", "ghana": "GH",
	"乌干达": "UG", "uganda": "UG",
	"坦桑尼亚": "TZ", "tanzania": "TZ",
	"埃塞俄比亚": "ET", "ethiopia": "ET",
	"委内瑞拉": "VE", "venezuela": "VE",
	"厄瓜多尔": "EC", "ecuador": "EC",
	"乌拉圭": "UY", "uruguay": "UY",
	"巴拉圭": "PY", "paraguay": "PY",
	"玻利维亚": "BO", "bolivia": "BO",
}
