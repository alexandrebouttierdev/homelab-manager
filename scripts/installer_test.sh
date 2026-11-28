#!/usr/bin/env bash
# Lightweight checks for the public Linux installer and uninstaller.
set -Eeuo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

bash -n "$root/install.sh"
bash -n "$root/uninstall.sh"

# Exercise the exact release-tag extraction pipeline used by install.sh.
release_json='{"url":"https://api.github.com/repos/example/monoops/releases/1","tag_name":"v3.0.1","prerelease":false}'
tag="$(printf '%s\n' "$release_json" | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+"' | head -n 1 | sed -E 's/.*"(v[0-9]+\.[0-9]+\.[0-9]+)"/\1/')"
[[ "$tag" == "v3.0.1" ]]

# Guardrails: integrity verification, encrypted credentials and data preservation.
grep -q 'sha256sum --check' "$root/install.sh"
grep -q 'ImportCredential=monoops-secrets-key' "$root/install.sh"
grep -q 'LoadCredentialEncrypted=monoops-secrets-key' "$root/install.sh"
grep -q 'LoadCredentialEncrypted=monoops-secrets-key' "$root/deployments/systemd/monoops.service"
grep -q 'LoadCredentialEncrypted' "$root/deployments/config/manager.yaml"
grep -q 'MONOOPS_SECRETS_KEY_FILE' "$root/README.md"
# The scanner account and group must exist before tmpfiles applies ownership rules.
grep -q 'groupadd --system monoops-vulnscan' "$root/install.sh"
grep -q 'useradd --system --gid monoops-vulnscan' "$root/install.sh"
grep -q 'systemd-tmpfiles --create monoops-vulnscan.conf' "$root/install.sh"
account_line="$(grep -n 'groupadd --system monoops-vulnscan' "$root/install.sh" | cut -d: -f1)"
tmpfiles_line="$(grep -n 'systemd-tmpfiles --create monoops-vulnscan.conf' "$root/install.sh" | cut -d: -f1)"
(( account_line < tmpfiles_line ))

# The installer ends with the current status banner; credentials are never printed.
grep -q '██ ██' "$root/install.sh"
grep -Fq '█   █   ███' "$root/install.sh"
grep -q 'API : opérationnelle' "$root/install.sh"
grep -q 'Service : monoops (systemd)' "$root/install.sh"
grep -q 'Configuration : %s' "$root/install.sh"
grep -q 'Données : /var/lib/monoops' "$root/install.sh"
grep -q 'green=\\$.*1;32m' "$root/install.sh"
! grep -q 'Mot de passe par défaut : admin' "$root/install.sh"
! grep -q 'Identifiant par défaut : admin' "$root/install.sh"
grep -q 'rm -rf "$CONFIG_DIR" "$DATA_DIR"' "$root/uninstall.sh"
grep -q -- '--purge-data' "$root/uninstall.sh"

if grep -q '^User=monoops$' "$root/deployments/systemd/monoops-docker-proxy.service"; then
  echo "The Docker proxy must not run as the Manager user." >&2
  exit 1
fi

echo "installer checks: ok"
