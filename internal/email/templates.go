package email

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"
)

type templateData struct {
	HTMLLang string

	AppName     string
	CompanyName string
	LogoURL     string

	Title              string
	Preheader          string
	Greeting           string
	IntroLines         []string
	ButtonText         string
	ButtonURL          string
	ButtonFallbackHelp string
	OutroLines         []string
	FooterLines        []string

	SystemNotice string
}

// renderHTML renders a modern, email-client-friendly HTML.
func renderHTML(d templateData) (string, error) {
	const tpl = `<!doctype html>
<html lang="{{.HTMLLang}}">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="x-apple-disable-message-reformatting" />
  <title>{{.Title}}</title>
  <style>
    body{margin:0;padding:40px 0 32px 0;background:#f6f7fb;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,"PingFang TC","Noto Sans TC","Microsoft JhengHei",sans-serif;color:#111827}
    a{color:#2563eb;text-decoration:none}
    .container{max-width:640px;margin:0 auto;padding:0 16px}
    .card{background:#ffffff;border-radius:14px;box-shadow:0 8px 24px rgba(17,24,39,.08);overflow:hidden;border:1px solid #eef2ff}
    .header{padding:12px 18px;background:#ffffff;color:#111827;display:flex;align-items:center;gap:12px;border-bottom:1px solid #eef2ff}
    .logo{height:32px;display:block}
    .brand{font-weight:800;font-size:16px;letter-spacing:.2px}
    .content{padding:18px 18px 16px 18px}
    .h1{margin:0 0 8px 0;font-size:20px;line-height:1.3}
    .p{margin:0 0 10px 0;line-height:1.55;color:#374151;font-size:14px}
    .button{display:inline-block;padding:11px 15px;border-radius:10px;background:#2563eb;color:#fff !important;font-weight:700;font-size:14px}
    .button-row{text-align:left;margin:10px 0 8px 0;}
    .muted{color:#6b7280;font-size:12px;line-height:1.45}
    .divider{height:1px;background:#eef2ff;margin:14px 0}
    .footer{padding:14px 18px 18px 18px;background:#f8fafc;border-top:1px solid #eef2ff}
    .preheader{display:none!important;visibility:hidden;opacity:0;color:transparent;height:0;width:0;overflow:hidden;mso-hide:all}
  </style>
</head>
<body>
  <div class="preheader">{{.Preheader}}</div>
  <div class="container">
    <div class="card">
      <div class="header">
        {{if .LogoURL}}<img src="{{.LogoURL}}" alt="Logo" class="logo">{{else}}<div class="brand">{{.CompanyName}}</div>{{end}}
      </div>
      <div class="content">
        <div class="h1">{{.Title}}</div>
        {{if .Greeting}}<p class="p">{{.Greeting}}</p>{{end}}
        {{range .IntroLines}}<p class="p">{{.}}</p>{{end}}

        {{if .ButtonURL}}
          <div class="button-row">
            <a class="button" href="{{.ButtonURL}}" target="_blank" rel="noopener">{{.ButtonText}}</a>
          </div>
          {{if .ButtonFallbackHelp}}<p class="muted" style="margin-top:6px;">{{.ButtonFallbackHelp}}</p>{{end}}
          <p class="muted" style="word-break:break-all;">{{.ButtonURL}}</p>
        {{end}}

        {{if .OutroLines}}
          <div class="divider"></div>
          {{range .OutroLines}}<p class="p">{{.}}</p>{{end}}
        {{end}}
      </div>
      <div class="footer">
        {{range .FooterLines}}<div class="muted">{{.}}</div>{{end}}
        <div class="muted">© 2025 {{if .CompanyName}}{{.CompanyName}}{{else}}V-sys Limited{{end}}. All rights reserved.</div>
      </div>
    </div>
    <div style="height:24px"></div>
    {{if .SystemNotice}}<div class="muted" style="text-align:center;">{{.SystemNotice}}</div>{{end}}
  </div>
</body>
</html>`

	t, err := template.New("email").Parse(tpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func brandReplace(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "U-nAi", "vWork")
	s = strings.ReplaceAll(s, "U-n.ai", "vworkai.com")
	return s
}

func buildText(d templateData) string {
	var b strings.Builder
	if d.Title != "" {
		b.WriteString(d.Title)
		b.WriteString("\n\n")
	}
	if d.Greeting != "" {
		b.WriteString(d.Greeting)
		b.WriteString("\n\n")
	}
	for _, line := range d.IntroLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if d.ButtonURL != "" {
		b.WriteString("\n")
		b.WriteString(d.ButtonText)
		b.WriteString(": ")
		b.WriteString(d.ButtonURL)
		b.WriteString("\n")
	}
	if len(d.OutroLines) > 0 {
		b.WriteString("\n")
		for _, line := range d.OutroLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if len(d.FooterLines) > 0 {
		b.WriteString("\n")
		for _, line := range d.FooterLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	// Footer branding
	if strings.TrimSpace(d.CompanyName) != "" {
		b.WriteString("\n")
		b.WriteString("© 2025 " + strings.TrimSpace(d.CompanyName) + ". All rights reserved.\n")
	} else {
		b.WriteString("\n")
		b.WriteString("© 2025 V-sys Limited. All rights reserved.\n")
	}
	return strings.TrimSpace(brandReplace(b.String()))
}

type emailPhrases struct {
	HTMLLang           string
	ButtonFallbackHelp string
	SystemNotice       string
	GreetPrefix        string
	GreetSuffix        string
}

func phrasesFor(lang string) emailPhrases {
	// normalize
	l := strings.ToLower(strings.TrimSpace(lang))
	if strings.HasPrefix(l, "en") {
		return emailPhrases{
			HTMLLang:           "en",
			ButtonFallbackHelp: "If the button doesn't work, copy and paste this link into your browser:",
			SystemNotice:       "This is an automated email. Please do not reply.",
			GreetPrefix:        "Hi ",
			GreetSuffix:        ",",
		}
	}
	// zh-Hans (Simplified)
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		return emailPhrases{
			HTMLLang:           "zh-Hans",
			ButtonFallbackHelp: "如果按钮无法点击，请复制以下链接到浏览器打开：",
			SystemNotice:       "此为系统自动发送邮件，请勿直接回复。",
			GreetPrefix:        "你好 ",
			GreetSuffix:        "，",
		}
	}
	// default zh-Hant (Traditional)
	return emailPhrases{
		HTMLLang:           "zh-Hant",
		ButtonFallbackHelp: "如果按鈕無法點擊，請複製以下連結到瀏覽器開啟：",
		SystemNotice:       "此為系統自動寄送郵件，請勿直接回覆。",
		GreetPrefix:        "你好 ",
		GreetSuffix:        "，",
	}
}

func WelcomeEmail(b Branding, tenantSubdomain string, toName string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}
	baseDomain := c.Domain.BaseDomain
	if strings.Contains(baseDomain, "vworkai.com") {
		baseDomain = "www.vworkai.com"
	}
	loginURL := buildTenantURL(c.Domain.Scheme, "", baseDomain, "/login")

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(toName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(toName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "註冊成功，歡迎使用 " + c.AppName
	preheader := "你的帳戶已建立，立即登入開始使用。"
	intro := []string{
		"你的帳戶已建立完成。你可以使用以下連結登入系統。",
	}
	btnText := "前往登入"
	outro := []string{"若這不是你本人操作，請忽略此郵件。"}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = "注册成功，欢迎使用 " + c.AppName
		preheader = "你的账户已建立，立即登录开始使用。"
		intro = []string{
			"你的账户已建立完成。你可以使用以下链接登录系统。",
		}
		btnText = "前往登录"
		outro = []string{"若这不是你本人操作，请忽略此邮件。"}
	} else if strings.HasPrefix(l, "en") {
		title = "Welcome to " + c.AppName
		preheader = "Your account is ready. Log in to get started."
		intro = []string{
			"Your account has been created. You can log in using the link below.",
		}
		btnText = "Go to login"
		outro = []string{"If you didn't do this, you can ignore this email."}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          loginURL,
		OutroLines:         outro,
		FooterLines:        []string{},
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	subjectOut := brandReplace(d.Title)
	textOut := brandReplace(buildText(d))
	htmlOut := brandReplace(html)

	return subjectOut, textOut, htmlOut, nil
}

func PasswordResetEmail(b Branding, tenantSubdomain string, toName string, resetURL string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(toName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(toName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "重設密碼"
	preheader := "點擊連結重設你的密碼（有效期有限）。"
	intro := []string{
		"我們收到你的密碼重置請求。",
		"請點擊下面按鈕設定新密碼。此連結具有時效性，請盡快完成。",
	}
	btnText := "重設密碼"
	outro := []string{"如果你沒有發起此請求，請忽略此郵件，你的密碼不會被更改。"}
	footer := []string{}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = "重置密码"
		preheader = "点击链接重置你的密码（有效期有限）。"
		intro = []string{
			"我们收到你的密码重置请求。",
			"请点击下面按钮设置新密码。此链接具有时效性，请尽快完成。",
		}
		btnText = "重置密码"
		outro = []string{"如果你没有发起此请求，请忽略此邮件，你的密码不会被更改。"}
		footer = []string{}
	} else if strings.HasPrefix(l, "en") {
		title = "Reset your password"
		preheader = "Use the link to reset your password (expires soon)."
		intro = []string{
			"We received a request to reset your password.",
			"Click the button below to set a new password. This link expires soon.",
		}
		btnText = "Reset password"
		outro = []string{"If you didn't request this, you can ignore this email and your password will stay the same."}
		footer = []string{}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          resetURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		OutroLines:         outro,
		FooterLines:        footer,
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}
	subjectOut := brandReplace(d.Title)
	textOut := brandReplace(buildText(d))
	htmlOut := brandReplace(html)
	return subjectOut, textOut, htmlOut, nil
}

func TrialLimitExceededEmail(b Branding, tenantSubdomain string, toName string, graceUntil time.Time, lang string) (subject, textBody, htmlBody string, err error) {
	phrases := phrasesFor(lang)

	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	deadlineText := graceUntil.Format("2006-01-02 15:04")
	buttonURL := buildTenantURL(c.Domain.Scheme, strings.TrimSpace(tenantSubdomain), c.Domain.BaseDomain, "/subscription-required")

	data := templateData{
		HTMLLang:    phrases.HTMLLang,
		AppName:     c.AppName,
		CompanyName: b.CompanyName,
		LogoURL:     b.LogoURL,
		Title:       "免費試用已達上限",
		Preheader:   "請於一週內完成訂閱以繼續使用",
		Greeting:    phrases.GreetPrefix + strings.TrimSpace(toName) + phrases.GreetSuffix,
		IntroLines: []string{
			fmt.Sprintf("您的免費試用已達上限，請於 %s 前完成訂閱，以繼續使用所有功能。", deadlineText),
			"完成訂閱後即可立即解除限制。",
		},
		ButtonText:         "前往訂閱",
		ButtonURL:          buttonURL,
		ButtonFallbackHelp: phrases.ButtonFallbackHelp,
		OutroLines:         []string{"如已完成付款，請忽略此封通知。"},
		FooterLines:        []string{"此信件用於提醒訂閱付款期限。"},
		SystemNotice:       phrases.SystemNotice,
	}

	htmlBody, err = renderHTML(data)
	if err != nil {
		return "", "", "", err
	}
	textBody = buildText(data)
	return brandReplace(data.Title), textBody, htmlBody, nil
}

func ContactEmail(b Branding, name, email, phone, product, subject, message string, lang string) (subjectOut, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	phoneInfo := ""
	if strings.TrimSpace(phone) != "" {
		phoneInfo = "\n電話：" + strings.TrimSpace(phone)
	}
	productInfo := ""
	if strings.TrimSpace(product) != "" {
		productInfo = "\n相關產品：" + strings.TrimSpace(product)
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "新的聯絡表單提交"
	preheader := "收到來自 " + strings.TrimSpace(name) + " 的聯絡表單"
	greeting := "你好，"
	intro := []string{
		"收到新的聯絡表單提交，詳情如下：",
		"",
		"姓名：" + strings.TrimSpace(name),
		"電子郵件：" + strings.TrimSpace(email) + phoneInfo + productInfo,
		"主題：" + strings.TrimSpace(subject),
		"",
		"訊息內容：",
		strings.TrimSpace(message),
	}
	footer := []string{}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		phoneInfo = ""
		if strings.TrimSpace(phone) != "" {
			phoneInfo = "\n电话：" + strings.TrimSpace(phone)
		}
		productInfo = ""
		if strings.TrimSpace(product) != "" {
			productInfo = "\n相关产品：" + strings.TrimSpace(product)
		}
		title = "新的联系表单提交"
		preheader = "收到来自 " + strings.TrimSpace(name) + " 的联系表单"
		greeting = "你好，"
		intro = []string{
			"收到新的联系表单提交，详情如下：",
			"",
			"姓名：" + strings.TrimSpace(name),
			"电子邮件：" + strings.TrimSpace(email) + phoneInfo + productInfo,
			"主题：" + strings.TrimSpace(subject),
			"",
			"消息内容：",
			strings.TrimSpace(message),
		}
		footer = []string{}
	} else if strings.HasPrefix(l, "en") {
		phoneInfo = ""
		if strings.TrimSpace(phone) != "" {
			phoneInfo = "\nPhone: " + strings.TrimSpace(phone)
		}
		productInfo = ""
		if strings.TrimSpace(product) != "" {
			productInfo = "\nProduct: " + strings.TrimSpace(product)
		}
		title = "New contact form submission"
		preheader = "Received a contact form submission from " + strings.TrimSpace(name)
		greeting = "Hello,"
		intro = []string{
			"New contact form submission details:",
			"",
			"Name: " + strings.TrimSpace(name),
			"Email: " + strings.TrimSpace(email) + phoneInfo + productInfo,
			"Subject: " + strings.TrimSpace(subject),
			"",
			"Message:",
			strings.TrimSpace(message),
		}
		footer = []string{}
	}

	d := templateData{
		HTMLLang:     p.HTMLLang,
		AppName:      c.AppName,
		CompanyName:  strings.TrimSpace(b.CompanyName),
		LogoURL:      strings.TrimSpace(b.LogoURL),
		Title:        title,
		Preheader:    preheader,
		Greeting:     greeting,
		IntroLines:   intro,
		FooterLines:  footer,
		SystemNotice: p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	subjectOut = "【" + c.AppName + "】聯絡表單：" + strings.TrimSpace(subject)
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		subjectOut = "【" + c.AppName + "】联系表单：" + strings.TrimSpace(subject)
	} else if strings.HasPrefix(l, "en") {
		subjectOut = "[" + c.AppName + "] Contact form: " + strings.TrimSpace(subject)
	}
	subjectOut = brandReplace(subjectOut)
	return subjectOut, brandReplace(buildText(d)), brandReplace(html), nil
}

func MemberLevelUpgradeEmail(b Branding, tenantSubdomain string, customerName string, oldLevelName string, newLevelName string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}
	loginURL := buildTenantURL(c.Domain.Scheme, tenantSubdomain, c.Domain.BaseDomain, "/login")

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(customerName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(customerName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "會員等級升級成功"
	preheader := "恭喜！你的會員等級已升級。"
	intro := []string{
		"恭喜！你的會員等級已成功升級。",
		"",
		"原等級：" + strings.TrimSpace(oldLevelName),
		"新等級：" + strings.TrimSpace(newLevelName),
		"",
		"感謝你的支持，繼續享受更多會員專屬優惠！",
	}
	btnText := "前往系統"
	footer := []string{}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = "会员等级升级成功"
		preheader = "恭喜！你的会员等级已升级。"
		intro = []string{
			"恭喜！你的会员等级已成功升级。",
			"",
			"原等级：" + strings.TrimSpace(oldLevelName),
			"新等級：" + strings.TrimSpace(newLevelName),
			"",
			"感谢你的支持，继续享受更多会员专属优惠！",
		}
		btnText = "前往系统"
		footer = []string{}
	} else if strings.HasPrefix(l, "en") {
		title = "Membership level upgraded"
		preheader = "Congrats! Your membership level has been upgraded."
		intro = []string{
			"Congrats! Your membership level has been upgraded successfully.",
			"",
			"Previous level: " + strings.TrimSpace(oldLevelName),
			"New level: " + strings.TrimSpace(newLevelName),
			"",
			"Thank you for your support!",
		}
		btnText = "Go to system"
		footer = []string{}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          loginURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		FooterLines:        footer,
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}
	return brandReplace(d.Title), brandReplace(buildText(d)), brandReplace(html), nil
}

// PromotionEmail generates an EDM/promotion email using the promotion content as HTML body.
// The content is rendered as-is (HTML), with branding header and unsubscribe footer.
func PromotionEmail(b Branding, tenantSubdomain string, customerName string, promotionTitle string, htmlContent string, unsubscribeURL string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(customerName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(customerName) + p.GreetSuffix
	}

	// Build a simple text version by stripping HTML content
	textContent := strings.TrimSpace(htmlContent)
	// Very basic HTML tag removal for text version
	for _, tag := range []string{"<br>", "<br/>", "<br />", "</p>", "</div>", "</li>"} {
		textContent = strings.ReplaceAll(textContent, tag, "\n")
	}
	// Remove remaining HTML tags for text version
	inTag := false
	var textBuf strings.Builder
	for _, ch := range textContent {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			textBuf.WriteRune(ch)
		}
	}
	textContent = strings.TrimSpace(textBuf.String())

	_ = c // used for appName

	// For promotion emails, use a dedicated HTML template with raw content rendering
	const promotionTpl = `<!doctype html>
<html lang="{{.HTMLLang}}">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="x-apple-disable-message-reformatting" />
  <title>{{.Title}}</title>
  <style>
    body{margin:0;padding:40px 0 32px 0;background:#f6f7fb;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,"PingFang TC","Noto Sans TC","Microsoft JhengHei",sans-serif;color:#111827}
    a{color:#2563eb;text-decoration:none}
    .container{max-width:640px;margin:0 auto;padding:0 16px}
    .card{background:#ffffff;border-radius:14px;box-shadow:0 8px 24px rgba(17,24,39,.08);overflow:hidden;border:1px solid #eef2ff}
    .header{padding:12px 18px;background:#ffffff;color:#111827;display:flex;align-items:center;gap:12px;border-bottom:1px solid #eef2ff}
    .logo{height:32px;display:block}
    .brand{font-weight:800;font-size:16px;letter-spacing:.2px}
    .content{padding:18px 18px 16px 18px}
    .h1{margin:0 0 12px 0;font-size:20px;line-height:1.3}
    .p{margin:0 0 10px 0;line-height:1.55;color:#374151;font-size:14px}
    .promo-body{line-height:1.6;color:#374151;font-size:14px}
    .promo-body img{max-width:100%;height:auto;border-radius:8px}
    .muted{color:#6b7280;font-size:12px;line-height:1.45}
    .divider{height:1px;background:#eef2ff;margin:14px 0}
    .footer{padding:14px 18px 18px 18px;background:#f8fafc;border-top:1px solid #eef2ff}
    .preheader{display:none!important;visibility:hidden;opacity:0;color:transparent;height:0;width:0;overflow:hidden;mso-hide:all}
    .unsub{color:#9ca3af;font-size:11px;text-decoration:underline}
  </style>
</head>
<body>
  <div class="preheader">{{.Preheader}}</div>
  <div class="container">
    <div class="card">
      <div class="header">
        {{if .LogoURL}}<img src="{{.LogoURL}}" alt="Logo" class="logo">{{else}}<div class="brand">{{.CompanyName}}</div>{{end}}
      </div>
      <div class="content">
        <div class="h1">{{.Title}}</div>
        {{if .Greeting}}<p class="p">{{.Greeting}}</p>{{end}}
        <div class="promo-body">{{.RawContent}}</div>
      </div>
      <div class="footer">
        <div class="muted">&copy; 2025 {{if .CompanyName}}{{.CompanyName}}{{else}}V-sys Limited{{end}}. All rights reserved.</div>
        {{if .UnsubscribeURL}}<div style="margin-top:8px"><a class="unsub" href="{{.UnsubscribeURL}}">{{.UnsubscribeText}}</a></div>{{end}}
      </div>
    </div>
  </div>
</body>
</html>`

	type promoData struct {
		HTMLLang        string
		Title           string
		Preheader       string
		Greeting        string
		CompanyName     string
		LogoURL         string
		RawContent      template.HTML
		UnsubscribeURL  string
		UnsubscribeText string
	}

	unsubText := "取消訂閱"
	l := strings.ToLower(strings.TrimSpace(lang))
	if strings.HasPrefix(l, "en") {
		unsubText = "Unsubscribe"
	} else if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		unsubText = "取消订阅"
	}

	pd := promoData{
		HTMLLang:        p.HTMLLang,
		Title:           strings.TrimSpace(promotionTitle),
		Preheader:       strings.TrimSpace(promotionTitle),
		Greeting:        greet,
		CompanyName:     strings.TrimSpace(b.CompanyName),
		LogoURL:         strings.TrimSpace(b.LogoURL),
		RawContent:      template.HTML(htmlContent),
		UnsubscribeURL:  strings.TrimSpace(unsubscribeURL),
		UnsubscribeText: unsubText,
	}

	t, err := template.New("promotion").Parse(promotionTpl)
	if err != nil {
		return "", "", "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, pd); err != nil {
		return "", "", "", err
	}

	htmlOut := brandReplace(buf.String())
	subjectOut := brandReplace(strings.TrimSpace(promotionTitle))

	// Build text body
	var tb strings.Builder
	if pd.Title != "" {
		tb.WriteString(pd.Title + "\n\n")
	}
	if greet != "" {
		tb.WriteString(greet + "\n\n")
	}
	tb.WriteString(textContent)
	if strings.TrimSpace(unsubscribeURL) != "" {
		tb.WriteString("\n\n---\n" + unsubText + ": " + unsubscribeURL)
	}
	if strings.TrimSpace(b.CompanyName) != "" {
		tb.WriteString("\n\n© 2025 " + strings.TrimSpace(b.CompanyName) + ". All rights reserved.\n")
	}
	textOut := brandReplace(strings.TrimSpace(tb.String()))

	return subjectOut, textOut, htmlOut, nil
}

// TemplatePreview 用於後台檢視所有內建模板
type TemplatePreview struct {
	Kind    string `json:"kind"`
	Lang    string `json:"lang"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// ListTemplatePreviews 生成所有內建模板的預覽（使用示例資料）
func ListTemplatePreviews() ([]TemplatePreview, error) {
	c, err := GetConfig()
	if err != nil {
		return nil, err
	}
	tenant := "demo"
	res := []TemplatePreview{}

	langs := []string{"zh-hant", "zh-hans", "en"}
	// System emails (welcome, password_reset, contact_form) use system Branding
	systemB := SystemBranding()
	systemB.LogoURL = PublicAssetURL(systemB.LogoURL)
	// Other emails use demo Branding
	demoB := Branding{CompanyName: "Demo Company", LogoURL: ""}

	for _, lang := range langs {
		// welcome - use system Branding
		if s, t, h, err := WelcomeEmail(systemB, tenant, "Demo User", lang); err == nil {
			res = append(res, TemplatePreview{Kind: "welcome", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// password_reset - use system Branding
		if s, t, h, err := PasswordResetEmail(systemB, tenant, "Demo User", buildTenantURL(c.Domain.Scheme, tenant, c.Domain.BaseDomain, "/reset?token=example"), lang); err == nil {
			res = append(res, TemplatePreview{Kind: "password_reset", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// contact_form - use system Branding
		if s, t, h, err := ContactEmail(systemB, "訪客", "guest@example.com", "+852 1234 5678", "vWork", "詢問產品", "這是一段示例訊息。", lang); err == nil {
			res = append(res, TemplatePreview{Kind: "contact_form", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// member_level_upgrade - use demo Branding (tenant-specific)
		if s, t, h, err := MemberLevelUpgradeEmail(demoB, tenant, "Demo Member", "Silver", "Gold", lang); err == nil {
			res = append(res, TemplatePreview{Kind: "member_level_upgrade", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// trial_limit_exceeded - use system Branding (system email)
		if s, t, h, err := TrialLimitExceededEmail(systemB, tenant, "Demo User", time.Now().Add(168*time.Hour), lang); err == nil {
			res = append(res, TemplatePreview{Kind: "trial_limit_exceeded", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// customer_invite - use demo Branding (tenant-specific)
		if s, t, h, err := CustomerInviteEmail(demoB, tenant, "Demo Customer", buildTenantURL(c.Domain.Scheme, tenant, c.Domain.BaseDomain, "/co/demo/login?token=example"), lang); err == nil {
			res = append(res, TemplatePreview{Kind: "customer_invite", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// order_confirmation - use demo Branding (tenant-specific)
		if s, t, h, err := OrderConfirmationEmail(demoB, tenant, "Demo Customer", "A0001", "2025-12-24", 123.45, "order", lang); err == nil {
			res = append(res, TemplatePreview{Kind: "order_confirmation", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// invoice - use demo Branding (tenant-specific)
		if s, t, h, err := InvoiceEmail(demoB, tenant, "Demo Customer", "INV-20251224-001", 123.45, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "invoice", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// user_invite - use demo Branding (tenant-specific)
		if s, t, h, err := UserInviteEmail(demoB, tenant, "Demo Company", "Demo User", buildTenantURL(c.Domain.Scheme, tenant, c.Domain.BaseDomain, "/invite?token=example"), lang); err == nil {
			res = append(res, TemplatePreview{Kind: "user_invite", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// promotion - use demo Branding (tenant-specific)
		if s, t, h, err := PromotionEmail(demoB, tenant, "Demo Customer", "示例推廣活動", "<p>這是一封示例推廣郵件。<br>歡迎查看我們的最新優惠！</p>", "https://example.com/unsubscribe?token=demo", lang); err == nil {
			res = append(res, TemplatePreview{Kind: "promotion", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// admin_notification (new_registration) - use system Branding
		demoRegDetails := map[string]string{
			"tenant_name": "Demo Company",
			"subdomain":   "demo",
			"user_name":   "Demo User",
			"user_email":  "demo@example.com",
			"timestamp":   "2026-03-17 12:00:00",
		}
		if s, t, h, err := AdminNotificationEmail(systemB, "new_registration", demoRegDetails, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "admin_notification:new_registration", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// admin_notification (new_subscription) - use system Branding
		demoSubDetails := map[string]string{
			"tenant_name": "Demo Company",
			"subdomain":   "demo",
			"plan_name":   "Pro Plan",
			"status":      "active",
			"timestamp":   "2026-03-17 12:00:00",
		}
		if s, t, h, err := AdminNotificationEmail(systemB, "new_subscription", demoSubDetails, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "admin_notification:new_subscription", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// admin_notification (vcoin_purchase) - use system Branding
		demoCoinDetails := map[string]string{
			"tenant_name": "Demo Company",
			"subdomain":   "demo",
			"coins":       "500",
			"amount":      "$49.99",
			"timestamp":   "2026-03-17 12:00:00",
		}
		if s, t, h, err := AdminNotificationEmail(systemB, "vcoin_purchase", demoCoinDetails, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "admin_notification:vcoin_purchase", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// admin_notification (auto_outreach_quota_exceeded) - use system Branding
		demoQuotaDetails := map[string]string{
			"tenant_name": "Demo Company",
			"subdomain":   "demo",
			"daily_usage": "305",
			"daily_limit": "300",
			"timestamp":   "2026-03-17 12:00:00",
		}
		if s, t, h, err := AdminNotificationEmail(systemB, "auto_outreach_quota_exceeded", demoQuotaDetails, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "admin_notification:auto_outreach_quota_exceeded", Lang: lang, Subject: s, Text: t, HTML: h})
		}
		// admin_notification (serper_credit_exhausted) - use system Branding
		demoSerperDetails := map[string]string{
			"tenant_name": "Demo Company",
			"subdomain":   "demo",
			"timestamp":   "2026-03-17 12:00:00",
		}
		if s, t, h, err := AdminNotificationEmail(systemB, "serper_credit_exhausted", demoSerperDetails, lang); err == nil {
			res = append(res, TemplatePreview{Kind: "admin_notification:serper_credit_exhausted", Lang: lang, Subject: s, Text: t, HTML: h})
		}
	}
	return res, nil
}

func CustomerInviteEmail(b Branding, tenantSubdomain string, customerName string, inviteURL string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(customerName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(customerName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "網店登入邀請"
	preheader := "點擊連結設定密碼並登入網店。"
	intro := []string{
		"歡迎加入我們的網店！",
		"請點擊下面按鈕設定您的登入密碼，完成後即可使用網店功能。",
	}
	btnText := "設定密碼"
	outro := []string{
		"此連結具有時效性，請盡快完成設定。",
		"如果你沒有收到此邀請，請忽略此郵件。",
	}
	footer := []string{}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = "网店登录邀请"
		preheader = "点击链接设置密码并登录网店。"
		intro = []string{
			"欢迎加入我们的网店！",
			"请点击下面按钮设置你的登录密码，完成后即可使用网店功能。",
		}
		btnText = "设置密码"
		outro = []string{
			"此链接具有时效性，请尽快完成设置。",
			"如果你没有收到此邀请，请忽略此邮件。",
		}
		footer = []string{}
	} else if strings.HasPrefix(l, "en") {
		title = "Online store login invitation"
		preheader = "Set a password to access the online store."
		intro = []string{
			"Welcome!",
			"Click the button below to set your password and log in to the online store.",
		}
		btnText = "Set password"
		outro = []string{
			"This link expires soon. Please complete it as soon as possible.",
			"If you weren't expecting this invitation, you can ignore this email.",
		}
		footer = []string{}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          inviteURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		OutroLines:         outro,
		FooterLines:        footer,
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	return brandReplace(d.Title), brandReplace(buildText(d)), brandReplace(html), nil
}

// UserInviteEmail generates email content for inviting a user to join a tenant
func UserInviteEmail(b Branding, tenantSubdomain string, tenantName string, userName string, inviteURL string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(userName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(userName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	title := "邀請您加入 " + strings.TrimSpace(tenantName)
	preheader := "您已被邀請加入團隊，請設定密碼以完成註冊。"
	intro := []string{
		"您已被邀請加入「" + strings.TrimSpace(tenantName) + "」團隊！",
		"請點擊下面按鈕設定您的登入密碼，您也可以選擇使用 Google 帳號登入。",
	}
	btnText := "設定密碼"
	outro := []string{
		"此連結在 7 天內有效，請盡快完成設定。",
		"如果您不認識此團隊，請忽略此郵件。",
	}
	footer := []string{}
	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = "邀请您加入 " + strings.TrimSpace(tenantName)
		preheader = "您已被邀请加入团队，请设置密码以完成注册。"
		intro = []string{
			"您已被邀请加入「" + strings.TrimSpace(tenantName) + "」团队！",
			"请点击下面按钮设置您的登录密码，您也可以选择使用 Google 帐号登录。",
		}
		btnText = "设置密码"
		outro = []string{
			"此链接在 7 天内有效，请尽快完成设置。",
			"如果您不认识此团队，请忽略此邮件。",
		}
		footer = []string{}
	} else if strings.HasPrefix(l, "en") {
		title = "You are invited to join " + strings.TrimSpace(tenantName)
		preheader = "You have been invited to join a team. Please set your password to complete registration."
		intro = []string{
			"You have been invited to join \"" + strings.TrimSpace(tenantName) + "\"!",
			"Click the button below to set your login password. You can also choose to sign in with Google.",
		}
		btnText = "Set Password"
		outro = []string{
			"This link is valid for 7 days. Please complete the setup as soon as possible.",
			"If you don't recognize this team, please ignore this email.",
		}
		footer = []string{}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          inviteURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		OutroLines:         outro,
		FooterLines:        footer,
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	return brandReplace(d.Title), brandReplace(buildText(d)), brandReplace(html), nil
}

// TenantInviteEmail builds the email for inviting someone to join a tenant.
// The invite link goes to /accept-invite which handles both new and existing users.
func TenantInviteEmail(b Branding, tenantName string, inviterName string, inviteURL string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor(lang)
	// No personal greeting since we don't know the recipient's name
	greet := ""

	l := strings.ToLower(strings.TrimSpace(lang))
	tn := strings.TrimSpace(tenantName)
	inv := strings.TrimSpace(inviterName)

	title := inv + " 邀請您加入 " + tn
	preheader := "您已被邀請加入「" + tn + "」，請點擊連結接受邀請。"
	intro := []string{
		inv + " 邀請您加入「" + tn + "」！",
		"如果您已有 vWork 帳號，請點擊下方按鈕登入並加入團隊。",
		"如果您尚未註冊，點擊按鈕後會引導您完成註冊，之後自動加入團隊。",
	}
	btnText := "接受邀請"
	outro := []string{
		"此連結在 7 天內有效，請盡快接受邀請。",
		"如果您不認識邀請者，請忽略此郵件。",
	}

	if strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg") {
		title = inv + " 邀请您加入 " + tn
		preheader = "您已被邀请加入「" + tn + "」，请点击链接接受邀请。"
		intro = []string{
			inv + " 邀请您加入「" + tn + "」！",
			"如果您已有 vWork 帐号，请点击下方按钮登录并加入团队。",
			"如果您尚未注册，点击按钮后会引导您完成注册，之后自动加入团队。",
		}
		btnText = "接受邀请"
		outro = []string{
			"此链接在 7 天内有效，请尽快接受邀请。",
			"如果您不认识邀请者，请忽略此邮件。",
		}
	} else if strings.HasPrefix(l, "en") {
		title = inv + " invited you to join " + tn
		preheader = "You have been invited to join \"" + tn + "\". Click the link to accept."
		intro = []string{
			inv + " invited you to join \"" + tn + "\"!",
			"If you already have a vWork account, click the button below to sign in and join the team.",
			"If you don't have an account yet, clicking the button will guide you through registration and automatically add you to the team.",
		}
		btnText = "Accept Invitation"
		outro = []string{
			"This link is valid for 7 days. Please accept the invitation as soon as possible.",
			"If you don't recognize the sender, please ignore this email.",
		}
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          inviteURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		OutroLines:         outro,
		FooterLines:        []string{},
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	return brandReplace(d.Title), brandReplace(buildText(d)), brandReplace(html), nil
}

func InvoiceEmail(b Branding, tenantSubdomain string, customerName string, invoiceNumber string, amount float64, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}
	loginURL := buildTenantURL(c.Domain.Scheme, tenantSubdomain, c.Domain.BaseDomain, "/login")

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(customerName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(customerName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	isEn := strings.HasPrefix(l, "en")
	isHans := strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg")

	title := "您的發票已經準備好了"
	preheader := "請查收您的發票詳情。"
	intro := []string{
		"您的發票已經準備好了，詳情如下：",
		"",
		"發票編號：" + strings.TrimSpace(invoiceNumber),
		fmt.Sprintf("支付金額：$%.2f", amount),
		"",
		"發票 PDF 已作為附件發送到此郵件。",
		"",
		"感謝您的支持！",
	}
	btnText := "查看詳情"
	subjectPrefix := "【" + c.AppName + "】您的發票："

	if isHans {
		title = "您的发票已经准备好了"
		preheader = "请查收您的发票详情。"
		intro = []string{
			"您的发票已经准备好了，详情如下：",
			"",
			"发票编号：" + strings.TrimSpace(invoiceNumber),
			fmt.Sprintf("支付金额：$%.2f", amount),
			"",
			"发票 PDF 已作为附件发送到此邮件。",
			"",
			"感谢您的支持！",
		}
		btnText = "查看详情"
		subjectPrefix = "【" + c.AppName + "】您的发票："
	} else if isEn {
		title = "Your invoice is ready"
		preheader = "Please find your invoice details below."
		intro = []string{
			"Your invoice is ready. Details:",
			"",
			"Invoice Number: " + strings.TrimSpace(invoiceNumber),
			fmt.Sprintf("Amount Paid: $%.2f", amount),
			"",
			"The invoice PDF has been attached to this email.",
			"",
			"Thank you for your business!",
		}
		btnText = "View Details"
		subjectPrefix = "[" + c.AppName + "] Your Invoice: "
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          loginURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		FooterLines:        []string{},
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	subject = subjectPrefix + strings.TrimSpace(invoiceNumber)
	return brandReplace(subject), brandReplace(buildText(d)), brandReplace(html), nil
}

// AdminNotificationEmail generates an admin notification email for events like
// new registration, new subscription, vCoin purchase, auto outreach quota exceeded,
// or serper credit exhausted.
// eventType: "new_registration", "new_subscription", "vcoin_purchase",
// "auto_outreach_quota_exceeded", "serper_credit_exhausted"
func AdminNotificationEmail(b Branding, eventType string, details map[string]string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}

	p := phrasesFor("zh-hant") // admin emails always in zh-hant

	// Build event-specific content
	var title, preheader string
	var intro []string

	switch eventType {
	case "new_registration":
		tenantName := details["tenant_name"]
		userName := details["user_name"]
		userEmail := details["user_email"]
		subdomain := details["subdomain"]
		title = "新租戶註冊通知"
		preheader = "有新租戶完成註冊：" + tenantName
		intro = []string{
			"有新租戶完成註冊，詳情如下：",
			"",
			"租戶名稱：" + tenantName,
			"子域名：" + subdomain,
			"用戶名：" + userName,
			"Email：" + userEmail,
			"",
			"時間：" + details["timestamp"],
		}
	case "new_subscription":
		tenantName := details["tenant_name"]
		planName := details["plan_name"]
		subdomain := details["subdomain"]
		title = "新訂閱通知"
		preheader = tenantName + " 已訂閱 " + planName
		intro = []string{
			"有租戶完成訂閱，詳情如下：",
			"",
			"租戶名稱：" + tenantName,
			"子域名：" + subdomain,
			"訂閱方案：" + planName,
			"狀態：" + details["status"],
			"",
			"時間：" + details["timestamp"],
		}
	case "vcoin_purchase":
		tenantName := details["tenant_name"]
		coins := details["coins"]
		amount := details["amount"]
		title = "vCoin 購買通知"
		preheader = tenantName + " 購買了 " + coins + " vCoins"
		intro = []string{
			"有租戶完成 vCoin 購買，詳情如下：",
			"",
			"租戶名稱：" + tenantName,
			"子域名：" + details["subdomain"],
			"購買數量：" + coins + " vCoins",
			"支付金額：" + amount,
			"",
			"時間：" + details["timestamp"],
		}
	case "auto_outreach_quota_exceeded":
		tenantName := details["tenant_name"]
		title = "自動外展每日配額超出"
		preheader = tenantName + " 的自動外展每日配額已超出"
		intro = []string{
			"租戶的自動外展每日 email 配額已超出，詳情如下：",
			"",
			"租戶名稱：" + tenantName,
			"子域名：" + details["subdomain"],
		}
		if details["daily_usage"] != "" {
			intro = append(intro, "今日已發送："+details["daily_usage"])
		}
		if details["daily_limit"] != "" {
			intro = append(intro, "每日上限："+details["daily_limit"])
		}
		intro = append(intro, "", "時間："+details["timestamp"])
	case "serper_credit_exhausted":
		title = "Serper 搜尋額度耗盡"
		preheader = "Serper.dev 搜尋額度已用完"
		intro = []string{
			"Serper.dev 搜尋 API 額度已耗盡，Lead Finder 及自動外展搜尋功能將暫停。",
			"",
			"請儘快充值或更換 API Key。",
		}
		if details["tenant_name"] != "" {
			intro = append(intro, "", "觸發租戶："+details["tenant_name"])
		}
		if details["subdomain"] != "" {
			intro = append(intro, "子域名："+details["subdomain"])
		}
		intro = append(intro, "", "時間："+details["timestamp"])
	default:
		title = "管理通知"
		preheader = "vWork 管理系統通知"
		intro = []string{"收到一個管理事件通知。"}
		for k, v := range details {
			intro = append(intro, k+"："+v)
		}
	}

	d := templateData{
		HTMLLang:     p.HTMLLang,
		AppName:      c.AppName,
		CompanyName:  strings.TrimSpace(b.CompanyName),
		LogoURL:      strings.TrimSpace(b.LogoURL),
		Title:        title,
		Preheader:    preheader,
		Greeting:     "管理員你好，",
		IntroLines:   intro,
		FooterLines:  []string{},
		SystemNotice: p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	subjectOut := "【" + c.AppName + "】" + title
	return brandReplace(subjectOut), brandReplace(buildText(d)), brandReplace(html), nil
}

func OrderConfirmationEmail(b Branding, tenantSubdomain string, customerName string, orderNumber string, orderDate string, totalAmount float64, orderType string, lang string) (subject, textBody, htmlBody string, err error) {
	c, err := GetConfig()
	if err != nil {
		return "", "", "", err
	}
	loginURL := buildTenantURL(c.Domain.Scheme, tenantSubdomain, c.Domain.BaseDomain, "/login")

	p := phrasesFor(lang)
	greet := ""
	if strings.TrimSpace(customerName) != "" {
		greet = p.GreetPrefix + strings.TrimSpace(customerName) + p.GreetSuffix
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	isEn := strings.HasPrefix(l, "en")
	isHans := strings.Contains(l, "hans") || strings.HasPrefix(l, "zh-cn") || strings.HasPrefix(l, "zh-sg")
	orderTypeText := "訂單"
	viewText := "查看"
	if orderType == "service_order" {
		orderTypeText = "服務單"
	}
	if isHans {
		orderTypeText = "订单"
		viewText = "查看"
		if orderType == "service_order" {
			orderTypeText = "服务单"
		}
	} else if isEn {
		orderTypeText = "Order"
		viewText = "View"
		if orderType == "service_order" {
			orderTypeText = "Service order"
		}
	}

	title := orderTypeText + "確認"
	preheader := "你的" + orderTypeText + "已確認，感謝你的支持。"
	intro := []string{
		"你的" + orderTypeText + "已確認，詳情如下：",
		"",
		"編號：" + strings.TrimSpace(orderNumber),
		"日期：" + strings.TrimSpace(orderDate),
		fmt.Sprintf("總金額：$%.2f", totalAmount),
		"",
		"感謝你的支持！",
	}
	btnText := viewText + orderTypeText
	subjectPrefix := "【" + c.AppName + "】" + orderTypeText + "確認："
	if isHans {
		title = orderTypeText + "确认"
		preheader = "你的" + orderTypeText + "已确认，感谢你的支持。"
		intro = []string{
			"你的" + orderTypeText + "已确认，详情如下：",
			"",
			"编号：" + strings.TrimSpace(orderNumber),
			"日期：" + strings.TrimSpace(orderDate),
			fmt.Sprintf("总金额：$%.2f", totalAmount),
			"",
			"感谢你的支持！",
		}
		btnText = viewText + orderTypeText
		subjectPrefix = "【" + c.AppName + "】" + orderTypeText + "确认："
	} else if isEn {
		title = orderTypeText + " confirmation"
		preheader = "Your " + orderTypeText + " has been confirmed. Thank you."
		intro = []string{
			"Your " + orderTypeText + " has been confirmed. Details:",
			"",
			"Number: " + strings.TrimSpace(orderNumber),
			"Date: " + strings.TrimSpace(orderDate),
			fmt.Sprintf("Total: $%.2f", totalAmount),
			"",
			"Thank you!",
		}
		btnText = viewText + " " + orderTypeText
		subjectPrefix = "【" + c.AppName + "】" + orderTypeText + " confirmation: "
	}

	d := templateData{
		HTMLLang:           p.HTMLLang,
		AppName:            c.AppName,
		CompanyName:        strings.TrimSpace(b.CompanyName),
		LogoURL:            strings.TrimSpace(b.LogoURL),
		Title:              title,
		Preheader:          preheader,
		Greeting:           greet,
		IntroLines:         intro,
		ButtonText:         btnText,
		ButtonURL:          loginURL,
		ButtonFallbackHelp: p.ButtonFallbackHelp,
		FooterLines:        []string{},
		SystemNotice:       p.SystemNotice,
	}

	html, err := renderHTML(d)
	if err != nil {
		return "", "", "", err
	}

	subject = subjectPrefix + strings.TrimSpace(orderNumber)
	return brandReplace(subject), brandReplace(buildText(d)), brandReplace(html), nil
}
