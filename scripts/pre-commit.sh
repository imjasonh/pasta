#!/usr/bin/env bash
# pre-commit / CI checks for pasta.
#
# Same script runs on the developer's machine (as a git pre-commit
# hook) and in GitHub Actions, so locally-clean changes can't fail
# CI on something the hook would have caught.
#
# Install locally:
#   ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit
#
# Never skip these checks (no `git commit --no-verify`).

set -euo pipefail

# cd to the repo root via git. Stable whether invoked directly, by
# .git/hooks/pre-commit (a symlink, where $0 is misleading), or by CI.
cd "$(git rev-parse --show-toplevel)"

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
step()  { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }

step "gofmt"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  red "gofmt would reformat:"
  echo "$unformatted"
  exit 1
fi

step "go vet"
go vet ./...

step "go build"
go build ./...

# The whole analyzer suite — every analyzers/*/ and testdata/*/
# directory — runs through pasta_test.go's TestAnalyzers /
# TestExtensionDemos. No per-analyzer Go test files; this one
# invocation is the entire verification surface.
step "go test"
go test ./... -count=1

green "all checks passed"
