# Spec — `gcsgrep` lee objetos de S3 (`s3://`)

> Spec **brownfield** sobre el módulo `greenfield/gcsgrep`. Se apoya en la spec de `gcsgrep`
> ([`gcsgrep-spec.md`](gcsgrep-spec.md)), en [`gcsgrep-design.md`](gcsgrep-design.md) y en la sonda del
> SDK de AWS ([`gcsgrep-s3-sonda.md`](gcsgrep-s3-sonda.md), medida el 2026-10-08). Los IDs de esta spec
> (`FR-n`, `BR-n`, `NFR-n`, `INV-n`, `VC-…`) son **propios**: no se confunden con los de `gcsgrep-spec.md`,
> que se citan siempre como "`gcsgrep-spec.md` FR-n".

**Repo:** repositorio del TP (`ITBA/SDD`), módulo `greenfield/gcsgrep` · commit base **`b16e609`** (2026-10-08) — es el commit explorado; los símbolos citados se verificaron contra ese checkout.
**Entrega:** el commit que cierra la iteración 4 del plan de esta spec.

## Propósito

Permitir que `gcsgrep` busque un patrón en los objetos de un bucket de S3 con la ubicación
`s3://bucket/prefijo`, igual que hoy lo hace con `gs://`, en un binario que se compila con una guarda
opt-in. El límite: sin la guarda el binario es el de hoy; con ella, `gs://` no cambia y S3 solo recibe
operaciones de lectura.

## Glosario

Un término por concepto; el resto de la spec lo usa sin variantes.

| Término | Significado |
|---|---|
| **ubicación** | El segundo argumento posicional: `gs://bucket[/prefijo]` o `s3://bucket[/prefijo]` |
| **esquema** | `gs` o `s3`, la parte de la ubicación anterior a `://` |
| **build base** | El binario compilado sin el tag `s3` (`go build ./cmd/gcsgrep`) |
| **build S3** | El binario compilado con el tag `s3` (`go build -tags s3 ./cmd/gcsgrep`) |
| **objeto** | Un elemento listado bajo la ubicación. En la salida se escribe como `<objeto>`: su clave tal como la lista el servicio, sin esquema ni bucket |
| **prefijo** | La parte de la ubicación posterior al bucket. Filtra por comienzo de nombre, sin agrupar por delimitador |
| **aviso** | Línea `gcsgrep: warning: …` por `stderr`: la corrida sigue |
| **mensaje de error** | Línea `gcsgrep: error: …` por `stderr`: la corrida termina |
| **error de uso** | Rechazo de la invocación antes de crear el cliente: flag, argumentos, ubicación o patrón (BR-4, órdenes 1 a 5) |
| **objeto fallido** | Objeto que no se pudo leer completo: FR-6, FR-7, FR-8, FR-9, FR-10 o reintentos agotados al abrirlo (NFR-2) |
| **falla de S3** | Cualquiera de: un objeto fallido; FR-11, FR-12 o FR-13 (listado); FR-14 o FR-15 (cliente); reintentos agotados al listar (NFR-2). La ubicación sin objetos (FR-5) **no** es una falla |
| **credenciales S3** | Las que resuelve la cadena por defecto del SDK de AWS (BR-2) |
| **servidor S3 de prueba** | El servidor HTTP del harness (sección "Harness de prueba") |
| **`ENV_S3`** | El entorno de los VCs, definido en el harness |
| **progreso** | Las líneas `gcsgrep: progress: …` de `stderr` (`gcsgrep-spec.md` FR-10): los VCs las excluyen al comparar `stderr` |

## Alcance

### Dentro

Todos los paths son relativos a `greenfield/gcsgrep/`, salvo `greenfield/README.md` y los documentos de `greenfield/`.

| Archivo | Cambio |
|---|---|
| `internal/s3client/s3.go` (**nuevo**, `//go:build s3`) | `gcsclient.Client` sobre el SDK de AWS: `New`, `List`, `Open` y la traducción de errores (D8, D11) |
| `internal/s3client/s3faketest/` (**nuevo**, `//go:build s3`) | El servidor S3 de prueba |
| `internal/s3client/s3_test.go` (**nuevo**, `//go:build s3`) | Los tests de los VCs de esta spec |
| `internal/cli/cli.go` | `Args`: campo `Scheme`. `parseLocation`: acepta `s3://` solo en el build S3; el mensaje de ubicación inválida no cambia (D5) |
| `internal/cli/schemes_base.go` (**nuevo**, `//go:build !s3`) y `internal/cli/schemes_s3.go` (**nuevo**, `//go:build s3`) | Esquemas aceptados por build |
| `internal/app/app.go` | Entrada nueva `RunWithProviders(ctx, argv, stdout, stderr, term Terminals, providers map[string]Provider) int`, con `Provider{NewClient ClientFactory; NoClientMessage string}`: elige el proveedor por `args.Scheme` e imprime `NoClientMessage` seguido de `: <error>` si la fábrica falla. `Run` y `RunWithTerminals` conservan su firma y llaman a `RunWithProviders` con solo `gs` y el mensaje `no Application Default Credentials found` (D21) |
| `internal/scanner/scanner.go` | `Config`: campo `Scheme` (cero = `gs`). `Run` y `describeListError`: el esquema en los mensajes (D4) |
| `internal/reader/reader.go` | `sniffBinary`: un corte del stream antes de 8192 bytes deja de confundirse con el fin del objeto (D16) |
| `cmd/gcsgrep/main.go`, `cmd/gcsgrep/providers_base.go` (**nuevo**, `!s3`), `cmd/gcsgrep/providers_s3.go` (**nuevo**, `s3`) | Registro de proveedores por build |
| `internal/invariants/s3_readonly_test.go` (**nuevo**) | Chequeo estático de BR-1 sobre `internal/s3client` |
| `go.mod`, `go.sum` | Dependencias del SDK de AWS (versiones en D1) |
| `greenfield/README.md` | Build S3, ubicación `s3://`, permisos mínimos (BR-7) |
| `greenfield/gcsgrep-s3-baseline-pass.txt` (**nuevo**) | Los 163 nombres de test en verde medidos en el commit base (INV-3) |

Los tests existentes (`*_test.go` de `app`, `cli`, `gcsclient`, `invariants`, `match`, `output`, `reader`, `scanner`) **no se editan**.

### Fuera de alcance

- **`internal/gcsclient/gcs.go`** — la traducción de errores de GCS no se toca; `s3client` tiene la suya.
- **`internal/gcsclient/gcsclient.go`** — la interfaz `Client` (solo `List` y `Open`) no cambia: sostiene BR-1.
- **`internal/gcsclient/retry.go`** — `WithRetries` y `DefaultRetryPolicy` se reutilizan tal cual.
- **`internal/gcsclient/gcsclienttest/fake.go`** — el cliente falso de GCS no se toca.
- **`internal/match/`, `internal/output/`, `internal/tty/`, `internal/reader/limits.go`, `internal/reader/gzip.go`** — sin cambios.
- **`integration/` y `testdata/setup-testdata.sh`** — los VCs contra GCS real no cambian.
- **`greenfield/gcsgrep-spec.md`, `gcsgrep-design.md`, `gcsgrep-plan.md`** — documentos del TP greenfield, sin cambios.
- **Concurrencia (`gcsgrep-spec.md` FR-13, FR-14)** — no existe en el código (`scanner.Run` es secuencial); sigue sin existir.
- **Flags nuevos** (`--profile`, `--region`, `--endpoint`) — fuera de alcance (D6, D17).
- **Otras formas de nombrar el bucket** — ARN de access point, S3 Express One Zone (directory buckets), buckets requester-pays.
- **Versiones de objeto (`versionId`), cifrado con clave del cliente (SSE-C), restauración de objetos archivados** — fuera de alcance.
- **Escritura en S3 de cualquier tipo** — prohibida por BR-1.
- **Otros proveedores** (`az://`, `https://`) — fuera de alcance.

