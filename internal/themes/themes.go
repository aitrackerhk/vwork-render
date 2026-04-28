package themes

import (
	"fmt"
	"strings"
)

// ThemePreset defines a website theme with CSS variables
type ThemePreset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`        // Display name (i18n key)
	Description string            `json:"description"` // Description (i18n key)
	Variables   map[string]string `json:"variables"`   // CSS variable name → value
}

// All available theme presets
var Presets = map[string]ThemePreset{
	"default": {
		ID:          "default",
		Name:        "pages.websiteTheme.themes.default.name",
		Description: "pages.websiteTheme.themes.default.description",
		Variables: map[string]string{
			// Background
			"--theme-bg":            "#ffffff",
			"--theme-surface":       "#f8f9fa",
			"--theme-surface-hover": "#e9ecef",
			// Text
			"--theme-text":          "#333333",
			"--theme-text-muted":    "#6c757d",
			"--theme-heading-color": "#333333",
			// Brand / Accent
			"--theme-primary":       "#007bff",
			"--theme-primary-hover": "#0056b3",
			"--theme-primary-rgb":   "0,123,255",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#333333",
			"--theme-nav-hover":  "#007bff",
			"--theme-nav-border": "#dee2e6",
			// Hero
			"--theme-hero-bg":      "#007bff",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer
			"--theme-footer-bg":     "#f8f9fa",
			"--theme-footer-text":   "#6c757d",
			"--theme-footer-border": "rgba(0,0,0,0.1)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#dee2e6",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(0,0,0,0.075)",
			// Button
			"--theme-btn-primary-bg":    "#007bff",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#0056b3",
			// Typography
			"--theme-font-family":   "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
			"--theme-heading-font":  "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
			"--theme-border-radius": "0.375rem",
		},
	},
	"modern-dark": {
		ID:          "modern-dark",
		Name:        "pages.websiteTheme.themes.modernDark.name",
		Description: "pages.websiteTheme.themes.modernDark.description",
		Variables: map[string]string{
			// Background
			"--theme-bg":            "#0f172a",
			"--theme-surface":       "#1e293b",
			"--theme-surface-hover": "#334155",
			// Text
			"--theme-text":          "#e2e8f0",
			"--theme-text-muted":    "#94a3b8",
			"--theme-heading-color": "#f1f5f9",
			// Brand / Accent
			"--theme-primary":       "#6366f1",
			"--theme-primary-hover": "#818cf8",
			"--theme-primary-rgb":   "99,102,241",
			// Nav
			"--theme-nav-bg":     "#1e293b",
			"--theme-nav-text":   "#e2e8f0",
			"--theme-nav-hover":  "#818cf8",
			"--theme-nav-border": "#334155",
			// Hero
			"--theme-hero-bg":      "#1e293b",
			"--theme-hero-text":    "#e2e8f0",
			"--theme-hero-heading": "#f1f5f9",
			// Footer
			"--theme-footer-bg":     "#0f172a",
			"--theme-footer-text":   "#94a3b8",
			"--theme-footer-border": "rgba(255,255,255,0.1)",
			// Card
			"--theme-card-bg":     "#1e293b",
			"--theme-card-border": "#334155",
			"--theme-card-shadow": "0 0.125rem 0.5rem rgba(0,0,0,0.3)",
			// Button
			"--theme-btn-primary-bg":    "#6366f1",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#818cf8",
			// Typography
			"--theme-font-family":   "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.5rem",
		},
	},
	"nature-green": {
		ID:          "nature-green",
		Name:        "pages.websiteTheme.themes.natureGreen.name",
		Description: "pages.websiteTheme.themes.natureGreen.description",
		Variables: map[string]string{
			// Background — warm off-white with a slight green undertone
			"--theme-bg":            "#fafdf7",
			"--theme-surface":       "#f0f5eb",
			"--theme-surface-hover": "#e4edd9",
			// Text
			"--theme-text":          "#2d3a2e",
			"--theme-text-muted":    "#6b7c6b",
			"--theme-heading-color": "#1a2e1a",
			// Brand / Accent — fresh leaf green
			"--theme-primary":       "#2e7d32",
			"--theme-primary-hover": "#1b5e20",
			"--theme-primary-rgb":   "46,125,50",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#2d3a2e",
			"--theme-nav-hover":  "#2e7d32",
			"--theme-nav-border": "#d5e3d0",
			// Hero — deep forest green
			"--theme-hero-bg":      "#2e7d32",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer — earthy dark green
			"--theme-footer-bg":     "#1a2e1a",
			"--theme-footer-text":   "#a8c5a0",
			"--theme-footer-border": "rgba(255,255,255,0.12)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#d5e3d0",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(46,125,50,0.08)",
			// Button
			"--theme-btn-primary-bg":    "#2e7d32",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#1b5e20",
			// Typography
			"--theme-font-family":   "'Nunito', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Nunito', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.5rem",
		},
	},

	"ocean-blue": {
		ID:          "ocean-blue",
		Name:        "pages.websiteTheme.themes.oceanBlue.name",
		Description: "pages.websiteTheme.themes.oceanBlue.description",
		Variables: map[string]string{
			// Background — crisp white with cool undertone
			"--theme-bg":            "#f7fafc",
			"--theme-surface":       "#edf2f7",
			"--theme-surface-hover": "#e2e8f0",
			// Text
			"--theme-text":          "#1a365d",
			"--theme-text-muted":    "#4a5568",
			"--theme-heading-color": "#0d2137",
			// Brand / Accent — deep ocean blue
			"--theme-primary":       "#2b6cb0",
			"--theme-primary-hover": "#1a4e8a",
			"--theme-primary-rgb":   "43,108,176",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#1a365d",
			"--theme-nav-hover":  "#2b6cb0",
			"--theme-nav-border": "#cbd5e0",
			// Hero — rich ocean gradient feel
			"--theme-hero-bg":      "#2b6cb0",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer — deep navy
			"--theme-footer-bg":     "#0d2137",
			"--theme-footer-text":   "#a0aec0",
			"--theme-footer-border": "rgba(255,255,255,0.1)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#cbd5e0",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(43,108,176,0.08)",
			// Button
			"--theme-btn-primary-bg":    "#2b6cb0",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#1a4e8a",
			// Typography
			"--theme-font-family":   "'Source Sans Pro', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Source Sans Pro', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.375rem",
		},
	},
	"warm-sunset": {
		ID:          "warm-sunset",
		Name:        "pages.websiteTheme.themes.warmSunset.name",
		Description: "pages.websiteTheme.themes.warmSunset.description",
		Variables: map[string]string{
			// Background — warm cream
			"--theme-bg":            "#fffaf5",
			"--theme-surface":       "#fef3e7",
			"--theme-surface-hover": "#fde8d0",
			// Text
			"--theme-text":          "#3d2c1e",
			"--theme-text-muted":    "#7c6b5d",
			"--theme-heading-color": "#2d1a0e",
			// Brand / Accent — warm orange
			"--theme-primary":       "#e8590c",
			"--theme-primary-hover": "#c44509",
			"--theme-primary-rgb":   "232,89,12",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#3d2c1e",
			"--theme-nav-hover":  "#e8590c",
			"--theme-nav-border": "#f0d9c4",
			// Hero — vibrant sunset orange
			"--theme-hero-bg":      "#e8590c",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer — deep warm brown
			"--theme-footer-bg":     "#2d1a0e",
			"--theme-footer-text":   "#c4a882",
			"--theme-footer-border": "rgba(255,255,255,0.1)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#f0d9c4",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(232,89,12,0.08)",
			// Button
			"--theme-btn-primary-bg":    "#e8590c",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#c44509",
			// Typography
			"--theme-font-family":   "'Poppins', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Poppins', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.625rem",
		},
	},
	"rose-elegant": {
		ID:          "rose-elegant",
		Name:        "pages.websiteTheme.themes.roseElegant.name",
		Description: "pages.websiteTheme.themes.roseElegant.description",
		Variables: map[string]string{
			// Background — soft blush white
			"--theme-bg":            "#fdf8f9",
			"--theme-surface":       "#f9eff2",
			"--theme-surface-hover": "#f3e1e7",
			// Text
			"--theme-text":          "#3d2b33",
			"--theme-text-muted":    "#7c6872",
			"--theme-heading-color": "#2d1a22",
			// Brand / Accent — dusty rose
			"--theme-primary":       "#be4b6e",
			"--theme-primary-hover": "#9e3958",
			"--theme-primary-rgb":   "190,75,110",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#3d2b33",
			"--theme-nav-hover":  "#be4b6e",
			"--theme-nav-border": "#edcdd6",
			// Hero — rich rose
			"--theme-hero-bg":      "#be4b6e",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer — deep plum
			"--theme-footer-bg":     "#2d1a22",
			"--theme-footer-text":   "#c5a0ac",
			"--theme-footer-border": "rgba(255,255,255,0.1)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#edcdd6",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(190,75,110,0.08)",
			// Button
			"--theme-btn-primary-bg":    "#be4b6e",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#9e3958",
			// Typography
			"--theme-font-family":   "'Playfair Display', Georgia, 'Times New Roman', serif",
			"--theme-heading-font":  "'Playfair Display', Georgia, 'Times New Roman', serif",
			"--theme-border-radius": "0.5rem",
		},
	},
	"bold-yellow": {
		ID:          "bold-yellow",
		Name:        "pages.websiteTheme.themes.boldYellow.name",
		Description: "pages.websiteTheme.themes.boldYellow.description",
		Variables: map[string]string{
			// Background — deep black
			"--theme-bg":            "#0a0a0a",
			"--theme-surface":       "#1a1a1a",
			"--theme-surface-hover": "#2a2a2a",
			// Text
			"--theme-text":          "#f5f5f5",
			"--theme-text-muted":    "#a0a0a0",
			"--theme-heading-color": "#ffd600",
			// Brand / Accent — electric yellow
			"--theme-primary":       "#ffd600",
			"--theme-primary-hover": "#ffea00",
			"--theme-primary-rgb":   "255,214,0",
			// Nav
			"--theme-nav-bg":     "#0a0a0a",
			"--theme-nav-text":   "#f5f5f5",
			"--theme-nav-hover":  "#ffd600",
			"--theme-nav-border": "#333333",
			// Hero — black with yellow accent
			"--theme-hero-bg":      "#1a1a1a",
			"--theme-hero-text":    "#f5f5f5",
			"--theme-hero-heading": "#ffd600",
			// Footer — pure black
			"--theme-footer-bg":     "#050505",
			"--theme-footer-text":   "#a0a0a0",
			"--theme-footer-border": "rgba(255,214,0,0.2)",
			// Card
			"--theme-card-bg":     "#1a1a1a",
			"--theme-card-border": "#333333",
			"--theme-card-shadow": "0 0.125rem 0.5rem rgba(0,0,0,0.4)",
			// Button — yellow with black text
			"--theme-btn-primary-bg":    "#ffd600",
			"--theme-btn-primary-text":  "#0a0a0a",
			"--theme-btn-primary-hover": "#ffea00",
			// Typography
			"--theme-font-family":   "'Barlow', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Barlow Condensed', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.25rem",
		},
	},
	"cyberpunk-dark": {
		ID:          "cyberpunk-dark",
		Name:        "pages.websiteTheme.themes.cyberpunkDark.name",
		Description: "pages.websiteTheme.themes.cyberpunkDark.description",
		Variables: map[string]string{
			// Background — near-black with purple undertone
			"--theme-bg":            "#0d0221",
			"--theme-surface":       "#1a0a3e",
			"--theme-surface-hover": "#2d1166",
			// Text
			"--theme-text":          "#e0d7f5",
			"--theme-text-muted":    "#9586b5",
			"--theme-heading-color": "#00f0ff",
			// Brand / Accent — neon cyan
			"--theme-primary":       "#00f0ff",
			"--theme-primary-hover": "#ff2a6d",
			"--theme-primary-rgb":   "0,240,255",
			// Nav
			"--theme-nav-bg":     "#0d0221",
			"--theme-nav-text":   "#e0d7f5",
			"--theme-nav-hover":  "#ff2a6d",
			"--theme-nav-border": "#2d1166",
			// Hero — deep purple with neon
			"--theme-hero-bg":      "#1a0a3e",
			"--theme-hero-text":    "#e0d7f5",
			"--theme-hero-heading": "#00f0ff",
			// Footer — darkest purple
			"--theme-footer-bg":     "#05010f",
			"--theme-footer-text":   "#9586b5",
			"--theme-footer-border": "rgba(0,240,255,0.15)",
			// Card
			"--theme-card-bg":     "#1a0a3e",
			"--theme-card-border": "#2d1166",
			"--theme-card-shadow": "0 0.125rem 0.75rem rgba(0,240,255,0.1)",
			// Button — neon cyan with dark text
			"--theme-btn-primary-bg":    "#00f0ff",
			"--theme-btn-primary-text":  "#0d0221",
			"--theme-btn-primary-hover": "#ff2a6d",
			// Typography
			"--theme-font-family":   "'Rajdhani', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Orbitron', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.125rem",
		},
	},
	"arctic-frost": {
		ID:          "arctic-frost",
		Name:        "pages.websiteTheme.themes.arcticFrost.name",
		Description: "pages.websiteTheme.themes.arcticFrost.description",
		Variables: map[string]string{
			// Background — icy white
			"--theme-bg":            "#f0f6fa",
			"--theme-surface":       "#e3eef5",
			"--theme-surface-hover": "#d1e3ef",
			// Text
			"--theme-text":          "#1c3144",
			"--theme-text-muted":    "#5a7d95",
			"--theme-heading-color": "#0f2132",
			// Brand / Accent — glacial blue
			"--theme-primary":       "#3a8fc2",
			"--theme-primary-hover": "#2c7aab",
			"--theme-primary-rgb":   "58,143,194",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#1c3144",
			"--theme-nav-hover":  "#3a8fc2",
			"--theme-nav-border": "#c8dce8",
			// Hero — cool steel blue
			"--theme-hero-bg":      "#2c5f7c",
			"--theme-hero-text":    "#f0f6fa",
			"--theme-hero-heading": "#ffffff",
			// Footer — deep arctic navy
			"--theme-footer-bg":     "#0f2132",
			"--theme-footer-text":   "#8fb3cc",
			"--theme-footer-border": "rgba(255,255,255,0.08)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#c8dce8",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(58,143,194,0.06)",
			// Button
			"--theme-btn-primary-bg":    "#3a8fc2",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#2c7aab",
			// Typography
			"--theme-font-family":   "'IBM Plex Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'IBM Plex Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.5rem",
		},
	},
	"mono-gray": {
		ID:          "mono-gray",
		Name:        "pages.websiteTheme.themes.monoGray.name",
		Description: "pages.websiteTheme.themes.monoGray.description",
		Variables: map[string]string{
			// Background — pure white
			"--theme-bg":            "#ffffff",
			"--theme-surface":       "#f5f5f5",
			"--theme-surface-hover": "#e8e8e8",
			// Text
			"--theme-text":          "#2c2c2c",
			"--theme-text-muted":    "#888888",
			"--theme-heading-color": "#111111",
			// Brand / Accent — mid gray
			"--theme-primary":       "#555555",
			"--theme-primary-hover": "#333333",
			"--theme-primary-rgb":   "85,85,85",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#2c2c2c",
			"--theme-nav-hover":  "#111111",
			"--theme-nav-border": "#e0e0e0",
			// Hero — charcoal
			"--theme-hero-bg":      "#2c2c2c",
			"--theme-hero-text":    "#f5f5f5",
			"--theme-hero-heading": "#ffffff",
			// Footer — near black
			"--theme-footer-bg":     "#1a1a1a",
			"--theme-footer-text":   "#999999",
			"--theme-footer-border": "rgba(255,255,255,0.08)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#e0e0e0",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(0,0,0,0.06)",
			// Button — black with white text
			"--theme-btn-primary-bg":    "#2c2c2c",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#111111",
			// Typography
			"--theme-font-family":   "'Helvetica Neue', Helvetica, Arial, sans-serif",
			"--theme-heading-font":  "'Helvetica Neue', Helvetica, Arial, sans-serif",
			"--theme-border-radius": "0.25rem",
		},
	},
	"noir-red": {
		ID:          "noir-red",
		Name:        "pages.websiteTheme.themes.noirRed.name",
		Description: "pages.websiteTheme.themes.noirRed.description",
		Variables: map[string]string{
			// Background — deep black
			"--theme-bg":            "#0e0e0e",
			"--theme-surface":       "#1c1c1c",
			"--theme-surface-hover": "#2a2a2a",
			// Text
			"--theme-text":          "#e5e5e5",
			"--theme-text-muted":    "#999999",
			"--theme-heading-color": "#ffffff",
			// Brand / Accent — crimson red
			"--theme-primary":       "#dc2626",
			"--theme-primary-hover": "#ef4444",
			"--theme-primary-rgb":   "220,38,38",
			// Nav
			"--theme-nav-bg":     "#0e0e0e",
			"--theme-nav-text":   "#e5e5e5",
			"--theme-nav-hover":  "#dc2626",
			"--theme-nav-border": "#2a2a2a",
			// Hero — black with red accent
			"--theme-hero-bg":      "#1c1c1c",
			"--theme-hero-text":    "#e5e5e5",
			"--theme-hero-heading": "#ffffff",
			// Footer — pure black
			"--theme-footer-bg":     "#050505",
			"--theme-footer-text":   "#999999",
			"--theme-footer-border": "rgba(220,38,38,0.2)",
			// Card
			"--theme-card-bg":     "#1c1c1c",
			"--theme-card-border": "#2a2a2a",
			"--theme-card-shadow": "0 0.125rem 0.5rem rgba(0,0,0,0.4)",
			// Button — red with white text
			"--theme-btn-primary-bg":    "#dc2626",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#ef4444",
			// Typography
			"--theme-font-family":   "'DM Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'DM Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.375rem",
		},
	},
	"lavender-dream": {
		ID:          "lavender-dream",
		Name:        "pages.websiteTheme.themes.lavenderDream.name",
		Description: "pages.websiteTheme.themes.lavenderDream.description",
		Variables: map[string]string{
			// Background — soft lavender white
			"--theme-bg":            "#faf8ff",
			"--theme-surface":       "#f0ebfa",
			"--theme-surface-hover": "#e4dcf5",
			// Text
			"--theme-text":          "#2e2444",
			"--theme-text-muted":    "#7b6e96",
			"--theme-heading-color": "#1e1533",
			// Brand / Accent — soft purple
			"--theme-primary":       "#7c3aed",
			"--theme-primary-hover": "#6d28d9",
			"--theme-primary-rgb":   "124,58,237",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#2e2444",
			"--theme-nav-hover":  "#7c3aed",
			"--theme-nav-border": "#e0d4f5",
			// Hero — rich purple
			"--theme-hero-bg":      "#7c3aed",
			"--theme-hero-text":    "#ffffff",
			"--theme-hero-heading": "#ffffff",
			// Footer — deep indigo
			"--theme-footer-bg":     "#1e1533",
			"--theme-footer-text":   "#a897c4",
			"--theme-footer-border": "rgba(255,255,255,0.1)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#e0d4f5",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(124,58,237,0.06)",
			// Button
			"--theme-btn-primary-bg":    "#7c3aed",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#6d28d9",
			// Typography
			"--theme-font-family":   "'Quicksand', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Quicksand', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.75rem",
		},
	},
	"clean-amber": {
		ID:          "clean-amber",
		Name:        "pages.websiteTheme.themes.cleanAmber.name",
		Description: "pages.websiteTheme.themes.cleanAmber.description",
		Variables: map[string]string{
			// Background — clean white
			"--theme-bg":            "#ffffff",
			"--theme-surface":       "#fefce8",
			"--theme-surface-hover": "#fef9c3",
			// Text
			"--theme-text":          "#1c1917",
			"--theme-text-muted":    "#78716c",
			"--theme-heading-color": "#0c0a09",
			// Brand / Accent — amber yellow
			"--theme-primary":       "#d97706",
			"--theme-primary-hover": "#b45309",
			"--theme-primary-rgb":   "217,119,6",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#1c1917",
			"--theme-nav-hover":  "#d97706",
			"--theme-nav-border": "#e7e5e4",
			// Hero — deep black with amber accent
			"--theme-hero-bg":      "#1c1917",
			"--theme-hero-text":    "#fefce8",
			"--theme-hero-heading": "#fbbf24",
			// Footer — near black
			"--theme-footer-bg":     "#0c0a09",
			"--theme-footer-text":   "#a8a29e",
			"--theme-footer-border": "rgba(217,119,6,0.2)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#e7e5e4",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(0,0,0,0.06)",
			// Button — amber with black text
			"--theme-btn-primary-bg":    "#d97706",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#b45309",
			// Typography
			"--theme-font-family":   "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.375rem",
		},
	},
	"clean-crimson": {
		ID:          "clean-crimson",
		Name:        "pages.websiteTheme.themes.cleanCrimson.name",
		Description: "pages.websiteTheme.themes.cleanCrimson.description",
		Variables: map[string]string{
			// Background — clean white
			"--theme-bg":            "#ffffff",
			"--theme-surface":       "#fef2f2",
			"--theme-surface-hover": "#fee2e2",
			// Text
			"--theme-text":          "#1c1917",
			"--theme-text-muted":    "#78716c",
			"--theme-heading-color": "#0c0a09",
			// Brand / Accent — crimson red
			"--theme-primary":       "#dc2626",
			"--theme-primary-hover": "#b91c1c",
			"--theme-primary-rgb":   "220,38,38",
			// Nav
			"--theme-nav-bg":     "#ffffff",
			"--theme-nav-text":   "#1c1917",
			"--theme-nav-hover":  "#dc2626",
			"--theme-nav-border": "#e7e5e4",
			// Hero — deep black with red accent
			"--theme-hero-bg":      "#1c1917",
			"--theme-hero-text":    "#fef2f2",
			"--theme-hero-heading": "#fca5a5",
			// Footer — near black
			"--theme-footer-bg":     "#0c0a09",
			"--theme-footer-text":   "#a8a29e",
			"--theme-footer-border": "rgba(220,38,38,0.2)",
			// Card
			"--theme-card-bg":     "#ffffff",
			"--theme-card-border": "#e7e5e4",
			"--theme-card-shadow": "0 0.125rem 0.25rem rgba(0,0,0,0.06)",
			// Button — red with white text
			"--theme-btn-primary-bg":    "#dc2626",
			"--theme-btn-primary-text":  "#ffffff",
			"--theme-btn-primary-hover": "#b91c1c",
			// Typography
			"--theme-font-family":   "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.375rem",
		},
	},
	"midnight-teal": {
		ID:          "midnight-teal",
		Name:        "pages.websiteTheme.themes.midnightTeal.name",
		Description: "pages.websiteTheme.themes.midnightTeal.description",
		Variables: map[string]string{
			// Background — deep blue-black
			"--theme-bg":            "#0f1729",
			"--theme-surface":       "#172033",
			"--theme-surface-hover": "#1e2a42",
			// Text
			"--theme-text":          "#d1d5db",
			"--theme-text-muted":    "#9ca3af",
			"--theme-heading-color": "#f3f4f6",
			// Brand / Accent — teal
			"--theme-primary":       "#14b8a6",
			"--theme-primary-hover": "#2dd4bf",
			"--theme-primary-rgb":   "20,184,166",
			// Nav
			"--theme-nav-bg":     "#0f1729",
			"--theme-nav-text":   "#d1d5db",
			"--theme-nav-hover":  "#14b8a6",
			"--theme-nav-border": "#1e2a42",
			// Hero — dark with teal accent
			"--theme-hero-bg":      "#172033",
			"--theme-hero-text":    "#d1d5db",
			"--theme-hero-heading": "#14b8a6",
			// Footer — darkest
			"--theme-footer-bg":     "#080d18",
			"--theme-footer-text":   "#9ca3af",
			"--theme-footer-border": "rgba(20,184,166,0.15)",
			// Card
			"--theme-card-bg":     "#172033",
			"--theme-card-border": "#1e2a42",
			"--theme-card-shadow": "0 0.125rem 0.5rem rgba(0,0,0,0.3)",
			// Button — teal with dark text
			"--theme-btn-primary-bg":    "#14b8a6",
			"--theme-btn-primary-text":  "#0f1729",
			"--theme-btn-primary-hover": "#2dd4bf",
			// Typography
			"--theme-font-family":   "'Space Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-heading-font":  "'Space Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
			"--theme-border-radius": "0.5rem",
		},
	},
}

// GetThemeCSS returns a CSS string of :root variables for the given theme ID.
// Falls back to "default" if theme not found.
func GetThemeCSS(themeID string) string {
	themeID = strings.TrimSpace(themeID)
	if themeID == "" {
		themeID = "default"
	}

	preset, ok := Presets[themeID]
	if !ok {
		preset = Presets["default"]
	}

	var sb strings.Builder
	for name, value := range preset.Variables {
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", name, value))
	}
	return sb.String()
}

// GetThemeIDs returns all available theme IDs in display order.
func GetThemeIDs() []string {
	return []string{"default", "modern-dark", "nature-green", "ocean-blue", "warm-sunset", "rose-elegant", "bold-yellow", "cyberpunk-dark", "arctic-frost", "mono-gray", "noir-red", "lavender-dream", "clean-amber", "clean-crimson", "midnight-teal"}
}

// GetPreset returns a single preset (or default if not found).
func GetPreset(themeID string) ThemePreset {
	themeID = strings.TrimSpace(themeID)
	if themeID == "" {
		themeID = "default"
	}
	preset, ok := Presets[themeID]
	if !ok {
		return Presets["default"]
	}
	return preset
}
