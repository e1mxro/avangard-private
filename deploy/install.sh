#!/usr/bin/env bash
# AVANGARD server one-liner installer.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/e1mxro/avangard-private/init/deploy/install.sh \
#       | sudo DOMAIN=tunnel.example.com EMAIL=admin@example.com bash
set -euo pipefail

REPO_OWNER="${REPO_OWNER:-e1mxro}"
REPO_NAME="${REPO_NAME:-avangard-private}"
REF="${REF:-init}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/usr/local/bin}"
ETC_DIR="${ETC_DIR:-/etc/avangard}"
LIB_DIR="${LIB_DIR:-/var/lib/avangard}"
LOG_DIR="${LOG_DIR:-/var/log/avangard}"
PORT="${PORT:-443}"
DECOY="${DECOY:-www.yandex.ru}"
USER_NAME="${USER_NAME:-avangard}"
GO_VERSION="${GO_VERSION:-1.25.1}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Please run as root (sudo)." >&2
  exit 1
fi

if [[ -z "${DOMAIN:-}" ]]; then
  echo "Set DOMAIN=<your.domain> (used for Let's Encrypt and the AVANGARD URI)." >&2
  exit 2
fi
EMAIL="${EMAIL:-admin@${DOMAIN}}"

echo "==> Installing AVANGARD on $(hostname)"
echo "    domain        = ${DOMAIN}"
echo "    decoy         = ${DECOY}"
echo "    listen port   = ${PORT}"
echo "    install dir   = ${INSTALL_PREFIX}"
echo "    config dir    = ${ETC_DIR}"

# 1. Dependencies
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  curl ca-certificates jq tar git build-essential

# 2. Install Go (if missing or wrong version)
if ! command -v go >/dev/null || ! go version | grep -q "go${GO_VERSION%.*}"; then
  echo "==> Installing Go ${GO_VERSION}"
  cd /tmp
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o go.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf go.tar.gz
  rm -f go.tar.gz
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
fi

# 3. Build avangard from source
echo "==> Building AVANGARD from source"
SRC_DIR="$(mktemp -d -t avangard.XXXXXX)"
trap 'rm -rf "${SRC_DIR}"' EXIT
git clone --depth 1 --branch "${REF}" "https://github.com/${REPO_OWNER}/${REPO_NAME}.git" "${SRC_DIR}"
cd "${SRC_DIR}"
go build -trimpath -ldflags='-s -w' -o "${INSTALL_PREFIX}/avangard-server" ./cmd/avangard-server
go build -trimpath -ldflags='-s -w' -o "${INSTALL_PREFIX}/avangard-client" ./cmd/avangard-client

# 4. System user / dirs
if ! id "${USER_NAME}" >/dev/null 2>&1; then
  useradd --system --home-dir "${LIB_DIR}" --shell /usr/sbin/nologin "${USER_NAME}"
fi
install -d -m 0750 -o "${USER_NAME}" -g "${USER_NAME}" "${ETC_DIR}" "${LIB_DIR}" "${LOG_DIR}"

# 5. ACME certificate via certbot (if not already present)
if [[ ! -f "${ETC_DIR}/cert.pem" || ! -f "${ETC_DIR}/key.pem" ]]; then
  echo "==> Obtaining Let's Encrypt certificate for ${DOMAIN}"
  apt-get install -y -qq --no-install-recommends certbot
  certbot certonly --standalone --non-interactive --agree-tos -m "${EMAIL}" -d "${DOMAIN}" \
    --preferred-challenges http --http-01-port 80 || {
      echo "ACME failed. Falling back to self-signed cert (dev only)."
      "${INSTALL_PREFIX}/avangard-server" selfcert --out "${ETC_DIR}" --host "${DOMAIN}"
    }
  if [[ -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]]; then
    cp "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" "${ETC_DIR}/cert.pem"
    cp "/etc/letsencrypt/live/${DOMAIN}/privkey.pem"   "${ETC_DIR}/key.pem"
    chown "${USER_NAME}:${USER_NAME}" "${ETC_DIR}/cert.pem" "${ETC_DIR}/key.pem"
  fi
fi

# 6. Generate keypair + URI if config doesn't exist
if [[ ! -f "${ETC_DIR}/server.yaml" ]]; then
  echo "==> Generating server keypair + URI"
  KEY_OUT="$("${INSTALL_PREFIX}/avangard-server" keygen \
      --host "${DOMAIN}" --port "${PORT}" --decoy "${DECOY}" --out "${ETC_DIR}")"
  echo "${KEY_OUT}"
  echo "${KEY_OUT}" > "${ETC_DIR}/uri.txt"
  chown "${USER_NAME}:${USER_NAME}" "${ETC_DIR}/server.yaml" "${ETC_DIR}/uri.txt"
  chmod 0600 "${ETC_DIR}/server.yaml"
fi

# 7. systemd unit
install -m 0644 "${SRC_DIR}/deploy/systemd/avangard-server.service" /etc/systemd/system/avangard-server.service
systemctl daemon-reload
systemctl enable --now avangard-server.service

# 8. Show client URI
echo
echo "============================================================"
echo "AVANGARD installed. Service status:"
systemctl --no-pager status avangard-server.service | head -5 || true
echo
echo "Client URI (paste into avangard-client):"
echo
grep '^avangard://' "${ETC_DIR}/uri.txt" || cat "${ETC_DIR}/uri.txt"
echo
echo "============================================================"
