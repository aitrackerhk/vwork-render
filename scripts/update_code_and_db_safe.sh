#!/usr/bin/env bash
set -euo pipefail

die() { echo "ERROR: $*" 1>&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing command: $1"; }

usage() {
  cat <<'USAGE'
Usage:
  APP_DIR=/opt/vwork bash scripts/update_code_and_db_safe.sh [--git|--gdrive] [--ref <git-ref>] <migration.sql> [more.sql...]

Examples:
  APP_DIR=/opt/vwork bash scripts/update_code_and_db_safe.sh 145_create_activity_logs.sql
  APP_DIR=/opt/vwork bash scripts/update_code_and_db_safe.sh --ref v1.2.3 145_create_activity_logs.sql 146_fix_index.sql
  APP_DIR=/opt/vwork CODE_FILE_ID=xxxx bash scripts/update_code_and_db_safe.sh --gdrive 145_create_activity_logs.sql

Notes:
  - This script does NOT drop DB and does NOT restore SQL dumps.
  - You MUST explicitly list migrations to apply.
USAGE
}

APP_DIR="${APP_DIR:-/opt/vwork}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
GIT_BRANCH="${GIT_BRANCH:-main}"
GIT_REF="${GIT_REF:-}"
CODE_FILE_ID="${CODE_FILE_ID:-}"

FORCE_MODE=""
MIGRATIONS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --git)
      FORCE_MODE="git"
      shift
      ;;
    --gdrive)
      FORCE_MODE="gdrive"
      shift
      ;;
    --ref)
      shift
      [[ $# -gt 0 ]] || die "--ref requires a value"
      GIT_REF="$1"
      shift
      ;;
    *)
      MIGRATIONS+=("$1")
      shift
      ;;
  esac
done

[[ -d "${APP_DIR}" ]] || die "APP_DIR not found: ${APP_DIR}"
[[ ${#MIGRATIONS[@]} -gt 0 ]] || die "no migrations specified. Run with --help for usage."

ALLOW_REBUILD_MIGRATIONS="${ALLOW_REBUILD_MIGRATIONS:-false}"
for f in "${MIGRATIONS[@]}"; do
  base="$(basename "$f")"
  if [[ "${base}" == "rebuild_db.sql" ]]; then
    die "refusing to run rebuild_db.sql on production"
  fi
  if [[ "${base}" == 001_* || "${base}" == 002_* ]]; then
    if [[ "${ALLOW_REBUILD_MIGRATIONS}" != "true" ]]; then
      die "refusing to run ${base}. Set ALLOW_REBUILD_MIGRATIONS=true to override."
    fi
  fi
done

need bash
need tar
need go
need curl

cd "${APP_DIR}"

update_via_git() {
  need git
  [[ -d .git ]] || die "not a git repo under ${APP_DIR}; use --gdrive or convert deployment to git"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "git working tree is not clean under ${APP_DIR}"
  fi

  git fetch --tags --prune "${GIT_REMOTE}"
  if [[ -n "${GIT_REF}" ]]; then
    git checkout --detach "${GIT_REF}"
  else
    git checkout "${GIT_BRANCH}"
    git pull --ff-only "${GIT_REMOTE}" "${GIT_BRANCH}"
  fi
}

update_via_gdrive_zip() {
  [[ -n "${CODE_FILE_ID}" ]] || die "CODE_FILE_ID is empty; required for --gdrive"
  need python3
  need unzip

  VENV_DIR="/tmp/vwork-update-venv"
  python3 -m venv "${VENV_DIR}"
  "${VENV_DIR}/bin/pip" install -U pip >/dev/null
  "${VENV_DIR}/bin/pip" install -U gdown >/dev/null
  GDOWN="${VENV_DIR}/bin/gdown"

  ZIP_PATH="/tmp/vwork-code.zip"
  "${GDOWN}" "https://drive.google.com/uc?id=${CODE_FILE_ID}" -O "${ZIP_PATH}"

  TMP_DIR="$(mktemp -d /tmp/vwork-code.XXXXXX)"
  unzip -q "${ZIP_PATH}" -d "${TMP_DIR}"

  ROOT_CANDIDATE="${TMP_DIR}"
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
  [[ -f "${ROOT_CANDIDATE}/go.mod" ]] || die "go.mod not found in extracted zip"

  PRESERVE_DIR="$(mktemp -d /tmp/vwork-preserve.XXXXXX)"
  if [[ -f "${APP_DIR}/.env" ]]; then
    cp -a "${APP_DIR}/.env" "${PRESERVE_DIR}/.env"
  fi
  if [[ -d "${APP_DIR}/web/uploads" ]]; then
    mkdir -p "${PRESERVE_DIR}/web"
    cp -a "${APP_DIR}/web/uploads" "${PRESERVE_DIR}/web/uploads"
  fi

  rm -rf "${APP_DIR:?}/"* || true
  rm -rf "${APP_DIR}"/.[!.]* "${APP_DIR}"/..?* 2>/dev/null || true

  (cd "${ROOT_CANDIDATE}" && tar -cf - .) | (cd "${APP_DIR}" && tar -xpf -)

  if [[ -f "${PRESERVE_DIR}/.env" ]]; then
    cp -a "${PRESERVE_DIR}/.env" "${APP_DIR}/.env"
  fi
  if [[ -d "${PRESERVE_DIR}/web/uploads" ]]; then
    mkdir -p "${APP_DIR}/web"
    rm -rf "${APP_DIR}/web/uploads" || true
    cp -a "${PRESERVE_DIR}/web/uploads" "${APP_DIR}/web/uploads"
  fi
}

if [[ "${FORCE_MODE}" == "git" ]]; then
  echo "==> Updating code via git (${GIT_REMOTE}/${GIT_BRANCH})"
  update_via_git
elif [[ "${FORCE_MODE}" == "gdrive" ]]; then
  echo "==> Updating code via Google Drive zip"
  update_via_gdrive_zip
else
  if [[ -d .git ]]; then
    echo "==> Updating code via git (${GIT_REMOTE}/${GIT_BRANCH})"
    update_via_git
  else
    echo "==> Updating code via Google Drive zip"
    update_via_gdrive_zip
  fi
fi

[[ -f "${APP_DIR}/.env" ]] || die ".env not found under ${APP_DIR} (required for migrations)"

echo "==> Building new binaries"
mkdir -p bin
go mod download
go build -o bin/vwork-api.new ./cmd/api
go build -o bin/vwork-cronjob.new ./cmd/cronjob

if command -v systemctl >/dev/null 2>&1; then
  echo "==> Stopping services"
  sudo systemctl stop vwork-api || true
  sudo systemctl stop vwork-cronjob || true
fi

mv -f bin/vwork-api.new bin/vwork-api
mv -f bin/vwork-cronjob.new bin/vwork-cronjob

echo "==> Applying migrations"
go run ./cmd/apply_migrations/main.go "${MIGRATIONS[@]}"

if command -v systemctl >/dev/null 2>&1; then
  echo "==> Starting services"
  sudo systemctl start vwork-api
  sudo systemctl start vwork-cronjob
fi

APP_PORT="$(grep -E '^SERVER_PORT=' "${APP_DIR}/.env" | head -n 1 | cut -d= -f2 || true)"
APP_PORT="${APP_PORT:-3001}"
echo "==> Health check: http://127.0.0.1:${APP_PORT}/health"
curl -fsS "http://127.0.0.1:${APP_PORT}/health" >/dev/null

echo "==> OK"
