#!/usr/bin/env bash
set -euo pipefail

if [[ -f .env ]]; then
  set -a
  source .env
  set +a
fi

# Master key of the secrets box for development only (TOTP). Production
# imports it from systemd-creds (README).
key_file=backend/data/secrets.key
if [[ ! -f "$key_file" ]]; then
  mkdir -p backend/data
  (umask 077 && head -c 32 /dev/urandom | base64 > "$key_file")
fi
export HOMELAB_SECRETS_KEY_FILE="${HOMELAB_SECRETS_KEY_FILE:-./data/secrets.key}"

go -C backend run ./cmd/manager &
backend_pid=$!
npm --prefix frontend run dev -- --host 127.0.0.1 &
frontend_pid=$!

cleanup() {
  kill "$backend_pid" "$frontend_pid" 2>/dev/null || true
  wait "$backend_pid" "$frontend_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

wait