## Invariantes

| # | Invariante | VC |
|---|---|---|
| **INV-1** | El build base compila y su binario no incluye el SDK de AWS | VC-INV1.1, VC-INV1.2 |
| **INV-2** | El build base trata `s3://…` como hoy: ubicación inválida, sin llamadas de red | VC-INV2.1, VC-INV2.2 |
| **INV-3** | Todo test que pasaba en el commit base sigue pasando, en el build base y en el build S3 | VC-INV3.1, VC-INV3.2 |
| **INV-4** | En el build S3, una corrida con `gs://` o sin esquema dice lo mismo que hoy | VC-INV4.1, VC-INV4.2, VC-INV4.3 |

## Línea de base de regresión

Medida en el commit `b16e609` (checkout limpio salvo el borrado de `greenfield/gcsgrep-s3-*.md` en el índice), `go1.26.1 linux/amd64`, desde `greenfield/gcsgrep/`, **antes** de tocar código:

- `go test -count=1 ./...` → 8 paquetes `ok` (`app`, `cli`, `gcsclient`, `invariants`, `match`, `output`, `reader`, `scanner`), 4 sin tests.
- `go test -count=1 -v ./... | grep -c -- '--- PASS'` → `163`; `grep -c -- '--- FAIL'` → `0`. Los 163 nombres, ordenados, están en `greenfield/gcsgrep-s3-baseline-pass.txt`.
- `go test -count=1 -tags s3 -v ./... | grep -c -- '--- PASS'` → `163` (el tag no hace nada todavía).
- `go vet ./...` → exit `0`, sin salida.
- `go build -o $BIN ./cmd/gcsgrep` → binario de 48.786.546 bytes; `go version -m $BIN | grep -c aws` → `0`; `go list -deps ./cmd/gcsgrep | grep -c aws` → `0`.
- `$BIN timeout s3://b/p/` → stdout vacío, stderr `gcsgrep: error: invalid location "s3://b/p/": must start with gs://`, exit `2`.
- `$BIN timeout 'gs:///logs/'` → stderr `gcsgrep: error: invalid location "gs:///logs/": missing bucket name`, exit `2`.
- `$BIN -l -c timeout mybucket/x` → `gcsgrep: error: -l and -c cannot be used together`; `$BIN '(' 'gs:///logs/'` → el error de ubicación, no el de patrón.

## Harness de prueba

**Servidor S3 de prueba** (`internal/s3client/s3faketest`): servidor HTTP en `127.0.0.1:<puerto libre>`, estilo de ruta (la sonda muestra que el SDK lo usa con un endpoint IP).

- `GET /<bucket>?list-type=2[&prefix=P][&delimiter=D][&continuation-token=T]` responde un `ListBucketResult` con las claves en orden de bytes, hasta 1000 por página, con `IsTruncated` y `NextContinuationToken`. Con `delimiter` agrupa en `CommonPrefixes` como S3, de modo que un listado con delimitador deja los resultados sin objetos.
- `GET /<bucket>/<clave>` responde el cuerpo del objeto con su `Content-Length`. No envía cabeceras de checksum (como el servidor de la sonda).
- Un bucket que no está definido responde 404 `NoSuchBucket` al listado.
- Por operación y clave se puede inyectar, para los intentos sucesivos: un estado HTTP con su código de error XML, el corte del cuerpo tras N bytes con un `Content-Length` mayor, o el cierre de la conexión con RST.
- Registra cada request: método, ruta, query, instante en ns y cabecera `Authorization`. Responde 405 a todo método distinto de `GET` y lo cuenta.
- Genera por streaming los cuerpos grandes (no los guarda en memoria).

**`ENV_S3`:** se borra toda variable `AWS_*`; `HOME` apunta a un directorio vacío; luego se fijan `AWS_EC2_METADATA_DISABLED=true`, `AWS_ACCESS_KEY_ID=AKIDTEST`, `AWS_SECRET_ACCESS_KEY=secrettest`, `AWS_REGION=us-east-1` y `AWS_ENDPOINT_URL_S3=<url del servidor S3 de prueba>`. Los VCs que dicen "sin credenciales" o "sin región" quitan la variable correspondiente.

**Ejecución:** los VCs son tests de Go con `-tags s3` que levantan el servidor en el mismo proceso, fijan `ENV_S3` con `t.Setenv` y llaman al mismo punto de entrada que `cmd/gcsgrep` (`app.RunWithProviders` con los proveedores del build S3), como hace `integration/integration_test.go:run`. Los VCs que dicen "binario" (INV-1, INV-2, NFR-1) compilan y ejecutan el binario como proceso aparte. En los VCs, `gcsgrep …` significa "ejecutar con `ENV_S3`" salvo que se indique otro entorno, y "stderr vacío" excluye el progreso.

**Objetos del bucket `bkt`** (los demás se definen en cada VC):

- **F1:** `logs/a.log` = `INFO start\nERROR timeout\n`, `logs/b.log` = `INFO ok\n`, `logs-old/c.log` = `timeout viejo\n`.

## Requerimientos

### FR-1 · Búsqueda bajo un prefijo de S3

**Dado** el bucket `bkt` con los objetos de F1,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** `stdout` es exactamente `logs/a.log:2:ERROR timeout` seguido de un salto de línea.

**VC-1.1:** F1, `gcsgrep timeout s3://bkt/logs/` → stdout `logs/a.log:2:ERROR timeout\n`, stderr vacío, exit `0`; el servidor recibió 1 request de listado (`GET /bkt?list-type=2&prefix=logs%2F`) y 2 de lectura (`/bkt/logs/a.log`, `/bkt/logs/b.log`).

### FR-2 · Bucket completo con `s3://bucket` sin barra

**Dado** el bucket `bkt` con los objetos de F1,
**cuando** el usuario ejecuta `gcsgrep -c timeout s3://bkt`,
**entonces** `stdout` es exactamente `logs-old/c.log:1\nlogs/a.log:1\nlogs/b.log:0\n`.

**VC-2.1:** F1, `gcsgrep -c timeout s3://bkt` → stdout `logs-old/c.log:1\nlogs/a.log:1\nlogs/b.log:0\n`, stderr vacío, exit `0` (`-` ordena antes que `/`).
**VC-2.2:** F1, `gcsgrep -c timeout s3://bkt/` → stdout idéntico byte a byte al de VC-2.1, exit `0`.

### FR-3 · Un prefijo sin `/` final filtra por comienzo de nombre

