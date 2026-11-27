#!/usr/bin/env bash
# Install or update the latest stable MonoOps release on a systemd Linux host.
set -Eeuo pipefail

readonly REPO="alexandrebouttierdev/monoops"
readonly API_URL="https://api.github.com/repos/${REPO}/releases/latest"
readonly INSTALL_DIR="/opt/monoops"
readonly CONFIG_DIR="/etc/monoops"
readonly CREDENTIAL_DIR="/etc/credstore.encrypted"
readonly CREDENTIAL_FILE="${CREDENTIAL_DIR}/monoops-secrets-key"
readonly SERVICE_DIR="/etc/systemd/system"

log() { printf '[MonoOps] %s\n' "$*"; }
die() { printf '[MonoOps] ERREUR : %s\n' "$*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "Commande requise absente : $1"; }

[[ "${EUID}" -eq 0 ]] || die "Lancez ce script en root : curl ... | sudo bash"
[[ "$(uname -s)" == "Linux" ]] || die "MonoOps nécessite Linux."
[[ -d /run/systemd/system ]] || die "systemd n'est pas le gestionnaire de services actif."
need_cmd systemctl
need_cmd systemd-creds
need_cmd curl
need_cmd tar
need_cmd sha256sum
need_cmd awk
need_cmd sed
need_cmd install
need_cmd useradd
need_cmd getent

systemd_version="$(systemctl --version | awk 'NR == 1 { print $2 }')"
[[ "$systemd_version" =~ ^[0-9]+$ ]] || die "Impossible de déterminer la version de systemd."
(( systemd_version >= 254 )) || die "systemd 254 ou supérieur est requis pour LoadCredentialEncrypted (version détectée : $systemd_version)."

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) die "Architecture non prise en charge : $(uname -m). Seuls amd64 et arm64 sont disponibles." ;;
esac

need_docker_proxy=false
if getent group docker >/dev/null 2>&1; then
  need_docker_proxy=true
fi

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

log "Recherche de la dernière release stable…"
release_json="$(curl --proto '=https' --tlsv1.2 -fsSL --retry 3 "$API_URL")"
tag="$(printf '%s\n' "$release_json" | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+"' | head -n 1 | sed -E 's/.*"(v[0-9]+\.[0-9]+\.[0-9]+)"/\1/')"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "La dernière release stable n'a pas de tag SemVer reconnu."
version="${tag#v}"
archive="monoops_${version}_linux_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${tag}"

log "Téléchargement de MonoOps ${version} (${arch})…"
curl --proto '=https' --tlsv1.2 -fL --retry 3 "$base/$archive" -o "$tmp/$archive"
curl --proto '=https' --tlsv1.2 -fL --retry 3 "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

expected="$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp/SHA256SUMS")"
[[ "$expected" =~ ^[[:xdigit:]]{64}$ ]] || die "SHA256SUMS ne contient pas une somme valide pour $archive."
printf '%s  %s\n' "$expected" "$tmp/$archive" | sha256sum --check --status || die "Échec de vérification SHA-256."

archive_dir="monoops_${version}_linux_${arch}"
tar -tzf "$tmp/$archive" | awk -v prefix="$archive_dir/" '
  index($0, prefix) != 1 || $0 ~ /(^|\/)\.\.($|\/)/ || $0 ~ /^\// { bad = 1 }
  END { exit bad }
' || die "L'archive contient un chemin inattendu."
tar -xzf "$tmp/$archive" -C "$tmp" --no-same-owner
payload="$tmp/$archive_dir"
for required in bin/monoops bin/monoops-backup-runner deployments/config/manager.yaml deployments/systemd/monoops.service frontend/dist/index.html; do
  [[ -e "$payload/$required" ]] || die "Fichier absent de l'archive : $required"
done
if $need_docker_proxy; then
  [[ -x "$payload/bin/monoops-docker-proxy" ]] || die "Le binaire du proxy Docker est absent."
fi

log "Préparation des comptes système…"
if ! getent passwd monoops >/dev/null 2>&1; then
  useradd --system --user-group --no-create-home --shell /usr/sbin/nologin monoops
fi
if $need_docker_proxy; then
  if ! getent passwd monoops-docker-proxy >/dev/null 2>&1; then
    useradd --system --user-group --no-create-home --shell /usr/sbin/nologin monoops-docker-proxy
  fi
  usermod -aG docker monoops-docker-proxy
fi

log "Préservation de la configuration et de la clé existantes…"
install -d -m 0755 "$CONFIG_DIR"
if [[ ! -e "$CONFIG_DIR/manager.yaml" ]]; then
  install -m 0644 "$payload/deployments/config/manager.yaml" "$CONFIG_DIR/manager.yaml"
fi
if [[ ! -e "$CREDENTIAL_FILE" ]]; then
  install -d -m 0700 "$CREDENTIAL_DIR"
  (umask 077; head -c 32 /dev/urandom | base64 | systemd-creds encrypt --name=monoops-secrets-key - "$CREDENTIAL_FILE")
  chmod 0600 "$CREDENTIAL_FILE"
else
  log "Clé systemd existante conservée."
fi

