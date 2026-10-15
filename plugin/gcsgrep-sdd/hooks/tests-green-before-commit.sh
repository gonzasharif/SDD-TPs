#!/usr/bin/env bash
# tests-green-before-commit.sh — no commit with the existing suite in red.
#
# Event:  PreToolUse, matcher "Bash".
# Effect: when the command is a `git commit`, runs the gcsgrep unit suite
#         (`go test ./...`; integration tests need the "integration" tag, so they
#         are not included and no GCS credentials are needed). Red suite => veto.
#
# Hook contract:
#   - Receives the event as JSON on stdin.
#   - exit 0 → lets the action through.
#   - exit 2 → VETOES the action; stderr is shown to the agent.
#   - Any other code → non-blocking error (the action goes through anyway).
#
# Requires: Go (on PATH, at /usr/local/go/bin/go, or via $GO_BIN). jq is optional.
set -uo pipefail

EVENT="$(cat)"

# Without jq the event is inspected raw: the JSON still contains the command text, so a
# `git commit` is still recognised and vetoed. Failing open here would switch the guardrail
# off exactly when the machine lacks a tool.
if command -v jq >/dev/null 2>&1; then
  COMMAND="$(printf '%s' "$EVENT" | jq -r '.tool_input.command // ""')"
else
  COMMAND="$EVENT"
fi

# Only `git commit` (also `git -C dir commit`) is checked.
printf '%s' "$COMMAND" | grep -qE '(^|[;&|"[:space:]])git([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+commit([[:space:]"]|$)' \
  || exit 0

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"
MODULE_DIR="$PROJECT_DIR/greenfield/gcsgrep"   # Adjust to your project.

GO_BIN="${GO_BIN:-$(command -v go || echo /usr/local/go/bin/go)}"
if [ ! -x "$GO_BIN" ]; then
  {
    echo "COMMIT BLOCKED — Go not found, so the regression suite cannot run."
    echo "Install Go or set GO_BIN to its path. Committing unchecked is what this hook prevents."
  } >&2
  exit 2
fi

OUTPUT="$(cd "$MODULE_DIR" && "$GO_BIN" test ./... 2>&1)"
STATUS=$?
[ "$STATUS" -eq 0 ] && exit 0

# --- block -------------------------------------------------------------------------
{
  echo "COMMIT BLOCKED — the existing gcsgrep test suite is red (go test ./... exit $STATUS)."
  echo
  printf '%s\n' "$OUTPUT" | grep -E -- '^(--- FAIL|FAIL|panic:|.*_test\.go:[0-9]+:)' | head -20
  echo
  echo "A previously green test that turns red is a regression, not an outdated test."
  echo "Fix the code so the suite passes again; do not edit or skip the test to make it pass."
  echo "Then retry the commit. Reproduce with: (cd greenfield/gcsgrep && go test ./...)"
} >&2

exit 2