**Dado** el bucket `bkt` con los objetos de F1,
**cuando** el usuario ejecuta `gcsgrep -l timeout s3://bkt/logs`,
**entonces** `stdout` es exactamente `logs-old/c.log\nlogs/a.log\n`.

**VC-3.1:** F1, `gcsgrep -l timeout s3://bkt/logs` → stdout `logs-old/c.log\nlogs/a.log\n`, stderr vacío, exit `0`; el listado recibido tiene `prefix=logs` y ningún parámetro `delimiter`.

### FR-4 · Un listado de más de 1000 objetos se lee completo

**Dado** el prefijo `many/` con 2500 objetos `many/0000.log` … `many/2499.log`, todos con el contenido `x\n` salvo `many/2499.log` con `needle\n`,
**cuando** el usuario ejecuta `gcsgrep --max 0 -l needle s3://bkt/many/`,
**entonces** `stdout` es exactamente `many/2499.log\n`.

**VC-4.1:** el prefijo `many/` descrito, `gcsgrep --max 0 -l needle s3://bkt/many/` → stdout `many/2499.log\n`, stderr vacío, exit `0`; el servidor recibió 3 requests de listado (el 2º y el 3º con `continuation-token`) y 2500 de lectura.

### FR-5 · Una ubicación sin objetos

**Dado** el bucket `bkt` sin objetos bajo `vacio/`,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/vacio/`,
**entonces** `stderr` es exactamente `gcsgrep: warning: no objects under s3://bkt/vacio/`.

**VC-5.1:** F1, `gcsgrep timeout s3://bkt/vacio/` → stdout vacío, stderr `gcsgrep: warning: no objects under s3://bkt/vacio/\n`, exit `1`, 0 requests de lectura.

### FR-6 · Un objeto sin permiso de lectura

**Dado** el objeto `acl/denied.log` cuya lectura responde HTTP 403 `AccessDenied`,
**cuando** el usuario ejecuta `gcsgrep secreto s3://bkt/acl/denied.log`,
**entonces** `stderr` es exactamente `gcsgrep: warning: acl/denied.log: permission denied`.

**VC-6.1:** `acl/denied.log` listado, lectura con 403 `AccessDenied`, `gcsgrep secreto s3://bkt/acl/denied.log` → stdout vacío, stderr `gcsgrep: warning: acl/denied.log: permission denied\n`, exit `2`, exactamente 1 request de lectura.

### FR-7 · Un objeto que dejó de existir

**Dado** el objeto `gone/a.log`, que el listado devuelve y cuya lectura responde HTTP 404 `NoSuchKey`,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/gone/`,
**entonces** `stderr` es exactamente `gcsgrep: warning: gone/a.log: object not found`.

**VC-7.1:** `gone/a.log` listado con tamaño 8 bytes, lectura con 404 `NoSuchKey`, `gcsgrep timeout s3://bkt/gone/` → stdout vacío, stderr `gcsgrep: warning: gone/a.log: object not found\n`, exit `2`, exactamente 1 request de lectura.

### FR-8 · Un objeto archivado

**Dado** el objeto `arch/old.log` cuya lectura responde HTTP 403 `InvalidObjectState`,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/arch/`,
**entonces** `stderr` es la línea `gcsgrep: warning: arch/old.log: read failed: <detalle>`, donde `<detalle>` es el texto del error del servicio, que incluye el código `InvalidObjectState`.

**VC-8.1:** `arch/old.log` listado, lectura con 403 `InvalidObjectState`, `gcsgrep timeout s3://bkt/arch/` → stdout vacío, stderr con una sola línea que empieza con `gcsgrep: warning: arch/old.log: read failed: ` y contiene `InvalidObjectState` (sin `permission denied`), exit `2`, exactamente 1 request de lectura.

### FR-9 · El cuerpo se corta antes de los primeros 8192 bytes

**Dado** el objeto `cut/early.log` con `Content-Length` 4096 cuyo cuerpo se corta tras 17 bytes (`timeout uno\nINFO\n`),
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/cut/early.log`,
**entonces** `stderr` es exactamente `gcsgrep: warning: cut/early.log: read interrupted: unexpected EOF`.

**VC-9.1:** `cut/early.log` con `Content-Length: 4096` y corte tras 17 bytes, `gcsgrep timeout s3://bkt/cut/early.log` → stdout `cut/early.log:1:timeout uno\n`, stderr `gcsgrep: warning: cut/early.log: read interrupted: unexpected EOF\n`, exit `2`, exactamente 1 request de lectura.

### FR-10 · El cuerpo se corta después de los primeros 8192 bytes

**Dado** el objeto `cut/late.log` con `Content-Length` 20000 cuyo cuerpo se corta tras 9000 bytes (`timeout uno\n` seguido de 8988 bytes `x`),
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/cut/late.log`,
**entonces** `stderr` es exactamente `gcsgrep: warning: cut/late.log: read interrupted: unexpected EOF`.

**VC-10.1:** `cut/late.log` con `Content-Length: 20000` y corte tras 9000 bytes, `gcsgrep timeout s3://bkt/cut/late.log` → stdout `cut/late.log:1:timeout uno\n`, stderr `gcsgrep: warning: cut/late.log: read interrupted: unexpected EOF\n`, exit `2`, exactamente 1 request de lectura.

### FR-11 · Listado denegado

**Dado** credenciales sin permiso `s3:ListBucket` (el listado responde HTTP 403 `AccessDenied`),
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** `stderr` es exactamente `gcsgrep: error: permission denied listing s3://bkt/logs/`.

**VC-11.1:** listado de `bkt` con 403 `AccessDenied`, `gcsgrep timeout s3://bkt/logs/` → stdout vacío, stderr `gcsgrep: error: permission denied listing s3://bkt/logs/\n`, exit `2`, exactamente 1 request de listado y 0 de lectura.

### FR-12 · Bucket inexistente

**Dado** una ubicación cuyo bucket no existe (el listado responde HTTP 404 `NoSuchBucket`),
**cuando** el usuario ejecuta `gcsgrep timeout s3://nobucket/`,
**entonces** `stderr` es exactamente `gcsgrep: error: bucket nobucket does not exist`.

**VC-12.1:** el servidor sin el bucket `nobucket`, `gcsgrep timeout s3://nobucket/` → stdout vacío, stderr `gcsgrep: error: bucket nobucket does not exist\n`, exit `2`, exactamente 1 request de listado.

### FR-13 · Otro error permanente de listado

**Dado** un listado que responde HTTP 400 `AuthorizationHeaderMalformed`,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** `stderr` es la línea `gcsgrep: error: could not list s3://bkt/logs/: <detalle>`, donde `<detalle>` es el texto del error del servicio, que incluye el código `AuthorizationHeaderMalformed`.

**VC-13.1:** listado de `bkt` con 400 `AuthorizationHeaderMalformed`, `gcsgrep timeout s3://bkt/logs/` → stdout vacío, stderr con una sola línea que empieza con `gcsgrep: error: could not list s3://bkt/logs/: ` y contiene `AuthorizationHeaderMalformed`, exit `2`, exactamente 1 request de listado.

### FR-14 · Sin credenciales

