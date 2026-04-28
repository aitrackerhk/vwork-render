## Email to Cloudflare - Domain Connect template onboarding

To: `domain-connect@cloudflare.com`
Subject: Domain Connect template onboarding request - vwork / www-cname

Hello Cloudflare team,

We would like to onboard a Domain Connect template for our SaaS platform.

- Provider ID: `vwork`
- Template ID (serviceId): `www-cname`
- Template link (GitHub): <PASTE_TEMPLATE_URL_HERE>
- Purpose: Create a single CNAME record for customer domains (www only)
  - `www` -> CNAME -> `cname.vworkai.com`
- Default proxy status for A/AAAA/CNAME: DNS only (please confirm if you need a specific default)
- Logo (SVG): <ATTACH_OR_LINK_LOGO_SVG>
- Test Cloudflare account ID (optional): <PASTE_TEST_ACCOUNT_ID_IF_NEEDED>

Notes:
- We only support `www` subdomain. We do not configure apex records.
- SSL is handled by Cloudflare for SaaS / SSL for SaaS.

Thank you!
<YOUR_NAME>
<YOUR_COMPANY>
<YOUR_EMAIL>


