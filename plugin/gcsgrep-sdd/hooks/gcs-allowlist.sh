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
# Prefijos del server: en el repo (mcp__gcs__) y empaquetado como plugin (mcp__plugin_gcsgrep-sdd_gcs__).
ALLOWED_PREFIXES="mcp__gcs__ mcp__plugin_gcsgrep-sdd_gcs__"
ALLOWED_BUCKET="${GCSGREP_ALLOWED_BUCKET:-${CLAUDE_PLUGIN_OPTION_BUCKET_NAME:-gcsgrep-mcp-e4c0ad}}"

EVENT="$(cat)"
if ! command -v jq >/dev/null 2>&1; then
  echo "BLOQUEADO por gcs-allowlist: falta jq, no puedo inspeccionar la tool." >&2
  exit 2
fi

FULL="$(printf '%s' "$EVENT" | jq -r '.tool_name // ""')"

# Se compara el nombre COMPLETO (prefijo + tool), no un sufijo: una tool futura llamada
# p. ej. "admin__read_object_content" no puede colarse por terminar igual que una permitida.
ALLOWED=0
for prefix in $ALLOWED_PREFIXES; do
  for tool in $ALLOWED_TOOLS; do
    [ "$FULL" = "$prefix$tool" ] && ALLOWED=1
  done
done
if [ "$ALLOWED" -ne 1 ]; then
  echo "BLOQUEADO por gcs-allowlist: la tool '$FULL' no está en la allowlist ($ALLOWED_TOOLS)." >&2
  echo "Este agente solo LEE el bucket de prueba. No reintentes con otra tool que escriba o borre." >&2
  exit 2
fi

# Las tres tools de lectura actúan sobre un bucket: sin bucket explícito, no hay lectura que permitir.
BUCKET="$(printf '%s' "$EVENT" | jq -r '.tool_input.bucket_name // .tool_input.bucket // ""')"
if [ "$BUCKET" != "$ALLOWED_BUCKET" ]; then
  echo "BLOQUEADO por gcs-allowlist: bucket '${BUCKET:-<vacío>}' != '$ALLOWED_BUCKET' (el único permitido)." >&2
  exit 2
fi
exit 0
