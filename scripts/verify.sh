#!/usr/bin/env bash
set -euo pipefail

unformatted=$(gofmt -l backend)
if [[ -n "$unformatted" ]]; then
  echo "Fichiers Go non formatés (lancer gofmt -w) :" >&2
  echo "$unformatted" >&2
  exit 1
fi
go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
npm --prefix frontend run format:check
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend run test
npm --prefix frontend run build
