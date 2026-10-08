# Spec — `gcsgrep` lee objetos de Amazon S3 además de GCS

> Spec **brownfield** sobre el código de [`gcsgrep/`](gcsgrep/). Los números `FR-n`, `BR-n`,
> `NFR-n` e `INV-n` de este documento son **de este cambio**; los de
> [`gcsgrep-spec.md`](gcsgrep-spec.md) se citan siempre como "spec base".

**Repo:** repositorio `SDD` (carpeta `greenfield/gcsgrep`, módulo Go `gcsgrep`) · commit base **`a3ed3afbd452`** (2026-10-08)
**Entrega:** rama `feature-s3`, creada desde `a3ed3afbd452`; su `HEAD` al cerrar la iteración 5 del plan es el commit de entrega, que se anota en la descripción del pull request.

## Propósito

Hoy `gcsgrep PATRÓN gs://bucket/prefijo` busca dentro de objetos de Google Cloud Storage.
El cambio agrega la ubicación `s3://bucket/prefijo`: la misma búsqueda, con la misma salida,
los mismos modos (`-i`, `-n`, `-l`, `-c`), los mismos guardarraíles y la misma política de
reintentos, pero leyendo de un bucket de Amazon S3 con las credenciales estándar de AWS.

**El límite:** el cambio agrega un cliente de almacenamiento (una implementación nueva de la
interfaz `gcsclient.Client`) y el cableado que elige cliente según el esquema de la
ubicación. El motor de búsqueda (`internal/reader`, `internal/match`, `internal/output`), el
cliente de GCS y el decorador de reintentos **no se modifican**. Una corrida con `gs://` se
comporta como antes, byte a byte.

## Glosario

Un término por concepto; el resto de la spec lo usa sin variantes.

| Término | Significado |
|---|---|
| **ubicación** | El segundo argumento posicional: `gs://<bucket>[/<prefijo>]` o `s3://<bucket>[/<prefijo>]` |
| **esquema** | `gs` o `s3`: lo que precede a `://` en una ubicación |
| **bucket** / **prefijo** | La parte de la ubicación entre `://` y el primer `/`, y lo que sigue a ese `/` (vacío si no hay) |
| **nombre** | El nombre de un objeto tal como lo lista el servicio (en S3, la *key*); es lo que `gcsgrep` imprime como `<objeto>` |
| **corrida S3** / **corrida GCS** | Una ejecución de `gcsgrep` cuya ubicación tiene esquema `s3` / `gs` |
| **aviso** | Línea `gcsgrep: warning: …` por stderr |
| **error** | Línea `gcsgrep: error: …` por stderr |
| **progreso** | Líneas `gcsgrep: progress: <n>/<total> (<p>%)` por stderr cuando stderr no es una terminal (`output.Writer.Progress`). En los VCs se filtran con `grep -v '^gcsgrep: progress: '`; "stderr" en un VC significa stderr sin progreso |
| **error de uso** | Rechazo de la invocación antes de construir ningún cliente y de hacer ninguna llamada de red: argumentos, flags, patrón o ubicación inválidos (FR-4, FR-27 y los de la spec base FR-22) |
| **error de corrida** | La corrida termina con un error que no es de uso: sin región (FR-8), sin credenciales (FR-9), listado denegado (FR-10), bucket inexistente (FR-11), listado agotado (NFR-1), `--max` superado (FR-22) o tope total alcanzado (FR-24) |
| **objeto fallido** | Objeto que no se pudo leer entero; emite un aviso `<objeto>: <causa>` y la corrida sigue con el resto: FR-12 … FR-16, FR-20, FR-23 y la apertura agotada de NFR-1 |
| **llamada** | Una petición HTTP al servicio de almacenamiento |
| **S3 de prueba** | El servidor `testdata/fake-s3/fake_s3.py` descripto en "Harness de prueba" |
| **env S3** | El entorno de las corridas S3 de los VCs: ver "Harness de prueba" |

Los caminos de falla de este cambio son cuatro y cada uno tiene FR y VC: **sin coincidencia o
vacío** (FR-5), **recurso ilegible** (FR-12 … FR-16, FR-20), **sin credenciales o sin permiso**
(FR-8, FR-9, FR-10, FR-12), **entrada inválida** (FR-4, FR-27). La precedencia entre varios
errores de uso o de configuración a la vez es BR-4.

## Alcance

### Dentro

| Archivo | Cambio |
|---|---|
| `internal/s3client/s3.go` (**nuevo**) | `New(ctx)`: arma el cliente S3 con la cadena de credenciales y la región estándar de AWS. Implementa `gcsclient.Client` (`List`, `Open`). Traduce errores del SDK a los errores de dominio de `gcsclient`. Único archivo de producción que importa `github.com/aws/` |
| `internal/s3client/s3_test.go` (**nuevo**) | Tests de la traducción de errores y del listado paginado |
| `internal/gcsclient/gcsclient.go` | Un error de dominio nuevo, `ErrObjectArchived`, junto a `ErrPermissionDenied`, `ErrBucketNotFound` y `ErrObjectNotFound` |
| `internal/cli/cli.go` | `Args` gana `Scheme`; `parseLocation` acepta `s3://` con la misma forma que `gs://` |
| `internal/app/app.go` | `RunWithClients` (nuevo) elige la fábrica de clientes por esquema; `Run` y `RunWithTerminals` conservan su firma y delegan en él con la fábrica de GCS. Si la fábrica falla, imprime `gcsgrep: error: ` más el mensaje de ese esquema (decisión 19) |
| `internal/scanner/scanner.go` | `Config` gana `Scheme`; `Run` y `describeListError` arman `<esquema>://<bucket>/<prefijo>` en los mensajes que hoy fijan `gs://`; `describeOpenError` traduce `ErrObjectArchived` |
| `cmd/gcsgrep/main.go` | Pasa las dos fábricas (`gcsclient.New`, `s3client.New`) |
| `go.mod`, `go.sum` | Dependencias `github.com/aws/aws-sdk-go-v2` ≥ v1.47.1, `…/config` ≥ v1.33.7, `…/service/s3` ≥ v1.114.1 y `github.com/aws/smithy-go` ≥ v1.28.1 (las versiones de la sonda en `gcsgrep-s3-sonda.md`) |
| `internal/invariants/invariants_test.go` | Función nueva `TestReadOnlyObjectStoreSurface`: falla si el código de producción usa operaciones de escritura de S3 (BR-1, VC-INV3.1). Las funciones existentes no cambian |
| `integration/s3_integration_test.go` (**nuevo**) | Los VCs de este documento que corren el binario contra el S3 de prueba (build tag `integration`) |
| `testdata/fake-s3/fake_s3.py` (**nuevo**) | El S3 de prueba |
| `testdata/setup-testdata-s3.sh` (**nuevo**) | Crea los datos de prueba (`$ROOT`) |

### Fuera de alcance

- **`internal/gcsclient/gcs.go`, `internal/gcsclient/retry.go`** — el cliente de GCS y el
  decorador `WithRetries` se reutilizan sin tocar (INV-4).
- **`internal/reader/*`, `internal/match/*`, `internal/output/*`, `internal/tty/*`** — el
  tratamiento del contenido (binarios, `.gz`, líneas largas, progreso, color) es el mismo para
  los dos esquemas (INV-4).
- **`internal/gcsclient/gcsclienttest/fake.go`** — el cliente falso ya cumple
  `gcsclient.Client`; no se modifica.
- **Tests existentes** — ninguno se modifica ni se borra; los archivos de test que se tocan solo ganan funciones nuevas (`invariants_test.go`: `TestReadOnlyObjectStoreSurface`).
- **Flags para región, endpoint, perfil o claves** — no existen; todo sale de la configuración
  estándar de AWS (BR-5).
- **Acceso anónimo a buckets públicos** — sin credenciales la corrida falla (FR-9).
- **Detección automática de la región del bucket** — la región se configura (FR-8).
- **Access points, ARNs de bucket, URLs `https://`, `s3a://`, `s3n://`** — solo `s3://<bucket>[/<prefijo>]` (FR-27).
- **Versionado, `versionId`, requester-pays, objetos cifrados con clave del cliente (SSE-C)** — se
  leen con los valores por defecto del servicio; un rechazo del servicio sigue FR-15.
- **S3 Express One Zone (directory buckets)** — fuera de alcance.
- **Iniciar el restore de un objeto archivado** — `gcsgrep` solo lee (BR-1); un objeto archivado es un objeto fallido (FR-14).
- **Mezclar `gs://` y `s3://` en una corrida** — la ubicación es una sola.
- **`-i`, `-n` y el sabor de regex (RE2)** — lógica pura de `internal/match`, idéntica para los dos esquemas.
- **Concurrencia (`--concurrency`)** — no existe en el código del commit base (`cli.Parse` no la define).

