package config

import (
	"fmt"
	"os"
)

type Config struct {
	Database                    DatabaseConfig
	Server                      ServerConfig
	JWT                         JWTConfig
	LLM                         LLMConfig
	Stripe                      StripeConfig
	QFPay                       QFPayConfig
	GoogleMapsAPIKey            string
	HardwarePurchaseCatalogFile string
	CompanyName                 string
	AppName                     string
	TrialDays                   int
	DataRetentionDays           int // 數據保留天數（默認 90 天）
	WhatsApp                    WhatsAppConfig
	Domain                      DomainConfig
	Email                       EmailConfig
	Upload                      UploadConfig
	Vision                      VisionConfig
	Speech                      SpeechConfig
	GoogleOAuth                 GoogleOAuthConfig
	GoogleAdSensePublisherID    string // Google AdSense publisher ID (e.g. ca-pub-XXXXXXXX)
	GitHub                      GitHubConfig
	IAP                         IAPConfig
	GoogleSearch                GoogleSearchConfig
	Serper                      SerperConfig
	Ark                         ArkConfig
	Veo                         VeoConfig   // DEPRECATED — replaced by Kling
	Kling                       KlingConfig // Kling 3.0 Omni video generation
	DID                         DIDConfig   // DEPRECATED — Kling has native audio
	TTS                         TTSConfig
	Lyria                       LyriaConfig
}

// SerperConfig holds Serper.dev API credentials for Lead Finder web search
type SerperConfig struct {
	APIKey string // Serper.dev API Key
}

