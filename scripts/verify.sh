#!/usr/bin/env bash
set -euo pipefail

find backend -name '*.go' -print0 | xargs -0 gofmt -w
go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend run test
npm --prefix frontend run build
