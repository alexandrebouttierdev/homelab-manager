#!/usr/bin/env bash
# Installs the dependency audit tools at pinned versions, released at least
# two weeks before they were adopted (docs/CODE_RULES.md § 6.1), into
# $(go env GOPATH)/bin. Used by the Audit and Release workflows; locally,
# any recent version of the tools does (docs/CODE_RULES.md § 6.5).
set -euo pipefail

govulncheck_version=v1.9.0  # 2026-10-11
osv_scanner_version=v2.7.1 # 2026-11-09

# Built outside the module, with the Go of the PATH: govulncheck must not be
# older than the go command that loads the packages.
cd "$(mktemp -d)"
go install "golang.org/x/vuln/cmd/govulncheck@${govulncheck_version}"
# OSV-Scanner v2.7.1 needs Go 1.27.3 to build: go downloads that official
# toolchain (checksum-verified) for this build only.
GOTOOLCHAIN=go1.27.3 go install "github.com/google/osv-scanner/v2/cmd/osv-scanner@${osv_scanner_version}"
"$(go env GOPATH)/bin/govulncheck" -version | head -n 2
"$(go env GOPATH)/bin/osv-scanner" --version | head -n 1