**Dado** un entorno sin credenciales S3,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** `stderr` tiene una línea que empieza con `gcsgrep: error: no AWS credentials found: `.

**VC-14.1:** `ENV_S3` sin `AWS_ACCESS_KEY_ID` ni `AWS_SECRET_ACCESS_KEY`, `gcsgrep timeout s3://bkt/logs/` → stdout vacío, stderr con una sola línea que empieza con `gcsgrep: error: no AWS credentials found: `, exit `2`, 0 requests al servidor.

### FR-15 · Sin región

**Dado** un entorno con credenciales S3 y sin región,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** `stderr` es exactamente `gcsgrep: error: no AWS region found: set AWS_REGION`.

**VC-15.1:** `ENV_S3` sin `AWS_REGION`, `gcsgrep timeout s3://bkt/logs/` → stdout vacío, stderr `gcsgrep: error: no AWS region found: set AWS_REGION\n`, exit `2`, 0 requests al servidor.

### FR-16 · Ubicación sin bucket

**Dado** una ubicación `s3://` sin nombre de bucket,
**cuando** el usuario ejecuta `gcsgrep timeout s3:///logs/`,
**entonces** `stderr` es exactamente `gcsgrep: error: invalid location "s3:///logs/": missing bucket name`.

**VC-16.1:** `gcsgrep timeout s3:///logs/` → stdout vacío, stderr `gcsgrep: error: invalid location "s3:///logs/": missing bucket name\n`, exit `2`, 0 requests al servidor.

### FR-17 · `stderr` solo lleva líneas de `gcsgrep`

**Dado** el bucket `bkt` con los objetos de F1,
**cuando** el usuario ejecuta `gcsgrep timeout s3://bkt/logs/`,
**entonces** toda línea de `stderr` empieza con `gcsgrep: `.

**VC-17.1:** F1 (el servidor no envía checksum), `gcsgrep timeout s3://bkt/logs/` → stdout `logs/a.log:2:ERROR timeout\n`, 0 líneas de `stderr` que no empiecen con `gcsgrep: ` (el progreso sí empieza así), exit `0`.

### BR-1 · Solo lectura, con dos operaciones

**Regla:** `gcsgrep` solo envía a S3 requests `GET` de listado y de lectura de un objeto. Los permisos mínimos de quien invoca son `s3:ListBucket` sobre el bucket y `s3:GetObject` sobre los objetos. El código de `internal/s3client` no usa ninguna operación de escritura ni otra de lectura.

**Fundamento:** una herramienta de búsqueda no debe poder alterar un bucket, y pedir solo estos dos permisos permite darle una política mínima.

**Excepciones:** ninguna.

**VC-BR1.1:** `gcsgrep timeout s3://bkt/logs/` con F1 → el servidor registra 0 requests con método distinto de `GET` (contador de 405 en `0`), y cada request es `/<bucket>?list-type=2…` o `/<bucket>/<clave>?x-id=GetObject`: 0 requests con otra query (`location`, `acl`, `versioning`).
**VC-BR1.2:** búsqueda estática en `internal/s3client` sin los `_test.go` → 0 apariciones de `PutObject`, `DeleteObject`, `DeleteObjects`, `CopyObject`, `CreateMultipartUpload`, `PutObjectAcl`, `RestoreObject`, `HeadObject`, `HeadBucket` y `GetBucketLocation`, y `go test ./internal/invariants` en `0` fallos (incluye la prueba existente de solo lectura de GCS, que recorre también los archivos nuevos).

### BR-2 · La identidad es la del entorno

**Regla:** las credenciales S3 son las que resuelve la cadena por defecto del SDK de AWS (variables `AWS_*` y archivos compartidos de credenciales y configuración, que esta spec ejercita; las demás fuentes del SDK las resuelve el SDK y esta spec no las ejercita). `gcsgrep` no tiene ningún flag para elegir otra identidad, otra región ni otro endpoint, y nunca muestra contenido al que esa identidad no puede acceder.

**Fundamento:** la herramienta opera dentro del límite de autorización de quien la ejecuta (`gcsgrep-spec.md` BR-2) y sin segunda forma de elegir identidad (`gcsgrep-design.md`, decisión 2).

**Excepciones:** ninguna.

**VC-BR2.1:** F1, dos corridas de `gcsgrep timeout s3://bkt/logs/`, la 1ª con `AWS_ACCESS_KEY_ID=AKIDONE` y la 2ª con `AWS_ACCESS_KEY_ID=AKIDTWO` → la cabecera `Authorization` de cada request de la 1ª contiene `Credential=AKIDONE/` y la de cada request de la 2ª `Credential=AKIDTWO/`; el stdout de ambas es `logs/a.log:2:ERROR timeout\n`.
**VC-BR2.2:** `ENV_S3` sin `AWS_ACCESS_KEY_ID` ni `AWS_SECRET_ACCESS_KEY` y `HOME` con `.aws/credentials` que define `[default]` con `aws_access_key_id = AKIDFILE` y `aws_secret_access_key = secretfile`, F1, `gcsgrep timeout s3://bkt/logs/` → exit `0` y `Authorization` con `Credential=AKIDFILE/` en cada request.
**VC-BR2.3:** `gcsgrep --profile prod timeout s3://bkt/logs/`, `gcsgrep --region us-east-1 timeout s3://bkt/logs/` y `gcsgrep --endpoint http://x timeout s3://bkt/logs/` → en cada una stdout vacío, exit `2` y stderr `gcsgrep: error: unknown flag -profile\n`, `gcsgrep: error: unknown flag -region\n` y `gcsgrep: error: unknown flag -endpoint\n` respectivamente; 0 requests al servidor.

### BR-3 · Exit code

**Regla:** toda falla de S3 y todo error de uso terminan la corrida con exit code `2`, incluso si ya se imprimieron matches. Una corrida sin fallas termina con `0` si hubo al menos un match y con `1` si no hubo ninguno.

**Fundamento:** es la convención de `gcsgrep-spec.md` FR-8, fijada acá para todas las fallas de esta spec.

**Excepciones:** ninguna.

**VC-BR3.1:** el listado con 403 `AccessDenied` (VC-11.1) → exit `2`.
**VC-BR3.2:** `acl/denied.log` (403) y `acl/ok.log` = `secreto ok\n`, `gcsgrep secreto s3://bkt/acl/` → stdout `acl/ok.log:1:secreto ok\n` y exit `2` (no `0`).
**VC-BR3.4:** `gcsgrep timeout s3:///logs/` (error de uso, VC-16.1) → exit `2`.
**VC-BR3.3:** F1, `gcsgrep inexistente s3://bkt/logs/` → stdout vacío, stderr vacío, exit `1`.

### BR-4 · Precedencia de los errores de uso

**Regla:** cuando una invocación tiene varios errores, solo se emite el primero de este orden: (1) flag desconocido o con valor inválido; (2) `-l` junto con `-c`; (3) cantidad de argumentos distinta de 2; (4) ubicación inválida; (5) patrón inválido; (6) credenciales ausentes; (7) región ausente. Los órdenes 1 a 5 son errores de uso: no crean el cliente ni hacen requests.

**Fundamento:** es el orden en que `cli.Parse`, `match.New` y la fábrica de cliente ya se ejecutan en `app.RunWithTerminals`; esta regla lo fija para S3.

