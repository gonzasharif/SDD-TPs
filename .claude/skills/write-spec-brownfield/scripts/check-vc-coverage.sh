#!/usr/bin/env bash
# check-vc-coverage.sh — the mechanical checks of a brownfield spec, without the model counting.
#
# Usage: check-vc-coverage.sh [--strict] <spec.md>
#
# Spec conventions it relies on (the skill's template follows them):
#   - Requirement headings:  "### FR-n · …", "#### FR-n · …" (also BR-n, NFR-n).
#   - Invariants:            table rows starting with "| **INV-n** |".
#   - VCs, one per line:     "**VC-3.1:** …" for FR-3, "**VC-BR1.1:**", "**VC-NFR2.1:**",
#                            "**VC-INV4.2:**". The ID carries the requirement it observes.
#   - Plan table under "## Plan de iteraciones"; the last column lists the requirements the
#     iteration closes ("FR-1 … FR-7" ranges allowed).
#
# FAIL checks (exit 1):
#   1. every FR/BR/NFR/INV has at least one VC;
#   2. every VC points to a requirement that exists, and VC IDs are unique;
#   3. #VC >= #FR + #BR + #NFR + #INV;
#   4. every requirement closes in exactly one iteration of the plan;
#   5. each FR has exactly one "cuando";
#   6. each VC line has at least one literal in backticks (command, text or value);
#   7. required sections exist: Fuera de alcance, Invariantes, Glosario, Decisiones
#      (with a "Descartado" column), Plan de iteraciones; and a base commit hash is declared;
#   8. no TBD-style wording ("a confirmar", "pendiente de", "sin medir", …);
#   9. no process history ("versión anterior", "tras la corrección", …).
#
# WARN checks (printed; exit 1 only with --strict):
#   - an FR whose "entonces" contains " y ", " o " or "en cambio" (possible non-atomic FR);
#   - an FR whose "dado" or "cuando" contains " o " (alternatives).
set -uo pipefail

STRICT=0
if [ "${1:-}" = "--strict" ]; then STRICT=1; shift; fi
SPEC="${1:-}"
if [ -z "$SPEC" ] || [ ! -f "$SPEC" ]; then
  echo "usage: $0 [--strict] <spec.md>" >&2
  exit 2
fi

fail=0; warn=0
report() { echo "FAIL: $*"; fail=1; }
caution() { echo "WARN: $*"; warn=1; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
tr -d '\r' < "$SPEC" > "$TMP/spec.md"
S="$TMP/spec.md"

# --- requirements -----------------------------------------------------------------
grep -oE '^#{3,4} (FR|BR|NFR)-[0-9]+' "$S" | awk '{print $2}' | sort -u > "$TMP/reqs"
grep -oE '^\| \*\*INV-[0-9]+\*\*' "$S" | grep -oE 'INV-[0-9]+' | sort -u >> "$TMP/reqs"
sort -u -o "$TMP/reqs" "$TMP/reqs"
NREQ="$(grep -c . "$TMP/reqs")"

# --- VCs: "**VC-<id>:**" at the start of a line; id -> requirement ------------------
grep -oE '^\*\*VC-[A-Za-z]*[0-9]+(\.[0-9]+)?:\*\*' "$S" | sed -E 's/^\*\*VC-//; s/:\*\*$//' > "$TMP/vcids"
NVC="$(grep -c . "$TMP/vcids")"
vc_req() { # VC id -> requirement id
  local id="$1" base="${1%.*}"
  case "$base" in
    [0-9]*) echo "FR-$base" ;;
    *)      echo "$base" | sed -E 's/^([A-Za-z]+)([0-9]+)$/\1-\2/' ;;
  esac
}
: > "$TMP/vcreqs"
while read -r id; do [ -n "$id" ] && echo "$(vc_req "$id")" >> "$TMP/vcreqs"; done < "$TMP/vcids"
sort -u -o "$TMP/vcreqs" "$TMP/vcreqs"

# 1. every requirement has a VC
comm -23 "$TMP/reqs" "$TMP/vcreqs" | while read -r r; do echo "FAIL: $r has no VC"; done | grep . && fail=1
# 2. no VC points to a missing requirement; IDs unique
comm -13 "$TMP/reqs" "$TMP/vcreqs" | while read -r r; do echo "FAIL: a VC points to $r, which does not exist"; done | grep . && fail=1
sort "$TMP/vcids" | uniq -d | while read -r id; do echo "FAIL: VC-$id is defined more than once"; done | grep . && fail=1
# 3. count
if [ "$NVC" -lt "$NREQ" ]; then report "only $NVC VCs for $NREQ requirements (need #VC >= #FR + #BR + #NFR + #INV)"; fi

# --- 4. plan: each requirement closes in exactly one iteration ----------------------
PLAN="$(awk '/^## Plan de iteraciones/ {on=1; next} /^## / {on=0} on' "$S")"
if [ -z "$PLAN" ]; then
  report "missing section '## Plan de iteraciones'"
