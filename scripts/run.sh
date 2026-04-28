#!/usr/bin/env bash
set -euo pipefail

# Run this script in **Google Cloud Shell** (recommended) so gcloud is already authenticated.
#
# What it does (fast path, not security-hardened):
# - SSH into a single GCE VM, install deps (postgres, golang)
# - Download code zip + SQL from Google Drive (requires "Anyone with the link" sharing)
# - Unzip to /opt/vwork, write .env, restore DB, build Go binaries
# - Create External HTTPS Load Balancer + Google-managed certificate (Google Trust Services)
#
# You MUST do manually:
# - Share Google Drive files: "Anyone with the link" -> Viewer
# - Point DNS A record of DOMAIN to the created LB IP

########################################
# OPTIONAL: load local config file
########################################
# You can put your settings into a file and avoid typing exports every time:
#   cp deploy.local.env.example deploy.local.env
#   nano deploy.local.env
# Then run:
#   bash deploy_gcp_n2d_from_drive_cloudshell.sh
#
# Or specify:
#   CONFIG_FILE=/path/to/deploy.local.env bash deploy_gcp_n2d_from_drive_cloudshell.sh
CONFIG_FILE="${CONFIG_FILE:-./deploy.local.env}"
if [[ -f "${CONFIG_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
fi

########################################
# REQUIRED: fill these before running
########################################
# Defaults are hard-coded for your environment (you can still override by exporting vars before running).
PROJECT_ID="${PROJECT_ID:-avid-keel-482217-u4}"      # e.g. my-gcp-project
REGION="${REGION:-asia-east1}"    # used for convenience; LB is global
ZONE="${ZONE:-asia-southeast1-b}"
VM_NAME="${VM_NAME:-vwork}"           # e.g. nwork-vm-1
DOMAIN="${DOMAIN:-www.vworkai.com}"   # e.g. example.com
ADMIN_EMAIL="${ADMIN_EMAIL:-info@vworkai.com}" # only used for messages; not sent anywhere

# Optional: Second domain for vsysai.com (will create additional SSL certificate)
# If set, both domains will use the same Load Balancer but with separate SSL certificates
VSYSAI_DOMAIN="${VSYSAI_DOMAIN:-www.vsysai.com}"  # e.g. www.vsysai.com (set to empty "" to disable)

# Production sub-domains (all point to the same LB backend / VM / port)
VOFFICE_DOMAIN="${VOFFICE_DOMAIN:-voffice.vsysai.com}"
VAI_DOMAIN="${VAI_DOMAIN:-vai.vsysai.com}"
VMARKET_DOMAIN="${VMARKET_DOMAIN:-www.vmarketai.com}"

# Google Drive file IDs from your links:
CODE_FILE_ID="${CODE_FILE_ID:-1b5jLbc_7Z-XndU2-SL2lwWCKHipZJL2Z}"

# App/DB settings (fast defaults)
APP_DIR="${APP_DIR:-/opt/vwork}"
APP_PORT="${APP_PORT:-3001}"
DB_NAME="${DB_NAME:-vwork}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"   # not secure; per your request

# Performance defaults (extreme-ish, safe toggles)
APP_ENV="${APP_ENV:-production}"
FIBER_PREFORK="${FIBER_PREFORK:-true}"
APP_REQUEST_LOGGER="${APP_REQUEST_LOGGER:-false}"
GORM_LOG_LEVEL="${GORM_LOG_LEVEL:-warn}"

# Backup before applying DB changes (recommended)
BACKUP_BEFORE_APPLY="${BACKUP_BEFORE_APPLY:-true}"
# Optional: backup to GCS bucket (e.g. gs://my-backups). Leave empty to skip upload.
BACKUP_GCS_BUCKET="${BACKUP_GCS_BUCKET:-}"
# Local backup directory on VM
BACKUP_DIR="${BACKUP_DIR:-/var/backups/vwork}"

# Load balancer names (must be globally unique-ish within project)
LB_NAME="${LB_NAME:-vwork-lb}"

# APP 
APP_NAME=vWork
COMPANY_NAME="V-sys Limited"
GEONAMES_USERNAME=tednv88

# --- vWorkAdmin ---
VWORK_ADMIN_USER="${VWORK_ADMIN_USER:-vadmin}"
VWORK_ADMIN_PASS="${VWORK_ADMIN_PASS:-Vvhk_96340115!}"

# --- SMTP (Brevo) ---
SMTP_HOST=${SMTP_HOST:-smtp-relay.brevo.com}
SMTP_PORT=${SMTP_PORT:-587}
SMTP_USE_STARTTLS=${SMTP_USE_STARTTLS:-true}
SMTP_INSECURE_SKIP_VERIFY_TLS=${SMTP_INSECURE_SKIP_VERIFY_TLS:-false}

# Brevo SMTP credentials
SMTP_USER=${SMTP_USER:-9f0077001@smtp-brevo.com}
SMTP_PASSWORD=${SMTP_PASSWORD:-xsmtpsib-0a2cfb1d137b629bd7a568331b7c9bd8ce1beca2082a326e94e1e56dd1a4309b-4DmELMju6vz3ZF0F}

# Sender information
SMTP_FROM_EMAIL=${SMTP_FROM_EMAIL:-no-reply@mail.vworkai.com}
SMTP_FROM_NAME=${SMTP_FROM_NAME:-vWork}
CONTACT_EMAIL=${CONTACT_EMAIL:-}

STRIPE_SECRET_KEY="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"
STRIPE_PUBLISHABLE_KEY="pk_live_51PHqrlERiBpo86aEtQdpgeN3ItfRl5Vaek9DxeIeC8H8rn5ArrMh7ckmoulZ7frZzuikOQ1rd9sl71mHvUgmb8RJ00LykMoCPW"
STRIPE_WEBHOOK_SECRET="whsec_RrRvPDUJf5OYt3ECHMT5hhMvI0AMVCYS"
STRIPE_PRICE_MONTHLY="price_1SkHYTERiBpo86aEUKhbvLxy"
STRIPE_PRICE_YEARLY="price_1SkHZLERiBpo86aE5puFKEHh"
STRIPE_PRICE_MONTHLY_PRO="price_1T48isERiBpo86aEWlZZCTa9"
STRIPE_PRICE_YEARLY_PRO="price_1T48iSERiBpo86aE64YWDbFM"
STRIPE_PRICE_MONTHLY_PRO_PLUS="price_1TFKdZERiBpo86aEYqCes0F9"
STRIPE_PRICE_YEARLY_PRO_PLUS="price_1TFKdeERiBpo86aE2VoZzGJt"
STRIPE_SUCCESS_URL="https://www.vworkai.com/billing?checkout=success"
STRIPE_CANCEL_URL="https://www.vworkai.com/billing?checkout=cancel"
BASE_DOMAIN=${BASE_DOMAIN:-vworkai.com}
PUBLIC_SCHEME=${PUBLIC_SCHEME:-https}

# --- LLM (Gemini API) ---
LLM_API_KEY="AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
LLM_MODEL="gemini-2.5-flash"
LLM_IMAGE_MODEL="gemini-3-pro-image-preview"
LLM_PROVIDER="gemini"
LLM_ENDPOINT=""

# --- BytePlus ModelArk (Seedance 1.5 Pro video generation) ---
ARK_API_KEY="2f47f48a-53d9-4d68-8a3c-3bb194dce088"
ARK_ENDPOINT_ID="seedance-1-5-pro-251215"

# --- Kling (Video Generation) ---
KLING_ACCESS_KEY="AJ93hdmYK49fFGPgnaFbgKhdFbrKnAp3"
KLING_SECRET_KEY="8nCgmLQ3rHT9Eegk3C8MTmDdhTHyRJAH"
KLING_MODEL="kling-v3-omni"
KLING_BASE_URL="https://api-singapore.klingai.com"

# --- Google Cloud Vision API ---
GOOGLE_VISION_API_KEY="AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
GOOGLE_CLOUD_API_KEY="AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
GOOGLE_CLOUD_PROJECT_ID="${GOOGLE_CLOUD_PROJECT_ID:-}"

# --- Google Maps API ---
GOOGLE_MAPS_API_KEY="AIzaSyDLD6pIeBSfOD57-PPTvZ0EdgzHKJuFJ5k"

# --- Google OAuth 2.0 (Login/Register) ---
GOOGLE_OAUTH_CLIENT_ID="150732813516-90v6kgagrtcoagdrppr7o0beo69b7f96.apps.googleusercontent.com"
GOOGLE_OAUTH_CLIENT_SECRET="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"
GOOGLE_OAUTH_ENABLED="true"

# --- Google Search (Custom Search / Serper) ---
GOOGLE_SEARCH_API_KEY="AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
GOOGLE_SEARCH_ENGINE_ID="d1c5071e429864c28"
SERPER_API_KEY="c2a17f0c392e6997088008e3baaf0d568ea3c80f"

# --- Google AdSense ---
GOOGLE_ADSENSE_PUBLISHER_ID="ca-pub-5412668204831240"

########################################
# helpers
########################################
die() { echo "ERROR: $*" 1>&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing command: $1"; }

# If you override to empty, fail fast.
if [[ -z "${PROJECT_ID}" ]]; then die "PROJECT_ID is empty"; fi
if [[ -z "${VM_NAME}" ]]; then die "VM_NAME is empty"; fi
if [[ -z "${DOMAIN}" ]]; then die "DOMAIN is empty"; fi

need gcloud

echo "==> Using project=${PROJECT_ID}, zone=${ZONE}, vm=${VM_NAME}, domain=${DOMAIN}"
gcloud config set project "${PROJECT_ID}" >/dev/null

echo "==> 1) Prepare VM (install deps, download code, build, systemd)"
echo "==> Backup before apply: ${BACKUP_BEFORE_APPLY} (bucket: ${BACKUP_GCS_BUCKET:-none})"
echo "==> NOTE: Database is NEVER dropped/overwritten. Only safe additive migrations are applied."
cat <<'REMOTE' | gcloud compute ssh "${VM_NAME}" --zone "${ZONE}" --quiet --command "export APP_DIR='${APP_DIR}' APP_PORT='${APP_PORT}' DB_NAME='${DB_NAME}' DB_USER='${DB_USER}' DB_PASSWORD='${DB_PASSWORD}' DOMAIN='${DOMAIN}' CODE_FILE_ID='${CODE_FILE_ID}' APP_ENV='${APP_ENV}' FIBER_PREFORK='${FIBER_PREFORK}' APP_REQUEST_LOGGER='${APP_REQUEST_LOGGER}' GORM_LOG_LEVEL='${GORM_LOG_LEVEL}' BACKUP_BEFORE_APPLY='${BACKUP_BEFORE_APPLY}' BACKUP_GCS_BUCKET='${BACKUP_GCS_BUCKET}' BACKUP_DIR='${BACKUP_DIR}' STRIPE_SECRET_KEY='${STRIPE_SECRET_KEY}' STRIPE_PUBLISHABLE_KEY='${STRIPE_PUBLISHABLE_KEY}' STRIPE_WEBHOOK_SECRET='${STRIPE_WEBHOOK_SECRET}' STRIPE_PRICE_MONTHLY='${STRIPE_PRICE_MONTHLY}' STRIPE_PRICE_YEARLY='${STRIPE_PRICE_YEARLY}' STRIPE_PRICE_MONTHLY_PRO='${STRIPE_PRICE_MONTHLY_PRO}' STRIPE_PRICE_YEARLY_PRO='${STRIPE_PRICE_YEARLY_PRO}' STRIPE_PRICE_MONTHLY_PRO_PLUS='${STRIPE_PRICE_MONTHLY_PRO_PLUS}' STRIPE_PRICE_YEARLY_PRO_PLUS='${STRIPE_PRICE_YEARLY_PRO_PLUS}' STRIPE_SUCCESS_URL='${STRIPE_SUCCESS_URL}' STRIPE_CANCEL_URL='${STRIPE_CANCEL_URL}' BASE_DOMAIN='${BASE_DOMAIN}' PUBLIC_SCHEME='${PUBLIC_SCHEME}' LLM_API_KEY='${LLM_API_KEY}' LLM_MODEL='${LLM_MODEL}' LLM_IMAGE_MODEL='${LLM_IMAGE_MODEL}' LLM_PROVIDER='${LLM_PROVIDER}' LLM_ENDPOINT='${LLM_ENDPOINT}' ARK_API_KEY='${ARK_API_KEY}' ARK_ENDPOINT_ID='${ARK_ENDPOINT_ID}' KLING_ACCESS_KEY='${KLING_ACCESS_KEY}' KLING_SECRET_KEY='${KLING_SECRET_KEY}' KLING_MODEL='${KLING_MODEL}' KLING_BASE_URL='${KLING_BASE_URL}' GOOGLE_VISION_API_KEY='${GOOGLE_VISION_API_KEY}' GOOGLE_CLOUD_API_KEY='${GOOGLE_CLOUD_API_KEY}' GOOGLE_CLOUD_PROJECT_ID='${GOOGLE_CLOUD_PROJECT_ID}' GOOGLE_OAUTH_CLIENT_ID='${GOOGLE_OAUTH_CLIENT_ID}' GOOGLE_OAUTH_CLIENT_SECRET='${GOOGLE_OAUTH_CLIENT_SECRET}' GOOGLE_OAUTH_ENABLED='${GOOGLE_OAUTH_ENABLED}' GOOGLE_MAPS_API_KEY='${GOOGLE_MAPS_API_KEY}' GOOGLE_SEARCH_API_KEY='${GOOGLE_SEARCH_API_KEY}' GOOGLE_SEARCH_ENGINE_ID='${GOOGLE_SEARCH_ENGINE_ID}' SERPER_API_KEY='${SERPER_API_KEY}' GOOGLE_ADSENSE_PUBLISHER_ID='${GOOGLE_ADSENSE_PUBLISHER_ID}' VSYSAI_DOMAIN='${VSYSAI_DOMAIN}' VWORK_ADMIN_USER='${VWORK_ADMIN_USER}' VWORK_ADMIN_PASS='${VWORK_ADMIN_PASS}'; bash -s" 
set -euo pipefail

echo "[VM] installing packages..."
sudo apt update -y
sudo apt install -y postgresql postgresql-contrib unzip curl ca-certificates golang-go python3-venv file gzip

echo "[VM] installing Chinese fonts for PDF generation..."
# Install fontconfig and Noto CJK fonts (supports Traditional/Simplified Chinese, Japanese, Korean)
sudo apt install -y fontconfig fonts-noto-cjk || true

# Also try to manually install if package installation didn't work
# Create font directories if they don't exist
sudo mkdir -p /usr/share/fonts/truetype/noto
sudo mkdir -p /usr/share/fonts/opentype/noto

# Try to find and link any installed Noto CJK fonts
if command -v fc-list >/dev/null 2>&1; then
  # Find installed Noto fonts and create symlinks in expected locations
  NOTO_FONTS=$(fc-list : family | grep -i "noto.*cjk" | head -1 || true)
  if [ -n "$NOTO_FONTS" ]; then
    echo "[VM] Found Noto CJK fonts via fc-list"
    # Use fc-list to find actual font file paths
    NOTO_FONT_FILE=$(fc-list : file | grep -i "noto.*cjk.*\.\(ttf\|ttc\|otf\)" | head -1 || true)
    if [ -n "$NOTO_FONT_FILE" ] && [ -f "$NOTO_FONT_FILE" ]; then
      # Create symlink in expected location
      sudo ln -sf "$NOTO_FONT_FILE" /usr/share/fonts/truetype/noto/ 2>/dev/null || true
      echo "[VM] Created symlink for font: $NOTO_FONT_FILE"
    fi
  fi
fi

# Try to download font directly if still not found
# Check for various Noto CJK font variants
FONT_EXISTS=false
for check_path in \
  /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc \
  /usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf \
  /usr/share/fonts/truetype/noto/NotoSansSC-VF.ttf \
  /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc \
  /usr/share/fonts/opentype/noto/NotoSansTC-Regular.otf; do
  if [ -f "$check_path" ]; then
    FONT_EXISTS=true
    echo "[VM] Font already exists: $check_path"
    break
  fi
done

if [ "$FONT_EXISTS" = false ]; then
  echo "[VM] Font not found in expected locations, attempting manual download..."
  cd /tmp
  # Download NotoSansTC-VF.ttf (Traditional Chinese, smaller and more compatible)
  FONT_URL="https://github.com/notofonts/noto-cjk/raw/main/Sans/Variable/TTF/Subset/NotoSansTC-VF.ttf"
  if wget -q --spider "$FONT_URL" 2>/dev/null; then
    wget -q "$FONT_URL" -O NotoSansTC-VF.ttf && \
    sudo cp NotoSansTC-VF.ttf /usr/share/fonts/truetype/noto/ && \
    sudo chmod 644 /usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf && \
    echo "[VM] Successfully downloaded and installed NotoSansTC-VF.ttf" || true
  else
    # Fallback: try the full CJK font from Google's CDN
    FALLBACK_URL="https://raw.githubusercontent.com/notofonts/noto-cjk/main/Sans/OTC/NotoSansCJK-Regular.ttc"
    echo "[VM] Trying fallback URL: $FALLBACK_URL"
    wget -q "$FALLBACK_URL" -O NotoSansCJK-Regular.ttc 2>/dev/null && \
    sudo cp NotoSansCJK-Regular.ttc /usr/share/fonts/truetype/noto/ && \
    sudo chmod 644 /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc && \
    echo "[VM] Successfully downloaded and installed NotoSansCJK-Regular.ttc" || true
  fi
fi

# Create legacy relative path symlink for old code (e.g., /opt/vwork/usr/share/fonts/...)
if [ -n "${APP_DIR:-}" ]; then
  sudo mkdir -p "${APP_DIR}/usr/share/fonts/truetype/noto"
  if [ -f "/usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf" ]; then
    sudo ln -sf /usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf "${APP_DIR}/usr/share/fonts/truetype/noto/" || true
    echo "[VM] Created legacy symlink for NotoSansTC-VF.ttf under ${APP_DIR}";
  fi
fi

# Refresh font cache to ensure fonts are available immediately
sudo fc-cache -fv || true

# Verify font installation folder (requested by ops)
echo "[VM] /usr/share/fonts/truetype/noto contents:"
ls -la /usr/share/fonts/truetype/noto/ || true

# Verify font installation by checking actual file locations
echo "[VM] Verifying font installation..."
FONT_FOUND=false
for font_path in \
  /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc \
  /usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf \
  /usr/share/fonts/truetype/noto/NotoSansSC-VF.ttf \
  /usr/share/fonts/truetype/noto/NotoSansHK-VF.ttf \
  /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc \
  /usr/share/fonts/opentype/noto/NotoSansTC-Regular.otf; do
  if [ -f "$font_path" ]; then
    echo "[VM] ✓ Found font: $font_path"
    FONT_FOUND=true
    break
  fi
done

if [ "$FONT_FOUND" = false ]; then
  echo "[VM] WARNING: No Noto CJK font files found in expected locations"
  echo "[VM] PDF generation may fail for Chinese characters"
  # Try to find any Noto fonts
  find /usr/share/fonts -name "*Noto*CJK*" -type f 2>/dev/null | head -5 || echo "[VM] No Noto CJK fonts found in /usr/share/fonts"
fi

echo "[VM] installing gdown..."
# Newer Ubuntu/Debian blocks system-wide pip installs (PEP 668). Use a venv to keep it non-interactive and reliable.
VENV_DIR="/tmp/vwork-venv"
python3 -m venv "${VENV_DIR}"
"${VENV_DIR}/bin/pip" install -U pip >/dev/null
"${VENV_DIR}/bin/pip" install -U gdown >/dev/null
GDOWN="${VENV_DIR}/bin/gdown"

echo "[VM] create app dir: ${APP_DIR}"
sudo mkdir -p "${APP_DIR}"
sudo chown -R "$USER:$USER" "${APP_DIR}"
cd "${APP_DIR}"

echo "[VM] download code zip from Google Drive (requires shared link access)..."
"${GDOWN}" "https://drive.google.com/uc?id=${CODE_FILE_ID}" -O /tmp/vwork.zip

# Preserve uploads and .env before wiping app directory
PRESERVE_DIR="$(mktemp -d /tmp/vwork-preserve.XXXXXX)"
if [[ -f "${APP_DIR}/.env" ]]; then
  echo "[VM] preserving .env"
  cp -a "${APP_DIR}/.env" "${PRESERVE_DIR}/.env"
fi
if [[ -d "${APP_DIR}/web/uploads" ]]; then
  echo "[VM] preserving web/uploads ($(du -sh "${APP_DIR}/web/uploads" 2>/dev/null | cut -f1))"
  mkdir -p "${PRESERVE_DIR}/web"
  cp -a "${APP_DIR}/web/uploads" "${PRESERVE_DIR}/web/uploads"
fi

rm -rf "${APP_DIR:?}/"*
unzip -q /tmp/vwork.zip -d "${APP_DIR}"

# If the zip contains wrapper directories like "nwork/" or even "nwork/nwork/", normalize by flattening
# into ${APP_DIR} until go.mod is at the root (max 3 levels).
ROOT_CANDIDATE="${APP_DIR}"
for _ in 1 2 3; do
  if [[ -f "${ROOT_CANDIDATE}/go.mod" ]]; then
    break
  fi
  if [[ -d "${ROOT_CANDIDATE}/nwork" ]]; then
    ROOT_CANDIDATE="${ROOT_CANDIDATE}/nwork"
    continue
  fi
  if [[ -d "${ROOT_CANDIDATE}/vwork" ]]; then
    ROOT_CANDIDATE="${ROOT_CANDIDATE}/vwork"
    continue
  fi
  break
done

if [[ "${ROOT_CANDIDATE}" != "${APP_DIR}" && -f "${ROOT_CANDIDATE}/go.mod" ]]; then
  echo "[VM] flattening project root from ${ROOT_CANDIDATE} -> ${APP_DIR}"
  TMP_FLAT="$(mktemp -d /tmp/vwork-flat.XXXXXX)"
  shopt -s dotglob
  mv "${ROOT_CANDIDATE}/"* "${TMP_FLAT}/" || true
  shopt -u dotglob
  rm -rf "${APP_DIR:?}/"* "${APP_DIR}"/.[!.]* "${APP_DIR}"/..?* 2>/dev/null || true
  shopt -s dotglob
  mv "${TMP_FLAT}/"* "${APP_DIR}/" || true
  shopt -u dotglob
  rmdir "${TMP_FLAT}" 2>/dev/null || true
fi

# Restore preserved uploads and .env
if [[ -f "${PRESERVE_DIR}/.env" ]]; then
  echo "[VM] restoring preserved .env"
  cp -a "${PRESERVE_DIR}/.env" "${APP_DIR}/.env"
fi
if [[ -d "${PRESERVE_DIR}/web/uploads" ]]; then
  echo "[VM] restoring preserved web/uploads"
  mkdir -p "${APP_DIR}/web"
  rm -rf "${APP_DIR}/web/uploads" 2>/dev/null || true
  cp -a "${PRESERVE_DIR}/web/uploads" "${APP_DIR}/web/uploads"
fi
rm -rf "${PRESERVE_DIR}" 2>/dev/null || true

test -f "${APP_DIR}/go.mod" || { echo "[VM] go.mod not found under ${APP_DIR}"; exit 1; }

echo "[VM] write .env"
cat > "${APP_DIR}/.env" <<ENV
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=${DB_NAME}
DB_SSLMODE=disable

SERVER_HOST=0.0.0.0
SERVER_PORT=${APP_PORT}

APP_ENV=${APP_ENV}
FIBER_PREFORK=${FIBER_PREFORK}
APP_REQUEST_LOGGER=${APP_REQUEST_LOGGER}
GORM_LOG_LEVEL=${GORM_LOG_LEVEL}

PUBLIC_SCHEME=${PUBLIC_SCHEME}
BASE_DOMAIN=${BASE_DOMAIN}

# --- JWT ---
JWT_SECRET=nwork-secret-key-123456

APP_NAME=vWork
COMPANY_NAME=V-sys Limited
GEONAMES_USERNAME=tednv88

# --- vWorkAdmin ---
VWORK_ADMIN_USER=${VWORK_ADMIN_USER}
VWORK_ADMIN_PASS=${VWORK_ADMIN_PASS}

# --- SMTP (Brevo) ---
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USE_STARTTLS=true
SMTP_INSECURE_SKIP_VERIFY_TLS=false

# Brevo SMTP credentials
SMTP_USER=9f0077001@smtp-brevo.com
SMTP_PASSWORD=xsmtpsib-0a2cfb1d137b629bd7a568331b7c9bd8ce1beca2082a326e94e1e56dd1a4309b-4DmELMju6vz3ZF0F

# Sender information
SMTP_FROM_EMAIL=no-reply@mail.vworkai.com
SMTP_FROM_NAME=vWork

# --- Stripe ---
STRIPE_SECRET_KEY=${STRIPE_SECRET_KEY}
STRIPE_PUBLISHABLE_KEY=${STRIPE_PUBLISHABLE_KEY}
STRIPE_WEBHOOK_SECRET=${STRIPE_WEBHOOK_SECRET}
STRIPE_PRICE_MONTHLY=${STRIPE_PRICE_MONTHLY}
STRIPE_PRICE_YEARLY=${STRIPE_PRICE_YEARLY}
STRIPE_PRICE_MONTHLY_PRO=${STRIPE_PRICE_MONTHLY_PRO}
STRIPE_PRICE_YEARLY_PRO=${STRIPE_PRICE_YEARLY_PRO}
STRIPE_PRICE_MONTHLY_PRO_PLUS=${STRIPE_PRICE_MONTHLY_PRO_PLUS}
STRIPE_PRICE_YEARLY_PRO_PLUS=${STRIPE_PRICE_YEARLY_PRO_PLUS}
STRIPE_SUCCESS_URL=${STRIPE_SUCCESS_URL}
STRIPE_CANCEL_URL=${STRIPE_CANCEL_URL}
BASE_DOMAIN="vworkai.com"
PUBLIC_SCHEME="https"

# --- LLM (Gemini API) ---
LLM_API_KEY=${LLM_API_KEY}
LLM_MODEL=${LLM_MODEL}
LLM_IMAGE_MODEL=${LLM_IMAGE_MODEL}
LLM_PROVIDER=${LLM_PROVIDER}
LLM_ENDPOINT=${LLM_ENDPOINT}

# --- BytePlus ModelArk (Seedance 1.5 Pro video generation) ---
ARK_API_KEY=${ARK_API_KEY}
ARK_ENDPOINT_ID=${ARK_ENDPOINT_ID}

# --- Kling (Video Generation) ---
KLING_ACCESS_KEY=${KLING_ACCESS_KEY}
KLING_SECRET_KEY=${KLING_SECRET_KEY}
KLING_MODEL=${KLING_MODEL}
KLING_BASE_URL=${KLING_BASE_URL}

# --- Google Cloud Vision & Speech-to-Text API ---
GOOGLE_VISION_API_KEY=${GOOGLE_VISION_API_KEY}
GOOGLE_CLOUD_API_KEY=${GOOGLE_CLOUD_API_KEY}
GOOGLE_CLOUD_PROJECT_ID=${GOOGLE_CLOUD_PROJECT_ID}

# --- Google OAuth 2.0 (Login/Register) ---
GOOGLE_OAUTH_CLIENT_ID=${GOOGLE_OAUTH_CLIENT_ID}
GOOGLE_OAUTH_CLIENT_SECRET=${GOOGLE_OAUTH_CLIENT_SECRET}
GOOGLE_OAUTH_ENABLED=${GOOGLE_OAUTH_ENABLED}

# --- Google Maps API ---
GOOGLE_MAPS_API_KEY=${GOOGLE_MAPS_API_KEY}

# --- Google Search / Serper ---
GOOGLE_SEARCH_API_KEY=${GOOGLE_SEARCH_API_KEY}
GOOGLE_SEARCH_ENGINE_ID=${GOOGLE_SEARCH_ENGINE_ID}
SERPER_API_KEY=${SERPER_API_KEY}

# --- Google AdSense ---
GOOGLE_ADSENSE_PUBLISHER_ID=${GOOGLE_ADSENSE_PUBLISHER_ID}

# --- vSysAI ---
VSYSAI_DOMAIN=${VSYSAI_DOMAIN}

# --- Custom domains / Cloudflare for SaaS ---
# Note: keep these configurable via deploy.local.env (CONFIG_FILE) to avoid hardcoding secrets.
CUSTOM_DOMAIN_CNAME_TARGET="${CUSTOM_DOMAIN_CNAME_TARGET:-cname.vworkai.com}"
CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:-wHLzwAH4KiZDyK96jKX7v3Hc6-CrKZJebYZdC5xM}"
CLOUDFLARE_ZONE_ID="${CLOUDFLARE_ZONE_ID:-1c5ea8116c409c59b57d88513ba33009}"
ENV

echo "[VM] set postgres password (fast, not secure)..."
sudo -u postgres psql -c "ALTER USER ${DB_USER} WITH PASSWORD '${DB_PASSWORD}';" >/dev/null
sudo systemctl restart postgresql

# Backup current DB before applying changes
if [[ "${BACKUP_BEFORE_APPLY}" == "true" ]]; then
  echo "[VM] backup DB before apply..."
  ts="$(date +%Y%m%d-%H%M%S)"
  backup_file="${BACKUP_DIR}/${DB_NAME}-${ts}.dump"
  sudo mkdir -p "${BACKUP_DIR}"
  sudo chown postgres:postgres "${BACKUP_DIR}" || true

  if sudo -u postgres pg_dump -Fc "${DB_NAME}" -f "${backup_file}"; then
    echo "[VM] backup saved: ${backup_file}"
    if [[ -n "${BACKUP_GCS_BUCKET}" ]]; then
      echo "[VM] uploading backup to ${BACKUP_GCS_BUCKET}"
      if command -v gsutil >/dev/null 2>&1; then
        gsutil cp "${backup_file}" "${BACKUP_GCS_BUCKET}/" || echo "[VM] WARNING: gsutil upload failed"
      else
        echo "[VM] WARNING: gsutil not found; skip upload"
      fi
    fi
  else
    echo "[VM] WARNING: backup failed; continuing"
  fi
fi

# Ensure DB exists (NEVER drop — production data is sacred)
echo "[VM] ensuring DB ${DB_NAME} exists (will NOT drop or overwrite)..."
sudo -u postgres psql -d postgres -v ON_ERROR_STOP=1 -c "
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${DB_NAME}') THEN
    EXECUTE format('CREATE DATABASE %I', '${DB_NAME}');
    RAISE NOTICE 'Created database %', '${DB_NAME}';
  ELSE
    RAISE NOTICE 'Database % already exists — keeping as-is', '${DB_NAME}';
  END IF;
END
\$\$;
" >/dev/null

# Apply safe, additive migrations only (003+).
# SAFETY: migrations containing DROP TABLE or TRUNCATE are ALWAYS skipped.
# To apply destructive migrations, use a separate manual process.
echo "[VM] applying safe migrations..."
if [[ -d "${APP_DIR}/migrations" ]]; then
  for mig in $(ls "${APP_DIR}/migrations/"*.sql 2>/dev/null | sort); do
    mig_name="$(basename "${mig}")"
    # Skip initial schema files (001/002) and special files
    case "${mig_name}" in
      001_*|002_*|rebuild_db.sql|__inspect_columns.sql) continue ;;
    esac

    # ALWAYS skip destructive migrations — production data is sacred
    if grep -qiE '^\s*(DROP\s+TABLE|TRUNCATE\s)' "${mig}"; then
      echo "[VM]   SKIP (destructive DROP TABLE/TRUNCATE): ${mig_name}"
      continue
    fi

    echo "[VM]   applying: ${mig_name}"
    sudo -u postgres psql -v ON_ERROR_STOP=0 -d "${DB_NAME}" -f "${mig}" 2>&1 | tail -5 || true
  done
  echo "[VM] migrations done."
else
  echo "[VM] WARNING: migrations directory not found at ${APP_DIR}/migrations"
fi

echo "[VM] build api binary..."
cd "${APP_DIR}"
mkdir -p bin
go mod download
go build -o bin/vwork-api ./cmd/api

echo "[VM] build cronjob binary..."
go build -o bin/vwork-cronjob ./cmd/cronjob

echo "[VM] systemd service for API..."
sudo tee /etc/systemd/system/vwork-api.service >/dev/null <<EOF
[Unit]
Description=vwork api
After=network.target postgresql.service

[Service]
Type=simple
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/bin/vwork-api
Restart=always
RestartSec=2
EnvironmentFile=${APP_DIR}/.env

[Install]
WantedBy=multi-user.target
EOF

echo "[VM] systemd service for cronjob..."
sudo tee /etc/systemd/system/vwork-cronjob.service >/dev/null <<EOF
[Unit]
Description=vwork cronjob (scheduler + email queue worker)
After=network.target postgresql.service

[Service]
Type=simple
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/bin/vwork-cronjob
Restart=always
RestartSec=2
EnvironmentFile=${APP_DIR}/.env

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl stop vwork-api || true
sudo systemctl stop vwork-email-worker || true
sudo systemctl stop vwork-cronjob || true
sudo systemctl enable vwork-api
sudo systemctl disable vwork-email-worker || true
sudo systemctl enable vwork-cronjob
sudo systemctl start vwork-api
sudo systemctl start vwork-cronjob

echo "[VM] quick health check direct to Go:"
curl -sS "http://127.0.0.1:${APP_PORT}/health" || true

echo "[VM] checking service status..."
sleep 2
sudo systemctl status vwork-api --no-pager -l || true
echo ""
sudo systemctl status vwork-cronjob --no-pager -l || true

echo "[VM] Deploy complete. Production data was NOT touched."
REMOTE

LB_BACKEND_PORT="${APP_PORT}"
echo "==> 2) Tag VM + allow LB health checks/proxy to reach backend port ${LB_BACKEND_PORT}"
# Tag instance so firewall rule can target it
gcloud compute instances add-tags "${VM_NAME}" --zone "${ZONE}" --tags "${LB_NAME}-backend" --quiet || true

# Allow traffic from Google LB/health-check ranges to backend port
# (fast path; if rule exists, ignore errors)
gcloud compute firewall-rules create "${LB_NAME}-allow-hc" \
  --allow "tcp:${LB_BACKEND_PORT}" \
  --target-tags "${LB_NAME}-backend" \
  --source-ranges "130.211.0.0/22,35.191.0.0/16" \
  --quiet || true

echo "==> 3) Create External HTTPS LB + Google-managed certificate (Google Trust Services)"
gcloud compute addresses create "${LB_NAME}-ip" --global --quiet || true
LB_IP="$(gcloud compute addresses describe "${LB_NAME}-ip" --global --format='get(address)')"
echo "==> LB IP: ${LB_IP}"
echo "==> IMPORTANT: set DNS A records to ${LB_IP}:"
echo "     ${DOMAIN} -> ${LB_IP}"
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  echo "     ${VSYSAI_DOMAIN} -> ${LB_IP} (wait for DNS propagation)"
fi
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -n "${_SUB_DOMAIN}" ]]; then
    echo "     ${_SUB_DOMAIN} -> ${LB_IP}"
    if [[ "${_SUB_DOMAIN}" == www.* ]]; then
      echo "     ${_SUB_DOMAIN#www.} -> ${LB_IP}"
    fi
  fi