log "Installation des binaires et de l'interface…"
install -d -m 0755 /usr/local/bin "$INSTALL_DIR/frontend"
install -m 0755 "$payload/bin/monoops" /usr/local/bin/monoops
install -m 0755 "$payload/bin/monoops-backup-runner" /usr/local/bin/monoops-backup-runner
if $need_docker_proxy; then
  install -m 0755 "$payload/bin/monoops-docker-proxy" /usr/local/bin/monoops-docker-proxy
fi

new_frontend="$INSTALL_DIR/.dist-new"
old_frontend="$INSTALL_DIR/.dist-old"
rm -rf "$new_frontend" "$old_frontend"
install -d -m 0755 "$new_frontend"
cp -a "$payload/frontend/dist/." "$new_frontend/"
if [[ -d "$INSTALL_DIR/frontend/dist" ]]; then
  mv "$INSTALL_DIR/frontend/dist" "$old_frontend"
fi
if ! mv "$new_frontend" "$INSTALL_DIR/frontend/dist"; then
  [[ ! -d "$old_frontend" ]] || mv "$old_frontend" "$INSTALL_DIR/frontend/dist"
  die "Impossible de remplacer l'interface ; l'ancienne version a été restaurée."
fi
rm -rf "$old_frontend"
chown -R root:root "$INSTALL_DIR"
chmod -R a+rX "$INSTALL_DIR"

log "Installation des unités systemd…"
install -m 0644 "$payload/deployments/systemd/monoops.service" "$SERVICE_DIR/monoops.service"
for unit in "$payload"/deployments/systemd/*.service; do
  name="$(basename "$unit")"
  case "$name" in
    monoops.service) continue ;;
    monoops-docker-proxy.service)
      if $need_docker_proxy; then install -m 0644 "$unit" "$SERVICE_DIR/$name"; fi
      ;;
    *) install -m 0644 "$unit" "$SERVICE_DIR/$name" ;;
  esac
done
if $need_docker_proxy; then
  install -m 0644 "$payload/deployments/systemd/monoops-docker-proxy.socket" "$SERVICE_DIR/monoops-docker-proxy.socket"
else
  systemctl disable --now monoops-docker-proxy.socket >/dev/null 2>&1 || true
  rm -f "$SERVICE_DIR/monoops-docker-proxy.service" "$SERVICE_DIR/monoops-docker-proxy.socket"
  rm -f /usr/local/bin/monoops-docker-proxy
  log "Docker non détecté (groupe docker absent) : proxy non activé."
fi

if compgen -G "$payload/deployments/tmpfiles/*.conf" >/dev/null; then
  install -d -m 0755 /usr/lib/tmpfiles.d
  install -m 0644 "$payload"/deployments/tmpfiles/*.conf /usr/lib/tmpfiles.d/
  systemd-tmpfiles --create >/dev/null
fi
if [[ -d "$payload/deployments/systemd/monoops-backup@restore-in-place.service.d" ]]; then
  install -d -m 0755 "$SERVICE_DIR/monoops-backup@restore-in-place.service.d"
  install -m 0644 "$payload"/deployments/systemd/monoops-backup@restore-in-place.service.d/*.conf \
    "$SERVICE_DIR/monoops-backup@restore-in-place.service.d/"
fi
if compgen -G "$payload/deployments/polkit/*.rules" >/dev/null; then
  install -d -m 0755 /etc/polkit-1/rules.d
  install -m 0644 "$payload"/deployments/polkit/*.rules /etc/polkit-1/rules.d/
fi

if [[ -f "$CONFIG_DIR/manager.env" ]]; then
  chmod 0600 "$CONFIG_DIR/manager.env"
fi

systemctl daemon-reload
if $need_docker_proxy; then
  systemctl enable --now monoops-docker-proxy.socket
fi
systemctl enable monoops.service
if systemctl is-active --quiet monoops.service; then
  systemctl restart monoops.service
else
  systemctl start monoops.service
fi

if ! systemctl is-active --quiet monoops.service; then
  systemctl --no-pager --full status monoops.service || true
  journalctl -u monoops.service -n 40 --no-pager || true
  die "Le service MonoOps n'est pas actif. Consultez les journaux ci-dessus."
fi

log "MonoOps ${version} est installé et le service est actif."
if journalctl -u monoops.service -n 80 --no-pager 2>/dev/null | grep -q "Secrets key loaded"; then
  log "Clé du coffre de secrets chargée."
else
  log "AVERTISSEMENT : le journal ne confirme pas le chargement de la clé du coffre. Vérifiez : journalctl -u monoops -n 80"
fi
if curl --proto '=http,https' -fsS --max-time 5 http://127.0.0.1:8080/health >/dev/null 2>&1; then
  log "Contrôle HTTP local réussi : http://127.0.0.1:8080"
else
  log "AVERTISSEMENT : contrôle HTTP sur le port 8080 non confirmé. Vérifiez manager.yaml et le journal systemd."
fi
log "Compte initial : admin / admin (changement obligatoire à la première connexion, sauf configuration préalable)."
log "Configuration et données conservées lors des mises à jour : $CONFIG_DIR et /var/lib/monoops."
