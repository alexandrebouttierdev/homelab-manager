#!/usr/bin/env bash
# Remove MonoOps services and program files while preserving user data by default.
set -Eeuo pipefail

readonly CONFIG_DIR="/etc/monoops"
readonly DATA_DIR="/var/lib/monoops"
readonly CREDENTIAL_FILE="/etc/credstore.encrypted/monoops-secrets-key"

log() { printf '[MonoOps] %s\n' "$*"; }
die() { printf '[MonoOps] ERREUR : %s\n' "$*" >&2; exit 1; }

[[ "${EUID}" -eq 0 ]] || die "Lancez ce script en root."
purge=false
case "${1:-}" in
  "") ;;
  --purge-data) purge=true ;;
  -h|--help)
    printf 'Usage: sudo bash uninstall.sh [--purge-data]\n\nSans option, les configurations, la base SQLite, les sauvegardes et la clé sont conservées.\n--purge-data demande une confirmation explicite et supprime aussi ces données.\n'
    exit 0
    ;;
  *) die "Option inconnue : $1" ;;
esac

if $purge; then
  printf '%s\n' "ATTENTION : cette opération supprimera définitivement la configuration, la base SQLite, les sauvegardes et la clé de chiffrement MonoOps."
  printf 'Tapez SUPPRIMER pour confirmer : '
  read -r answer
  [[ "$answer" == "SUPPRIMER" ]] || die "Désinstallation annulée."
fi

if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  systemctl disable --now monoops.service monoops-docker-proxy.socket >/dev/null 2>&1 || true
  systemctl stop monoops-docker-proxy.service monoops-vulnscan.service >/dev/null 2>&1 || true
fi

rm -f /etc/systemd/system/monoops.service
rm -f /etc/systemd/system/monoops-docker-proxy.service /etc/systemd/system/monoops-docker-proxy.socket
rm -f /etc/systemd/system/monoops-backup@.service
rm -f /etc/systemd/system/monoops-journal-vacuum@.service
rm -f /etc/systemd/system/monoops-vulnscan.service
rm -f /etc/systemd/system/monoops-backup@restore-in-place.service.d/*.conf
rmdir /etc/systemd/system/monoops-backup@restore-in-place.service.d 2>/dev/null || true
rm -f /usr/lib/tmpfiles.d/monoops*.conf /etc/tmpfiles.d/monoops*.conf
rm -f /usr/share/polkit-1/rules.d/*monoops*.rules
rm -f /usr/local/bin/monoops /usr/local/bin/monoops-docker-proxy /usr/local/bin/monoops-backup-runner
rm -rf /opt/monoops

if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  systemctl daemon-reload
  systemctl reset-failed >/dev/null 2>&1 || true
fi

if $purge; then
  rm -rf "$CONFIG_DIR" "$DATA_DIR" /var/lib/monoops-backup /var/lib/monoops-vulnscan
  rm -f "$CREDENTIAL_FILE"
  log "Données, configuration et clé MonoOps supprimées."
else
  log "Configuration conservée : $CONFIG_DIR"
  log "Base et données conservées : $DATA_DIR, /var/lib/monoops-backup et /var/lib/monoops-vulnscan"
  log "Clé chiffrée conservée : $CREDENTIAL_FILE"
fi

log "Comptes système monoops et monoops-docker-proxy conservés volontairement."
log "Désinstallation des composants MonoOps terminée."
