#!/usr/bin/env bash
set -euo pipefail

# Restore vWork production DB from a backup on VM.
# Run from Google Cloud Shell:
#
#   bash restore_from_backup.sh              # interactive: lists backups, lets you pick
#   BACKUP_FILE=vwork-20260224-120000.dump bash restore_from_backup.sh   # use specific file

PROJECT_ID="${PROJECT_ID:-avid-keel-482217-u4}"
ZONE="${ZONE:-asia-southeast1-b}"
VM_NAME="${VM_NAME:-vwork}"
DB_NAME="${DB_NAME:-vwork}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/vwork}"
BACKUP_FILE="${BACKUP_FILE:-}"

if [[ -z "${BACKUP_FILE}" ]]; then
  echo "==> Listing available backups on ${VM_NAME}..."
  echo ""
  gcloud compute ssh "${VM_NAME}" --zone "${ZONE}" --project "${PROJECT_ID}" --quiet --command "
    echo 'Available backups (newest first):'
    echo '-----------------------------------'
    i=1
    for f in \$(ls -t /var/backups/vwork/*.dump 2>/dev/null); do
      size=\$(du -h \"\$f\" | cut -f1)
      ts=\$(stat -c '%y' \"\$f\" 2>/dev/null | cut -d. -f1)
      name=\$(basename \"\$f\")
      printf '  [%d] %s  (%s, %s)\n' \"\$i\" \"\$name\" \"\$size\" \"\$ts\"
      i=\$((i+1))
    done
  "
  echo ""
  echo "Pick the backup BEFORE this deploy (usually [2] or older)."
  echo "Copy the filename and run:"
  echo ""
  echo "  BACKUP_FILE=<filename> bash restore_from_backup.sh"
  echo ""
  echo "Example:"
  echo "  BACKUP_FILE=vwork-20260224-153000.dump bash restore_from_backup.sh"
  exit 0
fi

echo "==> Restoring ${BACKUP_FILE} on ${VM_NAME}..."

gcloud compute ssh "${VM_NAME}" --zone "${ZONE}" --project "${PROJECT_ID}" --quiet --command "
set -euo pipefail

DB_NAME='${DB_NAME}'
BACKUP_DIR='${BACKUP_DIR}'
BACKUP_FILE='${BACKUP_FILE}'
TARGET=\"\${BACKUP_DIR}/\${BACKUP_FILE}\"

if [[ ! -f \"\${TARGET}\" ]]; then
  echo \"[VM] ERROR: \${TARGET} not found\"
  exit 1
fi
echo \"[VM] restoring from: \${TARGET}\"
ls -lh \"\${TARGET}\"

echo '[VM] stopping services...'
sudo systemctl stop vwork-api 2>/dev/null || true
sudo systemctl stop vwork-cronjob 2>/dev/null || true

echo '[VM] drop + recreate DB...'
sudo -u postgres psql -d postgres -c \"DROP DATABASE IF EXISTS \\\"\${DB_NAME}\\\" WITH (FORCE);\"
sudo -u postgres psql -d postgres -c \"CREATE DATABASE \\\"\${DB_NAME}\\\";\"

echo '[VM] restoring backup (this may take a minute)...'
sudo -u postgres pg_restore --no-owner --no-privileges -d \"\${DB_NAME}\" \"\${TARGET}\" || {
  echo '[VM] WARNING: pg_restore had some non-fatal errors (usually OK)'
}

echo '[VM] restarting services...'
sudo systemctl start vwork-api
sudo systemctl start vwork-cronjob
sleep 2

echo '[VM] health check...'
curl -sS http://127.0.0.1:3001/health || echo '[VM] WARNING: health check failed'

echo ''
echo '[VM] verifying data...'
sudo -u postgres psql -d \"\${DB_NAME}\" -c \"
  SELECT 'tenants' AS tbl, COUNT(*) FROM tenants
  UNION ALL SELECT 'users', COUNT(*) FROM users
  UNION ALL SELECT 'roles', COUNT(*) FROM roles
  UNION ALL SELECT 'subscription_plans', COUNT(*) FROM subscription_plans;
\"

echo ''
echo '[VM] restore complete.'
"

echo "==> Done. Check https://www.vworkai.com to verify."