## Invariantes

Lo que sigue siendo verdad **después** del cambio. Cada invariante tiene sus VCs en "VCs de invariantes".

| # | Invariante | VC |
|---|---|---|
| **INV-1** | Una corrida GCS se comporta como antes: la suite base sigue en verde y el rechazo de una ubicación sin esquema reconocido conserva su texto | VC-INV1.1, VC-INV1.2 |
| **INV-2** | Una corrida GCS sin ADC falla con el mismo error aunque el entorno tenga variables de AWS | VC-INV2.1 |
| **INV-3** | `gcsgrep` no tiene ninguna operación de escritura sobre S3 ni sobre GCS | VC-INV3.1, VC-INV3.2 |
| **INV-4** | El motor de búsqueda, el cliente de GCS y el decorador de reintentos no cambian | VC-INV4.1 |
| **INV-5** | Cada SDK de nube se importa desde un solo paquete | VC-INV5.1 |
| **INV-6** | Ningún camino de código escribe contenido de objetos a disco | VC-INV6.1 |
| **INV-7** | Una corrida construye solo el cliente de su esquema | VC-INV7.1, VC-INV7.2 |

## Línea de base de regresión

Medida **antes** de tocar una línea, sobre `a3ed3afbd452`, en `greenfield/gcsgrep`
(Linux, Go 1.26.1):

```bash
go vet ./...                                            # exit 0, sin salida
go test ./... -count=1                                  # exit 0; 8 paquetes "ok", 0 "FAIL"
go test ./... -count=1 -v | grep -c '^--- PASS'         # 123
go test ./... -count=1 -v | grep -c '^--- FAIL'         # 0
```

La línea de base es "esos 123 tests de primer nivel en `PASS` y 0 en `FAIL`". Al cerrar cada
iteración la misma corrida da `FAIL = 0` y `PASS ≥ 123`; un test de la línea de base que se pone
rojo es una regresión y se corrige en el código, no en el test. Los tests contra GCS real
(`go test -tags integration ./integration/...`) necesitan credenciales y no forman parte de la
línea de base medida; los VCs de este documento no los reemplazan.

## Harness de prueba

Lo usan los VCs que corren el binario. Todo vive en `testdata/` y en
`integration/s3_integration_test.go`; el feature nunca lo invoca.

- **Binario** `$G`: `go build -o $G ./cmd/gcsgrep` (en `greenfield/gcsgrep`).
- **S3 de prueba** (`testdata/fake-s3/fake_s3.py`, Python 3 estándar, sin dependencias):
  servidor HTTP sin TLS en `127.0.0.1:$S3PORT`, direccionamiento *path-style*
  (`/<bucket>?list-type=2&prefix=…`, `/<bucket>/<nombre>`). Solo implementa `ListObjectsV2`
  (`GET /<bucket>?list-type=2`, respuesta XML, claves codificadas en URL si el pedido trae
  `encoding-type=url`, páginas de `--page-size` objetos con `NextContinuationToken`) y
  `GetObject` (`GET /<bucket>/<nombre>`, con `Content-Length`). Cualquier otro método
  responde `405` (queda en el log).
  - `--root $ROOT`: el bucket `b` es el directorio `$ROOT/b`; el objeto `k`, el archivo
    `$ROOT/b/k`. Un bucket sin directorio responde `404` con código `NoSuchBucket`. La
    ordenación del listado es por bytes UTF-8 del nombre.
  - `--page-size N` (default `1000`).
  - `--log $LOG`: una línea por petición: `<epoch_ms> <MÉTODO> <ruta?query> <status> <bytes_enviados> <access_key>`, donde `<access_key>` se lee de `Credential=<access_key>/` en `Authorization`.
  - `--rule <op>:<bucket>/<clave>:<acción>` (repetible). `<op>` es `list` (la clave es el prefijo pedido, literal) o `get`. Acciones: `status=<código>[,times=<n>]` (responde ese HTTP con cuerpo de error S3; `times` limita a las primeras `n` peticiones, sin `times` son todas), `archived` (en `get`: `403` con código `InvalidObjectState`), `reset[,times=<n>]` (en `get`: cierra el socket con RST antes de responder) y `cut=<bytes>` (en `get`: declara el `Content-Length` completo, envía `<bytes>` del cuerpo, espera 100 ms y cierra el socket con RST, `SO_LINGER` en 0). Códigos S3 por status: `400` `InvalidRequest`, `403` `AccessDenied`, `404` `NoSuchKey` en `get` y `NoSuchBucket` en `list`, `408` `RequestTimeout`, `503` `ServiceUnavailable`.
  - Arranque: `python3 testdata/fake-s3/fake_s3.py --port $S3PORT --root $ROOT --page-size 2 --log $LOG [--rule …] &`; se espera hasta que `curl -s -o /dev/null http://127.0.0.1:$S3PORT/b` responda. Un VC que pide `--rule` reinicia el servidor con esas reglas y un `$LOG` vacío.
- **Datos** `$ROOT` (los crea `testdata/setup-testdata-s3.sh`; `\n` es un salto de línea):

| Bucket / prefijo | Objetos y contenido | Usado en |
|---|---|---|
| `b/logs/` | `logs/a.log` = `INFO start\nERROR timeout\n`; `logs/b.log` = `INFO ok\n` | VC-1.1, VC-BR1.1, VC-BR2.x, VC-BR3.1, VC-INV7.x |
| bucket `whole` | `x.log` = `timeout\n`; `d/y.log` = `INFO\ntimeout\n` | VC-2.1 |
| `b/pfx/` | `pfx/logs/a.log`, `pfx/logs-old/b.log`, `pfx/other/c.log` = `timeout\n` cada uno | VC-3.1, VC-3.2 |
| `b/pg/` | `pg/00.log` … `pg/04.log` = `timeout\n` cada uno | VC-6.1 |
| `b/sp/` | `sp/a b+c%é.log` = `timeout\n` | VC-7.1 |
| `b/acl/` | `acl/1.log`, `acl/2.log` = `timeout\n`; `acl/denied.log` = `secreto timeout\n` | VC-12.1 |
| `b/gone/` | `gone/a.log` = `timeout\n`; `gone/b.log` = `timeout\n` | VC-13.1 |
| `b/arch/` | `arch/a.log` = `timeout\n`; `arch/b.log` = `timeout\n` | VC-14.1 |
| `b/odd/` | `odd/a.log` = `timeout\n`; `odd/b.log` = `timeout\n` | VC-15.1 |
| `b/r/` | `r/a.log` = `timeout\n`; `r/mid.log` = `timeout uno\nINFO\ntimeout tres\n` | VC-16.1, VC-NFR1.x |
| `b/e/` | `e/empty.log` = 0 bytes; `e/a.log` = `timeout\n` | VC-17.1 |
| `b/n/` | `n/last.log` = `uno\ndos timeout` (sin `\n` final) | VC-18.1 |
| `b/gz/` | `gz/app.log.gz` = gzip de `INFO start\nERROR timeout\n` | VC-19.1 |
| `b/gzbad/` | `gzbad/ok.log` = `timeout\n`; `gzbad/bad.gz` = `esto no es gzip\n` | VC-20.1, VC-BR2.3 |
| `b/bin/` | `bin/icon.png` = los bytes `89 50 4E 47 0D 0A 1A 0A 00 00 00 0D 49 48 44 52`; `bin/a.log` = `timeout\n` | VC-21.1 |
| `b/max/` | `max/00.log` … `max/02.log` = `timeout\n` cada uno | VC-22.1 |
| `b/big/` | `big/big.log` = 2 MiB: línea 1 `timeout`, resto líneas `INFO ok` | VC-23.1 |
| `b/tot/` | `tot/1.log` … `tot/5.log` = 1 MiB cada uno: línea 1 `timeout`, resto líneas `INFO ok` | VC-24.1 |
| `b/l/` | `l/big.log` = 100 MiB: línea 1 `timeout`, resto líneas `INFO ok` | VC-25.1 |
| `b/c/` | `c/three.log` = `timeout 1\nINFO\ntimeout 2\nINFO\ntimeout 3\n` | VC-26.1, VC-BR1.1 |
| `b/mem/` | `mem/small.log` = 5 MiB y `mem/large.log` = 500 MiB, ambos de líneas `INFO ok` | VC-NFR2.1 |

  `b/vacio/` no tiene ningún objeto. Los buckets `b` y `whole` existen; `nobucket` no.
