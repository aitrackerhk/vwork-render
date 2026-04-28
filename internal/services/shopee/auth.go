package shopee

import (
	"encoding/json"
	"fmt"
	"time"
)

// TokenResponse represents the response from token-related APIs
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireIn     int    `json:"expire_in"`
	ShopID       int64  `json:"shop_id"`
	PartnerID    int64  `json:"partner_id"`
}

// ShopInfo represents shop information
type ShopInfo struct {
	ShopID          int64  `json:"shop_id"`
	ShopName        string `json:"shop_name"`
	Region          string `json:"region"`
	Status          string `json:"status"`
	IsCNSC          bool   `json:"is_cnsc"`
	IsSIP           bool   `json:"is_sip"`
	AuthTime        int64  `json:"auth_time"`
	ExpireTime      int64  `json:"expire_time"`
	ShopDescription string `json:"shop_description"`
}

// GetAccessToken exchanges the authorization code for access token
// This should be called after the user authorizes the app
func (c *Client) GetAccessToken(code string, shopID int64) (*TokenResponse, error) {
	path := "/api/v2/auth/token/get"

	body := map[string]interface{}{
		"code":       code,
		"shop_id":    shopID,
		"partner_id": c.PartnerID,
	}

	// For token/get, we don't need access_token in signature
	originalToken := c.AccessToken
	c.AccessToken = ""
	defer func() { c.AccessToken = originalToken }()

	resp, err := c.Post(path, nil, body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(resp.Response, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAccessToken refreshes the access token using the refresh token
func (c *Client) RefreshAccessToken() (*TokenResponse, error) {
	path := "/api/v2/auth/access_token/get"

	body := map[string]interface{}{
		"refresh_token": c.RefreshToken,
		"shop_id":       c.ShopID,
		"partner_id":    c.PartnerID,
	}

	// For refresh token, we don't need access_token in signature
	originalToken := c.AccessToken
	c.AccessToken = ""
	defer func() { c.AccessToken = originalToken }()

	resp, err := c.Post(path, nil, body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(resp.Response, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Update client tokens
	c.AccessToken = tokenResp.AccessToken
	c.RefreshToken = tokenResp.RefreshToken

	return &tokenResp, nil
}

// GetShopInfo retrieves the shop information
func (c *Client) GetShopInfo() (*ShopInfo, error) {
	path := "/api/v2/shop/get_shop_info"

	resp, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}

	var shopInfo ShopInfo
	if err := json.Unmarshal(resp.Response, &shopInfo); err != nil {
		return nil, fmt.Errorf("failed to parse shop info: %w", err)
	}

	return &shopInfo, nil
}

// TokenNeedsRefresh checks if the access token needs to be refreshed
// Shopee access tokens typically expire in 4 hours (14400 seconds)
func TokenNeedsRefresh(expireTime int64) bool {
	// Refresh if token expires in less than 30 minutes
	return time.Now().Unix() > (expireTime - 1800)
}

// IsTokenExpired checks if the refresh token is expired
// Shopee refresh tokens typically expire in 30 days
func IsRefreshTokenExpired(authTime int64) bool {
	// Refresh token expires 30 days after authorization
	return time.Now().Unix() > (authTime + 30*24*60*60)
}
