package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"nwork/internal/database"
	"nwork/internal/models"
)

// Country 国家数据结构
type Country struct {
	Code string `json:"code"`
	Name struct {
		Common   string `json:"common"`
		Official string `json:"official"`
		NativeName struct {
			Zho struct {
				Common   string `json:"common"`
				Official string `json:"official"`
			} `json:"zho"`
		} `json:"nativeName"`
	} `json:"name"`
	Translations struct {
		Zho struct {
			Common   string `json:"common"`
			Official string `json:"official"`
		} `json:"zho"`
	} `json:"translations"`
	CCA2 string `json:"cca2"` // ISO 3166-1 alpha-2
}

// Region 地区数据结构
type Region struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// We intentionally do NOT persist translations in DB.
// region_name in DB remains English (canonical). For zh display we fetch from GeoNames at request time.

// GetCountries 获取国家列表（从 REST Countries API）
func GetCountries(c *fiber.Ctx) error {
	search := c.Query("search", "")
	lang := strings.ToLower(strings.TrimSpace(c.Query("lang", "")))
	useEn := strings.HasPrefix(lang, "en")

	// Try local translation file first (stable/offline-ish)
	type countryTranslationsFile struct {
		Countries map[string]map[string]string `json:"countries"` // code -> {en, zh, zh-CN}
	}
	var (
		ctOnce sync.Once
		ctData countryTranslationsFile
		ctErr  error
	)
	loadCountryTranslations := func() (countryTranslationsFile, error) {
		ctOnce.Do(func() {
			p := filepath.Join("data", "country_translations.json")
			b, err := os.ReadFile(p)
			if err != nil {
				if exe, e2 := os.Executable(); e2 == nil {
					p2 := filepath.Join(filepath.Dir(exe), "data", "country_translations.json")
					if b2, e3 := os.ReadFile(p2); e3 == nil {
						b = b2
						err = nil
					}
				}
			}
			if err != nil {
				ctErr = err
				return
			}
			if e := json.Unmarshal(b, &ctData); e != nil {
				ctErr = e
				return
			}
			if ctData.Countries == nil {
				ctData.Countries = map[string]map[string]string{}
			}
		})
		return ctData, ctErr
	}

	if fileData, err := loadCountryTranslations(); err == nil && fileData.Countries != nil && len(fileData.Countries) > 0 {
		// Convert to standard format and filter
		var result []map[string]interface{}
		for code, names := range fileData.Countries {
			cc := strings.ToUpper(strings.TrimSpace(code))
			if cc == "" {
				continue
			}
			en := strings.TrimSpace(names["en"])
			zh := strings.TrimSpace(names["zh"])
			zhCN := strings.TrimSpace(names["zh-CN"])
			display := zh
			if strings.Contains(lang, "zh-cn") && zhCN != "" {
				display = zhCN
			}
			if useEn || display == "" {
				display = en
			}
			if display == "" {
				display = cc
			}
			if en == "" {
				en = cc
			}

			// Search filter
			if search != "" {
				s := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(cc), s) &&
					!strings.Contains(strings.ToLower(display), s) &&
					!strings.Contains(strings.ToLower(en), s) &&
					!strings.Contains(strings.ToLower(zh), s) {
					continue
				}
			}

			result = append(result, map[string]interface{}{
				"code":    cc,
				"name":    display,
				"name_en": en,
				"name_zh": zh,
			})
		}

		// Sort by display name
		sort.Slice(result, func(i, j int) bool {
			return result[i]["name"].(string) < result[j]["name"].(string)
		})

		// Pagination
		page := c.QueryInt("page", 1)
		limit := c.QueryInt("limit", 250)
		offset := (page - 1) * limit
		total := len(result)
		start := offset
		end := offset + limit
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}
		var data []map[string]interface{}
		if start < end {
			data = result[start:end]
		}

		return c.JSON(fiber.Map{
			"data":  data,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
	
	// 从 REST Countries API 获取数据（包含 translations 字段以获取中文名称）
	resp, err := http.Get("https://restcountries.com/v3.1/all?fields=name,cca2,translations")
	if err != nil {
		log.Printf("❌ Failed to fetch countries from REST Countries API: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch countries: " + err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ REST Countries API returned status %d", resp.StatusCode)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("REST Countries API returned status %d", resp.StatusCode)})
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Failed to read response from REST Countries API: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to read response: " + err.Error()})
	}

	var countries []Country
	if err := json.Unmarshal(body, &countries); err != nil {
		log.Printf("❌ Failed to parse countries JSON: %v", err)
		bodyLen := len(body)
		if bodyLen > 500 {
			bodyLen = 500
		}
		log.Printf("Response body (first %d chars): %s", bodyLen, string(body[:bodyLen]))
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse countries: " + err.Error()})
	}

	// 转换为标准格式并过滤
	var result []map[string]interface{}
	for _, country := range countries {
		code := country.CCA2
		if code == "" {
			code = country.Code
		}
		if code == "" {
			continue // 跳过没有代码的国家
		}
		
		// 生成英文/中文兩種名稱（name_en / name_zh），並依 lang 決定預設 name
		nameEn := strings.TrimSpace(country.Name.Common)
		if nameEn == "" {
			nameEn = strings.TrimSpace(country.Name.Official)
		}
		nameZh := strings.TrimSpace(country.Translations.Zho.Common)
		if nameZh == "" {
			nameZh = strings.TrimSpace(country.Translations.Zho.Official)
		}
		// Fallback: some REST Countries recognizes Chinese under nativeName.zho instead of translations.zho
		if nameZh == "" {
			nameZh = strings.TrimSpace(country.Name.NativeName.Zho.Common)
		}
		if nameZh == "" {
			nameZh = strings.TrimSpace(country.Name.NativeName.Zho.Official)
		}
		name := nameZh
		if useEn || name == "" {
			name = nameEn
		}
		if name == "" {
			name = code
		}
		
		// 搜索过滤
		if search != "" {
			searchLower := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(code), searchLower) &&
				!strings.Contains(strings.ToLower(name), searchLower) &&
				!strings.Contains(strings.ToLower(nameEn), searchLower) &&
				!strings.Contains(strings.ToLower(nameZh), searchLower) {
				continue
			}
		}
		
		result = append(result, map[string]interface{}{
			"code":    code,
			"name":    name,
			"name_en": nameEn,
			"name_zh": nameZh,
		})
	}

	// 按名称排序
	sort.Slice(result, func(i, j int) bool {
		return result[i]["name"].(string) < result[j]["name"].(string)
	})

	// 分页
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 250)
	offset := (page - 1) * limit
	
	total := len(result)
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	
	var data []map[string]interface{}
	if start < end {
		data = result[start:end]
	}

	return c.JSON(fiber.Map{
		"data":  data,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetCountryRegions 根据国家代码获取地区列表
// 优先从数据库读取，如果没有则从 GeoNames API 获取并存储
func GetCountryRegions(c *fiber.Ctx) error {
	countryCode := strings.TrimSpace(c.Query("country_code", ""))
	countryCodes := strings.TrimSpace(c.Query("country_codes", ""))
	search := strings.TrimSpace(c.Query("search", ""))
	lang := strings.ToLower(strings.TrimSpace(c.Query("lang", "")))
	useEn := strings.HasPrefix(lang, "en")

	// fetchGeoNamesLang returns map[region_code]localized_name for a country (no DB writes)
	type regionTranslationsFile struct {
		Regions map[string]map[string]map[string]string `json:"regions"` // country -> region -> {en/zh/zh-CN}
	}
	var (
		rtOnce sync.Once
		rtData regionTranslationsFile
		rtErr  error
	)
	loadRegionTranslations := func() (regionTranslationsFile, error) {
		rtOnce.Do(func() {
			// Try cwd-relative first (works when running from nwork/)
			p := filepath.Join("data", "region_translations.json")
			b, err := os.ReadFile(p)
			if err != nil {
				// fallback: locate near executable
				if exe, e2 := os.Executable(); e2 == nil {
					p2 := filepath.Join(filepath.Dir(exe), "data", "region_translations.json")
					if b2, e3 := os.ReadFile(p2); e3 == nil {
						b = b2
						err = nil
					}
				}
			}
			if err != nil {
				rtErr = err
				return
			}
			if e := json.Unmarshal(b, &rtData); e != nil {
				rtErr = e
				return
			}
		})
		return rtData, rtErr
	}

	getRegionDisplay := func(country, regionCode, lang string) (nameZh string, nameZhCN string) {
		f, err := loadRegionTranslations()
		if err != nil {
			return "", ""
		}
		cc := strings.ToUpper(strings.TrimSpace(country))
		rc := strings.TrimSpace(regionCode)
		if cc == "" || rc == "" {
			return "", ""
		}
		m1, ok := f.Regions[cc]
		if !ok {
			return "", ""
		}
		m2, ok := m1[rc]
		if !ok {
			return "", ""
		}
		return strings.TrimSpace(m2["zh"]), strings.TrimSpace(m2["zh-CN"])
	}

	// 沒有 country 參數：返回空列表（給 select2 依賴初始化用）
	if countryCode == "" && countryCodes == "" {
		return c.JSON(fiber.Map{"data": []map[string]interface{}{}})
	}

	// 多國家：country_codes=US,CA...
	if countryCodes != "" {
		parts := strings.Split(countryCodes, ",")
		seen := map[string]bool{}
		result := make([]map[string]interface{}, 0)
		for _, p := range parts {
			cc := strings.ToUpper(strings.TrimSpace(p))
			if cc == "" {
				continue
			}
			regions, err := getRegionsForCountry(cc)
			if err != nil {
				continue
			}
			for _, r := range regions {
				rc := strings.TrimSpace(r.RegionCode)
				rn := strings.TrimSpace(r.RegionName)
				if rc == "" {
					continue
				}
				nameEn := rn
				zh, zhCN := getRegionDisplay(cc, rc, lang)
				nameZh := zh
				if strings.Contains(lang, "zh-cn") && zhCN != "" {
					nameZh = zhCN
				}
				name := nameEn
				if !useEn && nameZh != "" {
					name = nameZh
				}
				combined := cc + "-" + rc
				display := strings.TrimSpace(cc + " " + name)
				if display == "" {
					display = combined
				}
				if search != "" {
					s := strings.ToLower(search)
					if !strings.Contains(strings.ToLower(combined), s) && !strings.Contains(strings.ToLower(display), s) {
						continue
					}
				}
				if seen[combined] {
					continue
				}
				seen[combined] = true
				result = append(result, map[string]interface{}{
					"code":    combined,
					"name":    display,
					"name_en": strings.TrimSpace(cc + " " + nameEn),
					"name_zh": strings.TrimSpace(cc + " " + nameZh),
				})
			}
		}
		return c.JSON(fiber.Map{"data": result})
	}

	countryCodeUpper := strings.ToUpper(countryCode)

	// 首先尝试从数据库读取
	var regions []models.CountryRegion
	if err := database.DB.Where("country_code = ?", countryCodeUpper).Find(&regions).Error; err == nil && len(regions) > 0 {
		var result []map[string]interface{}
		for _, region := range regions {
			if search != "" {
				s := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(region.RegionCode), s) && !strings.Contains(strings.ToLower(region.RegionName), s) {
					continue
				}
			}
			rn := strings.TrimSpace(region.RegionName)
			rc := strings.TrimSpace(region.RegionCode)
			nameEn := rn
			zh, zhCN := getRegionDisplay(countryCodeUpper, rc, lang)
			nameZh := zh
			if strings.Contains(lang, "zh-cn") && zhCN != "" {
				nameZh = zhCN
			}
			name := nameEn
			if !useEn && nameZh != "" {
				name = nameZh
			}
			result = append(result, map[string]interface{}{
				"code":    rc,
				"name":    name,
				"name_en": nameEn,
				"name_zh": nameZh,
			})
		}
		return c.JSON(fiber.Map{
			"data": result,
		})
	}

	// 如果数据库中没有，从 GeoNames API 获取并存储
	regions, err := fetchAndStoreRegionsFromGeoNames(countryCodeUpper)
	if err != nil {
		log.Printf("⚠️  Failed to fetch regions from GeoNames for %s: %v", countryCodeUpper, err)
		// 如果 API 失败，回退到硬编码数据
	} else if len(regions) > 0 {
		// 成功获取并存储，返回数据
		var result []map[string]interface{}
		for _, region := range regions {
			if search != "" {
				s := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(region.RegionCode), s) && !strings.Contains(strings.ToLower(region.RegionName), s) {
					continue
				}
			}
			rn := strings.TrimSpace(region.RegionName)
			rc := strings.TrimSpace(region.RegionCode)
			nameEn := rn
			zh, zhCN := getRegionDisplay(countryCodeUpper, rc, lang)
			nameZh := zh
			if strings.Contains(lang, "zh-cn") && zhCN != "" {
				nameZh = zhCN
			}
			name := nameEn
			if !useEn && nameZh != "" {
				name = nameZh
			}
			result = append(result, map[string]interface{}{
				"code":    rc,
				"name":    name,
				"name_en": nameEn,
				"name_zh": nameZh,
			})
		}
		return c.JSON(fiber.Map{
			"data": result,
		})
	}

	// 主要国家/地区的硬编码数据（常用地区）
	regionsMap := map[string][]Region{
		"US": { // 美国州
			{Code: "AL", Name: "Alabama"}, {Code: "AK", Name: "Alaska"}, {Code: "AZ", Name: "Arizona"},
			{Code: "AR", Name: "Arkansas"}, {Code: "CA", Name: "California"}, {Code: "CO", Name: "Colorado"},
			{Code: "CT", Name: "Connecticut"}, {Code: "DE", Name: "Delaware"}, {Code: "FL", Name: "Florida"},
			{Code: "GA", Name: "Georgia"}, {Code: "HI", Name: "Hawaii"}, {Code: "ID", Name: "Idaho"},
			{Code: "IL", Name: "Illinois"}, {Code: "IN", Name: "Indiana"}, {Code: "IA", Name: "Iowa"},
			{Code: "KS", Name: "Kansas"}, {Code: "KY", Name: "Kentucky"}, {Code: "LA", Name: "Louisiana"},
			{Code: "ME", Name: "Maine"}, {Code: "MD", Name: "Maryland"}, {Code: "MA", Name: "Massachusetts"},
			{Code: "MI", Name: "Michigan"}, {Code: "MN", Name: "Minnesota"}, {Code: "MS", Name: "Mississippi"},
			{Code: "MO", Name: "Missouri"}, {Code: "MT", Name: "Montana"}, {Code: "NE", Name: "Nebraska"},
			{Code: "NV", Name: "Nevada"}, {Code: "NH", Name: "New Hampshire"}, {Code: "NJ", Name: "New Jersey"},
			{Code: "NM", Name: "New Mexico"}, {Code: "NY", Name: "New York"}, {Code: "NC", Name: "North Carolina"},
			{Code: "ND", Name: "North Dakota"}, {Code: "OH", Name: "Ohio"}, {Code: "OK", Name: "Oklahoma"},
			{Code: "OR", Name: "Oregon"}, {Code: "PA", Name: "Pennsylvania"}, {Code: "RI", Name: "Rhode Island"},
			{Code: "SC", Name: "South Carolina"}, {Code: "SD", Name: "South Dakota"}, {Code: "TN", Name: "Tennessee"},
			{Code: "TX", Name: "Texas"}, {Code: "UT", Name: "Utah"}, {Code: "VT", Name: "Vermont"},
			{Code: "VA", Name: "Virginia"}, {Code: "WA", Name: "Washington"}, {Code: "WV", Name: "West Virginia"},
			{Code: "WI", Name: "Wisconsin"}, {Code: "WY", Name: "Wyoming"}, {Code: "DC", Name: "District of Columbia"},
		},
		"CA": { // 加拿大省
			{Code: "AB", Name: "Alberta"}, {Code: "BC", Name: "British Columbia"}, {Code: "MB", Name: "Manitoba"},
			{Code: "NB", Name: "New Brunswick"}, {Code: "NL", Name: "Newfoundland and Labrador"},
			{Code: "NS", Name: "Nova Scotia"}, {Code: "NT", Name: "Northwest Territories"}, {Code: "NU", Name: "Nunavut"},
			{Code: "ON", Name: "Ontario"}, {Code: "PE", Name: "Prince Edward Island"}, {Code: "QC", Name: "Quebec"},
			{Code: "SK", Name: "Saskatchewan"}, {Code: "YT", Name: "Yukon"},
		},
		"CN": { // 中国省份
			{Code: "BJ", Name: "北京市"}, {Code: "TJ", Name: "天津市"}, {Code: "HE", Name: "河北省"},
			{Code: "SX", Name: "山西省"}, {Code: "NM", Name: "内蒙古自治区"}, {Code: "LN", Name: "辽宁省"},
			{Code: "JL", Name: "吉林省"}, {Code: "HL", Name: "黑龙江省"}, {Code: "SH", Name: "上海市"},
			{Code: "JS", Name: "江苏省"}, {Code: "ZJ", Name: "浙江省"}, {Code: "AH", Name: "安徽省"},
			{Code: "FJ", Name: "福建省"}, {Code: "JX", Name: "江西省"}, {Code: "SD", Name: "山东省"},
			{Code: "HA", Name: "河南省"}, {Code: "HB", Name: "湖北省"}, {Code: "HN", Name: "湖南省"},
			{Code: "GD", Name: "广东省"}, {Code: "GX", Name: "广西壮族自治区"}, {Code: "HI", Name: "海南省"},
			{Code: "CQ", Name: "重庆市"}, {Code: "SC", Name: "四川省"}, {Code: "GZ", Name: "贵州省"},
			{Code: "YN", Name: "云南省"}, {Code: "XZ", Name: "西藏自治区"}, {Code: "SN", Name: "陕西省"},
			{Code: "GS", Name: "甘肃省"}, {Code: "QH", Name: "青海省"}, {Code: "NX", Name: "宁夏回族自治区"},
			{Code: "XJ", Name: "新疆维吾尔自治区"}, {Code: "HK", Name: "香港特别行政区"}, {Code: "MO", Name: "澳门特别行政区"},
			{Code: "TW", Name: "台湾省"},
		},
		"GB": { // 英国
			{Code: "ENG", Name: "England"}, {Code: "SCT", Name: "Scotland"}, {Code: "WLS", Name: "Wales"},
			{Code: "NIR", Name: "Northern Ireland"},
		},
		"AU": { // 澳大利亚
			{Code: "NSW", Name: "New South Wales"}, {Code: "VIC", Name: "Victoria"}, {Code: "QLD", Name: "Queensland"},
			{Code: "WA", Name: "Western Australia"}, {Code: "SA", Name: "South Australia"}, {Code: "TAS", Name: "Tasmania"},
			{Code: "ACT", Name: "Australian Capital Territory"}, {Code: "NT", Name: "Northern Territory"},
		},
		"JP": { // 日本都道府县（主要）
			{Code: "01", Name: "北海道"}, {Code: "02", Name: "青森県"}, {Code: "03", Name: "岩手県"},
			{Code: "04", Name: "宮城県"}, {Code: "05", Name: "秋田県"}, {Code: "06", Name: "山形県"},
			{Code: "07", Name: "福島県"}, {Code: "08", Name: "茨城県"}, {Code: "09", Name: "栃木県"},
			{Code: "10", Name: "群馬県"}, {Code: "11", Name: "埼玉県"}, {Code: "12", Name: "千葉県"},
			{Code: "13", Name: "東京都"}, {Code: "14", Name: "神奈川県"}, {Code: "15", Name: "新潟県"},
			{Code: "16", Name: "富山県"}, {Code: "17", Name: "石川県"}, {Code: "18", Name: "福井県"},
			{Code: "19", Name: "山梨県"}, {Code: "20", Name: "長野県"}, {Code: "21", Name: "岐阜県"},
			{Code: "22", Name: "静岡県"}, {Code: "23", Name: "愛知県"}, {Code: "24", Name: "三重県"},
			{Code: "25", Name: "滋賀県"}, {Code: "26", Name: "京都府"}, {Code: "27", Name: "大阪府"},
			{Code: "28", Name: "兵庫県"}, {Code: "29", Name: "奈良県"}, {Code: "30", Name: "和歌山県"},
			{Code: "31", Name: "鳥取県"}, {Code: "32", Name: "島根県"}, {Code: "33", Name: "岡山県"},
			{Code: "34", Name: "広島県"}, {Code: "35", Name: "山口県"}, {Code: "36", Name: "徳島県"},
			{Code: "37", Name: "香川県"}, {Code: "38", Name: "愛媛県"}, {Code: "39", Name: "高知県"},
			{Code: "40", Name: "福岡県"}, {Code: "41", Name: "佐賀県"}, {Code: "42", Name: "長崎県"},
			{Code: "43", Name: "熊本県"}, {Code: "44", Name: "大分県"}, {Code: "45", Name: "宮崎県"},
			{Code: "46", Name: "鹿児島県"}, {Code: "47", Name: "沖縄県"},
		},
		"KR": { // 韩国
			{Code: "11", Name: "서울특별시"}, {Code: "26", Name: "부산광역시"}, {Code: "27", Name: "대구광역시"},
			{Code: "28", Name: "인천광역시"}, {Code: "29", Name: "광주광역시"}, {Code: "30", Name: "대전광역시"},
			{Code: "31", Name: "울산광역시"}, {Code: "41", Name: "경기도"}, {Code: "42", Name: "강원도"},
			{Code: "43", Name: "충청북도"}, {Code: "44", Name: "충청남도"}, {Code: "45", Name: "전라북도"},
			{Code: "46", Name: "전라남도"}, {Code: "47", Name: "경상북도"}, {Code: "48", Name: "경상남도"},
			{Code: "50", Name: "제주특별자치도"},
		},
		"TW": { // 台湾
			{Code: "TPE", Name: "台北市"}, {Code: "NTP", Name: "新北市"}, {Code: "TYC", Name: "桃園市"},
			{Code: "HSC", Name: "新竹市"}, {Code: "HSC", Name: "新竹縣"}, {Code: "MIA", Name: "苗栗縣"},
			{Code: "TXC", Name: "台中市"}, {Code: "CHA", Name: "彰化縣"}, {Code: "NAN", Name: "南投縣"},
			{Code: "YUN", Name: "雲林縣"}, {Code: "CHI", Name: "嘉義市"}, {Code: "CHI", Name: "嘉義縣"},
			{Code: "TNN", Name: "台南市"}, {Code: "KHH", Name: "高雄市"}, {Code: "PIF", Name: "屏東縣"},
			{Code: "ILA", Name: "宜蘭縣"}, {Code: "HUA", Name: "花蓮縣"}, {Code: "TTT", Name: "台東縣"},
			{Code: "PEN", Name: "澎湖縣"}, {Code: "KIN", Name: "金門縣"}, {Code: "LIE", Name: "連江縣"},
		},
		"HK": { // 香港
			{Code: "HK", Name: "香港島"}, {Code: "KL", Name: "九龍"}, {Code: "NT", Name: "新界"},
		},
		"SG": { // 新加坡
			{Code: "SG", Name: "Singapore"},
		},
		"MY": { // 马来西亚
			{Code: "JHR", Name: "Johor"}, {Code: "KDH", Name: "Kedah"}, {Code: "KTN", Name: "Kelantan"},
			{Code: "MLK", Name: "Melaka"}, {Code: "NSN", Name: "Negeri Sembilan"}, {Code: "PHG", Name: "Pahang"},
			{Code: "PRK", Name: "Perak"}, {Code: "PLS", Name: "Perlis"}, {Code: "PNG", Name: "Pulau Pinang"},
			{Code: "SBH", Name: "Sabah"}, {Code: "SWK", Name: "Sarawak"}, {Code: "SGR", Name: "Selangor"},
			{Code: "TRG", Name: "Terengganu"}, {Code: "KUL", Name: "Kuala Lumpur"}, {Code: "LBN", Name: "Labuan"},
			{Code: "PJY", Name: "Putrajaya"},
		},
		"TH": { // 泰国
			{Code: "10", Name: "กรุงเทพมหานคร"}, {Code: "11", Name: "สมุทรปราการ"}, {Code: "12", Name: "นนทบุรี"},
			{Code: "13", Name: "ปทุมธานี"}, {Code: "14", Name: "พระนครศรีอยุธยา"}, {Code: "15", Name: "อ่างทอง"},
			{Code: "16", Name: "ลพบุรี"}, {Code: "17", Name: "สิงห์บุรี"}, {Code: "18", Name: "ชัยนาท"},
			{Code: "19", Name: "สระบุรี"}, {Code: "20", Name: "ชลบุรี"}, {Code: "21", Name: " rayong"},
			{Code: "22", Name: "จันทบุรี"}, {Code: "23", Name: "ตราด"}, {Code: "24", Name: "ฉะเชิงเทรา"},
			{Code: "25", Name: "ปราจีนบุรี"}, {Code: "26", Name: "นครนายก"}, {Code: "27", Name: "สระแก้ว"},
			{Code: "30", Name: "นครราชสีมา"}, {Code: "31", Name: "บุรีรัมย์"}, {Code: "32", Name: "สุรินทร์"},
			{Code: "33", Name: "ศรีสะเกษ"}, {Code: "34", Name: "อุบลราชธานี"}, {Code: "35", Name: "ยโสธร"},
			{Code: "36", Name: "ชัยภูมิ"}, {Code: "37", Name: "อำนาจเจริญ"}, {Code: "38", Name: "บึงกาฬ"},
			{Code: "39", Name: "หนองบัวลำภู"}, {Code: "40", Name: "ขอนแก่น"}, {Code: "41", Name: "อุดรธานี"},
			{Code: "42", Name: "เลย"}, {Code: "43", Name: "หนองคาย"}, {Code: "44", Name: "มหาสารคาม"},
			{Code: "45", Name: "ร้อยเอ็ด"}, {Code: "46", Name: "กาฬสินธุ์"}, {Code: "47", Name: "สกลนคร"},
			{Code: "48", Name: "นครพนม"}, {Code: "49", Name: "มุกดาหาร"}, {Code: "50", Name: "เชียงใหม่"},
			{Code: "51", Name: "ลำปาง"}, {Code: "52", Name: "ลำพูน"}, {Code: "53", Name: "อุตรดิตถ์"},
			{Code: "54", Name: "แพร่"}, {Code: "55", Name: "น่าน"}, {Code: "56", Name: "พะเยา"},
			{Code: "57", Name: "เชียงราย"}, {Code: "58", Name: "แม่ฮ่องสอน"}, {Code: "60", Name: "นครสวรรค์"},
			{Code: "61", Name: "อุทัยธานี"}, {Code: "62", Name: "กำแพงเพชร"}, {Code: "63", Name: "ตาก"},
			{Code: "64", Name: "สุโขทัย"}, {Code: "65", Name: "พิษณุโลก"}, {Code: "66", Name: "พิจิตร"},
			{Code: "67", Name: "เพชรบูรณ์"}, {Code: "70", Name: "ราชบุรี"}, {Code: "71", Name: "กาญจนบุรี"},
			{Code: "72", Name: "สุพรรณบุรี"}, {Code: "73", Name: "นครปฐม"}, {Code: "74", Name: "สมุทรสาคร"},
			{Code: "75", Name: "สมุทรสงคราม"}, {Code: "76", Name: "เพชรบุรี"}, {Code: "77", Name: "ประจวบคีรีขันธ์"},
			{Code: "80", Name: "นครศรีธรรมราช"}, {Code: "81", Name: "กระบี่"}, {Code: "82", Name: "พังงา"},
			{Code: "83", Name: "ภูเก็ต"}, {Code: "84", Name: "สุราษฎร์ธานี"}, {Code: "85", Name: "ระนอง"},
			{Code: "86", Name: "ชุมพร"}, {Code: "90", Name: "สงขลา"}, {Code: "91", Name: "สตูล"},
			{Code: "92", Name: "ตรัง"}, {Code: "93", Name: "พัทลุง"}, {Code: "94", Name: "ปัตตานี"},
			{Code: "95", Name: "ยะลา"}, {Code: "96", Name: "นราธิวาส"},
		},
		"IN": { // 印度（主要邦）
			{Code: "AP", Name: "Andhra Pradesh"}, {Code: "AR", Name: "Arunachal Pradesh"}, {Code: "AS", Name: "Assam"},
			{Code: "BR", Name: "Bihar"}, {Code: "CT", Name: "Chhattisgarh"}, {Code: "GA", Name: "Goa"},
			{Code: "GJ", Name: "Gujarat"}, {Code: "HR", Name: "Haryana"}, {Code: "HP", Name: "Himachal Pradesh"},
			{Code: "JK", Name: "Jammu and Kashmir"}, {Code: "JH", Name: "Jharkhand"}, {Code: "KA", Name: "Karnataka"},
			{Code: "KL", Name: "Kerala"}, {Code: "MP", Name: "Madhya Pradesh"}, {Code: "MH", Name: "Maharashtra"},
			{Code: "MN", Name: "Manipur"}, {Code: "ML", Name: "Meghalaya"}, {Code: "MZ", Name: "Mizoram"},
			{Code: "NL", Name: "Nagaland"}, {Code: "OR", Name: "Odisha"}, {Code: "PB", Name: "Punjab"},
			{Code: "RJ", Name: "Rajasthan"}, {Code: "SK", Name: "Sikkim"}, {Code: "TN", Name: "Tamil Nadu"},
			{Code: "TG", Name: "Telangana"}, {Code: "TR", Name: "Tripura"}, {Code: "UP", Name: "Uttar Pradesh"},
			{Code: "UT", Name: "Uttarakhand"}, {Code: "WB", Name: "West Bengal"}, {Code: "AN", Name: "Andaman and Nicobar Islands"},
			{Code: "CH", Name: "Chandigarh"}, {Code: "DH", Name: "Dadra and Nagar Haveli"}, {Code: "DD", Name: "Daman and Diu"},
			{Code: "DL", Name: "Delhi"}, {Code: "LD", Name: "Lakshadweep"}, {Code: "PY", Name: "Puducherry"},
		},
		"DE": { // 德国
			{Code: "BW", Name: "Baden-Württemberg"}, {Code: "BY", Name: "Bayern"}, {Code: "BE", Name: "Berlin"},
			{Code: "BB", Name: "Brandenburg"}, {Code: "HB", Name: "Bremen"}, {Code: "HH", Name: "Hamburg"},
			{Code: "HE", Name: "Hessen"}, {Code: "MV", Name: "Mecklenburg-Vorpommern"}, {Code: "NI", Name: "Niedersachsen"},
			{Code: "NW", Name: "Nordrhein-Westfalen"}, {Code: "RP", Name: "Rheinland-Pfalz"}, {Code: "SL", Name: "Saarland"},
			{Code: "SN", Name: "Sachsen"}, {Code: "ST", Name: "Sachsen-Anhalt"}, {Code: "SH", Name: "Schleswig-Holstein"},
			{Code: "TH", Name: "Thüringen"},
		},
		"FR": { // 法国
			{Code: "ARA", Name: "Auvergne-Rhône-Alpes"}, {Code: "BFC", Name: "Bourgogne-Franche-Comté"},
			{Code: "BRE", Name: "Bretagne"}, {Code: "CVL", Name: "Centre-Val de Loire"}, {Code: "COR", Name: "Corse"},
			{Code: "GES", Name: "Grand Est"}, {Code: "HDF", Name: "Hauts-de-France"}, {Code: "IDF", Name: "Île-de-France"},
			{Code: "NOR", Name: "Normandie"}, {Code: "NAQ", Name: "Nouvelle-Aquitaine"}, {Code: "OCC", Name: "Occitanie"},
			{Code: "PDL", Name: "Pays de la Loire"}, {Code: "PAC", Name: "Provence-Alpes-Côte d'Azur"},
		},
		"IT": { // 意大利
			{Code: "ABR", Name: "Abruzzo"}, {Code: "BAS", Name: "Basilicata"}, {Code: "CAL", Name: "Calabria"},
			{Code: "CAM", Name: "Campania"}, {Code: "EMR", Name: "Emilia-Romagna"}, {Code: "FVG", Name: "Friuli-Venezia Giulia"},
			{Code: "LAZ", Name: "Lazio"}, {Code: "LIG", Name: "Liguria"}, {Code: "LOM", Name: "Lombardia"},
			{Code: "MAR", Name: "Marche"}, {Code: "MOL", Name: "Molise"}, {Code: "PAB", Name: "Piemonte"},
			{Code: "PUG", Name: "Puglia"}, {Code: "SAR", Name: "Sardegna"}, {Code: "SIC", Name: "Sicilia"},
			{Code: "TOS", Name: "Toscana"}, {Code: "TAA", Name: "Trentino-Alto Adige"}, {Code: "UMB", Name: "Umbria"},
			{Code: "VAO", Name: "Valle d'Aosta"}, {Code: "VEN", Name: "Veneto"},
		},
		"ES": { // 西班牙
			{Code: "AN", Name: "Andalucía"}, {Code: "AR", Name: "Aragón"}, {Code: "AS", Name: "Asturias"},
			{Code: "CB", Name: "Cantabria"}, {Code: "CL", Name: "Castilla y León"}, {Code: "CM", Name: "Castilla-La Mancha"},
			{Code: "CT", Name: "Cataluña"}, {Code: "EX", Name: "Extremadura"}, {Code: "GA", Name: "Galicia"},
			{Code: "IB", Name: "Islas Baleares"}, {Code: "CN", Name: "Canarias"}, {Code: "RI", Name: "La Rioja"},
			{Code: "MD", Name: "Madrid"}, {Code: "MC", Name: "Murcia"}, {Code: "NC", Name: "Navarra"},
			{Code: "PV", Name: "País Vasco"}, {Code: "VC", Name: "Valencia"},
		},
		"BR": { // 巴西
			{Code: "AC", Name: "Acre"}, {Code: "AL", Name: "Alagoas"}, {Code: "AP", Name: "Amapá"},
			{Code: "AM", Name: "Amazonas"}, {Code: "BA", Name: "Bahia"}, {Code: "CE", Name: "Ceará"},
			{Code: "DF", Name: "Distrito Federal"}, {Code: "ES", Name: "Espírito Santo"}, {Code: "GO", Name: "Goiás"},
			{Code: "MA", Name: "Maranhão"}, {Code: "MT", Name: "Mato Grosso"}, {Code: "MS", Name: "Mato Grosso do Sul"},
			{Code: "MG", Name: "Minas Gerais"}, {Code: "PA", Name: "Pará"}, {Code: "PB", Name: "Paraíba"},
			{Code: "PR", Name: "Paraná"}, {Code: "PE", Name: "Pernambuco"}, {Code: "PI", Name: "Piauí"},
			{Code: "RJ", Name: "Rio de Janeiro"}, {Code: "RN", Name: "Rio Grande do Norte"}, {Code: "RS", Name: "Rio Grande do Sul"},
			{Code: "RO", Name: "Rondônia"}, {Code: "RR", Name: "Roraima"}, {Code: "SC", Name: "Santa Catarina"},
			{Code: "SP", Name: "São Paulo"}, {Code: "SE", Name: "Sergipe"}, {Code: "TO", Name: "Tocantins"},
		},
		"MX": { // 墨西哥
			{Code: "AGU", Name: "Aguascalientes"}, {Code: "BCN", Name: "Baja California"}, {Code: "BCS", Name: "Baja California Sur"},
			{Code: "CAM", Name: "Campeche"}, {Code: "CHP", Name: "Chiapas"}, {Code: "CHH", Name: "Chihuahua"},
			{Code: "COA", Name: "Coahuila"}, {Code: "COL", Name: "Colima"}, {Code: "DIF", Name: "Ciudad de México"},
			{Code: "DUR", Name: "Durango"}, {Code: "GUA", Name: "Guanajuato"}, {Code: "GRO", Name: "Guerrero"},
			{Code: "HID", Name: "Hidalgo"}, {Code: "JAL", Name: "Jalisco"}, {Code: "MEX", Name: "México"},
			{Code: "MIC", Name: "Michoacán"}, {Code: "MOR", Name: "Morelos"}, {Code: "NAY", Name: "Nayarit"},
			{Code: "NLE", Name: "Nuevo León"}, {Code: "OAX", Name: "Oaxaca"}, {Code: "PUE", Name: "Puebla"},
			{Code: "QUE", Name: "Querétaro"}, {Code: "ROO", Name: "Quintana Roo"}, {Code: "SLP", Name: "San Luis Potosí"},
			{Code: "SIN", Name: "Sinaloa"}, {Code: "SON", Name: "Sonora"}, {Code: "TAB", Name: "Tabasco"},
			{Code: "TAM", Name: "Tamaulipas"}, {Code: "TLA", Name: "Tlaxcala"}, {Code: "VER", Name: "Veracruz"},
			{Code: "YUC", Name: "Yucatán"}, {Code: "ZAC", Name: "Zacatecas"},
		},
	}

	// 如果 GeoNames API 也失败，使用硬编码数据作为最后备用
	hardcodedRegions, exists := regionsMap[countryCodeUpper]
	if !exists {
		// 如果国家没有预定义地区，返回空数组
		return c.JSON(fiber.Map{
			"data": []interface{}{},
		})
	}

	// 转换为 API 格式
	var result []map[string]interface{}
	for _, region := range hardcodedRegions {
		if search != "" {
			s := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(region.Code), s) && !strings.Contains(strings.ToLower(region.Name), s) {
				continue
			}
		}
		result = append(result, map[string]interface{}{
			"code": region.Code,
			"name": region.Name,
		})
	}

	return c.JSON(fiber.Map{
		"data": result,
	})
}

