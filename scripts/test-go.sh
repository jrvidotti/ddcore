#!/usr/bin/env bash
# Runs the Go tests of ./internal/... with the two slow packages split across processes.
#
# internal/engine and internal/api run their tests one after another, and each takes over two
# minutes alone: they are the whole wall time of `go test ./internal/...`. Here each is split
# into shards, one `go test -run '^(...)$'` per shard, all running at once next to the other
# packages. A shard is a process of its own, so it gets databases of its own (testdb tags them
# with the process id), and `go test` caches each shard's result like any other run.
#
#   scripts/test-go.sh [go test flags]     e.g. scripts/test-go.sh -count=1
#   ENGINE_SHARDS=4 API_SHARDS=2 scripts/test-go.sh
#
# A -run among the flags means a narrower run than this script is for: it is passed to a plain
# `go test ./internal/...` instead.
set -u
cd "$(dirname "$0")/.."
export GOWORK=off

ENGINE_SHARDS=${ENGINE_SHARDS:-3}
API_SHARDS=${API_SHARDS:-2}
args=("$@")

for a in ${args[@]+"${args[@]}"}; do
  case $a in
    -run|-run=*|-test.run|-test.run=*) exec go test "${args[@]}" ./internal/... ;;
  esac
done

logs=$(mktemp -d "${TMPDIR:-/tmp}/ddcore-test-go.XXXXXX")
groups=()

# start NAME CMD...: runs CMD in the background, its output in NAME.log, "rc secs" in NAME.rc.
start() {
  local name=$1
  shift
  groups+=("$name")
  (
    s=$SECONDS
    "$@" >"$logs/$name.log" 2>&1
    echo "$? $((SECONDS - s))" >"$logs/$name.rc"
  ) &
}

# shard PKG N: splits PKG's top-level tests into N `go test -run` processes, round-robin over
# the sorted names so a shard holds the same tests from one run to the next.
shard() {
  local pkg=$1 n=$2 short=${1##*/} names i
  if ! go test -list '.*' "$pkg" >"$logs/$short.list" 2>&1; then
    # no list (a build error, say): run it whole, so its own output says why
    start "$short" go test ${args[@]+"${args[@]}"} "$pkg"
    return
  fi
  names=$(grep -E '^(Test|Example|Fuzz)' "$logs/$short.list" | sort)
  for ((i = 0; i < n; i++)); do
    local re
    re=$(echo "$names" | awk -v n="$n" -v i="$i" '(NR - 1) % n == i' | paste -sd '|' -)
    [ -n "$re" ] || continue
    echo "$re" | tr '|' '\n' | wc -l | tr -d ' ' >"$logs/$short-$((i + 1)).count"
    start "$short-$((i + 1))" go test ${args[@]+"${args[@]}"} -run "^($re)\$" "$pkg"
  done
}

rest=$(go list ./internal/... | grep -v -E '/internal/(engine|api)$')
# shellcheck disable=SC2086 # the package list is meant to split into words
start rest go test ${args[@]+"${args[@]}"} $rest
shard ./internal/engine "$ENGINE_SHARDS"
shard ./internal/api "$API_SHARDS"
wait

status=0
for g in "${groups[@]}"; do
  cat "$logs/$g.log"
done
echo
echo "go test, by group (seconds, top-level tests):"
for g in "${groups[@]}"; do
  read -r rc secs <"$logs/$g.rc"
  count=""
  [ -f "$logs/$g.count" ] && count=" $(cat "$logs/$g.count") tests"
  mark=ok
  [ "$rc" = 0 ] || { mark=FAIL; status=1; }
  printf '  %-4s %-10s %4ss%s\n' "$mark" "$g" "$secs" "$count"
done
rm -rf "$logs"
exit $status
