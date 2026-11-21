#!/usr/bin/env bash
set -euo pipefail

printf 'Backend: go -C backend run ./cmd/manager\n'
printf 'Frontend: npm --prefix frontend run dev\n'
