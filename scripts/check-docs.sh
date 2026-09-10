#!/usr/bin/env bash
# Fails when a `.ptbr.md` mirror is behind the English file it mirrors.
#
# Mirroring everything is an invitation to drift, so only what a human reads
# from outside is mirrored at all — and even that needs something better than
# good intentions. Each mirror declares its source in front matter:
#
#   <!-- mirror-of: DEVELOPMENT.md -->
#
# The rest comes from git, not from a stamp: a mirror is behind when the
# source has commits **after** the mirror's own last commit. Keeping the two
# in the same commit — which is the discipline this is here to enforce — is
# therefore all it takes to pass, and there is no hash to remember to bump.
set -euo pipefail

cd "$(dirname "$0")/.."

status=0
shopt -s nullglob
for mirror in *.ptbr.md docs/**/*.ptbr.md; do
  source_file=$(sed -n 's/^<!-- mirror-of: \(.*\) -->$/\1/p' "$mirror" | head -1)
  if [ -z "$source_file" ]; then
    echo "$mirror: no '<!-- mirror-of: ... -->' front matter"
    status=1
    continue
  fi
  if [ ! -f "$source_file" ]; then
    echo "$mirror: mirrors '$source_file', which does not exist"
    status=1
    continue
  fi

  last_mirror=$(git log -1 --format=%H -- "$mirror")
  if [ -z "$last_mirror" ]; then
    continue # never committed: nothing to compare against yet
  fi
  behind=$(git log --oneline "$last_mirror..HEAD" -- "$source_file")
  if [ -z "$behind" ]; then
    continue
  fi
  echo "$mirror is behind $source_file:"
  echo "$behind" | sed 's/^/    /'
  echo "    → update the translation and commit both files together"
  status=1
done

if [ "$status" = 0 ]; then
  echo "docs: every mirror is up to date"
fi
exit $status