- **env S3** (todas las corridas S3 salvo que el VC diga otra cosa): función `s3run`:
  `env -i PATH=/usr/bin:/bin HOME=$H AWS_ENDPOINT_URL_S3=http://127.0.0.1:$S3PORT AWS_ACCESS_KEY_ID=AKTEST AWS_SECRET_ACCESS_KEY=secret AWS_REGION=us-east-1 AWS_EC2_METADATA_DISABLED=true "$G" "$@"`,
  con `$H` un directorio temporal vacío (sin `~/.aws`). Un VC que cambia el entorno lo dice.
- **S3 real** (un solo VC, VC-1.2, y VC-BR6.1): bucket `<bucket-s3>` en `us-east-1` con `logs/a.log` y `logs/b.log` del mismo contenido que arriba; credenciales del usuario `<aws-admin>` (lectura y escritura) y del usuario `<aws-minimo>` con la política de BR-6. Los nombres reales no se versionan: están en `testenv.local.md`, como en la spec base.
- **Herramientas:** `python3` ≥ 3.8, `curl`, `gzip`, `nc` (netcat-openbsd, con `-l`), `/usr/bin/time` (GNU time, para `-v`), `awk`, `sort`, `cmp`, `env`, `bash`, `go` 1.26.1.

## Requerimientos

Un *cuando* y un resultado observable por FR. El exit code de cada corrida lo fija BR-2 y los VCs
lo verifican; los FRs describen el único resultado propio de cada caso.

### Ubicación

#### FR-1 · Una ubicación `s3://bucket/prefijo` busca en los objetos del prefijo

**Dado** el S3 de prueba con los datos de `b/logs/`,
**cuando** se ejecuta `gcsgrep timeout s3://b/logs/`,
**entonces** stdout contiene la línea `logs/a.log:2:ERROR timeout`.

**VC-1.1:** `s3run timeout s3://b/logs/` → stdout `logs/a.log:2:ERROR timeout` (una sola línea, 27 bytes con su `\n`), stderr vacío, exit `0`.
**VC-1.2:** contra el bucket `<bucket-s3>` real con las credenciales de `<aws-admin>` y `AWS_REGION=us-east-1`: `gcsgrep timeout s3://<bucket-s3>/logs/` → stdout `logs/a.log:2:ERROR timeout`, stderr vacío, exit `0`.

#### FR-2 · Una ubicación `s3://bucket` sin prefijo busca en todo el bucket

**Dado** el bucket `whole` con `x.log` y `d/y.log`,
**cuando** se ejecuta `gcsgrep timeout s3://whole`,
**entonces** stdout es la secuencia ordenada `d/y.log:2:timeout`, `x.log:1:timeout`.

**VC-2.1:** `s3run timeout s3://whole` → stdout exactamente `d/y.log:2:timeout\nx.log:1:timeout\n` (34 bytes), stderr vacío, exit `0`. El log del S3 de prueba muestra un `GET /whole?list-type=2` con `prefix=` vacío o ausente.

#### FR-3 · Un prefijo sin `/` final es un prefijo de nombre

**Dado** los objetos `pfx/logs/a.log`, `pfx/logs-old/b.log` y `pfx/other/c.log`,
**cuando** se ejecuta `gcsgrep timeout s3://b/pfx/logs`,
**entonces** stdout es la secuencia ordenada `pfx/logs-old/b.log:1:timeout`, `pfx/logs/a.log:1:timeout`.

**VC-3.1:** `s3run timeout s3://b/pfx/logs` → stdout exactamente `pfx/logs-old/b.log:1:timeout\npfx/logs/a.log:1:timeout\n` (54 bytes; `-` ordena antes que `/`), stderr vacío, exit `0`.
**VC-3.2:** `s3run timeout s3://b/pfx/logs/` → stdout exactamente `pfx/logs/a.log:1:timeout\n` (25 bytes), stderr vacío, exit `0`; `pfx/other/c.log` y `pfx/logs-old/b.log` no aparecen.

#### FR-4 · Una ubicación `s3://` sin nombre de bucket se rechaza

**Dado** una ubicación `s3://` sin nombre de bucket,
**cuando** se ejecuta `gcsgrep timeout s3://`,
**entonces** stderr contiene el error `gcsgrep: error: invalid location "s3://": missing bucket name`.

**VC-4.1:** `s3run timeout s3://` → stdout vacío, stderr exactamente `gcsgrep: error: invalid location "s3://": missing bucket name\n`, exit `2`, sin S3 de prueba levantado (cualquier llamada de red cambiaría el mensaje).
**VC-4.2:** `s3run timeout s3:///logs/` → stdout vacío, stderr exactamente `gcsgrep: error: invalid location "s3:///logs/": missing bucket name\n`, exit `2`, sin S3 de prueba levantado.

#### FR-5 · Un prefijo sin objetos avisa y no es un error

**Dado** el bucket `b`, sin objetos bajo el prefijo `vacio/`,
**cuando** se ejecuta `gcsgrep timeout s3://b/vacio/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: no objects under s3://b/vacio/`.

**VC-5.1:** `s3run timeout s3://b/vacio/` → stdout vacío, stderr exactamente `gcsgrep: warning: no objects under s3://b/vacio/\n`, exit `1`, cero `GET` de objeto en `$LOG` (solo el listado).

#### FR-6 · El listado recorre todas las páginas

**Dado** el prefijo `pg/` con 5 objetos y el S3 de prueba con `--page-size 2`,
**cuando** se ejecuta `gcsgrep timeout s3://b/pg/`,
**entonces** stdout contiene las 5 líneas `pg/00.log:1:timeout` … `pg/04.log:1:timeout`, en orden de nombre.

**VC-6.1:** `s3run timeout s3://b/pg/` → stdout exactamente las líneas `pg/00.log:1:timeout` … `pg/04.log:1:timeout` (5 líneas, 100 bytes), stderr vacío, exit `0`; `$LOG` tiene exactamente 3 peticiones `GET /b?list-type=2` (páginas de 2, 2 y 1 objetos).

#### FR-7 · Los nombres con caracteres especiales se listan y se leen

**Dado** el objeto `sp/a b+c%é.log` (espacio, `+`, `%` y un carácter no ASCII),
**cuando** se ejecuta `gcsgrep timeout s3://b/sp/`,
**entonces** stdout contiene la línea `sp/a b+c%é.log:1:timeout`.

**VC-7.1:** `s3run timeout s3://b/sp/` → stdout exactamente `sp/a b+c%é.log:1:timeout\n` (26 bytes), stderr vacío, exit `0`; `$LOG` tiene un `GET` de objeto con status `200`.

### Configuración y acceso

#### FR-8 · Sin región configurada la corrida falla antes de llamar al servicio

**Dado** un entorno con credenciales AWS y sin región (ni `AWS_REGION`, ni `AWS_DEFAULT_REGION`, ni región en el perfil),
**cuando** se ejecuta `gcsgrep timeout s3://b/logs/`,
**entonces** stderr contiene el error `gcsgrep: error: no AWS region configured: set AWS_REGION or a region in the AWS profile`.

**VC-8.1:** `s3run` sin la variable `AWS_REGION` (el resto del entorno igual, `HOME=$H` vacío) con `timeout s3://b/logs/` → stdout vacío, stderr exactamente `gcsgrep: error: no AWS region configured: set AWS_REGION or a region in the AWS profile\n`, exit `2`, cero llamadas en `$LOG`.

#### FR-9 · Sin credenciales AWS la corrida falla antes de llamar al servicio

**Dado** un entorno con región y sin credenciales AWS en ninguna fuente de la cadena estándar,
**cuando** se ejecuta `gcsgrep timeout s3://b/logs/`,
**entonces** stderr contiene una línea que empieza con `gcsgrep: error: no AWS credentials found: `.

**VC-9.1:** `env -i PATH=/usr/bin:/bin HOME=$H AWS_ENDPOINT_URL_S3=http://127.0.0.1:$S3PORT AWS_REGION=us-east-1 AWS_EC2_METADATA_DISABLED=true $G timeout s3://b/logs/` (`$H` vacío) → stdout vacío, stderr con una línea que empieza con `gcsgrep: error: no AWS credentials found: ` y ninguna otra, exit `2`, cero llamadas en `$LOG`.

#### FR-10 · Un listado denegado se informa y no se reintenta

**Dado** un bucket cuyo listado responde HTTP 403,
**cuando** se ejecuta `gcsgrep timeout s3://b/logs/`,
**entonces** stderr contiene el error `gcsgrep: error: permission denied listing s3://b/logs/`.

