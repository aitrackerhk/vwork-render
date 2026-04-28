#!/usr/bin/env bash
set -euo pipefail

# Download vOffice installer from Google Drive and place it in the public uploads directory.
# Usage (from Google Cloud Shell):
#   bash scripts/upload_voffice_release.sh
#
# Or override defaults:
#   DRIVE_FILE_ID=xxxxx PLATFORM=macos FILENAME=vOffice_Setup.dmg bash scripts/upload_voffice_release.sh

########################################
# Load local config (same as run.sh)
########################################
CONFIG_FILE="${CONFIG_FILE:-./deploy.local.env}"
if [[ -f "${CONFIG_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
fi

########################################
# Settings
########################################
PROJECT_ID="${PROJECT_ID:-avid-keel-482217-u4}"
ZONE="${ZONE:-asia-southeast1-b}"
VM_NAME="${VM_NAME:-vwork}"
APP_DIR="${APP_DIR:-/opt/vwork}"

# Google Drive file ID (extract from share link)
# https://drive.google.com/file/d/<FILE_ID>/view?usp=drive_link
DRIVE_FILE_ID="${DRIVE_FILE_ID:-1cW0DHb8QMegnpr-HWjVOdk5sdUK8qFZb}"

# Platform: windows / macos / linux
PLATFORM="${PLATFORM:-windows}"

# Output filename (if empty, uses the filename from Google Drive)
FILENAME="${FILENAME:-}"

########################################
# Helpers
########################################
die() { echo "ERROR: $*" 1>&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing command: $1"; }

[[ -z "${DRIVE_FILE_ID}" ]] && die "DRIVE_FILE_ID is required"
need gcloud

echo "==> Downloading vOffice release from Google Drive to VM"
echo "    Drive File ID: ${DRIVE_FILE_ID}"
echo "    Platform:      ${PLATFORM}"
echo "    VM:            ${VM_NAME} (${ZONE})"
echo ""

gcloud config set project "${PROJECT_ID}" >/dev/null

gcloud compute ssh "${VM_NAME}" --zone "${ZONE}" --quiet --command "
set -euo pipefail

APP_DIR='${APP_DIR}'
PLATFORM='${PLATFORM}'
DRIVE_FILE_ID='${DRIVE_FILE_ID}'
FILENAME='${FILENAME}'

# Install gdown if needed
VENV_DIR='/tmp/vwork-venv'
if [[ ! -f \"\${VENV_DIR}/bin/gdown\" ]]; then
  echo '[VM] installing gdown...'
  python3 -m venv \"\${VENV_DIR}\"
  \"\${VENV_DIR}/bin/pip\" install -U pip >/dev/null
  \"\${VENV_DIR}/bin/pip\" install -U gdown >/dev/null
fi
GDOWN=\"\${VENV_DIR}/bin/gdown\"

# Create target directory
UPLOAD_DIR=\"\${APP_DIR}/web/uploads/voffice-releases/\${PLATFORM}\"
mkdir -p \"\${UPLOAD_DIR}\"

# Download from Google Drive
TMP_FILE=\"/tmp/voffice-release-download\"
echo \"[VM] downloading from Google Drive (id=\${DRIVE_FILE_ID})...\"
\"\${GDOWN}\" \"https://drive.google.com/uc?id=\${DRIVE_FILE_ID}\" -O \"\${TMP_FILE}\"

# Detect filename from download if not specified
if [[ -z \"\${FILENAME}\" ]]; then
  # Try to get original filename from gdown output, fallback to a default
  FILENAME=\$(basename \"\$(\"\${GDOWN}\" --id \"\${DRIVE_FILE_ID}\" --print-filename 2>/dev/null || true)\" 2>/dev/null || true)
  if [[ -z \"\${FILENAME}\" || \"\${FILENAME}\" == \".\" ]]; then
    case \"\${PLATFORM}\" in
      windows) FILENAME='vOffice_Setup.exe' ;;
      macos)   FILENAME='vOffice_Setup.dmg' ;;
      linux)   FILENAME='vOffice_Setup.AppImage' ;;
    esac
  fi
fi

DEST=\"\${UPLOAD_DIR}/\${FILENAME}\"
mv \"\${TMP_FILE}\" \"\${DEST}\"
chmod 644 \"\${DEST}\"

# Show result
FILE_SIZE=\$(stat -c%s \"\${DEST}\" 2>/dev/null || stat -f%z \"\${DEST}\" 2>/dev/null || echo 0)
FILE_SIZE_MB=\$(echo \"scale=1; \${FILE_SIZE}/1048576\" | bc 2>/dev/null || echo '?')
CHECKSUM=\$(sha256sum \"\${DEST}\" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 \"\${DEST}\" 2>/dev/null | cut -d' ' -f1 || echo '')

echo ''
echo '================================================'
echo '[VM] vOffice release file ready!'
echo \"  Path:     \${DEST}\"
echo \"  URL:      /uploads/voffice-releases/\${PLATFORM}/\${FILENAME}\"
echo \"  Size:     \${FILE_SIZE} bytes (\${FILE_SIZE_MB} MB)\"
echo \"  SHA-256:  \${CHECKSUM}\"
echo '================================================'
echo ''
echo 'Use these values in vWorkAdmin > vOffice > version management to create/update a release.'
"

echo ""
echo "==> Done. The file is now publicly accessible at:"
echo "    https://www.vworkai.com/uploads/voffice-releases/${PLATFORM}/<filename>"