**Excepciones:** ninguna.

**VC-BR4.1:** `gcsgrep -v -l -c '(' mybucket/x` → stderr `gcsgrep: error: unknown flag -v\n`, exit `2`.
**VC-BR4.2:** `gcsgrep -l -c '(' s3:///x` → stderr `gcsgrep: error: -l and -c cannot be used together\n`, exit `2`.
**VC-BR4.3:** `gcsgrep '(' s3:///logs/` → stderr `gcsgrep: error: invalid location "s3:///logs/": missing bucket name\n`, exit `2`.
**VC-BR4.6:** `gcsgrep '(' s3://bkt/logs/ extra` → stderr `gcsgrep: error: expected 2 arguments (PATTERN and LOCATION), got 3\n`, exit `2` (el orden 3 va antes que el patrón inválido).
**VC-BR4.7:** `gcsgrep` sin argumentos → stderr `gcsgrep: error: expected 2 arguments (PATTERN and LOCATION), got 0\n`, exit `2`, 0 requests al servidor.
**VC-BR4.4:** `ENV_S3` sin credenciales, `gcsgrep '(' s3://bkt/logs/` → stderr `gcsgrep: error: invalid pattern "(": error parsing regexp: missing closing ): ` + `` `(` `` + `\n`, exit `2`, 0 requests al servidor.
**VC-BR4.5:** `ENV_S3` sin credenciales y sin `AWS_REGION`, `gcsgrep timeout s3://bkt/logs/` → una sola línea de `stderr` que empieza con `gcsgrep: error: no AWS credentials found: `, exit `2`.

### BR-5 · Lo posterior a abrir un objeto no depende del proveedor

**Regla:** el patrón es una expresión regular RE2 (`gcsgrep-spec.md` FR-1.3) y un patrón inválido se informa con el texto de error del paquete `regexp` de Go (`gcsgrep: error: invalid pattern "<patrón>": <error de regexp>`); los modos de salida, `-i`, la descompresión de `.gz` (por el sufijo del nombre), la detección de binarios, los bordes de contenido y los tres guardarraíles (`--max`, `--max-object-size`, `--max-total-size`, con los valores por defecto de siempre) se comportan con `s3://` igual que con `gs://`. S3 solo aporta el listado y la lectura.

**Fundamento:** `scanner.scanObject` y `reader.ProcessObject` reciben un `io.ReadCloser` y no conocen al proveedor.

**Excepciones:** ninguna.

**VC-BR5.1:** F1, `gcsgrep -l timeout s3://bkt/logs/` → stdout `logs/a.log\n`, exit `0`.
**VC-BR5.2:** F1, `gcsgrep -i TIMEOUT s3://bkt/logs/` → stdout `logs/a.log:2:ERROR timeout\n`, exit `0`.
**VC-BR5.3:** `gz/app.log.gz` = gzip de `INFO start\nERROR timeout\n`, `gcsgrep timeout s3://bkt/gz/` → stdout `gz/app.log.gz:2:ERROR timeout\n`, stderr vacío, exit `0`.
**VC-BR5.4:** `gz/bad.log.gz` = los 8 bytes `not gzip`, `gcsgrep timeout s3://bkt/gz/bad.log.gz` → stdout vacío, stderr `gcsgrep: warning: gz/bad.log.gz: corrupt gzip data\n`, exit `2`.
**VC-BR5.5:** `bin/blob.bin` = `ab\0cd timeout\n` (con un byte nulo), `gcsgrep timeout s3://bkt/bin/` → stdout vacío, stderr `gcsgrep: warning: bin/blob.bin: skipped (binary object)\n`, exit `1`.
**VC-BR5.6:** `empty/zero.log` de 0 bytes, `gcsgrep -c timeout s3://bkt/empty/` → stdout `empty/zero.log:0\n`, stderr vacío, exit `1`.
**VC-BR5.7:** `eol/nonl.log` = `INFO\nERROR timeout` (sin `\n` final), `gcsgrep timeout s3://bkt/eol/` → stdout `eol/nonl.log:2:ERROR timeout\n`, exit `0`.
**VC-BR5.8:** `many/` con 2500 objetos, `gcsgrep needle s3://bkt/many/` → stdout vacío, stderr `gcsgrep: error: the prefix has 2500 objects, which exceeds the limit of 1000 (use --max to raise it, or --max 0 to disable it)\n`, exit `2`, 0 requests de lectura.
**VC-BR5.9:** `big/big.log` = 2 MiB (2.097.152 bytes) de líneas `INFO\n` y `big/ok.log` = `timeout\n`, `gcsgrep --max-object-size 1048576 timeout s3://bkt/big/` → stdout `big/ok.log:1:timeout\n`, stderr `gcsgrep: warning: big/big.log: object size limit of 1048576 bytes reached, rest of the object not read\n`, exit `2`.
**VC-BR5.10:** `tot/1.log` … `tot/5.log` de 1 MiB cada uno, con `timeout` como línea 1 y relleno `INFO` en el resto, `gcsgrep --max-total-size 2621440 timeout s3://bkt/tot/` → stdout `tot/1.log:1:timeout\ntot/2.log:1:timeout\ntot/3.log:1:timeout\n`, stderr `gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete\n`, exit `2`, exactamente 3 requests de lectura.

### BR-6 · Un objeto fallido no interrumpe la corrida

**Regla:** tras un objeto fallido la corrida lee los objetos siguientes del listado. Las líneas que el objeto fallido ya imprimió se mantienen y no se repiten.

**Fundamento:** es el comportamiento de `scanner.Run`, que sigue con el siguiente objeto tras `scanObject` (`gcsgrep-spec.md` FR-9); esta regla lo fija para S3.

**Excepciones:** ninguna.

**VC-BR6.1:** `acl/denied.log` (403) y `acl/ok.log` = `secreto ok\n`, `gcsgrep secreto s3://bkt/acl/` → stdout `acl/ok.log:1:secreto ok\n`, stderr `gcsgrep: warning: acl/denied.log: permission denied\n`, exit `2`, 2 requests de lectura (uno por objeto).
**VC-BR6.2:** `cut/early.log` (VC-9.1) y `cut/zz.log` = `timeout dos\n`, `gcsgrep timeout s3://bkt/cut/` → stdout `cut/early.log:1:timeout uno\ncut/zz.log:1:timeout dos\n` (la línea 1 de `early.log` una sola vez), stderr `gcsgrep: warning: cut/early.log: read interrupted: unexpected EOF\n`, exit `2`.

### BR-7 · Los permisos mínimos y el build están documentados

**Regla:** `greenfield/README.md` declara cómo compilar el build S3, que la ubicación es `s3://bucket/prefijo`, que los permisos mínimos de quien invoca son `s3:ListBucket` y `s3:GetObject`, y que fuera de AWS conviene definir `AWS_EC2_METADATA_DISABLED=true` para que la falta de credenciales se informe sin esperar al servicio de metadata de EC2 (D18).

**Fundamento:** quien invoca necesita saber qué política IAM pedir; los permisos de BR-1 son un requisito del uso.

**Excepciones:** ninguna.