// GoogleSearchConfig holds Google Custom Search API credentials (deprecated, use Serper)
type GoogleSearchConfig struct {
	APIKey         string // Google Custom Search API Key
	SearchEngineID string // Programmable Search Engine ID (cx)
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type ServerConfig struct {
	Port string
	Host string
}

type JWTConfig struct {
	Secret string
}

type LLMConfig struct {
	Endpoint   string
	APIKey     string
	Model      string
	ImageModel string
	VideoModel string
	Provider   string // "openai" or "gemini"
}

// ArkConfig holds BytePlus ModelArk (Seedance) video generation credentials (DEPRECATED — use VeoConfig)
type ArkConfig struct {
	APIKey     string // BytePlus ModelArk API Key (Bearer token)
	EndpointID string // Seedance 1.5 Pro endpoint ID (e.g. "ep-xxx" or model name)
}

// VeoConfig holds Google Veo 3.1 video generation credentials (DEPRECATED — replaced by Kling)
type VeoConfig struct {
	ProjectID  string // Google Cloud project ID (e.g. "my-project-123")
	Location   string // Vertex AI location (e.g. "us-central1")
	APIKey     string // Google API Key (alternative to OAuth; if set, used as ?key= param)
	Model      string // Model name (e.g. "veo-3.1-fast-generate-preview")
	Resolution string // Default resolution: "720p", "1080p", "4k"
	Duration   int    // Default duration in seconds: 4, 6, or 8
}

// KlingConfig holds Kling Omni video generation credentials
type KlingConfig struct {
	AccessKey string // Kling API access key (JWT issuer)
	SecretKey string // Kling API secret key (JWT signing key, HS256)
	Model     string // Model name (default: "kling-v3-omni")
	BaseURL   string // API base URL (default: "https://api-singapore.klingai.com")
}

// DIDConfig holds D-ID V4 Expressive Avatars API credentials (lip-sync)
type DIDConfig struct {
	APIKey  string // D-ID API Key (used as Basic auth: base64(":<key>"))
	BaseURL string // D-ID API base URL (default "https://api.d-id.com")
}

// LyriaConfig holds Google Lyria music generation credentials
// Uses the Gemini v1alpha bidiGenerateMusic WebSocket endpoint
type LyriaConfig struct {
	APIKey string // Google API Key (falls back to LLM.APIKey)
	Model  string // Model name (default "lyria-realtime-exp")
}

// TTSVoiceMapping maps a BCP-47 locale to a default Google Cloud TTS voice name and tier
type TTSVoiceMapping struct {
	VoiceName string // e.g. "yue-HK-Chirp3-HD-Achernar"
	Tier      string // "Chirp3-HD", "Neural2", "Standard"
}

// TTSConfig holds Google Cloud TTS configuration
type TTSConfig struct {
	APIKey        string                     // Google Cloud TTS API Key (falls back to Speech.APIKey)
	DefaultLocale string                     // Default locale when detection fails (e.g. "zh-HK")
	VoiceMap      map[string]TTSVoiceMapping // locale → default voice (populated in Load())
}

type StripeConfig struct {
	SecretKey           string
	PublishableKey      string
	WebhookSecret       string
	PriceMonthly        string // vSuite monthly
	PriceYearly         string // vSuite yearly
	PriceMonthlyPro     string // vSuite Pro monthly
	PriceYearlyPro      string // vSuite Pro yearly
	PriceMonthlyProPlus string // vSuite Pro+ monthly
	PriceYearlyProPlus  string // vSuite Pro+ yearly
	PartnerProductID    string
	SuccessURL          string
	CancelURL           string
	// Stripe Connect
	ConnectApplicationFeePercent float64 // 平台抽成百分比（例如 2.0 = 2%）
}

type WhatsAppConfig struct {
	PhoneNumber       string // 顯示用的電話號碼
	APIToken          string // WhatsApp API 訪問令牌
	PhoneNumberID     string // WhatsApp 電話號碼 ID
	AppID             string // Meta 應用 ID
	AppSecret         string // Meta 應用密鑰
	VerifyToken       string // Webhook 驗證令牌
	APIVersion        string // API 版本（例如: v21.0）
	BusinessAccountID string // WhatsApp Business 帳戶 ID
	Enabled           bool   // 是否啟用 WhatsApp API
}

type DomainConfig struct {
	BaseDomain string // 例如: "vworkai.com"
	Scheme     string // 例如: "https" or "http"
}

type EmailConfig struct {
	SMTPHost              string
	SMTPPort              string
	SMTPUser              string
	SMTPPassword          string
	FromEmail             string
	FromName              string
	ContactEmail          string // 接收聯絡表單的 email（可在 .env 中配置）
	AdminEmails           string // 接收管理通知的 emails（逗號分隔，可在 .env 中配置 ADMIN_EMAILS）
	BrevoFreeDailyLimit   int    // Brevo 免費每日發送上限（預設 300）
	UseStartTLS           bool
	InsecureSkipVerifyTLS bool
	ConnectTimeoutSeconds int
	SendTimeoutSeconds    int
}

type UploadConfig struct {
	MaxResolution int // 最大分辨率（像素），默認 1000
	MaxFileSize   int // 最大文件大小（字節），默認 2GB
}

type VisionConfig struct {
	APIKey    string
	ProjectID string // 可選
}

type SpeechConfig struct {
	APIKey    string
	ProjectID string // 可選
}

type GoogleOAuthConfig struct {
	ClientID     string // Google OAuth 2.0 客戶端 ID
	ClientSecret string // Google OAuth 2.0 客戶端密鑰
	Enabled      bool   // 是否啟用 Google OAuth 登錄
}

type GitHubConfig struct {
	Token      string
	Owner      string
	Repo       string
	WorkflowID string
}

// IAPConfig In-App Purchase 配置（Google Play + Apple App Store）
type IAPConfig struct {
	GoogleServiceAccountJSON string // Google Play service account JSON (for server-side verification)
	GooglePackageName        string // Android package name (e.g. "com.vsys.vai")
	AppleSharedSecret        string // App Store shared secret (legacy, kept for compatibility)
	AppleBundleID            string // iOS bundle ID (e.g. "com.vsys.vai")
	AppleIssuerID            string // App Store Connect API Issuer ID
	AppleKeyID               string // App Store Connect API Key ID
	ApplePrivateKey          string // App Store Connect API private key (PEM)
	AppleEnvironment         string // "Production" or "Sandbox"
}

// QFPayConfig QFPay 聚合支付配置（FPS、PayMe、Alipay HK、WeChat Pay HK 等）
type QFPayConfig struct {
	AppCode   string // QFPay App Code
	ClientKey string // QFPay Client Key (用於簽名)
	BaseURL   string // QFPay API Base URL (e.g. "https://openapi-hk.qfapi.com")
	NotifyURL string // Webhook 通知 URL
	Enabled   bool   // 是否啟用 QFPay（平台級別）
}

func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "u-nai"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "3001"),
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		},
		LLM: LLMConfig{
			Endpoint:   getEnv("LLM_ENDPOINT", ""),
			APIKey:     getEnv("LLM_API_KEY", "AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"), // Gemini API Key
			Model:      getEnv("LLM_MODEL", "gemini-2.5-flash"),                          // 使用 gemini-2.5-flash (最新快速模型)
			ImageModel: getEnv("LLM_IMAGE_MODEL", "gemini-3-pro-image-preview"),          // Gemini best-quality image generation model (highest quality, preview)
			VideoModel: getEnv("LLM_VIDEO_MODEL", "veo-3.1-fast-generate-preview"),       // Google Veo 3.1（影片生成，取代 Seedance）
			Provider:   getEnv("LLM_PROVIDER", "gemini"),                                 // "openai" or "gemini"
		},
		Stripe: StripeConfig{
			SecretKey:                    getEnv("STRIPE_SECRET_KEY", ""),
			PublishableKey:               getEnv("STRIPE_PUBLISHABLE_KEY", ""),
			WebhookSecret:                getEnv("STRIPE_WEBHOOK_SECRET", ""),
			PriceMonthly:                 getEnv("STRIPE_PRICE_MONTHLY", ""),
			PriceYearly:                  getEnv("STRIPE_PRICE_YEARLY", ""),
			PriceMonthlyPro:              getEnv("STRIPE_PRICE_MONTHLY_PRO", ""),
			PriceYearlyPro:               getEnv("STRIPE_PRICE_YEARLY_PRO", ""),
			PriceMonthlyProPlus:          getEnv("STRIPE_PRICE_MONTHLY_PRO_PLUS", ""),
			PriceYearlyProPlus:           getEnv("STRIPE_PRICE_YEARLY_PRO_PLUS", ""),
			PartnerProductID:             getEnv("STRIPE_PARTNER_PRODUCT_ID", ""),
			SuccessURL:                   getEnv("STRIPE_SUCCESS_URL", ""),
			CancelURL:                    getEnv("STRIPE_CANCEL_URL", ""),
			ConnectApplicationFeePercent: getEnvFloat("STRIPE_CONNECT_APPLICATION_FEE_PERCENT", 2.0),
		},
		GoogleMapsAPIKey:            getEnv("GOOGLE_MAPS_API_KEY", ""),
		HardwarePurchaseCatalogFile: getEnv("HARDWARE_PURCHASE_CATALOG_FILE", "config/hardware_purchase_catalog.json"),
		CompanyName:                 getEnv("COMPANY_NAME", "V-sys Limited"),
		AppName:                     getEnv("APP_NAME", "vWork"),
		TrialDays:                   0,                                    // 默認試用天數（0 表示無日數上限）
		DataRetentionDays:           getEnvInt("DATA_RETENTION_DAYS", 90), // 默認數據保留 90 天
		WhatsApp: WhatsAppConfig{
			PhoneNumber:       getEnv("WHATSAPP_PHONE", "85246237234"),
			APIToken:          getEnv("WHATSAPP_API_TOKEN", ""),
			PhoneNumberID:     getEnv("WHATSAPP_PHONE_NUMBER_ID", ""),
			AppID:             getEnv("WHATSAPP_APP_ID", ""),
			AppSecret:         getEnv("WHATSAPP_APP_SECRET", ""),
			VerifyToken:       getEnv("WHATSAPP_VERIFY_TOKEN", ""),
			APIVersion:        getEnv("WHATSAPP_API_VERSION", "v21.0"),
			BusinessAccountID: getEnv("WHATSAPP_BUSINESS_ACCOUNT_ID", ""),
			Enabled:           getEnv("WHATSAPP_API_ENABLED", "false") == "true",
		},
		Domain: DomainConfig{
			BaseDomain: getEnv("BASE_DOMAIN", "vworkai.com"),
			Scheme:     getEnv("PUBLIC_SCHEME", "https"),
		},
		Email: EmailConfig{
			SMTPHost:              getEnv("SMTP_HOST", ""),
			SMTPPort:              getEnv("SMTP_PORT", "587"),
			SMTPUser:              getEnv("SMTP_USER", ""),
			SMTPPassword:          getEnv("SMTP_PASSWORD", ""),
			FromEmail:             getEnv("SMTP_FROM_EMAIL", ""),
			FromName:              getEnv("SMTP_FROM_NAME", "vWork"),
			ContactEmail:          getEnv("CONTACT_EMAIL", ""),              // 接收聯絡表單的 email（可在 .env 中配置 CONTACT_EMAIL）
			AdminEmails:           getEnv("ADMIN_EMAILS", ""),               // 接收管理通知的 emails（逗號分隔）
			BrevoFreeDailyLimit:   getEnvInt("BREVO_FREE_DAILY_LIMIT", 300), // Brevo 免費每日發送上限
			UseStartTLS:           getEnv("SMTP_USE_STARTTLS", "true") == "true",
			InsecureSkipVerifyTLS: getEnv("SMTP_INSECURE_SKIP_VERIFY_TLS", "false") == "true",
			ConnectTimeoutSeconds: getEnvInt("SMTP_CONNECT_TIMEOUT_SECONDS", 10),
			SendTimeoutSeconds:    getEnvInt("SMTP_SEND_TIMEOUT_SECONDS", 20),
		},
		Upload: UploadConfig{
			MaxResolution: getEnvInt("UPLOAD_MAX_RESOLUTION", 1000),
			MaxFileSize:   getEnvInt("UPLOAD_MAX_FILE_SIZE", 2*1024*1024*1024), // Default 2GB (vOffice installers can be 1-2GB)
		},
		Vision: VisionConfig{
			APIKey:    getEnv("GOOGLE_VISION_API_KEY", getEnv("GOOGLE_CLOUD_API_KEY", "")), // 如果沒有單獨配置，嘗試使用通用 API key
			ProjectID: getEnv("GOOGLE_CLOUD_PROJECT_ID", ""),
		},
		Speech: SpeechConfig{
			APIKey:    getEnv("GOOGLE_SPEECH_API_KEY", getEnv("GOOGLE_CLOUD_API_KEY", "")), // 如果沒有單獨配置，嘗試使用通用 API key
			ProjectID: getEnv("GOOGLE_CLOUD_PROJECT_ID", ""),
		},
		GoogleOAuth: GoogleOAuthConfig{
			ClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
			Enabled:      getEnv("GOOGLE_OAUTH_ENABLED", "false") == "true",
		},
		GoogleAdSensePublisherID: getEnv("GOOGLE_ADSENSE_PUBLISHER_ID", ""),
		GitHub: GitHubConfig{
			Token:      getEnv("GITHUB_TOKEN", ""),
			Owner:      getEnv("GITHUB_OWNER", ""),
			Repo:       getEnv("GITHUB_REPO", ""),
			WorkflowID: getEnv("GITHUB_WORKFLOW_ID", "build_app.yml"),
		},
		QFPay: QFPayConfig{
			AppCode:   getEnv("QFPAY_APP_CODE", ""),
			ClientKey: getEnv("QFPAY_CLIENT_KEY", ""),
			BaseURL:   getEnv("QFPAY_BASE_URL", "https://openapi-hk.qfapi.com"),
			NotifyURL: getEnv("QFPAY_NOTIFY_URL", ""),
			Enabled:   getEnv("QFPAY_ENABLED", "false") == "true",
		},
		IAP: IAPConfig{
			GoogleServiceAccountJSON: getEnv("IAP_GOOGLE_SERVICE_ACCOUNT_JSON", ""),
			GooglePackageName:        getEnv("IAP_GOOGLE_PACKAGE_NAME", "com.vsys.vai"),
			AppleSharedSecret:        getEnv("IAP_APPLE_SHARED_SECRET", ""),
			AppleBundleID:            getEnv("IAP_APPLE_BUNDLE_ID", "com.vsys.vai"),
			AppleIssuerID:            getEnv("IAP_APPLE_ISSUER_ID", ""),
			AppleKeyID:               getEnv("IAP_APPLE_KEY_ID", ""),
			ApplePrivateKey:          getEnv("IAP_APPLE_PRIVATE_KEY", ""),
			AppleEnvironment:         getEnv("IAP_APPLE_ENVIRONMENT", "Production"),
		},
		GoogleSearch: GoogleSearchConfig{
			APIKey:         getEnv("GOOGLE_SEARCH_API_KEY", ""),
			SearchEngineID: getEnv("GOOGLE_SEARCH_ENGINE_ID", ""),
		},
		Serper: SerperConfig{
			APIKey: getEnv("SERPER_API_KEY", ""),
		},
		Ark: ArkConfig{
			APIKey:     getEnv("ARK_API_KEY", ""),
			EndpointID: getEnv("ARK_ENDPOINT_ID", ""),
		},
		Veo: VeoConfig{
			ProjectID:  getEnv("VEO_PROJECT_ID", getEnv("GOOGLE_CLOUD_PROJECT_ID", "")),
			Location:   getEnv("VEO_LOCATION", "us-central1"),
			APIKey:     getEnv("VEO_API_KEY", getEnv("LLM_API_KEY", "")), // fallback to Gemini API key
			Model:      getEnv("VEO_MODEL", "veo-3.1-fast-generate-preview"),
			Resolution: getEnv("VEO_RESOLUTION", "720p"),
			Duration:   getEnvInt("VEO_DURATION", 8),
		},
		Kling: KlingConfig{
			AccessKey: getEnv("KLING_ACCESS_KEY", ""),
			SecretKey: getEnv("KLING_SECRET_KEY", ""),
			Model:     getEnv("KLING_MODEL", "kling-v3-omni"),
			BaseURL:   getEnv("KLING_BASE_URL", "https://api-singapore.klingai.com"),
		},
		DID: DIDConfig{
			APIKey:  getEnv("DID_API_KEY", ""),
			BaseURL: getEnv("DID_BASE_URL", "https://api.d-id.com"),
		},
		TTS: TTSConfig{
			APIKey:        getEnv("TTS_API_KEY", getEnv("GOOGLE_SPEECH_API_KEY", getEnv("GOOGLE_CLOUD_API_KEY", ""))),
			DefaultLocale: getEnv("TTS_DEFAULT_LOCALE", "yue-HK"),
			VoiceMap:      buildDefaultVoiceMap(),
		},
		Lyria: LyriaConfig{
			APIKey: getEnv("LYRIA_API_KEY", getEnv("LLM_API_KEY", "")), // fallback to Gemini API key
			Model:  getEnv("LYRIA_MODEL", "lyria-realtime-exp"),
		},
	}
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var result float64
		if _, err := fmt.Sscanf(value, "%f", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

// buildDefaultVoiceMap returns the default locale → TTS voice mapping.
// Uses Chirp3-HD (premium) where available, Neural2 where supported, Standard as fallback.
// This covers major languages for the vAi video narration system.
func buildDefaultVoiceMap() map[string]TTSVoiceMapping {
	return map[string]TTSVoiceMapping{
		// Chinese variants
		"yue-HK": {VoiceName: "yue-HK-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // 廣東話（女聲）
		"zh-HK":  {VoiceName: "yue-HK-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // 香港中文 → 粵語
		"zh-TW":  {VoiceName: "cmn-TW-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // 台灣華語
		"zh-CN":  {VoiceName: "cmn-CN-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // 普通話
		"zh":     {VoiceName: "cmn-TW-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // generic Chinese → 台灣華語

		// English variants
		"en-US": {VoiceName: "en-US-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},
		"en-GB": {VoiceName: "en-GB-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},
		"en-AU": {VoiceName: "en-AU-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},
		"en":    {VoiceName: "en-US-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // generic English → US

		// Japanese
		"ja-JP": {VoiceName: "ja-JP-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},
		"ja":    {VoiceName: "ja-JP-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},

		// Korean
		"ko-KR": {VoiceName: "ko-KR-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},
		"ko":    {VoiceName: "ko-KR-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},

		// Southeast Asian
		"th-TH":  {VoiceName: "th-TH-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},  // Thai
		"vi-VN":  {VoiceName: "vi-VN-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},  // Vietnamese
		"id-ID":  {VoiceName: "id-ID-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},  // Indonesian
		"ms-MY":  {VoiceName: "ms-MY-Chirp3-HD-Achernar", Tier: "Chirp3-HD"},  // Malay
		"fil-PH": {VoiceName: "fil-PH-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Filipino

		// South Asian
		"hi-IN": {VoiceName: "hi-IN-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Hindi

		// European
		"fr-FR": {VoiceName: "fr-FR-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // French
		"de-DE": {VoiceName: "de-DE-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // German
		"es-ES": {VoiceName: "es-ES-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Spanish (Spain)
		"es-US": {VoiceName: "es-US-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Spanish (US)
		"pt-BR": {VoiceName: "pt-BR-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Portuguese (Brazil)
		"pt-PT": {VoiceName: "pt-PT-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Portuguese (Portugal)
		"it-IT": {VoiceName: "it-IT-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Italian
		"nl-NL": {VoiceName: "nl-NL-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Dutch
		"ru-RU": {VoiceName: "ru-RU-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Russian
		"pl-PL": {VoiceName: "pl-PL-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Polish
		"uk-UA": {VoiceName: "uk-UA-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Ukrainian
		"tr-TR": {VoiceName: "tr-TR-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Turkish
		"sv-SE": {VoiceName: "sv-SE-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Swedish
		"da-DK": {VoiceName: "da-DK-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Danish
		"nb-NO": {VoiceName: "nb-NO-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Norwegian
		"fi-FI": {VoiceName: "fi-FI-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Finnish

		// Middle Eastern
		"ar-XA": {VoiceName: "ar-XA-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Arabic
		"he-IL": {VoiceName: "he-IL-Chirp3-HD-Achernar", Tier: "Chirp3-HD"}, // Hebrew
	}
}