else
  : > "$TMP/planreqs"
  printf '%s\n' "$PLAN" | grep -E '^\| \*\*[0-9]+\*\*' | while IFS= read -r row; do
    cell="$(printf '%s' "$row" | awk -F'|' '{print $(NF-1)}')"
    {
      # ranges "FR-a … FR-b" / "INV-a ... INV-b"
      printf '%s\n' "$cell" | grep -oE '(FR|BR|NFR|INV)-[0-9]+[[:space:]]*(…|\.\.\.)[[:space:]]*(FR|BR|NFR|INV)-[0-9]+' \
        | while read -r rng; do
            k="$(printf '%s' "$rng" | grep -oE '^[A-Z]+')"
            a="$(printf '%s' "$rng" | grep -oE '[0-9]+' | head -1)"; b="$(printf '%s' "$rng" | grep -oE '[0-9]+' | tail -1)"
            seq "$a" "$b" | sed "s/^/$k-/"
          done
      printf '%s\n' "$cell" | grep -oE '(FR|BR|NFR|INV)-[0-9]+'
    } | sort -u
  done > "$TMP/planreqs"
  sort "$TMP/planreqs" | uniq -c | awk '$1>1 {print $2}' | while read -r r; do echo "FAIL: $r closes in more than one iteration"; done | grep . && fail=1
  comm -23 "$TMP/reqs" <(sort -u "$TMP/planreqs") | while read -r r; do echo "FAIL: $r is not in any iteration"; done | grep . && fail=1
  comm -13 "$TMP/reqs" <(sort -u "$TMP/planreqs") | while read -r r; do echo "FAIL: the plan mentions $r, which does not exist"; done | grep . && fail=1
fi

# --- 5. each FR has exactly one "cuando"; WARN atomicity ---------------------------
awk '
  function flush() { if (id != "") {
      if (nc != 1) printf "FAIL: %s has %d \"cuando\" (needs exactly 1)\n", id, nc
      if (dado ~ / o /)    printf "WARN: %s: \"dado\" has alternatives (\" o \")\n", id
      if (cuando ~ / o /)  printf "WARN: %s: \"cuando\" has alternatives (\" o \")\n", id
      if (entonces ~ / y | o |en cambio/) printf "WARN: %s: \"entonces\" may hold more than one result (\" y \" / \" o \")\n", id
  } }
  /^###+ FR-[0-9]+/ { flush(); id=$2; nc=0; dado=""; cuando=""; entonces=""; mode=""; next }
  /^##+ /              { flush(); id=""; mode=""; next }
  id == "" { next }
  /^\*\*VC-/ { mode=""; next }
  /^\*\*Dado\*\*/ { mode="d" }
  /\*\*cuando\*\*/ { nc++; mode="c" }
  /^\*\*entonces\*\*/ { mode="e" }
  mode=="d" { dado = dado " " $0 }
  mode=="c" { cuando = cuando " " $0 }
  mode=="e" { entonces = entonces " " $0 }
  END { flush() }
' "$S" > "$TMP/fr-checks"
grep '^FAIL' "$TMP/fr-checks" && fail=1
grep '^WARN' "$TMP/fr-checks" | while IFS= read -r l; do echo "$l"; done
grep -q '^WARN' "$TMP/fr-checks" && warn=1

# --- 6. each VC (its whole paragraph) carries a literal ------------------------------
awk '
  function flush() { if (id != "" && text !~ /`/) printf "FAIL: %s has no literal (command, text or value in backticks)\n", id; id=""; text="" }
  /^\*\*VC-[A-Za-z]*[0-9]+(\.[0-9]+)?:\*\*/ { flush(); id=$1; sub(/^\*\*/, "", id); sub(/:\*\*$/, "", id); text=$0; next }
  /^[[:space:]]*$/ { flush(); next }
  /^#/ { flush(); next }
  id != "" { text = text " " $0 }
  END { flush() }
' "$S" | grep . && fail=1

# --- 7. sections, decisions column, base commit --------------------------------------
grep -qE '^### Fuera de alcance' "$S" || report "missing section '### Fuera de alcance'"
grep -qE '^## Invariantes' "$S"       || report "missing section '## Invariantes'"
grep -qE '^## Glosario' "$S"          || report "missing section '## Glosario'"
if grep -qE '^## Decisiones' "$S"; then
  awk '/^## Decisiones/ {on=1; next} /^## / {on=0} on' "$S" | grep -qE '^\|.*Descartado' \
    || report "'## Decisiones' has no 'Descartado' column (chosen / grounds / discarded)"
else
  report "missing section '## Decisiones'"
fi
grep -qiE 'commit base.*`?[0-9a-f]{7,}' "$S" || report "no base commit hash declared (\"commit base \`<hash>\`\")"

# --- 8. TBD-style wording ------------------------------------------------------------
grep -nEi 'TBD|a definir|a confirmar|provisori|a validar|sin medir|no se verific|pendiente de |queda para (la )?iteraci' "$S" \
  | while IFS= read -r l; do echo "FAIL: TBD-style wording at line ${l%%:*}: $(printf '%s' "${l#*:}" | cut -c1-90)"; done | grep . && fail=1

# --- 9. process history ----------------------------------------------------------------
grep -nEi 'versión anterior|tras la corrección|legacy|iteración [0-9] de la spec' "$S" \
  | while IFS= read -r l; do echo "FAIL: process history at line ${l%%:*}: $(printf '%s' "${l#*:}" | cut -c1-90)"; done | grep . && fail=1

if [ "$STRICT" -eq 1 ] && [ "$warn" -eq 1 ]; then fail=1; fi
if [ "$fail" -eq 0 ]; then
  echo "OK: $NVC VCs for $NREQ requirements; every requirement has a VC and closes in exactly one iteration"
fi
exit "$fail"