**VC-BR7.1:** `grep -c 's3:ListBucket' greenfield/README.md` y `grep -c 's3:GetObject' greenfield/README.md` → ambos `1` o más.
**VC-BR7.2:** `grep -c -e '-tags s3' greenfield/README.md` → `1` o más.
**VC-BR7.3:** `grep -c 'AWS_EC2_METADATA_DISABLED=true' greenfield/README.md` → `1` o más.
**VC-BR7.4:** `grep -c 's3://bucket/prefijo' greenfield/README.md` → `1` o más.

### NFR-1 · Memoria con objetos grandes en S3

**Umbral:** el pico de memoria residente (RSS) al procesar `mem/large.log` (500 MiB) supera al de procesar `mem/small.log` (5 MiB), con el mismo contenido repetido, en **≤ 5 MiB** (5.242.880 bytes). La lectura es por streaming.

**VC-NFR1.1:** el servidor S3 de prueba genera por streaming `mem/small.log` (5 MiB) y `mem/large.log` (500 MiB), ambos líneas de 1023 bytes `x` más `\n`; se compila `go build -tags s3 -o $BIN ./cmd/gcsgrep`; se ejecuta con `ENV_S3`, como proceso aparte, `$BIN --max-object-size 0 --max-total-size 0 needle s3://bkt/mem/small.log` y la misma línea con `mem/large.log`, 3 veces cada una; el RSS máximo de cada proceso sale de `getrusage` (el dato de `/usr/bin/time -f %M`) y se toma la mediana de cada una → las 6 corridas terminan con exit `1`, y RSS máximo de `large.log` − RSS máximo de `small.log` ≤ 5.242.880 bytes.

### NFR-2 · Reintentos ante fallas transitorias

**Umbral:** un error transitorio al **listar** o al **abrir** un objeto se reintenta hasta **3 intentos en total**. Es transitorio un HTTP 408, 429 o 5xx, o una conexión reseteada. La espera antes del 2º intento es de 500 ms ± 20 % (400 a 600 ms) y antes del 3º de 1 s ± 20 % (800 a 1200 ms). Un error permanente (HTTP 4xx distinto de 408 y 429) no se reintenta. Si se agotan los 3 intentos al abrir, el objeto es un objeto fallido, con el aviso `gcsgrep: warning: <objeto>: network error after 3 attempts: <detalle>`. Si se agotan al listar, no se abre ningún objeto y el mensaje de error es `gcsgrep: error: could not list s3://<bucket>/<prefijo> after 3 attempts: <detalle>`. Un error durante la lectura de un objeto ya abierto no se reintenta (FR-9, FR-10).

**VC-NFR2.1:** `r/a.log` = `timeout\n`, su lectura falla 2 veces con HTTP 503 `SlowDown` y la 3ª tiene éxito, `gcsgrep timeout s3://bkt/r/` → stdout `r/a.log:1:timeout\n`, stderr vacío, exit `0`, 3 requests de lectura, espera entre el 1º y el 2º de 400 a 600 ms y entre el 2º y el 3º de 800 a 1200 ms (instantes del servidor).
**VC-NFR2.2:** `r/a.log`, su lectura falla 3 veces con HTTP 503 → exactamente 3 requests de lectura, stderr con una línea que empieza con `gcsgrep: warning: r/a.log: network error after 3 attempts: `, exit `2`.
**VC-NFR2.3:** el listado de `r/` falla 3 veces con HTTP 503 → exactamente 3 requests de listado, 0 de lectura, stderr con una línea que empieza con `gcsgrep: error: could not list s3://bkt/r/ after 3 attempts: `, exit `2`.
**VC-NFR2.7:** F1, el primer listado de `logs/` responde HTTP 503 `SlowDown` y el segundo tiene éxito, `gcsgrep timeout s3://bkt/logs/` → stdout `logs/a.log:2:ERROR timeout\n`, exit `0`, 2 requests de listado con una espera entre ambos de 400 a 600 ms.
**VC-NFR2.4:** el listado de `r/` responde 403 `AccessDenied` → exactamente 1 request de listado, stderr `gcsgrep: error: permission denied listing s3://bkt/r/\n`, exit `2`; y la lectura de `r/a.log` responde 404 `NoSuchKey` → exactamente 1 request de lectura.
**VC-NFR2.5:** para cada estado de la lista `408`, `429`, `500`, `502`, `504`: la primera lectura de `r/a.log` responde ese estado y la segunda tiene éxito, `gcsgrep timeout s3://bkt/r/` → 2 requests de lectura, stdout `r/a.log:1:timeout\n`, exit `0`.
**VC-NFR2.6:** la primera lectura de `r/a.log` cierra la conexión con RST (`connection reset by peer`) y la segunda tiene éxito, `gcsgrep timeout s3://bkt/r/` → 2 requests de lectura, stdout `r/a.log:1:timeout\n`, exit `0`.

## VCs de invariantes

**VC-INV1.1:** desde `greenfield/gcsgrep/`, `go build -o $BIN ./cmd/gcsgrep` (sin tag) → exit `0`; `go version -m $BIN | grep -c 'aws-sdk-go-v2'` → `0`; `go list -deps ./cmd/gcsgrep | grep -c aws` → `0`.
**VC-INV1.2:** `go build ./... && go vet ./...` (sin tag) → exit `0` y sin salida; `go list -deps -tags s3 ./cmd/gcsgrep | grep -c 'aws-sdk-go-v2/service/s3$'` → `1`.
**VC-INV2.1:** el binario del build base, `$BIN timeout s3://b/p/` → stdout vacío, stderr `gcsgrep: error: invalid location "s3://b/p/": must start with gs://\n`, exit `2`.
**VC-INV2.2:** `strings $BIN | grep -c 'ListObjectsV2'` → `0` con el binario del build base y `1` o más con el binario de `go build -tags s3 -o $BIN3 ./cmd/gcsgrep`.
**VC-INV3.1:** `go test -count=1 -v ./... 2>&1 | grep -- '--- PASS' | sed -E 's/^ *--- PASS: ([^ ]+).*/\1/' | sort > $AFTER`; `comm -23 greenfield/gcsgrep-s3-baseline-pass.txt $AFTER` → sin salida (los 163 nombres siguen en verde) y `grep -c -- '--- FAIL'` de la misma corrida → `0`.
**VC-INV3.2:** lo mismo con `-tags s3` → `comm -23` sin salida y `0` líneas `--- FAIL`.
**VC-INV4.1:** build S3, `gcsgrep timeout mybucket/logs/` → stdout vacío, stderr `gcsgrep: error: invalid location "mybucket/logs/": must start with gs://\n`, exit `2`, 0 requests al servidor.
**VC-INV4.2:** build S3, `gs://b/p/` con un cliente GCS simulado (`gcsclienttest.Fake`) sin objetos → stdout vacío, stderr `gcsgrep: warning: no objects under gs://b/p/\n`, exit `1`, 0 requests al servidor S3 de prueba.
**VC-INV4.3:** build S3, `gcsgrep timeout gs://b/p/` con una fábrica de cliente GCS que falla con el error `boom` → stdout vacío, stderr `gcsgrep: error: no Application Default Credentials found: boom\n`, exit `2`.

## Matriz de cobertura

