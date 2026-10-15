#!/usr/bin/env bash
# run-gcsgrep.sh — corre gcsgrep y devuelve a la sesión SOLO lo que el contraste necesita.
#
# Por qué existe: la sesión que corre este script tiene Bash y red. El stdout de gcsgrep son
# líneas de objetos del bucket (contenido no confiable): si una línea con instrucciones matchea
# el patrón, leerlo crudo la metería en esa sesión. Este script se queda con `objeto:línea` y
# descarta el texto; el texto de los matches lo aporta bucket-reader, que no tiene Bash ni red
# y retiene las líneas que parecen instrucciones.
#
# Uso:   run-gcsgrep.sh [flags de gcsgrep] <patrón> gs://<bucket>/<prefijo>
# Env:   GCSGREP_DIR = módulo Go de gcsgrep (default: greenfield/gcsgrep, relativo al cwd).
# Salida: EXIT, la lista de objeto:línea y los avisos de stderr (nombre de objeto + causa; no traen
#         contenido). Límite conocido: un nombre de objeto que contenga ":<dígitos>:" se corta mal.
set -uo pipefail

DIR="${GCSGREP_DIR:-greenfield/gcsgrep}"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

if ! go -C "$DIR" build -o "$TMP/gcsgrep" ./cmd/gcsgrep 2>"$TMP/build.err"; then
  echo "BUILD: falló"; cat "$TMP/build.err"; exit 1
fi

"$TMP/gcsgrep" "$@" >"$TMP/out" 2>"$TMP/err"
rc=$?

echo "EXIT: $rc"
echo "MATCHES (objeto:línea; el texto se omite a propósito): $(wc -l <"$TMP/out" | tr -d ' ')"
perl -ne 'print "  $1:$2\n" if /^(.*?):(\d+):/' "$TMP/out"
echo "AVISOS (stderr de gcsgrep, sin líneas de progreso):"
tr '\r' '\n' <"$TMP/err" | grep '^gcsgrep: ' | sed 's/^/  /' || echo "  ninguno"
exit 0
