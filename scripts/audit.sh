#!/usr/bin/env bash
# Dependency security audits (docs/CODE_RULES.md § 6): npm audit (all and
# production dependencies), OSV-Scanner and govulncheck. Each result is
# reported as réussi, vulnérabilités détectées, outil indisponible or échec;
# a missing tool is never reported as a clean audit.
# Exit status: 0 all clean, 1 vulnerabilities found, 2 a tool missing or
# failed (and no vulnerability found). Needs network access to the
# vulnerability databases, which is why scripts/verify.sh does not run it.
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
log_dir=$(mktemp -d)
trap 'rm -rf "$log_dir"' EXIT

found=0
unavailable=0
summary=()

record() { # name status [detail]
  summary+=("$(printf '%-28s %s%s' "$1" "$2" "${3:+ — $3}")")
}

npm_audit() { # label [extra npm audit arguments]
  local label=$1 log="$log_dir/npm-$RANDOM.json"
  shift
  if ! command -v npm >/dev/null; then
    record "$label" "OUTIL INDISPONIBLE" "npm absent"
    unavailable=1
    return
  fi
  npm --prefix "$root/frontend" audit --json "$@" >"$log" 2>"$log.err"
  local total
  total=$(node -e '
    const report = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))
    if (!report.metadata) process.exit(2)
    const v = report.metadata.vulnerabilities
    console.log(`${v.total} ${v.critical} ${v.high} ${v.moderate} ${v.low}`)
  ' "$log" 2>/dev/null) || {
    record "$label" "ÉCHEC" "$(head -c 200 "$log.err" | tr '\n' ' ')"
    unavailable=1
    return
  }
  read -r all critical high moderate low <<<"$total"
  if [[ "$all" == 0 ]]; then
    record "$label" "RÉUSSI" "0 vulnérabilité"
  else
    record "$label" "VULNÉRABILITÉS DÉTECTÉES" "$all (critiques $critical, élevées $high, modérées $moderate, faibles $low) : lancer npm audit pour le détail"
    found=1
  fi
}

osv_scan() {
  if ! command -v osv-scanner >/dev/null; then
    record "OSV-Scanner" "OUTIL INDISPONIBLE" "installer : brew install osv-scanner, ou go install github.com/google/osv-scanner/v2/cmd/osv-scanner@latest"
    unavailable=1
    return
  fi
  local log="$log_dir/osv.txt" status
  (cd "$root" && osv-scanner scan source -r .) >"$log" 2>&1
  status=$?
  case $status in
    0) record "OSV-Scanner" "RÉUSSI" "aucune vulnérabilité connue" ;;
    1) record "OSV-Scanner" "VULNÉRABILITÉS DÉTECTÉES" "détail : osv-scanner scan source -r ."; found=1 ;;
    *) record "OSV-Scanner" "ÉCHEC" "code $status : $(tail -n 3 "$log" | tr '\n' ' ')"; unavailable=1 ;;
  esac
}

go_vulncheck() {
  local tool
  tool=$(command -v govulncheck || true)
  if [[ -z "$tool" && -x "$(go env GOPATH 2>/dev/null)/bin/govulncheck" ]]; then
    tool="$(go env GOPATH)/bin/govulncheck"
  fi
  if [[ -z "$tool" ]]; then
    record "govulncheck" "OUTIL INDISPONIBLE" "installer : go install golang.org/x/vuln/cmd/govulncheck@latest"
    unavailable=1
    return
  fi
  # Analyse with the Go version of go.mod, the one CI and releases build
  # with: the standard library is part of the scan.
  local log="$log_dir/govulncheck.txt" status toolchain
  toolchain="go$(sed -n 's/^go //p' "$root/backend/go.mod")"
  (cd "$root/backend" && GOTOOLCHAIN="$toolchain" "$tool" ./...) >"$log" 2>&1
  status=$?
  case $status in
    0) record "govulncheck" "RÉUSSI" "aucune vulnérabilité atteignable" ;;
    3) record "govulncheck" "VULNÉRABILITÉS DÉTECTÉES" "détail : (cd backend && govulncheck ./...)"; found=1 ;;
    *) record "govulncheck" "ÉCHEC" "code $status : $(tail -n 3 "$log" | tr '\n' ' ')"; unavailable=1 ;;
  esac
}

npm_audit "npm audit"
npm_audit "npm audit --omit=dev" --omit=dev
osv_scan
go_vulncheck

printf '%s\n' "${summary[@]}"
if [[ $found == 1 ]]; then
  exit 1
fi
if [[ $unavailable == 1 ]]; then
  echo "Audit incomplet : au moins un outil est indisponible ou a échoué ; ne pas le présenter comme réussi." >&2
  exit 2
fi