| Requisito | VC | Camino de falla incluido |
|---|---|---|
| FR-1 | VC-1.1 | — |
| FR-2 | VC-2.1, VC-2.2 | — |
| FR-3 | VC-3.1 | — |
| FR-4 | VC-4.1 | — |
| FR-5 | VC-5.1 | Sin objetos (camino 1) |
| FR-6 | VC-6.1 | Objeto ilegible (camino 2) |
| FR-7 | VC-7.1 | Objeto ilegible (camino 2) |
| FR-8 | VC-8.1 | Objeto ilegible (camino 2) |
| FR-9 | VC-9.1 | Corte de red (camino 2) |
| FR-10 | VC-10.1 | Corte de red (camino 2) |
| FR-11 | VC-11.1 | Sin permiso (camino 3) |
| FR-12 | VC-12.1 | Sin permiso (camino 3) |
| FR-13 | VC-13.1 | Error de listado (camino 3) |
| FR-14 | VC-14.1 | Sin credenciales (camino 3) |
| FR-15 | VC-15.1 | Sin región (camino 3) |
| FR-16 | VC-16.1 | Entrada inválida (camino 4) |
| FR-17 | VC-17.1 | — |
| BR-1 | VC-BR1.1, VC-BR1.2 | — |
| BR-2 | VC-BR2.1, VC-BR2.2, VC-BR2.3 | Flags de identidad rechazados (camino 4) |
| BR-3 | VC-BR3.1 a VC-BR3.4 | Exit code de toda falla |
| BR-4 | VC-BR4.1 a VC-BR4.7 | Precedencia (camino 4) |
| BR-5 | VC-BR5.1 a VC-BR5.10 | Binario, `.gz` corrupto, guardarraíles |
| BR-6 | VC-BR6.1, VC-BR6.2 | Qué pasa con el resto (camino 2) |
| BR-7 | VC-BR7.1 a VC-BR7.4 | — |
| NFR-1 | VC-NFR1.1 | — |
| NFR-2 | VC-NFR2.1 a VC-NFR2.7 | Reintentos agotados, errores permanentes |
| INV-1 a INV-4 | VC-INV1.1 a VC-INV4.3 | — |

## Plan de iteraciones

Las iteraciones son las de esta spec (1 a 4), no las de `gcsgrep-plan.md`. En cada una la suite existente se corre completa antes de cerrarla (INV-3). El servidor S3 de prueba crece con la iteración que lo necesita: listado y lectura en la 1; estados y errores inyectados en la 2; cortes, RST e instantes por request en la 3; cuerpos generados por streaming en la 4.

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Guarda de build `s3`, esquema `s3://` en `cli`, fábrica por esquema en `app`, `Config.Scheme`, cliente S3 con listado paginado y lectura, SDK sin log ni reintentos propios, servidor S3 de prueba mínimo, `go.mod` | INV-1, INV-2, INV-3, INV-4, FR-1, FR-2, FR-3, FR-4, FR-17, BR-1 |
| **2** | Errores de entrada, de credenciales, de región y de listado, con sus mensajes | FR-5, FR-11, FR-12, FR-13, FR-14, FR-15, FR-16, BR-2, BR-4 |
| **3** | Objetos fallidos, cortes de lectura (con el arreglo de `sniffBinary`) y reintentos | FR-6, FR-7, FR-8, FR-9, FR-10, BR-3, BR-6, NFR-2 |
| **4** | Paridad con GCS, memoria y documentación | BR-5, BR-7, NFR-1 |

## Decisiones

