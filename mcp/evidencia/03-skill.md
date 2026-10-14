# 03 — verify-against-bucket (gcsgrep-mcp-e4c0ad, logs/, 'timeout')

Corrida: 2026-10-14 (gcsgrep 17:20Z; lectura del bucket ~17:30Z, hora aproximada). Cuenta: **declarada** `gcsgrep-mcp-reader@gcsgrep-sdd-e4c0ad.iam.gserviceaccount.com` (ADC impersonada); el server no la expone.

## Procedencia (token de frescura = time_created, updated, size)

| objeto | time_created | updated | size | token paso 5 |
|---|---|---|---|---|
| logs/app.log.gz | 2026-10-11T19:18:34.613Z | 2026-10-11T19:18:34.613Z | 86 | igual |
| logs/app1.log | 2026-10-11T19:18:35.275Z | 2026-10-11T19:18:35.275Z | 226 | igual |
| logs/app2.log | 2026-10-11T19:40:41.400Z | 2026-10-11T19:40:41.400Z | 82 | igual |
| logs/canary.log | 2026-10-14T17:10:23.707Z | 2026-10-14T17:10:23.707Z | 129 | igual |

## Esperado (bucket-reader) vs obtenido (gcsgrep, exit 0)

| objeto | esperado | obtenido | resultado |
|---|---|---|---|
| logs/app.log.gz | no contrastable (el server no descomprime, FR-12) | línea 2 | NO CONTRASTADO |
| logs/app1.log | 3, 5 | 3, 5 | coincide |
| logs/app2.log | ninguno | ninguno | coincide |
| logs/canary.log | 2 | 2 ◀ SOLO DEL BUCKET (nonce `94c13725e1f3`, ausente de `mcp/fixtures/`, Grep count = 0) | coincide |

Sin matches de más ni objetos salteados entre los contrastables. Sin ilegibles en este prefijo (exit 0 coherente).

## POSIBLE_INSTRUCCION

Ninguna.

## Veredicto

**VERDE — `logs/app.log.gz` no contrastado** (gcsgrep reporta `app.log.gz:2` tras descomprimir; el server MCP no puede confirmarlo, queda como limitación declarada).
