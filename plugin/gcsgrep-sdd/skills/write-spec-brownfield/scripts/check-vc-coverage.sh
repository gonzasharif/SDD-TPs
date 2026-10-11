#!/usr/bin/env bash
# check-vc-coverage.sh — the mechanical checks of a spec, without the model counting.
#
# Usage: check-vc-coverage.sh <spec.md>
#
# Checks:
#   1. every "### FR-n / BR-n / NFR-n" section mentions at least one VC;
#   2. every defined VC ("**VC-n:**") is unique;
#   3. every defined VC appears in the "## Plan de iteraciones" section
#      (ranges such as "VC-14…VC-17" or "VC-14...VC-17" count);
#   4. the spec has "### Fuera de alcance" and "## Invariantes" sections.
#
# Exit 0 if everything passes, 1 otherwise. Findings go to stdout.
set -uo pipefail

SPEC="${1:-}"
if [ -z "$SPEC" ] || [ ! -f "$SPEC" ]; then
  echo "usage: $0 <spec.md>" >&2
  exit 2
fi

fail=0
report() { echo "FAIL: $*"; fail=1; }

# --- 1. each requirement has a VC ------------------------------------------------
awk '
  /^### (FR|BR|NFR)-[0-9]+/ { if (id != "" && !has) print id; id = $2; has = 0; next }
  /^##? /                   { if (id != "" && !has) print id; id = ""; next }
  id != "" && /VC-[0-9]+/   { has = 1 }
  END                       { if (id != "" && !has) print id }
' "$SPEC" | while read -r req; do
  echo "FAIL: $req has no VC"
done | grep . && fail=1

# --- 2. defined VCs are unique ----------------------------------------------------
DEFINED="$(grep -oE '^\*\*VC-[0-9]+:\*\*' "$SPEC" | grep -oE '[0-9]+' | sort -n)"
DUPS="$(printf '%s\n' "$DEFINED" | uniq -d)"
for n in $DUPS; do report "VC-$n is defined more than once"; done

# --- 3. every VC is in the iteration plan ----------------------------------------
PLAN="$(awk '/^## Plan de iteraciones/ {on=1; next} /^## / {on=0} on' "$SPEC")"
if [ -z "$PLAN" ]; then
  report "missing section '## Plan de iteraciones'"
else
  # Expand ranges "VC-a…VC-b" / "VC-a ... VC-b" (spaces optional) into individual numbers.
  IN_PLAN="$(
    { printf '%s\n' "$PLAN" | grep -oE 'VC-[0-9]+[[:space:]]*(…|\.\.\.)[[:space:]]*VC-[0-9]+' \
        | grep -oE '[0-9]+' | paste - - | while read -r a b; do seq "$a" "$b"; done
      printf '%s\n' "$PLAN" | grep -oE 'VC-[0-9]+' | grep -oE '[0-9]+'
    } | sort -nu
  )"
  for n in $(printf '%s\n' "$DEFINED" | sort -nu); do
    printf '%s\n' "$IN_PLAN" | grep -qx "$n" || report "VC-$n is not in any iteration"
  done
fi

# --- 4. brownfield sections exist -------------------------------------------------
grep -qE '^### Fuera de alcance' "$SPEC" || report "missing section '### Fuera de alcance'"
grep -qE '^## Invariantes' "$SPEC"       || report "missing section '## Invariantes'"

if [ "$fail" -eq 0 ]; then
  echo "OK: $(printf '%s\n' "$DEFINED" | grep -c .) VCs, all requirements covered, all in the plan"
fi
exit "$fail"