done

# Unmanaged instance group with single VM
gcloud compute instance-groups unmanaged create "${LB_NAME}-ig" --zone "${ZONE}" --quiet || true
gcloud compute instance-groups unmanaged add-instances "${LB_NAME}-ig" --instances "${VM_NAME}" --zone "${ZONE}" --quiet || true
gcloud compute instance-groups set-named-ports "${LB_NAME}-ig" --named-ports "backend:${LB_BACKEND_PORT}" --zone "${ZONE}" --quiet || true

gcloud compute health-checks create http "${LB_NAME}-hc" --port "${LB_BACKEND_PORT}" --request-path /health --quiet || true

gcloud compute backend-services create "${LB_NAME}-backend" \
  --protocol HTTP \
  --health-checks "${LB_NAME}-hc" \
  --global \
  --quiet || true

IG_URL="https://www.googleapis.com/compute/v1/projects/${PROJECT_ID}/zones/${ZONE}/instanceGroups/${LB_NAME}-ig"
if gcloud compute backend-services describe "${LB_NAME}-backend" --global --format="get(backends[].group)" 2>/dev/null | grep -q "${IG_URL}"; then
  echo "==> backend already attached; skip add-backend"
else
  gcloud compute backend-services add-backend "${LB_NAME}-backend" \
    --instance-group "${LB_NAME}-ig" \
    --instance-group-zone "${ZONE}" \
    --global \
    --quiet || true
