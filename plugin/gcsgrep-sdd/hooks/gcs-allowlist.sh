#!/usr/bin/env bash
# gcs-allowlist.sh — allowlist de tools del server MCP de GCS (capa 1 del muro).
#
# Evento: PreToolUse, matcher "mcp__gcs__.*" (en el plugin: "mcp__plugin_gcsgrep-sdd_gcs__.*").
# Efecto: deja pasar SOLO las tools de lectura que usa el skill; veta todo lo demás
#         (exit 2). Es allowlist, no denylist: una tool nueva de la próxima versión del
#         server queda bloqueada hasta que alguien la mire y la agregue acá.
#         Además veta lecturas contra un bucket que no sea el de prueba.
#
# Contrato: evento JSON por stdin; exit 0 deja pasar; exit 2 veta y stderr le llega al agente.
# Requiere: jq. Si falta, VETA (un guardrail que se apaga en silencio no garantiza nada).
set -uo pipefail

ALLOWED_TOOLS="list_objects read_object_metadata read_object_content"
ALLOWED_BUCKET="${GCSGREP_ALLOWED_BUCKET:-${CLAUDE_PLUGIN_OPTION_BUCKET_NAME:-gcsgrep-mcp-e4c0ad}}"

EVENT="$(cat)"
if ! command -v jq >/dev/null 2>&1; then
  echo "BLOQUEADO por gcs-allowlist: falta jq, no puedo inspeccionar la tool." >&2
  exit 2
fi

FULL="$(printf '%s' "$EVENT" | jq -r '.tool_name // ""')"
TOOL="${FULL##*__}"   # el nombre de la tool es lo que sigue al ÚLTIMO "__"

case " $ALLOWED_TOOLS " in
  *" $TOOL "*) ;;
  *)
    echo "BLOQUEADO por gcs-allowlist: la tool '$FULL' no está en la allowlist ($ALLOWED_TOOLS)." >&2
    echo "Este agente solo LEE el bucket de prueba. No reintentes con otra tool que escriba o borre." >&2
    exit 2 ;;
esac

BUCKET="$(printf '%s' "$EVENT" | jq -r '.tool_input.bucket_name // .tool_input.bucket // ""')"
if [ -n "$BUCKET" ] && [ "$BUCKET" != "$ALLOWED_BUCKET" ]; then
  echo "BLOQUEADO por gcs-allowlist: bucket '$BUCKET' != '$ALLOWED_BUCKET' (el único permitido)." >&2
  exit 2
fi
exit 0
