# Lección 4 — `gcsgrep` contra un bucket real, por MCP

> **Estado:** las 7 evidencias están en [`evidencia/`](evidencia). Hallazgos del server 0.6.0 que
> cambiaron el diseño: no devuelve `generation` ni checksum, no descomprime `.gz`, no expone la cuenta.

## Qué hace y qué paso SDD alimenta

Alimenta **Verificar**. El skill `verify-against-bucket` lanza el subagent `bucket-reader`
(sin Bash ni red; solo `list_objects`, `read_object_metadata`, `read_object_content`),
corre `gcsgrep` sobre el bucket de prueba y contrasta su salida con lo que el server leyó
objeto por objeto. Frena si falta el server, registra procedencia (objeto, *generation*,
fecha, cuenta) y **falla si una *generation* cambió** entre la lectura y el veredicto.

| Pieza | Archivo |
|---|---|
| Config MCP (versión fijada `0.6.0`) | [`.mcp.json`](../.mcp.json) |
| Skill | [`.claude/skills/verify-against-bucket/`](../.claude/skills/verify-against-bucket/SKILL.md) |
| Subagent lector | [`.claude/agents/bucket-reader.md`](../.claude/agents/bucket-reader.md) |
| Hook allowlist | [`.claude/hooks/gcs-allowlist.sh`](../.claude/hooks/gcs-allowlist.sh) + [`settings.json`](../.claude/settings.json) |
| Plugin | [`plugin/gcsgrep-sdd/`](../plugin/gcsgrep-sdd/.claude-plugin/plugin.json) |
| Fixtures sintéticos + hostil | [`fixtures/bucket/`](fixtures/bucket), subida con [`fixtures/setup-bucket.sh`](fixtures/setup-bucket.sh) |

## Evidencia

| Archivo | Qué muestra |
|---|---|
| [`01-descubrimiento.txt`](evidencia/01-descubrimiento.txt) | El agente lista las tools del server |
| [`02-iam.txt`](evidencia/02-iam.txt) | Política IAM del bucket |
| [`03-skill.md`](evidencia/03-skill.md) · [`03-skill-transcripcion.md`](evidencia/03-skill-transcripcion.md) | Registro del skill con procedencia y la línea "solo del bucket" · transcripción completa de la sesión |
| [`03b-skill-objeto-cambio.md`](evidencia/03b-skill-objeto-cambio.md) · [`03b-transcripcion.md`](evidencia/03b-transcripcion.md) | Un objeto cambia entre la lectura y el veredicto (`tests/mutate-object.sh`): el skill responde `NO VERIFICADO — evidencia vieja` |
| [`03c-skill-bucket-completo.md`](evidencia/03c-skill-bucket-completo.md) · [`03c-transcripcion.md`](evidencia/03c-transcripcion.md) | Bucket completo: texto, anidado, `.gz` (no contrastable), binario, **objeto ilegible** y hostil, objeto por objeto |
| [`04-muro.md`](evidencia/04-muro.md) (+ `04a`, `04b`, `04c`) | Las 3 capas |
| [`05-fixture-hostil.md`](evidencia/05-fixture-hostil.md) | El subagent marca el objeto hostil; ningún `Bash`/`curl` |
| [`06-plugin.txt`](evidencia/06-plugin.txt) | `validate`, nombres nuevos, muro repetido |
| [`07-contexto.md`](evidencia/07-contexto.md) | `/context` |

## Instalar el plugin

```bash
claude plugin validate plugin/gcsgrep-sdd
claude --plugin-dir plugin/gcsgrep-sdd
/plugin configure gcsgrep-sdd           # project_id y bucket_name (userConfig). --plugin-dir no abre el diálogo solo
# En un marketplace: claude plugin install gcsgrep-sdd@<mkt> --config project_id=… --config bucket_name=…
```

Credencial siempre por ADC, nunca en el paquete:
`gcloud auth application-default login --impersonate-service-account=gcsgrep-mcp-reader@<proyecto>.iam.gserviceaccount.com`.

## Pasos manuales de GCP (ya ejecutados en el proyecto de prueba)