fi

# Ensure the backend service uses the named port on the instance group
gcloud compute backend-services update "${LB_NAME}-backend" --port-name "backend" --global --quiet || true

# Extract base domain (e.g., www.vworkai.com -> vworkai.com)
BASE_DOMAIN_ONLY="${DOMAIN#www.}"
if [[ "${BASE_DOMAIN_ONLY}" == "${DOMAIN}" ]]; then
  # DOMAIN doesn't start with www., extract domain part (e.g., example.com from sub.example.com)
  BASE_DOMAIN_ONLY=$(echo "${DOMAIN}" | sed 's/^[^.]*\.//')
fi

gcloud compute url-maps create "${LB_NAME}-urlmap" --default-service "${LB_NAME}-backend" --quiet || true

# SSL certificate for primary domain: include both base domain and www domain if different
# This ensures SSL works for both vworkai.com and www.vworkai.com
# The application layer middleware will handle the redirect from base domain to www
PRIMARY_CERT_NAME="${LB_NAME}-cert"
if [[ "${DOMAIN}" == www.* && "${BASE_DOMAIN_ONLY}" != "${DOMAIN}" ]]; then
  echo "==> Creating SSL certificate for ${DOMAIN} and ${BASE_DOMAIN_ONLY}"
  echo "==> Note: Application will redirect ${BASE_DOMAIN_ONLY} -> ${DOMAIN} via middleware"
  gcloud compute ssl-certificates create "${PRIMARY_CERT_NAME}" --domains "${DOMAIN},${BASE_DOMAIN_ONLY}" --quiet || true
