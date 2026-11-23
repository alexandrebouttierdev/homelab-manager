#!/usr/bin/env bash
# Prints the next release version (without the "v" prefix) from the
# Conventional Commits since the last v* tag:
#   "type!:" or "BREAKING CHANGE:" → major, "feat" → minor, anything else → patch.
# Without any tag, prints INITIAL_VERSION (the product version, docs/PRODUCT_SPEC.md).
# Prints nothing when HEAD is already released, so that a re-run publishes nothing.
set -euo pipefail

initial="${INITIAL_VERSION:-2.3.0}"

if [[ -n "$(git tag --points-at HEAD --list 'v[0-9]*')" ]]; then
  exit 0
fi

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
if [[ -z "$last" ]]; then
  echo "$initial"
  exit 0
fi

if [[ ! "$last" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "last tag $last is not vMAJOR.MINOR.PATCH" >&2
  exit 1
fi
major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}
patch=${BASH_REMATCH[3]}

subjects=$(git log --no-merges --format=%s "$last..HEAD")
bodies=$(git log --no-merges --format=%b "$last..HEAD")

if grep -qE '^[a-z]+(\([^)]*\))?!:' <<<"$subjects" || grep -qE '^BREAKING[ -]CHANGE:' <<<"$bodies"; then
  echo "$((major + 1)).0.0"
elif grep -qE '^feat(\([^)]*\))?:' <<<"$subjects"; then
  echo "$major.$((minor + 1)).0"
else
  echo "$major.$minor.$((patch + 1))"
fi