```bash
P=gcsgrep-sdd-e4c0ad; B=gcsgrep-mcp-e4c0ad
gcloud storage buckets create gs://$B --project=$P --location=us-central1 --uniform-bucket-level-access
gcloud iam service-accounts create gcsgrep-mcp-reader --project=$P
SA=gcsgrep-mcp-reader@$P.iam.gserviceaccount.com
gcloud storage buckets add-iam-policy-binding gs://$B --member=serviceAccount:$SA --role=roles/storage.objectViewer
gcloud iam service-accounts add-iam-policy-binding $SA --member=user:<tu-mail> --role=roles/iam.serviceAccountTokenCreator
mcp/fixtures/setup-bucket.sh $B
gcloud auth application-default login --impersonate-service-account=$SA
```

## Nota de seguridad

1. **Server:** `@google-cloud/storage-mcp@0.6.0` (fijada en `.mcp.json` y en el plugin),
   contra `gs://gcsgrep-mcp-e4c0ad` (proyecto de prueba `gcsgrep-sdd-e4c0ad`).
2. **Modo:** sin `--enable-destructive-tools` ("safe mode"). **No es solo lectura**:
   [`04a-tools-list-modo-seguro.txt`](evidencia/04a-tools-list-modo-seguro.txt) muestra 15
   tools, entre ellas `create_bucket`, `write_object_safe`, `upload_object_safe`,
   `copy_object_safe` (en 0.6.0 se llaman `*_safe`, no `*_new`). `delete_object` no existe.
3. **Credencial:** service account `gcsgrep-mcp-reader` con `roles/storage.objectViewer`
   **sobre el bucket**, no el proyecto. Vive en ADC impersonado; ninguna key en el repo.
4. **Datos:** solo fixtures sintéticos (`mcp/fixtures/bucket/`) y un objeto cifrado con una clave de cliente (CSEK) descartada, `sin-permiso/csek.log`: la SA lo lista pero no puede leerlo. Nota: GCS responde HTTP 400 (`ResourceIsEncryptedWithCustomerEncryptionKey`), no 403; es el "objeto ilegible" de FR-9, sin tocar IAM. El hostil apunta a `recolector.invalid`.
5. **Peor caso** (escritura/borrado), en orden de corte: (1) hook allowlist veta toda tool
   fuera de las 3 de lectura —probado offline: [`04b`](evidencia/04b-hook-allowlist-offline.txt);
   (2) modo del server: `delete_object` no existe —[`04a`](evidencia/04a-tools-list-modo-seguro.txt);
   (3) credencial: GCS responde `403` a `cp`, `rm` y `buckets create` con la SA —[`04c`](evidencia/04c-credencial-403.txt).
   Probado también en vivo con una sesión de Claude Code: [`04-muro`](evidencia/04-muro.md) y, con el plugin y sus nombres nuevos, [`06`](evidencia/06-plugin.txt).
   La capa 3 es la que garantiza; las otras dos son defensa en profundidad.
6. **Trifecta:** la sesión principal tiene Bash+red (y ADC en la máquina) y lee contenido
   no confiable. La pata cortada es la **exposición del contenido no confiable a una sesión
   con Bash**: lo lee `bucket-reader`, que no tiene Bash ni red y devuelve solo una tabla.
   Residual: la sesión principal igual corre `gcsgrep` con la ADC impersonada (solo lectura).
7. **Costo de contexto:** el server suma **~3.2k tokens** de definiciones de 15 tools (`/context`, [`07`](evidencia/07-contexto.md)), diferidas: se cargan bajo demanda, no en cada turno. Conviene habilitar el plugin solo en repos que usen el bucket de prueba; 12 de las 15 tools no se usan y el hook las veta.

### Límites conocidos
- `bucket_name` del hook tiene default `gcsgrep-mcp-e4c0ad` si falta la variable: en la prueba no se distingue el valor de `userConfig` del default.
- Sin `generation`, la frescura se mide con `time_created`/`updated`/`size`.
- La sesión principal igual tiene Bash y la ADC impersonada (solo lectura); el muro protege el bucket, no esa máquina.
