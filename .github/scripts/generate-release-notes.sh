#!/usr/bin/env bash

set -euo pipefail

tag="${1:?release tag is required}"
repo_url="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:?repository is required}"

case "$tag" in
  v*)
    tag_pattern="v*"
    pathspecs=("api/" "pkg/" "framework/" "testing/" "go.mod" "go.sum")
    ;;
  *)
    printf 'unsupported release tag: %s\n' "$tag" >&2
    exit 1
    ;;
esac

tag_commit=$(git rev-parse "${tag}^{commit}")
previous_tag=$(git describe --tags --abbrev=0 --match "$tag_pattern" "${tag_commit}^" 2>/dev/null || true)

if [[ -n "$previous_tag" ]]; then
  revision_range=("${previous_tag}..${tag}")
  changelog_url="$repo_url/compare/${previous_tag}...${tag}"
else
  revision_range=("$tag")
  changelog_url="$repo_url/commits/${tag}"
fi

body_path="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-notes-${tag//\//-}.md"

{
  printf '## What\x27s Changed\n\n'

  has_changes=false
  while IFS=$'\t' read -r commit subject; do
    [[ -n "$commit" ]] || continue
    has_changes=true

    if [[ "$subject" =~ \(\#([0-9]+)\)$ ]]; then
      pull_request="${BASH_REMATCH[1]}"
      subject="${subject% (#${pull_request})}"
      printf '* %s ([#%s](%s/pull/%s))\n' "$subject" "$pull_request" "$repo_url" "$pull_request"
    elif [[ "$subject" =~ ^Merge\ pull\ request\ \#([0-9]+) ]]; then
      pull_request="${BASH_REMATCH[1]}"
      printf '* Pull request [#%s](%s/pull/%s)\n' "$pull_request" "$repo_url" "$pull_request"
    else
      printf '* [%s](%s/commit/%s)\n' "$subject" "$repo_url" "$commit"
    fi
  done < <(git log --first-parent --reverse --format='%H%x09%s' "${revision_range[@]}" -- "${pathspecs[@]}")

  if [[ "$has_changes" == false ]]; then
    printf 'No product module source changes in this release.\n'
  fi

  printf '\n**Full Repository Changelog**: %s\n' "$changelog_url"
} > "$body_path"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  printf 'body_path=%s\n' "$body_path" >> "$GITHUB_OUTPUT"
else
  printf '%s\n' "$body_path"
fi
