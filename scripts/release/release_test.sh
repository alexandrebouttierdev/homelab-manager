#!/usr/bin/env bash
# Tests next-version.sh and notes.sh against a throwaway Git repository.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q
git config user.email test@example.invalid
git config user.name test
git config commit.gpgsign false
git config tag.gpgsign false

failures=0
commit() { git commit -q --allow-empty -m "$1" ${2:+-m "$2"}; }
expect() {
  local name="$1" want="$2" got
  got=$(INITIAL_VERSION=2.3.0 "$here/next-version.sh")
  if [[ "$got" != "$want" ]]; then
    echo "FAIL $name: got '$got', want '$want'" >&2
    failures=$((failures + 1))
  fi
}

commit "feat: first feature"
expect "no tag yet" "2.3.0"
git tag v2.3.0
expect "HEAD already released" ""

commit "docs: explain"
commit "fix(api): repair"
expect "fixes and docs" "2.3.1"
commit "feat(ui): new page"
expect "a feature" "2.4.0"
commit "refactor: move" "BREAKING CHANGE: the API changed"
expect "breaking change in the body" "3.0.0"
git tag v3.0.0
commit "feat(api)!: remove a route"
expect "breaking marker" "4.0.0"
git tag v4.0.0
commit "Merge without convention"
expect "commit outside the convention" "4.0.1"

notes=$("$here/notes.sh" 4.0.1 https://github.com/example/repo)
for want in "## Homelab Manager v4.0.1" "### Autres changements" "- Merge without convention (" \
  "Différences : https://github.com/example/repo/compare/v4.0.0...v4.0.1"; do
  if ! grep -qF -- "$want" <<<"$notes"; then
    echo "FAIL notes: missing '$want'" >&2
    failures=$((failures + 1))
  fi
done
if grep -q "### Nouveautés" <<<"$notes"; then
  echo "FAIL notes: empty sections must be left out" >&2
  failures=$((failures + 1))
fi

git tag -d v4.0.0 v3.0.0 >/dev/null
notes=$("$here/notes.sh" 3.0.0)
for want in "### Changements majeurs" "- feat(api)!: remove a route (" "### Nouveautés" "- feat(ui): new page (" "### Corrections" "- fix(api): repair ("; do
  if ! grep -qF -- "$want" <<<"$notes"; then
    echo "FAIL grouped notes: missing '$want'" >&2
    failures=$((failures + 1))
  fi
done

if ((failures > 0)); then
  exit 1
fi
echo "release scripts: ok"