// getRegionsForCountry returns ADM1 regions for a country code.
// Best-effort: DB -> GeoNames (and store) -> error.
func getRegionsForCountry(countryCodeUpper string) ([]models.CountryRegion, error) {
	countryCodeUpper = strings.ToUpper(strings.TrimSpace(countryCodeUpper))
	if countryCodeUpper == "" {
		return nil, fmt.Errorf("empty country code")
	}

	// DB first
	var regions []models.CountryRegion
	if err := database.DB.Where("country_code = ?", countryCodeUpper).Find(&regions).Error; err == nil && len(regions) > 0 {
		return regions, nil
	}

	// GeoNames
	regions, err := fetchAndStoreRegionsFromGeoNames(countryCodeUpper)
	if err == nil && len(regions) > 0 {
		return regions, nil
	}

	return nil, fmt.Errorf("no regions found for country %s", countryCodeUpper)
}

// fetchAndStoreRegionsFromGeoNames 从 GeoNames API 获取地区数据并存储到数据库
func fetchAndStoreRegionsFromGeoNames(countryCode string) ([]models.CountryRegion, error) {
	// GeoNames API 文档: http://www.geonames.org/export/web-services.html
	// 从环境变量读取用户名，如果没有则使用 demo（有限制）
	username := os.Getenv("GEONAMES_USERNAME")
	if username == "" {
		username = "demo"
	}

	type geoItem struct {
		AdminCode1 string `json:"adminCode1"`
		Name       string `json:"name"`
	}
	type geoResp struct {
		Geonames []geoItem `json:"geonames"`
		Status   struct {
			Message string `json:"message"`
			Value   int    `json:"value"`
		} `json:"status"`
	}

	fetchLang := func(lang string) (map[string]string, error) {
		langParam := ""
		if strings.TrimSpace(lang) != "" {
			langParam = "&lang=" + strings.TrimSpace(lang)
		}
		geoNamesURL := fmt.Sprintf(
			"http://api.geonames.org/searchJSON?country=%s&featureClass=A&featureCode=ADM1&maxRows=1000&username=%s%s",
			countryCode, username, langParam,
		)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(geoNamesURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("GeoNames API returned status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		var data geoResp
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, err
		}
		if data.Status.Value > 0 {
			return nil, fmt.Errorf("GeoNames API error: %s", data.Status.Message)
		}
		if len(data.Geonames) == 0 {
			return nil, fmt.Errorf("no regions found for country %s", countryCode)
		}
		m := make(map[string]string)
		for _, it := range data.Geonames {
			code := strings.TrimSpace(it.AdminCode1)
			name := strings.TrimSpace(it.Name)
			if code == "" {
				code = name
			}
			if code == "" {
				continue
			}
			if name == "" {
				name = code
			}
			// 去重：同 code 以第一次為準
			if _, ok := m[code]; !ok {
				m[code] = name
			}
		}
		return m, nil
	}

	// Only fetch EN for DB storage (translations are NOT persisted)
	enMap, errEn := fetchLang("en")
	if errEn != nil {
		return nil, errEn
	}
	if enMap == nil {
		enMap = map[string]string{}
	}

	var regions []models.CountryRegion
	for code, name := range enMap {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		enName := strings.TrimSpace(name)
		if enName == "" {
			enName = code
		}

		region := models.CountryRegion{
			CountryCode: countryCode,
			RegionCode:  code,
			RegionName:  enName,
		}

		// FirstOrCreate then ensure region_name is English (best-effort)
		if err := database.DB.Where("country_code = ? AND region_code = ?", countryCode, code).
			FirstOrCreate(&region, models.CountryRegion{
				CountryCode: countryCode,
				RegionCode:  code,
				RegionName:  enName,
			}).Error; err != nil {
			log.Printf("⚠️  Failed to store region %s/%s: %v", countryCode, code, err)
			continue
		}

		if err := database.DB.Model(&models.CountryRegion{}).
			Where("country_code = ? AND region_code = ?", countryCode, code).
			Update("region_name", enName).Error; err != nil {
			log.Printf("⚠️  Failed to update region_name %s/%s: %v", countryCode, code, err)
		}

		region.RegionName = enName

		regions = append(regions, region)
	}

	return regions, nil
}