**VC-10.1:** con el S3 de prueba arrancado con `--rule list:b/logs/:status=403` → `s3run timeout s3://b/logs/` → stdout vacío, stderr exactamente `gcsgrep: error: permission denied listing s3://b/logs/\n`, exit `2`, exactamente 1 petición de listado en `$LOG`, cero `GET` de objeto.

#### FR-11 · Un bucket inexistente se informa y no se reintenta

**Dado** una ubicación cuyo bucket no existe,
**cuando** se ejecuta `gcsgrep timeout s3://nobucket/`,
**entonces** stderr contiene el error `gcsgrep: error: bucket nobucket does not exist`.

**VC-11.1:** `s3run timeout s3://nobucket/` → stdout vacío, stderr exactamente `gcsgrep: error: bucket nobucket does not exist\n`, exit `2`, exactamente 1 petición de listado en `$LOG` (status `404`).

#### FR-12 · Un objeto sin permiso de lectura es un objeto fallido

**Dado** el prefijo `acl/`, donde la lectura de `acl/denied.log` responde HTTP 403,
**cuando** se ejecuta `gcsgrep timeout s3://b/acl/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: acl/denied.log: permission denied`.

**VC-12.1:** con `--rule get:b/acl/denied.log:status=403` → `s3run timeout s3://b/acl/` → stdout exactamente `acl/1.log:1:timeout\nacl/2.log:1:timeout\n` (40 bytes), stderr exactamente `gcsgrep: warning: acl/denied.log: permission denied\n`, exit `2`, exactamente 1 petición `GET /b/acl/denied.log` en `$LOG`.

#### FR-13 · Un objeto que desaparece entre el listado y la lectura es un objeto fallido

**Dado** el prefijo `gone/`, donde la lectura de `gone/a.log` responde HTTP 404,
**cuando** se ejecuta `gcsgrep timeout s3://b/gone/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: gone/a.log: object not found`.

**VC-13.1:** con `--rule get:b/gone/a.log:status=404` → `s3run timeout s3://b/gone/` → stdout exactamente `gone/b.log:1:timeout\n`, stderr exactamente `gcsgrep: warning: gone/a.log: object not found\n`, exit `2`, exactamente 1 petición `GET /b/gone/a.log`.

#### FR-14 · Un objeto archivado es un objeto fallido

**Dado** el prefijo `arch/`, donde la lectura de `arch/a.log` responde HTTP 403 con código `InvalidObjectState`,
**cuando** se ejecuta `gcsgrep timeout s3://b/arch/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: arch/a.log: archived object`.

**VC-14.1:** con `--rule get:b/arch/a.log:archived` → `s3run timeout s3://b/arch/` → stdout exactamente `arch/b.log:1:timeout\n`, stderr exactamente `gcsgrep: warning: arch/a.log: archived object\n` (no `permission denied`), exit `2`, exactamente 1 petición `GET /b/arch/a.log`.

#### FR-15 · Cualquier otro rechazo al abrir un objeto es un objeto fallido con su detalle

**Dado** el prefijo `odd/`, donde la lectura de `odd/a.log` responde HTTP 400,
**cuando** se ejecuta `gcsgrep timeout s3://b/odd/`,
**entonces** stderr contiene una línea que empieza con `gcsgrep: warning: odd/a.log: read failed: `.

**VC-15.1:** con `--rule get:b/odd/a.log:status=400` → `s3run timeout s3://b/odd/` → stdout exactamente `odd/b.log:1:timeout\n`, stderr con una sola línea, que empieza con `gcsgrep: warning: odd/a.log: read failed: `, exit `2`, exactamente 1 petición `GET /b/odd/a.log`.

#### FR-16 · Un corte a mitad de la lectura de un objeto es un objeto fallido que no se reintenta

**Dado** el objeto `r/mid.log`, cuyo cuerpo se corta tras la línea 2,
**cuando** se ejecuta `gcsgrep timeout s3://b/r/`,
**entonces** stderr contiene una línea que empieza con `gcsgrep: warning: r/mid.log: read interrupted: `.

**VC-16.1:** con `--rule get:b/r/mid.log:cut=17` (17 bytes = `timeout uno\nINFO\n`) → `s3run timeout s3://b/r/` → stdout exactamente `r/a.log:1:timeout\nr/mid.log:1:timeout uno\n` (la línea de `r/mid.log` una sola vez; `r/a.log`, que va antes, se lee entero), stderr con una sola línea, que empieza con `gcsgrep: warning: r/mid.log: read interrupted: `, exit `2`, exactamente 1 petición `GET /b/r/mid.log` en `$LOG`.

### Contenido

#### FR-17 · Un objeto de 0 bytes no produce salida

**Dado** el objeto `e/empty.log` de 0 bytes y `e/a.log`,
**cuando** se ejecuta `gcsgrep timeout s3://b/e/`,
**entonces** stdout es `e/a.log:1:timeout`, sin ninguna línea de `e/empty.log`.

**VC-17.1:** `s3run timeout s3://b/e/` → stdout exactamente `e/a.log:1:timeout\n`, stderr vacío (sin aviso por `e/empty.log`), exit `0`; `$LOG` tiene 1 `GET /b/e/empty.log` con status `200` y `0` bytes enviados.

#### FR-18 · La última línea sin `\n` también se busca

**Dado** el objeto `n/last.log` cuya última línea es `dos timeout` sin `\n` final,
**cuando** se ejecuta `gcsgrep timeout s3://b/n/`,
**entonces** stdout contiene la línea `n/last.log:2:dos timeout`.

**VC-18.1:** `s3run timeout s3://b/n/` → stdout exactamente `n/last.log:2:dos timeout\n` (25 bytes), stderr vacío, exit `0`.

#### FR-19 · Un objeto cuyo nombre termina en `.gz` se descomprime al vuelo

**Dado** el objeto `gz/app.log.gz`, gzip de `INFO start\nERROR timeout\n`,
**cuando** se ejecuta `gcsgrep timeout s3://b/gz/`,
**entonces** stdout contiene la línea `gz/app.log.gz:2:ERROR timeout`.

**VC-19.1:** `s3run timeout s3://b/gz/` → stdout exactamente `gz/app.log.gz:2:ERROR timeout\n` (30 bytes), stderr vacío, exit `0`.

#### FR-20 · Un `.gz` corrupto es un objeto fallido

**Dado** el objeto `gzbad/bad.gz`, de texto plano con nombre `.gz`,
**cuando** se ejecuta `gcsgrep timeout s3://b/gzbad/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: gzbad/bad.gz: corrupt gzip data`.

**VC-20.1:** `s3run timeout s3://b/gzbad/` → stdout exactamente `gzbad/ok.log:1:timeout\n`, stderr exactamente `gcsgrep: warning: gzbad/bad.gz: corrupt gzip data\n`, exit `2`.

#### FR-21 · Un objeto binario se saltea con aviso

**Dado** el objeto `bin/icon.png`, con un byte `00` en sus primeros 8192 bytes,
**cuando** se ejecuta `gcsgrep timeout s3://b/bin/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: bin/icon.png: skipped (binary object)`.

**VC-21.1:** `s3run timeout s3://b/bin/` → stdout exactamente `bin/a.log:1:timeout\n`, stderr exactamente `gcsgrep: warning: bin/icon.png: skipped (binary object)\n`, exit `0` (un objeto saltado no es un error).

### Guardarraíles

#### FR-22 · Superar `--max` aborta antes de leer ningún objeto

**Dado** el prefijo `max/` con 3 objetos,
**cuando** se ejecuta `gcsgrep --max 2 timeout s3://b/max/`,
**entonces** stderr contiene el error `gcsgrep: error: the prefix has 3 objects, which exceeds the limit of 2 (use --max to raise it, or --max 0 to disable it)`.

**VC-22.1:** `s3run --max 2 timeout s3://b/max/` → stdout vacío, stderr exactamente `gcsgrep: error: the prefix has 3 objects, which exceeds the limit of 2 (use --max to raise it, or --max 0 to disable it)\n`, exit `2`, cero `GET` de objeto en `$LOG`.

#### FR-23 · Un objeto que supera `--max-object-size` se corta con aviso

**Dado** el objeto `big/big.log` de 2 MiB (2.097.152 bytes),
**cuando** se ejecuta `gcsgrep --max-object-size 1048576 timeout s3://b/big/`,
**entonces** stderr contiene el aviso `gcsgrep: warning: big/big.log: object size limit of 1048576 bytes reached, rest of the object not read`.

