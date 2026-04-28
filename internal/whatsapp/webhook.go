package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"nwork/config"
)

// WebhookVerifier 用於驗證 Webhook 請求
type WebhookVerifier struct {
	verifyToken string
	appSecret   string
}

// NewWebhookVerifier 創建新的 Webhook 驗證器
func NewWebhookVerifier(cfg *config.Config) *WebhookVerifier {
	return &WebhookVerifier{
		verifyToken: cfg.WhatsApp.VerifyToken,
		appSecret:   cfg.WhatsApp.AppSecret,
	}
}

// VerifyWebhook 驗證 Webhook 請求（用於 GET 請求）
func (v *WebhookVerifier) VerifyWebhook(mode, token, challenge string) (bool, string) {
	if mode == "subscribe" && token == v.verifyToken {
		return true, challenge
	}
	return false, ""
}

// VerifySignature 驗證 Webhook 簽名（用於 POST 請求）
func (v *WebhookVerifier) VerifySignature(signature string, body []byte) bool {
	if v.appSecret == "" {
		// 如果沒有配置 app secret，跳過簽名驗證（不推薦，但允許開發環境）
		return true
	}

	mac := hmac.New(sha256.New, []byte(v.appSecret))
	mac.Write(body)
	expectedSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// WebhookEvent 表示 WhatsApp Webhook 事件
type WebhookEvent struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WAID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Image *struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
						Sha256   string `json:"sha256"`
						Caption  string `json:"caption"`
					} `json:"image,omitempty"`
					Document *struct {
						ID       string `json:"id"`
						Filename string `json:"filename"`
						MimeType string `json:"mime_type"`
						Sha256   string `json:"sha256"`
					} `json:"document,omitempty"`
					Audio *struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
						Sha256   string `json:"sha256"`
					} `json:"audio,omitempty"`
					Video *struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
						Sha256   string `json:"sha256"`
						Caption  string `json:"caption"`
					} `json:"video,omitempty"`
					Location *struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location,omitempty"`
				} `json:"messages"`
				Statuses []struct {
					ID           string `json:"id"`
					Status       string `json:"status"` // sent, delivered, read, failed
					Timestamp    string `json:"timestamp"`
					RecipientID  string `json:"recipient_id"`
					Conversation *struct {
						ID     string `json:"id"`
						Origin struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation,omitempty"`
					Pricing *struct {
						Billable     bool   `json:"billable"`
						PricingModel string `json:"pricing_model"`
						Category     string `json:"category"`
					} `json:"pricing,omitempty"`
				} `json:"statuses"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// ParseWebhookEvent 解析 Webhook 事件
func ParseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("unmarshal webhook event: %w", err)
	}
	return &event, nil
}

// GetIncomingMessages 從 Webhook 事件中提取所有接收到的消息
func (e *WebhookEvent) GetIncomingMessages() []IncomingMessage {
	var messages []IncomingMessage

	for _, entry := range e.Entry {
		for _, change := range entry.Changes {
			if change.Field == "messages" {
				for _, msg := range change.Value.Messages {
					// 獲取發送者名稱
					var senderName string
					for _, contact := range change.Value.Contacts {
						if contact.WAID == msg.From {
							senderName = contact.Profile.Name
							break
						}
					}

					incomingMsg := IncomingMessage{
						From:          msg.From,
						FromName:      senderName,
						MessageID:     msg.ID,
						Timestamp:     msg.Timestamp,
						Type:          msg.Type,
						Text:          msg.Text.Body,
						PhoneNumberID: change.Value.Metadata.PhoneNumberID,
					}
					if msg.Image != nil {
						incomingMsg.Image = &struct {
							ID       string
							MimeType string
							Sha256   string
							Caption  string
						}{
							ID:       msg.Image.ID,
							MimeType: msg.Image.MimeType,
							Sha256:   msg.Image.Sha256,
							Caption:  msg.Image.Caption,
						}
					}
					if msg.Document != nil {
						incomingMsg.Document = &struct {
							ID       string
							Filename string
							MimeType string
							Sha256   string
						}{
							ID:       msg.Document.ID,
							Filename: msg.Document.Filename,
							MimeType: msg.Document.MimeType,
							Sha256:   msg.Document.Sha256,
						}
					}
					if msg.Audio != nil {
						incomingMsg.Audio = &struct {
							ID       string
							MimeType string
							Sha256   string
						}{
							ID:       msg.Audio.ID,
							MimeType: msg.Audio.MimeType,
							Sha256:   msg.Audio.Sha256,
						}
					}
					if msg.Video != nil {
						incomingMsg.Video = &struct {
							ID       string
							MimeType string
							Sha256   string
							Caption  string
						}{
							ID:       msg.Video.ID,
							MimeType: msg.Video.MimeType,
							Sha256:   msg.Video.Sha256,
							Caption:  msg.Video.Caption,
						}
					}
					if msg.Location != nil {
						incomingMsg.Location = &struct {
							Latitude  float64
							Longitude float64
							Name      string
							Address   string
						}{
							Latitude:  msg.Location.Latitude,
							Longitude: msg.Location.Longitude,
							Name:      msg.Location.Name,
							Address:   msg.Location.Address,
						}
					}
					messages = append(messages, incomingMsg)
				}
			}
		}
	}

	return messages
}

// GetStatusUpdates 從 Webhook 事件中提取所有狀態更新
func (e *WebhookEvent) GetStatusUpdates() []StatusUpdate {
	var updates []StatusUpdate

	for _, entry := range e.Entry {
		for _, change := range entry.Changes {
			if change.Field == "messages" {
				for _, status := range change.Value.Statuses {
					update := StatusUpdate{
						MessageID:   status.ID,
						Status:      status.Status,
						Timestamp:   status.Timestamp,
						RecipientID: status.RecipientID,
					}
					if status.Conversation != nil {
						update.Conversation = &struct {
							ID     string
							Origin struct {
								Type string
							}
						}{
							ID: status.Conversation.ID,
							Origin: struct {
								Type string
							}{
								Type: status.Conversation.Origin.Type,
							},
						}
					}
					if status.Pricing != nil {
						update.Pricing = &struct {
							Billable     bool
							PricingModel string
							Category     string
						}{
							Billable:     status.Pricing.Billable,
							PricingModel: status.Pricing.PricingModel,
							Category:     status.Pricing.Category,
						}
					}
					updates = append(updates, update)
				}
			}
		}
	}

	return updates
}

// IncomingMessage 表示接收到的消息
type IncomingMessage struct {
	From          string
	FromName      string
	MessageID     string
	Timestamp     string
	Type          string
	Text          string
	Image         *struct {
		ID       string
		MimeType string
		Sha256   string
		Caption  string
	}
	Document      *struct {
		ID       string
		Filename string
		MimeType string
		Sha256   string
	}
	Audio         *struct {
		ID       string
		MimeType string
		Sha256   string
	}
	Video         *struct {
		ID       string
		MimeType string
		Sha256   string
		Caption  string
	}
	Location      *struct {
		Latitude  float64
		Longitude float64
		Name      string
		Address   string
	}
	PhoneNumberID string
}

// StatusUpdate 表示消息狀態更新
type StatusUpdate struct {
	MessageID    string
	Status       string // sent, delivered, read, failed
	Timestamp    string
	RecipientID  string
	Conversation *struct {
		ID     string
		Origin struct {
			Type string
		}
	}
	Pricing *struct {
		Billable     bool
		PricingModel string
		Category     string
	}
}