else
  gcloud compute ssl-certificates create "${PRIMARY_CERT_NAME}" --domains "${DOMAIN}" --quiet || true
fi

# SSL certificate for vsysai.com domain (if configured)
CERT_NAMES="${PRIMARY_CERT_NAME}"
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  echo "==> Creating SSL certificate for ${VSYSAI_DOMAIN}"
  VSYSAI_BASE_DOMAIN="${VSYSAI_DOMAIN#www.}"
  if [[ "${VSYSAI_BASE_DOMAIN}" == "${VSYSAI_DOMAIN}" ]]; then
    VSYSAI_BASE_DOMAIN=$(echo "${VSYSAI_DOMAIN}" | sed 's/^[^.]*\.//')
  fi
  
  VSYSAI_CERT_NAME="${LB_NAME}-vsysai-cert"
  if [[ "${VSYSAI_DOMAIN}" == www.* && "${VSYSAI_BASE_DOMAIN}" != "${VSYSAI_DOMAIN}" ]]; then
    echo "==> Creating SSL certificate for ${VSYSAI_DOMAIN} and ${VSYSAI_BASE_DOMAIN}"
    echo "==> Note: Application will redirect ${VSYSAI_BASE_DOMAIN} -> ${VSYSAI_DOMAIN} via middleware"
    gcloud compute ssl-certificates create "${VSYSAI_CERT_NAME}" --domains "${VSYSAI_DOMAIN},${VSYSAI_BASE_DOMAIN}" --quiet || true
  else
    gcloud compute ssl-certificates create "${VSYSAI_CERT_NAME}" --domains "${VSYSAI_DOMAIN}" --quiet || true
  fi
  CERT_NAMES="${PRIMARY_CERT_NAME},${VSYSAI_CERT_NAME}"