**VC-23.1:** `s3run --max-object-size 1048576 timeout s3://b/big/` → stdout exactamente `big/big.log:1:timeout\n`, stderr exactamente el aviso anterior más `\n`, exit `2`.

#### FR-24 · Superar `--max-total-size` detiene la corrida con error

**Dado** el prefijo `tot/` con 5 objetos de 1 MiB (1.048.576 bytes) cada uno,
**cuando** se ejecuta `gcsgrep --max-total-size 2621440 timeout s3://b/tot/`,
**entonces** stderr contiene el error `gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete`.

**VC-24.1:** `s3run --max-total-size 2621440 timeout s3://b/tot/` → stdout exactamente `tot/1.log:1:timeout\ntot/2.log:1:timeout\ntot/3.log:1:timeout\n`, stderr exactamente el error anterior más `\n`, exit `2`, y `$LOG` no tiene ningún `GET` de `tot/4.log` ni de `tot/5.log`.

### Modos de salida

#### FR-25 · Con `-l`, el primer match detiene la lectura del objeto

**Dado** el objeto `l/big.log` de 100 MiB con `timeout` en la línea 1,
**cuando** se ejecuta `gcsgrep -l timeout s3://b/l/`,
**entonces** stdout contiene la línea `l/big.log`.

**VC-25.1:** `s3run -l timeout s3://b/l/` → stdout exactamente `l/big.log\n`, stderr vacío, exit `0`; el S3 de prueba registró para `GET /b/l/big.log` menos de 33.554.432 bytes enviados (32 MiB, de los 104.857.600 del objeto).

#### FR-26 · Con `-c`, se imprime la cantidad de líneas con match por objeto

**Dado** el objeto `c/three.log` con 3 líneas que contienen `timeout`,
**cuando** se ejecuta `gcsgrep -c timeout s3://b/c/`,
**entonces** stdout contiene la línea `c/three.log:3`.

**VC-26.1:** `s3run -c timeout s3://b/c/` → stdout exactamente `c/three.log:3\n`, stderr vacío, exit `0`.

### Esquema no soportado

#### FR-27 · Una ubicación sin esquema reconocido se rechaza con el texto de siempre

**Dado** una ubicación cuyo esquema no es `gs` ni `s3`,
**cuando** se ejecuta `gcsgrep timeout s3a://b/logs/`,
**entonces** stderr contiene el error `gcsgrep: error: invalid location "s3a://b/logs/": must start with gs://`.

**VC-27.1:** `s3run timeout s3a://b/logs/` → stdout vacío, stderr exactamente `gcsgrep: error: invalid location "s3a://b/logs/": must start with gs://\n`, exit `2`, sin S3 de prueba levantado.
**VC-27.2:** `s3run timeout S3://b/logs/` (esquema en mayúsculas) → stdout vacío, stderr exactamente `gcsgrep: error: invalid location "S3://b/logs/": must start with gs://\n`, exit `2`, sin S3 de prueba levantado.
**VC-27.3:** `s3run timeout b/logs/` (sin esquema) → stdout vacío, stderr exactamente `gcsgrep: error: invalid location "b/logs/": must start with gs://\n`, exit `2`, sin S3 de prueba levantado.

### Corridas GCS

#### FR-28 · Una ubicación `gs://` sin ADC sigue fallando con el error de ADC

**Dado** un entorno sin Application Default Credentials y con variables de AWS definidas,
**cuando** se ejecuta `gcsgrep timeout gs://b/logs/`,
**entonces** stderr contiene una línea que empieza con `gcsgrep: error: no Application Default Credentials found`.

**VC-28.1:** `env -i PATH=/usr/bin:/bin HOME=$H AWS_ACCESS_KEY_ID=AKTEST AWS_SECRET_ACCESS_KEY=secret AWS_REGION=us-east-1 GCE_METADATA_HOST=127.0.0.1:1 $G timeout gs://b/logs/` (`$H` vacío) → stdout vacío, stderr con una sola línea, que empieza con `gcsgrep: error: no Application Default Credentials found`, exit `2`.

## Reglas de negocio

### BR-1 · Solo lectura sobre S3

**Regla:** una corrida S3 solo emite peticiones `GET` de listado (`ListObjectsV2`) y de objeto (`GetObject`). Ningún camino de código emite `PUT`, `POST`, `DELETE` ni `HEAD`.

**VC-BR1.1:** `s3run timeout s3://b/logs/` y `s3run -c timeout s3://b/c/` → `awk '{print $2}' $LOG | sort -u` imprime exactamente `GET`.
**VC-BR1.2:** `go test ./internal/invariants -run TestReadOnlyObjectStoreSurface -count=1` → `ok` (la prueba es la de VC-INV3.1).

### BR-2 · Los exit codes son los de la spec base (spec base FR-8)

**Regla:** `0` si hubo al menos un match y ningún error ni objeto fallido; `1` si no hubo matches ni errores; `2` si hubo un error de uso, un error de corrida o algún objeto fallido, aunque otros objetos hayan dado match. Un objeto saltado por binario no es un error.

**VC-BR2.1:** `s3run timeout s3://b/logs/` → exit `0`.
**VC-BR2.2:** `s3run nomatchxyz s3://b/logs/` → stdout vacío, stderr vacío, exit `1`.
**VC-BR2.3:** `s3run timeout s3://b/gzbad/` (hay un match en `gzbad/ok.log` y un `.gz` corrupto en `gzbad/bad.gz`) → stdout `gzbad/ok.log:1:timeout`, exit `2`.

### BR-3 · Los mensajes nombran el esquema de la ubicación

**Regla:** todo mensaje que reproduce la ubicación usa el esquema con el que se invocó: `s3://` en una corrida S3, `gs://` en una corrida GCS. Una corrida S3 no imprime `gs://` en ningún mensaje y una corrida GCS no imprime `s3://`.

**VC-BR3.1:** se ejecutan los comandos de VC-5.1 y VC-10.1 y se concatena su stderr: `grep -c 'gs://'` da `0` y `grep -c 's3://'` da `2`.
**VC-BR3.2:** `go test ./internal/scanner -run 'TestRun_NoObjects|TestRun_ListFails' -count=1` → `ok` (esos tests fijan los mensajes con `gs://` de las corridas GCS).

### BR-4 · Precedencia entre errores simultáneos

**Regla:** si hay varios, se informa **uno solo**, el primero de esta lista: (1) error de uso de flags y argumentos (spec base FR-22, FR-7); (2) ubicación inválida (FR-4, FR-27); (3) patrón inválido (spec base FR-1.4); (4) región ausente (FR-8); (5) credenciales ausentes (FR-9); (6) errores de listado (FR-10, FR-11, NFR-1).

**VC-BR4.1:** `s3run '(' s3://` (patrón inválido y ubicación inválida) → stderr exactamente `gcsgrep: error: invalid location "s3://": missing bucket name\n`, exit `2` (ubicación antes que patrón).
**VC-BR4.2:** `env -i PATH=/usr/bin:/bin HOME=$H $G '(' s3://b/logs/` (sin región ni credenciales) → stderr con una sola línea, que empieza con `gcsgrep: error: invalid pattern "(": `, exit `2` (patrón antes que región), cero llamadas en `$LOG`.
**VC-BR4.3:** `env -i PATH=/usr/bin:/bin HOME=$H AWS_EC2_METADATA_DISABLED=true AWS_ENDPOINT_URL_S3=http://127.0.0.1:$S3PORT $G timeout s3://b/logs/` (sin región y sin credenciales) → stderr exactamente `gcsgrep: error: no AWS region configured: set AWS_REGION or a region in the AWS profile\n`, exit `2` (región antes que credenciales).
**VC-BR4.4:** `env -i PATH=/usr/bin:/bin HOME=$H AWS_EC2_METADATA_DISABLED=true AWS_REGION=us-east-1 AWS_ENDPOINT_URL_S3=http://127.0.0.1:$S3PORT $G timeout s3://nobucket/` (con región, sin credenciales, bucket inexistente) → stderr con una sola línea que empieza con `gcsgrep: error: no AWS credentials found: `, exit `2`, cero llamadas en `$LOG` (credenciales antes que listado).

**VC-BR4.5:** `s3run --max -1 timeout s3://` (flag inválido y ubicación inválida) → stderr exactamente `gcsgrep: error: invalid value "-1" for --max: must be >= 0\n`, exit `2` (flags antes que ubicación).

### BR-5 · La configuración de AWS es la estándar y no tiene flags propios

