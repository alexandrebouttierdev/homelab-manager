#!/usr/bin/env bash
# Writes the Markdown notes of a release from the commits since the last v*
# tag, grouped by Conventional Commits type.
# Usage: scripts/release/notes.sh <version> [repository URL]
set -euo pipefail

version="$1"
repository="${2:-}"
last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
range="${last:+$last..}HEAD"
commits=$(git log --no-merges --format='%s (%h)' "$range")

section() {
  local title="$1" lines="$2"
  if [[ -n "$lines" ]]; then
    printf '### %s\n\n' "$title"
    sed 's/^/- /' <<<"$lines"
    printf '\n'
  fi
}

matching() { grep -E "$1" <<<"$commits" || true; }

echo "## MonoOps v$version"
echo
section "Changements majeurs" "$(matching '^[a-z]+(\([^)]*\))?!:')"
section "Nouveautés" "$(matching '^feat(\([^)]*\))?:')"
section "Corrections" "$(matching '^fix(\([^)]*\))?:')"
section "Performances" "$(matching '^perf(\([^)]*\))?:')"
section "Autres changements" "$(grep -vE '^(feat|fix|perf)(\([^)]*\))?:|^[a-z]+(\([^)]*\))?!:' <<<"$commits" || true)"

cat <<NOTES
### Installation

Archives pour Linux \`amd64\` et \`arm64\` : binaires \`monoops\`,
\`monoops-docker-proxy\` et \`monoops-backup-runner\`, interface compilée
(\`frontend/dist\`), unités systemd, configuration et règles polkit. Vérifier l'archive avec \`sha256sum -c SHA256SUMS\`,
puis suivre la section « Installation » du README.
NOTES
if [[ -n "$repository" && -n "$last" ]]; then
  echo
  echo "Différences : $repository/compare/$last...v$version"
fi