fi

# SSL certificates for production sub-domains (voffice, vai, vmarket)
# Each gets its own Google-managed certificate; all share the same LB backend
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -z "${_SUB_DOMAIN}" ]]; then continue; fi

  # Derive a safe cert name from the sub-domain (e.g. voffice.vsysai.com -> vwork-lb-voffice-cert)
  _SUB_LABEL="${_SUB_DOMAIN%%.*}"
  _SUB_CERT_NAME="${LB_NAME}-${_SUB_LABEL}-cert"

  # If domain starts with www., also include the base domain in the certificate
  # (e.g. www.vmarketai.com -> also include vmarketai.com)
  if [[ "${_SUB_DOMAIN}" == www.* ]]; then
    _SUB_BASE_DOMAIN="${_SUB_DOMAIN#www.}"
    echo "==> Creating SSL certificate for ${_SUB_DOMAIN} and ${_SUB_BASE_DOMAIN} (${_SUB_CERT_NAME})"
    echo "==> Note: Application will redirect ${_SUB_BASE_DOMAIN} -> ${_SUB_DOMAIN} via middleware"
    gcloud compute ssl-certificates create "${_SUB_CERT_NAME}" --domains "${_SUB_DOMAIN},${_SUB_BASE_DOMAIN}" --quiet || true
  else
    echo "==> Creating SSL certificate for ${_SUB_DOMAIN} (${_SUB_CERT_NAME})"
    gcloud compute ssl-certificates create "${_SUB_CERT_NAME}" --domains "${_SUB_DOMAIN}" --quiet || true
  fi
  CERT_NAMES="${CERT_NAMES},${_SUB_CERT_NAME}"