**Regla:** credenciales, región, perfil y endpoint salen solo de la configuración estándar del SDK de AWS (variables `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`, `AWS_PROFILE`, `AWS_ENDPOINT_URL_S3`, y los archivos `~/.aws/credentials` y `~/.aws/config`); si una variable de entorno y un archivo dan credenciales distintas, gana la variable. `gcsgrep` no define flags `--region`, `--profile`, `--endpoint` ni de claves.

**VC-BR5.1:** `HOME=$H` con `$H/.aws/credentials` = `[default]\naws_access_key_id = AKFILE\naws_secret_access_key = secret\n` y `$H/.aws/config` = `[default]\nregion = us-east-1\n`; entorno solo con `AWS_ENDPOINT_URL_S3` y `AWS_EC2_METADATA_DISABLED=true` → `gcsgrep timeout s3://b/logs/` → stdout `logs/a.log:2:ERROR timeout`, exit `0`, y todas las peticiones de `$LOG` llevan `AKFILE` como `<access_key>`.
**VC-BR5.2:** con el mismo `$H` y además `AWS_ACCESS_KEY_ID=AKENV AWS_SECRET_ACCESS_KEY=secret` → `gcsgrep timeout s3://b/logs/` → todas las peticiones de `$LOG` llevan `AKENV`; stdout `logs/a.log:2:ERROR timeout`, stderr vacío, exit `0`.
**VC-BR5.4:** con `$H/.aws/credentials` = `[prod]\naws_access_key_id = AKPROF\naws_secret_access_key = secret\n` y entorno con `AWS_PROFILE=prod`, `AWS_REGION=us-east-1`, `AWS_ENDPOINT_URL_S3` y `AWS_EC2_METADATA_DISABLED=true` → `gcsgrep timeout s3://b/logs/` → stdout `logs/a.log:2:ERROR timeout`, stderr vacío, exit `0`, y todas las peticiones de `$LOG` llevan `AKPROF`.
**VC-BR5.3:** `s3run --region us-east-1 timeout s3://b/logs/` → stdout vacío, stderr exactamente `gcsgrep: error: unknown flag -region\n`, exit `2`, cero llamadas en `$LOG`.

### BR-6 · Permisos mínimos de quien invoca

**Regla:** una corrida S3 necesita únicamente `s3:ListBucket` sobre el bucket y `s3:GetObject` sobre sus objetos. Con esos dos permisos, la salida y el exit code son idénticos a los de un usuario con permisos completos.

**VC-BR6.1:** contra `<bucket-s3>` real, con `<aws-minimo>` (política: `Allow s3:ListBucket` sobre `arn:aws:s3:::<bucket-s3>` y `Allow s3:GetObject` sobre `arn:aws:s3:::<bucket-s3>/*`, y nada más), `gcsgrep timeout s3://<bucket-s3>/logs/` → stdout y exit idénticos a VC-1.2 (`logs/a.log:2:ERROR timeout`, exit `0`), comparados con `cmp` contra la salida de `<aws-admin>`: sin diferencias.

## Requerimientos no funcionales

Los umbrales están congelados.

### NFR-1 · Reintentos ante fallos transitorios

**Umbral:** un error transitorio (conexión reseteada, HTTP 408, 429 o 5xx; `gcsgrep` no configura timeouts de pedido, como tampoco `gcs.go`) al **listar** o al **abrir** un objeto se reintenta hasta **3 intentos en total** (cada intento es una llamada). Espera antes del 2º intento: 500 ms ± 20% (400–600 ms); antes del 3º: 1 s ± 20% (800–1200 ms). Un error permanente (HTTP 4xx distinto de 408 y 429) no se reintenta. Agotados los 3 intentos al abrir: aviso `gcsgrep: warning: <objeto>: network error after 3 attempts: <detalle>`. Agotados al listar: error `gcsgrep: error: could not list s3://<bucket>/<prefijo> after 3 attempts: <detalle>`. Los reintentos son los de la spec base (NFR-3), aplicados con el mismo decorador; el SDK de AWS no agrega intentos propios.

Condiciones de medición: S3 de prueba en `127.0.0.1` (sin latencia de red apreciable), 1 corrida por VC. Los instantes de los intentos salen de la columna `<epoch_ms>` de `$LOG`; el intervalo es la diferencia entre peticiones consecutivas a la misma ruta.

**VC-NFR1.1:** con `--rule get:b/r/a.log:status=503,times=2` → `s3run timeout s3://b/r/a.log` → stdout exactamente `r/a.log:1:timeout\n`, stderr vacío, exit `0`, exactamente 3 `GET /b/r/a.log` en `$LOG` (503, 503, 200), intervalo 1→2 entre 400 y 600 ms e intervalo 2→3 entre 800 y 1200 ms.
**VC-NFR1.2:** con `--rule get:b/r/a.log:status=503` → `s3run timeout s3://b/r/a.log` → exactamente 3 `GET /b/r/a.log` en `$LOG` con los intervalos de VC-NFR1.1, stderr con una línea que empieza con `gcsgrep: warning: r/a.log: network error after 3 attempts: `, exit `2`.
**VC-NFR1.3:** con `--rule get:b/r/a.log:status=403` → `s3run timeout s3://b/r/a.log` → exactamente 1 `GET /b/r/a.log`, stderr exactamente `gcsgrep: warning: r/a.log: permission denied\n`, exit `2`.
**VC-NFR1.5:** con `--rule get:b/r/a.log:status=429,times=1` → `s3run timeout s3://b/r/a.log` → stdout exactamente `r/a.log:1:timeout\n`, stderr vacío, exit `0`, exactamente 2 `GET /b/r/a.log` en `$LOG` (429, 200), intervalo entre 400 y 600 ms.
**VC-NFR1.6:** con `--rule get:b/r/a.log:status=408,times=1` → mismo resultado que VC-NFR1.5 (2 `GET`, exit `0`).
**VC-NFR1.7:** con `--rule get:b/r/a.log:reset,times=1` → mismo resultado que VC-NFR1.5 (2 `GET`, el primero sin respuesta, exit `0`).
**VC-NFR1.4:** con `--rule list:b/r/:status=503` → `s3run timeout s3://b/r/` → exactamente 3 peticiones de listado y cero `GET` de objeto en `$LOG`, stdout vacío, stderr con una línea que empieza con `gcsgrep: error: could not list s3://b/r/ after 3 attempts: `, exit `2`, intervalos como en VC-NFR1.1.

### NFR-2 · Memoria acotada con objetos grandes

**Umbral:** el pico de memoria residente (RSS) al procesar `mem/large.log` (500 MiB) supera al de `mem/small.log` (5 MiB) en **≤ 5 MiB** (5.242.880 bytes). La memoria por objeto no depende de su tamaño.

Condiciones de medición: `--max-object-size 0 --max-total-size 0`, patrón `nomatchxyz` (sin matches), S3 de prueba en `127.0.0.1`, 3 corridas por objeto como procesos aparte bajo `/usr/bin/time -v` y mediana de `Maximum resident set size`.

**VC-NFR2.1:** `s3run --max-object-size 0 --max-total-size 0 nomatchxyz s3://b/mem/small.log` y la misma con `mem/large.log`, con las condiciones de medición de NFR-2 → todas terminan con exit `1`, y `mediana(large) − mediana(small) ≤ 5120 KiB`.

## VCs de invariantes

