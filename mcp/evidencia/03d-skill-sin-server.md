# 03d — verify-against-bucket sin server MCP: el skill frena

Fecha: 2026-10-14T14:59:42Z. Transcripción completa: [`03d-transcripcion.md`](03d-transcripcion.md).

## Cómo se corrió

```
claude -p --strict-mcp-config --mcp-config '{"mcpServers":{}}' \
  --allowedTools "Skill,Agent,Read,Grep,Bash(date *)" --disallowedTools "Write,Edit" \
  --output-format stream-json --verbose  < prompt
```

Con `--strict-mcp-config` y una config vacía, el server `gcs` de `.mcp.json` no se carga: es el
caso "falta el server" (bug de config, server que no arranca o que no se aprobó). Mismo prompt que 03.

## Qué pasó, en orden

1. El skill `verify-against-bucket` se activó con el prompt (sin nombrarlo).
2. Paso 1: lanzó `bucket-reader`. Claude Code se negó a crearlo: *"would be spawned with zero tools
   — refusing … unrecognized [mcp__gcs__list_objects, …]"*. Sin server, el subagent lector no existe.
3. Buscó las tools (`ToolSearch "gcs list_objects"` → ninguna) y leyó `.mcp.json` para diagnosticar.
   Un `Bash` de diagnóstico (`ls; cat .mcp.json; claude mcp list`) quedó denegado por permisos.
4. **Frenó.** No hubo `gcloud`, `gsutil` ni `run-gcsgrep.sh`, y no escribió `03-skill.md`.

## Veredicto que dio

> **VEREDICTO: NO VERIFICADO — el server MCP no está conectado.**
> … Me detuve ahí, como pide la regla del skill: no usé `gcloud`/`gsutil` ni datos de memoria para
> reemplazar la lectura del bucket. Tampoco corrí `run-gcsgrep.sh`: sin la lectura del bucket no hay
> contra qué comparar su salida.

Resultado de la prueba: sin el bucket no hay veredicto verde ni evidencia inventada.