done

# Create or update HTTPS proxy with all SSL certificates
if gcloud compute target-https-proxies describe "${LB_NAME}-https-proxy" --global &>/dev/null; then
  echo "==> HTTPS proxy already exists, updating SSL certificates..."
  gcloud compute target-https-proxies update "${LB_NAME}-https-proxy" \
    --ssl-certificates "${CERT_NAMES}" \
    --global \
    --quiet || true
else
  echo "==> Creating HTTPS proxy with SSL certificates..."
  gcloud compute target-https-proxies create "${LB_NAME}-https-proxy" \
    --ssl-certificates "${CERT_NAMES}" \
    --url-map "${LB_NAME}-urlmap" \
    --quiet || true
fi

gcloud compute forwarding-rules create "${LB_NAME}-fr-https" \
  --global \
  --target-https-proxy "${LB_NAME}-https-proxy" \
  --ports 443 \
  --address "${LB_NAME}-ip" \
  --quiet || true

echo "==> 4) Wait for certificate(s) to become ACTIVE (needs DNS already pointing to LB IP)"
echo "==> IMPORTANT: Make sure DNS A records point to ${LB_IP}:"
echo "     ${DOMAIN} -> ${LB_IP}"
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  echo "     ${VSYSAI_DOMAIN} -> ${LB_IP}"
fi
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -n "${_SUB_DOMAIN}" ]]; then
    echo "     ${_SUB_DOMAIN} -> ${LB_IP}"
    if [[ "${_SUB_DOMAIN}" == www.* ]]; then
      echo "     ${_SUB_DOMAIN#www.} -> ${LB_IP}"
    fi
  fi