**VC-INV1.1:** en `greenfield/gcsgrep`, `go test ./... -count=1 -v | grep -c '^--- FAIL'` → `0` y `go test ./... -count=1 -v | grep -c '^--- PASS'` → un número `≥ 123`; `go vet ./...` sale con `0`.
**VC-INV1.2:** `gcsgrep timeout bucket/logs/` y `gcsgrep timeout gs://` → stderr exactamente `gcsgrep: error: invalid location "bucket/logs/": must start with gs://\n` y `gcsgrep: error: invalid location "gs://": missing bucket name\n`, ambos exit `2` (los textos de la línea de base, medidos sobre `a3ed3afbd452`).
**VC-INV2.1:** el comando de VC-28.1, comparado con el mismo comando sin las tres variables `AWS_*`: los dos dan stderr con la misma línea completa, stdout vacío y exit `2`; `diff` de los dos stderr: sin diferencias.
**VC-INV3.1:** `go test ./internal/invariants -run 'TestReadOnlyGCSSurface|TestReadOnlyObjectStoreSurface' -count=1` → `ok`. La segunda prueba falla si el código de producción (excluidos `_test.go`) contiene los selectores `PutObject`, `DeleteObject`, `DeleteObjects`, `CopyObject`, `CreateMultipartUpload`, `UploadPart`, `PutObjectAcl`, `PutBucketPolicy` o `RestoreObject`.
**VC-INV3.2:** `grep -rnE 'PutObject|DeleteObject|CopyObject|CreateMultipartUpload|RestoreObject' --include='*.go' cmd internal | grep -v '_test.go' | wc -l` → `0`.
**VC-INV4.1:** `git diff --stat a3ed3afbd452 -- greenfield/gcsgrep/internal/reader greenfield/gcsgrep/internal/match greenfield/gcsgrep/internal/output greenfield/gcsgrep/internal/tty greenfield/gcsgrep/internal/gcsclient/gcs.go greenfield/gcsgrep/internal/gcsclient/retry.go greenfield/gcsgrep/internal/gcsclient/gcsclienttest | wc -l` → `0`.
**VC-INV5.1:** `grep -rl 'github.com/aws/' --include='*.go' greenfield/gcsgrep | grep -v '_test.go'` → exactamente `greenfield/gcsgrep/internal/s3client/s3.go`, y `grep -rl 'cloud.google.com/go/storage' --include='*.go' greenfield/gcsgrep | grep -v '_test.go'` → exactamente `greenfield/gcsgrep/internal/gcsclient/gcs.go`.
**VC-INV6.1:** `go test ./internal/invariants -run TestNoDiskWrites -count=1` → `ok` (la prueba recorre también `internal/s3client/s3.go`).
**VC-INV7.1:** `s3run timeout s3://b/logs/` con `GCE_METADATA_HOST=127.0.0.1:1` y `HOME=$H` sin credenciales de Google → exit `0` y stdout `logs/a.log:2:ERROR timeout`: el cliente de GCS no se construyó (si se hubiera construido, habría fallado con `no Application Default Credentials found`).
**VC-INV7.2:** un listener `nc -l 127.0.0.1 $DEAD` y `env -i PATH=/usr/bin:/bin HOME=$H AWS_ENDPOINT_URL_S3=http://127.0.0.1:$DEAD AWS_ACCESS_KEY_ID=AKTEST AWS_SECRET_ACCESS_KEY=secret AWS_REGION=us-east-1 AWS_EC2_METADATA_DISABLED=true $G timeout gs://b/logs/` → exit `2` y el listener recibió `0` conexiones: el cliente de S3 no se construyó ni se llamó en una corrida GCS.

## Matriz de cobertura

| Requisito | VC | Camino de falla incluido |
|---|---|---|
| FR-1 … FR-3, FR-6, FR-7 | VC-1.1 … VC-7.1 | — (camino feliz) |
| FR-4 | VC-4.1, VC-4.2 | entrada inválida |
| FR-5 | VC-5.1 | vacío o sin coincidencia |
| FR-8, FR-9 | VC-8.1, VC-9.1 | sin credenciales / configuración |
| FR-10, FR-11 | VC-10.1, VC-11.1 | sin permiso / bucket inexistente |
| FR-12 … FR-16, FR-20 | VC-12.1 … VC-16.1, VC-20.1 | recurso ilegible y resto de la corrida |
| FR-17, FR-18, FR-19, FR-21 | VC-17.1 … VC-19.1, VC-21.1 | bordes de contenido |
| FR-22 … FR-24 | VC-22.1 … VC-24.1 | guardarraíl alcanzado |
| FR-25, FR-26 | VC-25.1, VC-26.1 | — |
| FR-27 | VC-27.1 … VC-27.3 | entrada inválida |
| FR-28 | VC-28.1 | sin credenciales |
| BR-1 … BR-6 | VC-BR1.1 … VC-BR6.1 | precedencia (BR-4), permisos (BR-6) |
| NFR-1, NFR-2 | VC-NFR1.1 … VC-NFR2.1 | corte de red, reintentos agotados |
| INV-1 … INV-7 | VC-INV1.1 … VC-INV7.2 | — |

## Plan de iteraciones

Cada requisito cierra en **exactamente una** iteración. Al cerrar cada una, la línea de base de
regresión (123 tests en `PASS`, 0 en `FAIL`) se vuelve a correr y da lo mismo o más.

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Esquema en `cli.Args`, `app.RunWithClients` con la fábrica de GCS registrada y `scanner.Config.Scheme`; rechazos de ubicación, sin cliente S3 ni red ni S3 de prueba | FR-4, FR-27, FR-28, INV-1, INV-2 |
| **2** | `internal/s3client` (listado paginado y lectura), `ErrObjectArchived`, el S3 de prueba (listar, leer, paginar, `--log`) y los datos; camino feliz y contenido | FR-1, FR-2, FR-3, FR-5, FR-6, FR-7, FR-17, FR-18, FR-19, FR-20, FR-21, BR-1, BR-2, INV-3, INV-5, INV-6, INV-7 |
| **3** | Configuración y errores de acceso: región, credenciales, listado denegado, bucket inexistente; el S3 de prueba gana `--rule` | FR-8, FR-9, FR-10, FR-11, BR-3, BR-4, BR-5 |
| **4** | Errores de apertura y de red: las acciones `archived` y `cut` de las reglas, reintentos | FR-12, FR-13, FR-14, FR-15, FR-16, NFR-1 |
| **5** | Guardarraíles, modos, memoria, permisos mínimos y cierre de invariantes | FR-22, FR-23, FR-24, FR-25, FR-26, NFR-2, BR-6, INV-4 |

## Decisiones

