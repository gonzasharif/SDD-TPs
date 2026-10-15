#!/usr/bin/env bash
# mutate-object.sh — simula que un objeto cambió mientras se verificaba (prueba de frescura).
# Sobrescribe un fixture con SU MISMO contenido: el bytes no cambian, pero GCS crea una generation
# nueva y mueve time_created/updated. Usa la cuenta dueña de gcloud (NO la ADC de solo lectura:
# esa no puede escribir, que es justo lo que prueba el muro).
set -euo pipefail
BUCKET="${GCSGREP_BUCKET:-gcsgrep-mcp-e4c0ad}"; OBJ="${1:-logs/app2.log}"
SRC="$(dirname "$0")/../fixtures/bucket/$OBJ"
gcloud storage cp "$SRC" "gs://$BUCKET/$OBJ" --account=gonzalocuri@gmail.com 2>&1 | tail -1
gcloud storage objects describe "gs://$BUCKET/$OBJ" --format='value(generation,updated)' --account=gonzalocuri@gmail.com
