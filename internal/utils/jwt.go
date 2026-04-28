package utils

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var jwtSecret = []byte(getJWTSecret())

func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "u-nai-secret-key-change-in-production" // 默認密鑰，生產環境必須更改
	}
	return secret
}

// Claims JWT 聲明
type Claims struct {
	UserID         uuid.UUID `json:"user_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	Platform       string    `json:"platform,omitempty"`        // "web" or "desktop"; empty = legacy (treated as "web")
	ImpersonatedBy string    `json:"impersonated_by,omitempty"` // non-empty when token was created via admin impersonation
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT Token
// 注意：為了保持長期登入，此處不設定 ExpiresAt，避免 token 自動過期。
// platform: "web" or "desktop" — controls which LoggedOutAt field is checked on logout.
func GenerateToken(userID, tenantID uuid.UUID, email, role, platform string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		TenantID: tenantID,
		Email:    email,
		Role:     role,
		Platform: platform,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "u-nai",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GenerateImpersonateToken 生成 admin impersonate JWT Token
// impersonatedBy: identifier of the admin who initiated the impersonation (e.g. "vworkadmin")
func GenerateImpersonateToken(userID, tenantID uuid.UUID, email, role, platform, impersonatedBy string) (string, error) {
	claims := &Claims{
		UserID:         userID,
		TenantID:       tenantID,
		Email:          email,
		Role:           role,
		Platform:       platform,
		ImpersonatedBy: impersonatedBy,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "u-nai",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken 驗證 JWT Token
func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