| # | Pregunta | Elegido | Fundamento (código del sistema) | Descartado y por qué | FR |
|---|---|---|---|---|---|
| 1 | ¿Cómo se escribe una ubicación S3? | `s3://<bucket>[/<prefijo>]`, parseada igual que `gs://` | `cli.parseLocation` ya separa host=bucket y path=prefijo con `url.Parse`; el esquema es lo único que cambia | `https://<bucket>.s3.amazonaws.com/…`, ARNs y access points: más sintaxis para el mismo resultado y sin uso real en el CLI | FR-1, FR-2, FR-4 |
| 2 | ¿Qué texto da una ubicación con esquema no reconocido? | El de hoy: `must start with gs://` | El texto está fijado por `cli_test.go` ("FR-16.1 location without gs://"), `app_test.go` (VC-25.1 de la spec base) y `integration_test.go` (`invalid location \"mybucket/logs/\": must start with gs://`); la línea de base exige que esos tests sigan en verde | `must start with gs:// or s3://`: más claro, pero pondría en rojo esos tres tests; cambiarlo modifica el contrato de la spec base (FR-16.1) y queda fuera de este cambio (ver Limitaciones) | FR-27, VC-INV1.2 |
| 3 | ¿Dónde vive el cliente S3? | Paquete nuevo `internal/s3client` que implementa `gcsclient.Client` | `scanner.Run` solo depende de `gcsclient.Client` (`List`, `Open`) y `gcsclienttest.Fake` ya lo implementa; el comentario de `gcs.go` fija que cada SDK vive en su paquete | Renombrar `gcsclient` a un nombre neutro: toca ~30 archivos y sus tests sin cambiar comportamiento. Meter S3 dentro de `gcsclient`: rompe el aislamiento de SDK (INV-5) | INV-5 |
| 4 | ¿Cómo se elige el cliente según el esquema? | `app.RunWithClients` recibe un mapa esquema→`ClientFactory`; `Run` y `RunWithTerminals` conservan su firma y registran solo `"gs"` | `app.RunWithTerminals` es llamada por `main.go` y `terminal_test.go`, y `app.Run` por `app_test.go` (cuyo helper usan `retry_test.go` y `size_test.go`), todos con la firma actual; `ClientFactory` solo se invoca tras validar la invocación | Cambiar la firma de `ClientFactory` o de `RunWithTerminals`: rompe la compilación de esos tests | INV-1, INV-7 |
| 5 | ¿Cómo viaja el esquema hasta los mensajes? | `scanner.Config.Scheme`; el valor vacío significa `gs` | `scanner.Run` y `describeListError` fijan `gs://%s/%s` en cuatro mensajes; los tests de `scanner` construyen `Config` sin esquema y esperan `gs://` | Un campo obligatorio: rompe esos tests | BR-3 |
| 6 | ¿Quién reintenta? | `gcsclient.WithRetries(client, DefaultRetryPolicy)` sobre el cliente S3; el SDK de AWS con 1 intento | `app.RunWithTerminals` ya envuelve el cliente; `gcs.go` hace lo mismo con `sc.SetRetry(storage.WithPolicy(storage.RetryNever))` para que "3 intentos" sean 3 | Dejar los reintentos propios del SDK: sumarían intentos a los 3 y romperían los intervalos medidos | NFR-1 |
| 7 | ¿Qué errores de S3 son transitorios? | Los mismos criterios de `gcsclient.isTransient`: HTTP 408, 429, 5xx, timeout, conexión reseteada; un contexto cancelado no | `gcs.go:isTransient`. Es privada y la deja sin tocar INV-4; `s3client` repite esos criterios leyendo el status del error del SDK y envuelve cada error transitorio con `gcsclient.Transient` (`gcsclient.go`), que es la marca que `retrying.do` exige para reintentar; sin esa marca NFR-1 no se cumple (lo ejercitan VC-NFR1.1 y VC-NFR1.5 a VC-NFR1.7). `gcs.go` no configura timeouts de pedido y `s3client` tampoco, por eso NFR-1 no incluye timeouts | Exportar `isTransient`: modifica `gcs.go` | NFR-1 |
| 8 | ¿Cómo se traducen los errores de S3? | `NoSuchBucket` → `ErrBucketNotFound` (listado); `NoSuchKey` → `ErrObjectNotFound` (apertura); HTTP 403 → `ErrPermissionDenied`; código `InvalidObjectState` → `ErrObjectArchived`, evaluado **antes** que el 403 | `gcs.go:translateListError` y `translateOpenError` hacen el mismo mapeo; `scanner.describeListError` y `describeOpenError` ya traducen esos errores a mensajes | Tratar `InvalidObjectState` como `permission denied`: S3 responde 403 para un objeto archivado y el mensaje mandaría a revisar permisos que están bien | FR-10 … FR-14 |
| 9 | ¿Qué causa se muestra para un objeto archivado? | `archived object` | Misma forma corta que `permission denied` y `object not found` en `scanner.describeOpenError` | Sugerir un `restore`: `gcsgrep` no escribe (BR-1) y no hay código del sistema que lo respalde | FR-14 |
| 10 | ¿Cómo se detecta la falta de credenciales? | `s3client.New` resuelve las credenciales al construirse (`Retrieve`) y falla con `no AWS credentials found: <detalle>` | `app.RunWithTerminals` ya trata el error de `newClient` como error de corrida antes de listar (`no Application Default Credentials found: %v`). Medido con la sonda de `gcsgrep-s3-sonda.md`: `config.LoadDefaultConfig` no falla sin credenciales; `cfg.Credentials.Retrieve` sí | Dejar que falle en el primer `List`: saldría como `could not list …` y se confundiría con un problema del bucket | FR-9, BR-4 |
| 11 | ¿Qué pasa sin región? | Error de corrida antes de pedir credenciales, sin llamadas | Medido con la sonda de `gcsgrep-s3-sonda.md`: sin región, el primer pedido falla con `Invalid region: region was not a valid DNS name`, un mensaje sin relación con la causa. Se pide la región antes que las credenciales porque es una comprobación local y falla con el mensaje correcto sin esperar a la cadena de credenciales | Región por defecto `us-east-1`: un bucket de otra región responde con una redirección y el error apunta al lugar equivocado. Autodetección con `GetBucketRegion`: una llamada y una dependencia más (`feature/s3/manager`) | FR-8 |
| 12 | ¿Cómo se configura el endpoint y el perfil? | Solo por la configuración estándar del SDK; `gcsgrep` no agrega flags | Medido con la sonda de `gcsgrep-s3-sonda.md`: `AWS_ENDPOINT_URL_S3` hacia `http://127.0.0.1:<puerto>` se respeta y, con una IP, el pedido sale *path-style* (`/<bucket>?list-type=2&prefix=…`). `cli.Parse` define hoy solo flags de búsqueda y de límites | Flags `--region`, `--profile`, `--endpoint`: superficie sin fundamento en el diseño; BR-5 los prohíbe | BR-5 |
| 13 | ¿Se envía un delimitador al listar? | No: `ListObjectsV2` con `Prefix` y sin `Delimiter` | `gcs.go:List` pasa solo `Prefix` en `storage.Query` y `cli.parseLocation` documenta que un prefijo sin `/` es un prefijo de nombre | Con `Delimiter=/` se listarían "carpetas" en vez de objetos | FR-3, FR-6 |
| 14 | ¿`.gz` por nombre o por `Content-Encoding`? | Por el sufijo `.gz` del nombre | `scanner.scanObject` fija `Gzip: strings.HasSuffix(name, ".gz")` para los dos esquemas | Leer `Content-Encoding`: `Open` devuelve solo un `io.ReadCloser`; cambiar la interfaz toca `gcs.go` | FR-19 |
| 15 | ¿Qué S3 usan los VCs? | Un S3 falso propio (`fake_s3.py`) para casi todos, y S3 real solo para VC-1.2 y VC-BR6.1 | `gcsclienttest.Fake` ya inyecta fallos por objeto (`OpenFailures`, `ReadErr`); el cliente S3 es lo que se prueba, así que la inyección tiene que estar un nivel más abajo, en HTTP | MinIO: no emula `InvalidObjectState`, ni N fallos 503 seguidos, ni el corte del cuerpo a mitad. Solo S3 real: no determinista y costoso para los reintentos | NFR-1, FR-12 … FR-16 |
| 16 | ¿Qué guardarraíles rigen en S3? | Los mismos valores por defecto: 1000 objetos, 250 MiB por objeto, 2 GiB por corrida | `scanner.DefaultMaxObjects`, `DefaultMaxObjectSize`, `DefaultMaxTotalSize`; `cli.Parse` los toma sin conocer el esquema | Valores distintos para S3: no hay un criterio que los distinga del de GCS | FR-22 … FR-24 |
| 17 | ¿Qué orden tiene la salida? | El del listado: ascendente por bytes UTF-8 del nombre | `scanner.Run` procesa en el orden en que `client.List` devuelve los objetos (spec base FR-19.1); el S3 de prueba y S3 listan en ese orden | Reordenar en el cliente: costo sin beneficio | FR-2, FR-3 |
| 18 | ¿El soporte S3 va detrás de una guarda de build opt-in? | No: se compila siempre | `cmd/gcsgrep/main.go` cablea el único cliente de forma estática y GCS no tiene guarda; el pedido es "agregar soporte", no un binario aparte | Build tag `s3`: deja el binario por defecto sin S3, que es lo contrario de lo pedido. Sin guarda, que el binario siga igual para quien usa `gs://` lo aseguran INV-1 (corrida GCS igual que antes) e INV-4 (archivos compartidos intactos) | INV-1, INV-4 |
| 19 | ¿Cómo llegan a `stderr` los errores de las fábricas? | `RunWithClients` imprime `gcsgrep: error: ` más el mensaje de la fábrica; para `gs` conserva el texto `no Application Default Credentials found: <detalle>`, para `s3` el error de `s3client.New` ya trae `no AWS region configured: …` o `no AWS credentials found: <detalle>` | `app.RunWithTerminals` hoy hace `w.Error("no Application Default Credentials found: %v", err)` para cualquier error de `newClient`; ese texto lo fijan `app_test.go` y VC-25.2 de la spec base | Un único mensaje "no credentials found": cambia el texto de GCS (INV-1) | FR-8, FR-9, FR-28 |

## Limitaciones conocidas

- **Texto del rechazo de esquema.** `s3a://…` y una ubicación sin esquema dicen
  `must start with gs://`, aunque ahora `s3://` también es válido. Es la consecuencia de la
  decisión 2; mejorar el texto exige actualizar los tres tests que lo fijan y cambia el contrato
  de la spec base (FR-16.1).
- **Cuerpo truncado con cierre ordenado.** Si el servicio corta el cuerpo de un objeto de menos de 8192 bytes con un cierre normal de la conexión (el SDK devuelve `io.ErrUnexpectedEOF`), `reader.sniffBinary` lo trata como fin del objeto (`case io.EOF, io.ErrUnexpectedEOF`) y no emite el aviso `read interrupted`. Vale igual para GCS; arreglarlo es tocar `internal/reader` (INV-4), por eso queda fuera. VC-16.1 usa un corte con RST, que sí se detecta.
- **Región fija.** Un bucket de otra región que la configurada falla con el detalle del servicio
  (redirección), no con un mensaje propio.
- **Listado completo en memoria.** Como en GCS, `List` devuelve todos los objetos del prefijo
  antes de leer ninguno; un prefijo de millones de claves consume memoria y llamadas aunque
  `--max` aborte después.
- **"Carpetas" de la consola de S3.** Los objetos de 0 bytes cuyo nombre termina en `/` se
  listan como cualquier objeto: cuentan para `--max` y no producen salida (FR-17).
- **Sin pruebas de rendimiento de red contra S3 real.** Los umbrales de NFR-1 y NFR-2 se miden
  contra el S3 de prueba; el throughput real depende de la red y de la región.