done

set +e
# Wait for primary certificate
echo "==> Waiting for primary certificate (${PRIMARY_CERT_NAME})..."
for i in {1..60}; do
  STATUS="$(gcloud compute ssl-certificates describe "${PRIMARY_CERT_NAME}" --global --format='get(managed.status)' 2>/dev/null)"
  echo "   ${PRIMARY_CERT_NAME} status: ${STATUS:-unknown} (try ${i}/60)"
  if [[ "${STATUS}" == "ACTIVE" ]]; then
    echo "==> Primary certificate ACTIVE."
    break
  fi
  sleep 10
done

# Wait for vsysai certificate if configured
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  VSYSAI_CERT_NAME="${LB_NAME}-vsysai-cert"
  echo "==> Waiting for vsysai certificate (${VSYSAI_CERT_NAME})..."
  for i in {1..60}; do
    STATUS="$(gcloud compute ssl-certificates describe "${VSYSAI_CERT_NAME}" --global --format='get(managed.status)' 2>/dev/null)"
    echo "   ${VSYSAI_CERT_NAME} status: ${STATUS:-unknown} (try ${i}/60)"
    if [[ "${STATUS}" == "ACTIVE" ]]; then
      echo "==> VSysAI certificate ACTIVE."
      break
    fi
    sleep 10
  done