| # | Pregunta | Elegido | Fundamento (código del sistema) | Descartado y por qué | FR |
|---|---|---|---|---|---|
| **D1** | ¿Cómo se aísla S3 del binario de hoy? | Tag de build `s3` (opt-in). SDK de AWS: `aws-sdk-go-v2 v1.47.1`, `config v1.33.7`, `credentials v1.20.7`, `service/s3 v1.114.1`, `smithy-go v1.28.1` (las de la sonda) | `internal/tty/tty_unix.go` (`//go:build unix`), `tty_windows.go` y `tty_other.go` eligen archivos por tag; `integration/integration_test.go` (`//go:build integration`); `gcsclient/gcs.go` es "the only code in gcsgrep that imports the SDK" | Siempre activo: INV-1 no podría cumplirse. Flag en runtime: el SDK igual quedaría enlazado en el binario. Ejecutable aparte: duplica `cli` y `app` | INV-1, INV-2 |
| **D2** | ¿Cómo se nombra una ubicación S3? | `s3://bucket/prefijo`, con el mismo tratamiento del prefijo que `gs://` | `cli.parseLocation` solo acepta `gs://`; `gcsgrep-design.md`, decisión 3: "otro proveedor (`s3://`) podría sumarse sin romper nada" | `https://…`: ambiguo entre proveedores. Inferir el proveedor por el bucket: no hay regla en el código | FR-1, FR-2, FR-3 |
| **D3** | ¿Dónde vive el cliente S3? | Paquete nuevo `internal/s3client` que implementa `gcsclient.Client` | `scanner.Run(ctx, client gcsclient.Client, …)` y `gcsclient.WithRetries(inner Client, …)` dependen solo de la interfaz; `gcsclient.go` es la "SDK-free part" | Renombrar `gcsclient`: toca los tests existentes (INV-3). Copiar `scanner` para S3: dos copias de los guardarraíles | BR-5 |
| **D4** | ¿Cómo dicen `s3://` los mensajes? | `scanner.Config.Scheme`; el valor cero es `gs` | `scanner.go` escribe `gs://%s/%s` en cuatro sitios (`Run` en la línea 85 y tres en `describeListError`, líneas 200, 202 y 206); los tests de `scanner` arman `Config` sin esquema | Parámetro obligatorio: rompe los tests de `scanner` (INV-3) | FR-5, FR-11, FR-13, NFR-2 |
| **D5** | ¿Qué dice el error de ubicación sin esquema en el build S3? | El mismo texto que hoy: `must start with gs://` | `cli_test.go:TestParse_UsageErrorMessages` compara `err.Error()` con `invalid location "mybucket/logs/": must start with gs://` por igualdad | `must start with gs:// or s3://`: rompería ese test en el build S3, una regresión que se arregla en el código, no en el test | INV-4 |
| **D6** | ¿Cómo se elige la identidad? | Solo la cadena por defecto del SDK; sin flags | `gcsclient.New`: "gcsgrep never accepts a service-account key file"; `gcsgrep-design.md`, decisión 2 | `--profile`, `--region`: abren una segunda forma de elegir identidad, la razón por la que el diseño descartó `--credentials` | BR-2 |
| **D7** | ¿Cuándo se detectan las credenciales ausentes? | En `New`: `Credentials.Retrieve` antes de cualquier request | La sonda: sin credenciales `LoadDefaultConfig` devuelve `nil` y solo `Retrieve` falla. `gcsclient.New` falla al construir y `app.RunWithTerminals` imprime el error antes de listar | Dejar que falle en el primer listado: el error del SDK sería `Invalid region…` o un fallo de firma, no un mensaje de credenciales | FR-14 |
| **D8** | ¿Qué pasa sin región? | Error en `New` si la región resuelta es vacía; sin valor por defecto | La sonda: sin región el primer request falla con `Invalid region: region was not a valid DNS name.` | Asumir `us-east-1`: elige por la persona una región que no declaró; `gcsclient.New` tampoco inventa una identidad | FR-15 |
| **D9** | ¿Qué error va primero si faltan credenciales y región? | Credenciales | `app.RunWithTerminals` ya trata el fallo de `newClient(ctx)` como el primer error tras validar la invocación (`no Application Default Credentials found`); el criterio es que la ausencia de credenciales sea ese mismo error también en S3, y la sonda la muestra como el primer error del SDK | Región primero: oculta que también faltan credenciales | BR-4 |
| **D10** | ¿Reintenta el SDK? | No: un solo intento por llamada; rige `gcsclient.WithRetries` | `gcs.go`: `sc.SetRetry(storage.WithPolicy(storage.RetryNever))`; `retry.go`: `DefaultRetryPolicy` es la única política | Dejar los reintentos del SDK: sumarían intentos y esperas a los de NFR-2 | NFR-2 |
| **D11** | ¿Cómo se clasifican los errores del SDK? | Por estado HTTP: 403 → permiso (salvo `InvalidObjectState`), 404 → bucket (listado) u objeto (lectura), 408, 429, 5xx y conexión reseteada → transitorio (sin timeout propio: `gcsgrep` no fija ninguno en la conexión) | `gcs.go`: `translateListError`, `translateOpenError`, `isTransient`, `httpStatus` usan la misma tabla; la sonda mide los códigos y estados de S3 | Por el texto del mensaje: frágil | FR-6, FR-7, FR-11, FR-12, NFR-2 |
| **D12** | ¿Cómo se informa un objeto archivado? | Como `read failed: <detalle>` | `scanner.describeOpenError`: la rama por defecto es `read failed: %v`; la sonda: S3 responde 403 `InvalidObjectState`, y "permission denied" sería falso | Mensaje propio: sumaría un caso sin pedido | FR-8 |
| **D13** | ¿Cómo se lista? | `ListObjectsV2` paginado hasta el final, con `Prefix` y sin `Delimiter`, devolviendo todos los objetos | `gcs.go:List` usa `storage.Query{Prefix: prefix}` sin delimitador y devuelve `[]ObjectInfo`; `scanner.Run` necesita `len(objects)` para BR-3 antes de leer | `Delimiter` `/`: un prefijo sin barra dejaría de ser filtro por nombre. Listado perezoso: BR-3 necesita el total | FR-3, FR-4 |
| **D14** | ¿Cómo se decide que un objeto es `.gz`? | Por el sufijo del nombre | `scanner.scanObject`: `Gzip: strings.HasSuffix(name, ".gz")`; la sonda: el SDK entrega el cuerpo sin decodificar aunque el objeto lleve `Content-Encoding: gzip` | Respetar `Content-Encoding`: dos criterios según el proveedor | BR-5 |
| **D15** | ¿Qué se hace con el log del SDK? | Se descarta | La sonda: el SDK escribe en `stderr` `SDK <fecha> DEBUG Response has no supported checksum…` por cada lectura; `output.go` declara que `stderr` solo lleva avisos, errores y progreso con sus prefijos | Dejarlo: rompe toda comparación exacta de `stderr`. Apagar la validación de checksum: cambia el comportamiento del SDK por una línea de log | FR-17 |
| **D16** | ¿Cómo se detecta un corte antes de 8192 bytes? | `sniffBinary` distingue el fin del objeto de un error del stream | `reader.go:sniffBinary` trata `io.ErrUnexpectedEOF` de `io.ReadFull` como fin del objeto. Probado en una copia del módulo: un stream de 20 bytes que termina en `io.ErrUnexpectedEOF` da `Failed=false` (con 9000 bytes da `read interrupted: unexpected EOF`); la sonda: `net/http` informa el cuerpo truncado como `unexpected EOF` | Dejarlo: un corte temprano termina con exit `0` y líneas perdidas. Comparar con `Content-Length` en `s3client`: no ve el cuerpo | FR-9 |
| **D17** | ¿Cómo se cambia el endpoint? | No hay flag; rige la variable del SDK `AWS_ENDPOINT_URL_S3`, que usa el harness | `gcsclient.New` llama `storage.NewClient(ctx)` sin opción de endpoint: tampoco GCS tiene flag. La sonda: con esa variable el SDK habla con el servidor falso. D6 | `--endpoint`: sin caso de uso en esta spec | BR-2 |
| **D18** | ¿Hay un timeout propio para resolver credenciales? | No | `gcsclient.New` tampoco fija un timeout (`storage.NewClient(ctx)` con el `context.Background()` de `cmd/gcsgrep/main.go`). Criterio: el SDK ya lo acota; la sonda midió 7,3 s y 7,8 s hasta el error fuera de AWS sin `AWS_EC2_METADATA_DISABLED`; un timeout propio cortaría resoluciones legítimas lentas (SSO, rol de instancia) | Timeout propio de 5 s: acortaría esas resoluciones | BR-7 |
| **D19** | ¿Con qué servidor se prueba? | Servidor HTTP propio en el proceso del test | La sonda: el SDK habla con un servidor HTTP falso vía `AWS_ENDPOINT_URL_S3`; `gcsclienttest/fake.go` es el patrón de fake en memoria; `integration_test.go:run` llama al punto de entrada en el mismo proceso | MinIO o localstack: dependencia externa y sin control para inyectar cortes y RST | NFR-2, FR-9 |
| **D20** | ¿Qué valores tienen los guardarraíles con S3? | Los de siempre: `--max` 1000, 250 MiB por objeto, 2 GiB acumulado | `scanner.DefaultMaxObjects`, `DefaultMaxObjectSize`, `DefaultMaxTotalSize` | Valores por proveedor: ramas y flags sin pedido | BR-5 |
| **D21** | ¿Cómo elige `app` la fábrica? | `RunWithProviders` recibe un `Provider` por esquema; `Run` y `RunWithTerminals` conservan su firma y le pasan solo `gs` | `app_test.go:runWith` y `integration_test.go:run` llaman `Run(ctx, argv, stdout, stderr, factory)` | Cambiar el tipo `ClientFactory`: rompe esas llamadas (INV-3) | INV-3 |

## Limitaciones conocidas

- No hay VC contra AWS real: el contrato HTTP se verifica con el servidor S3 de prueba y con la sonda (versiones en D1).
- Un `.gz` truncado por la red se informa como `corrupt gzip data`, porque `reader/gzip.go:classifyGzipError` trata `io.ErrUnexpectedEOF` como gzip inválido; no cambia.
- Los marcadores de carpeta (claves terminadas en `/`) se listan como objetos y no se filtran; con `-c` salen con conteo `0`.
- El mensaje de ubicación sin esquema menciona solo `gs://` en ambos builds (D5); el esquema `s3://` se documenta en `greenfield/README.md` (BR-7).
- Fuera de AWS y sin `AWS_EC2_METADATA_DISABLED=true`, el error por falta de credenciales tarda entre 7,3 s y 7,8 s (medido en la sonda).
- No hay concurrencia (`gcsgrep-spec.md` FR-13): los objetos se leen de a uno.