fi

# Wait for sub-domain certificates (voffice, vai, vmarket)
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -z "${_SUB_DOMAIN}" ]]; then continue; fi
  _SUB_LABEL="${_SUB_DOMAIN%%.*}"
  _SUB_CERT_NAME="${LB_NAME}-${_SUB_LABEL}-cert"
  echo "==> Waiting for ${_SUB_DOMAIN} certificate (${_SUB_CERT_NAME})..."
  for i in {1..60}; do
    STATUS="$(gcloud compute ssl-certificates describe "${_SUB_CERT_NAME}" --global --format='get(managed.status)' 2>/dev/null)"
    echo "   ${_SUB_CERT_NAME} status: ${STATUS:-unknown} (try ${i}/60)"
    if [[ "${STATUS}" == "ACTIVE" ]]; then
      echo "==> ${_SUB_DOMAIN} certificate ACTIVE."
      break
    fi
    sleep 10
  done
done
set -e

echo "==> Done."
echo "Next:"
echo " - If cert(s) are not ACTIVE yet: confirm DNS A records point to ${LB_IP}, wait a bit, re-run the status check:"
echo "     gcloud compute ssl-certificates describe ${PRIMARY_CERT_NAME} --global"
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  echo "     gcloud compute ssl-certificates describe ${LB_NAME}-vsysai-cert --global"
fi
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -z "${_SUB_DOMAIN}" ]]; then continue; fi
  _SUB_LABEL="${_SUB_DOMAIN%%.*}"
  echo "     gcloud compute ssl-certificates describe ${LB_NAME}-${_SUB_LABEL}-cert --global"
done
echo " - Test:"
echo "     curl -I https://${DOMAIN}/health"
if [[ -n "${VSYSAI_DOMAIN}" && "${VSYSAI_DOMAIN}" != "" ]]; then
  echo "     curl -I https://${VSYSAI_DOMAIN}/health"
fi
for _SUB_DOMAIN_VAR in VOFFICE_DOMAIN VAI_DOMAIN VMARKET_DOMAIN; do
  _SUB_DOMAIN="${!_SUB_DOMAIN_VAR}"
  if [[ -n "${_SUB_DOMAIN}" ]]; then
    echo "     curl -I https://${_SUB_DOMAIN}/health"
  fi
done
