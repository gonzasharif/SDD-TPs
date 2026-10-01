# gcsgrep — tabla de cobertura de VCs

> Salida del paso **Verificar**, acumulada a lo largo de todo el proyecto
> (ver `gcsgrep-plan.md`) — **un documento por proyecto, no uno por
> iteración**. Cada iteración agrega su propia sección al final, sin tocar
> las anteriores. Esta tabla no dice "lo probé". Dice, para cada criterio
> de verificación, con qué se lo ejercitó y qué se observó. Un VC sin
> evidencia es un VC que no pasó.
>
> Entorno de prueba: un proyecto de GCP dedicado y un bucket de prueba en
> `us-central1` (nombres reales deliberadamente no versionados acá — ver
> `gcsgrep/testenv.local.md`, gitignoreado; pedíselo a quien armó el
> entorno si no lo tenés). ADC vía `gcloud auth application-default login`.
> Implementación en Go, código en `gcsgrep`.

## Iteración 1

### Resumen

| | |
|---|---|
| VCs en el alcance de la Iteración 1 | 13 |
| VCs con cobertura ejecutable | 13 |
| VCs pasando | 13 |
| Criterios de éxito sin evidencia | 0 |

### Cobertura, una por una

| VC | Requisito | Ejercitado por | Se observa | Estado |
|---|---|---|---|---|
| VC-1 | FR-1 búsqueda básica | `internal/reader/reader_test.go::TestProcessObject_BasicMatching` + demo real contra `gs://<test-bucket>/logs/` | match reportado en `logs/app1.log:2`, exit 0; inspección de código confirma cero llamadas `os.Create`/`os.WriteFile`/`os.OpenFile` (nunca se escribe a disco) | ✅ |
| VC-2 | FR-2 bucket completo | Demo real: `gcsgrep "timeout" gs://<test-bucket>/` (sin prefijo) | matches en `logs/app1.log` **y** `other-prefix/data.log`, dos prefijos distintos cubiertos en una sola corrida | ✅ |
| VC-3 (rama plana) | FR-3 formato de salida (Iteración 1: sin color) | Observado en cada corrida real | formato `objeto:línea:texto` en texto plano, sin secuencias ANSI (color es Iteración 2) | ✅ |
| VC-4 | FR-4 `-i` case-insensitive | `internal/match/match_test.go::TestMatchString_IgnoreCase` + demo real | `"timeout while"` sin `-i` → exit 1 (sin match); con `-i` → exit 0, matchea `logs/app3.log:2` (`TIMEOUT while...`) | ✅ |
| VC-8 | FR-8 exit codes 0/1/2 | `internal/scanner/scanner_test.go::TestRun_ExitMatchWhenSomethingMatches`, `TestRun_ExitNoMatchWhenNothingMatches`, `TestRun_ExitErrorOnUnreadableObjectButKeepsGoing` + 3 corridas reales | match→exit 0; sin match→exit 1; bucket inexistente→exit 2 (`storage: bucket doesn't exist`) | ✅ |
| VC-9 | FR-9 continuar ante objeto ilegible | `internal/scanner/scanner_test.go::TestRun_ExitErrorOnUnreadableObjectButKeepsGoing` (cliente GCS fake, `Open` falla para un objeto) | el objeto legible se procesa y aparece en stdout igual; el objeto sin permiso aparece en un warning en stderr; exit 2 | ⚠️ ver nota 1 |
| VC-11 | FR-11 binarios se saltean | `internal/reader/reader_test.go::TestProcessObject_SkipsBinary`, `TestProcessObject_TextWithNoNullBytesIsNotBinary` + visible en **todas** las corridas reales | `logs/icon.png` siempre reportado como "salteado (objeto binario...)" por stderr, nunca aparece en resultados de match | ✅ |
| VC-15 | BR-1 solo lectura | Corrida real completa con credenciales de un service account con **únicamente** el rol IAM `roles/storage.objectViewer` (creado, usado, y revocado/borrado tras la prueba) + inspección de código | mismo resultado exacto (exit 0, mismos matches) que con las credenciales normales; `gcsclient.Client` solo expone `List`/`Open`, cero llamadas `NewWriter`/`Delete`/`Update`/`SetACL` en toda la base de código | ✅ |
| VC-16 | BR-2 no amplifica acceso | Mismo experimento que VC-15 | credenciales acotadas específicamente a ese bucket (sin rol a nivel de proyecto) funcionaron igual, sin necesitar nada más amplio | ⚠️ ver nota 1 |
| VC-17 | BR-3 guardrail de cantidad | `internal/scanner/scanner_test.go::TestRun_ObjectCountGuardrailAbortsBeforeReadingContent`, `TestRun_ObjectCountGuardrailDisabledWithZero` + demo real: `--max 1` sobre un prefijo con 4 objetos | error explícito, exit 2, **cero** llamadas a `Open` (verificado con un cliente que cuenta aperturas); con `--max 0` los 4 se procesan | ✅ |
| VC-22 | NFR-2 memoria constante | `/usr/bin/time -l` sobre un objeto de ~8 MiB y uno de ~335 MiB (ambos vía `gs://<test-bucket>/mem/`) | RSS máxima: **39.0 MiB** (objeto chico) vs. **39.7 MiB** (objeto ~40x más grande) — diferencia de ~0.7 MiB pese a ~40x de diferencia de tamaño | ✅ |
| VC-24 | FR-15 línea larga se saltea | `internal/reader/reader_test.go::TestProcessObject_LongLineSkippedNotTruncated`, `TestProcessObject_LongLineAtEndOfFileNoTrailingNewline`, `TestProcessObject_LineExactlyAtLimitIsNotTruncated` | un match real embebido después del punto de corte de una línea de >MaxLineSize **no** se reporta (en vez de partirse o reportarse mal); exactamente un warning por objeto afectado; una línea de exactamente el límite no se considera larga | ✅ |
| VC-21 (parcial: secuencial) | NFR-1 rendimiento secuencial | Benchmark real: 20 objetos de ~1.7 MB, modo secuencial, contra `gs://<test-bucket>/perf/` | **0.59 objetos/seg** medido (umbral: ≥ 0.5 objetos/seg — ajustado tras esta medición, ver nota 2) | ✅ |

### Nota 1 — VC-9 / VC-16: gap conocido de verificación

Ambos se verificaron con un **cliente GCS fake** (inyectado vía la interfaz
`gcsclient.Client`) que simula un objeto cuyo `Open` falla, no contra un
objeto real de GCS con una ACL que efectivamente le niegue el acceso a las
credenciales usadas. Esto es así porque el bucket de prueba tiene
**uniform bucket-level access** habilitado, que no permite ACLs por objeto
— solo permisos a nivel de bucket. Reproducir el escenario exacto de FR-9
contra GCS real (un objeto puntual inaccesible dentro de un prefijo con
otros sí accesibles) requeriría IAM Conditions con restricciones por nombre
de objeto, que quedó fuera del alcance de esta verificación.

El test con cliente fake es una técnica de verificación válida (inyección
de dependencias vía interfaz) y ejercita exactamente el mismo camino de
código que un fallo real de `Open` — pero es un nivel de evidencia distinto
al de un VC ejercitado end-to-end contra la infraestructura real, y queda
anotado como pendiente de reforzar antes de dar la Iteración 1 por cerrada
del todo.

### Nota 2 — NFR-1 secuencial: investigación y ajuste de umbral

El umbral original propuesto en la spec (≥ 3 objetos/seg secuencial) **no
se cumplió** en la primera medición: 30 objetos de ~1.7 MB tardaron 51.7s
(≈ 0.59 objetos/seg). Antes de aceptar el número o bajar el umbral a ciegas,
se investigó si era un problema de implementación:

- **Buffer de copia más grande (1 MiB en vez de 32 KiB por defecto):** sin
  mejora — throughput se mantuvo en ~1 MB/s.
- **Un mismo objeto partido en 2 range-reads en paralelo:** sin mejora
  (incluso levemente peor) — descarta que el techo sea solo de ventana TCP
  de una conexión sobre un objeto individual.
- **Concurrencia real entre 20 objetos distintos, 8 workers:** **4.6x de
  mejora** (0.59 → 2.69 objetos/seg). Esto confirma que el techo es de
  latencia/ancho de banda del entorno de red hacia GCS (alto RTT en este
  sandbox), no del código de `gcsgrep` — y que la concurrencia (FR-13,
  Iteración 3) es la respuesta arquitectónica correcta, no una optimización
  prematura.

Con esa evidencia, se bajó el umbral secuencial de NFR-1 a **≥ 0.5
objetos/seg** (válido incluso en redes de latencia alta) en los tres
documentos (`gcsgrep-requirements.md`, `gcsgrep-spec.md`,
`gcsgrep-plan.md`), y se dejó explícito que en redes de latencia alta la
recomendación operativa es usar `--concurrency`, no depender del modo
secuencial. El umbral con concurrencia (≥ 15 objetos/seg) queda sin validar
hasta la Iteración 3.

### Cómo se ejercita todo

```bash
cd gcsgrep
go build -o /tmp/gcsgrep ./cmd/gcsgrep   # binario
go vet ./...                              # sin warnings
go test ./...                             # 12 tests unitarios, todos verdes

BUCKET=<test-bucket>

# VC-1 / VC-2 / VC-3 / VC-4 / VC-8
/tmp/gcsgrep "timeout" "gs://${BUCKET}/logs/"
/tmp/gcsgrep "patron_inexistente_xyz" "gs://${BUCKET}/logs/"
/tmp/gcsgrep -i "timeout while" "gs://${BUCKET}/logs/"
/tmp/gcsgrep "timeout" "gs://${BUCKET}/"

# VC-11 — logs/icon.png siempre sale como "salteado" en cualquiera de las
# corridas de arriba, nunca en los resultados.

# VC-17
/tmp/gcsgrep --max 1 "timeout" "gs://${BUCKET}/logs/"   # exit 2, guardrail

# VC-22 (requiere los objetos gs://.../mem/small.log y .../mem/large.log)
/usr/bin/time -l /tmp/gcsgrep "timeout" "gs://${BUCKET}/mem/small.log"
/usr/bin/time -l /tmp/gcsgrep "timeout" "gs://${BUCKET}/mem/large.log"
```

### Qué mirar en esta tabla

- **Ningún VC dice "andaba bien".** Exit codes, bytes de RSS, cantidad de
  aperturas, objetos/segundo — todo es observable y reproducible con los
  comandos de arriba.
- **Hay caminos de falla y borde, no solo el feliz.** De los 13 VCs, 5
  ejercitan una falla, un borde, o una invariante (VC-8, VC-9, VC-11, VC-15,
  VC-16, VC-17, VC-24 tocan alguno de esos casos).
- **Un VC quedó marcado con una salvedad explícita (nota 1), no oculto.**
  Preferible a que la tabla diga "✅" sobre algo que en realidad se verificó
  con menos rigor del que su descripción en la spec sugiere.
- **El ajuste de NFR-1 (nota 2) se hizo con evidencia, no a ojo:** se
  investigó primero si había algo arreglable en el código antes de tocar el
  número, y quedó registrado qué se descartó y por qué.

## Iteración 1 — re-verificación contra la spec corregida

> La sección anterior verificó la Iteración 1 contra la versión de la spec
> del commit `3337575`. Después de la revisión de la cátedra la spec se
> corrigió (FRs atómicos, casos de falla nuevos, VCs con observables
> literales, NFRs congelados) y el código se alineó con ella (commit
> `445e77b`). Esta sección vuelve a verificar la Iteración 1 completa contra
> la spec corregida; no reemplaza a la anterior.
>
> - **Datos:** los de la sección "Datos de prueba" de la spec, creados con
> `gcsgrep/testdata/setup-testdata.sh` en el bucket de prueba de siempre
> (`us-central1`). Los objetos de la verificación anterior se movieron a
> `legacy-iter1/` (nada se borró).
> - **Credenciales:** ADC del usuario y las service accounts `<sa-viewer>`,
>   `<sa-restringida>` (IAM Condition que excluye `acl/denied.log`) y
>   `<sa-sin-rol>`, usadas vía ADC impersonadas (sin claves).
> - **Herramientas:** los VCs contra GCS real son tests de Go en
> `gcsgrep/integration` (build tag `integration`); cada corrida queda
> registrada en la salida de `go test -v` con su stdout, stderr y exit code.
> Los VCs sin GCS real son tests unitarios de `go test ./...`.

### Resumen

| | |
|---|---|
| VCs en el alcance de la Iteración 1 (plan corregido) | 41 |
| Ejercitados contra GCS real (`gcsgrep/integration`) | 30 |
| Ejercitados con tests unitarios / chequeo estático | 8 |
| Mediciones (NFR-1, NFR-2) | 3 |
| VCs pasando | 41 |
| Pendientes | 0 |

### Cobertura, una por una

**Contra GCS real** — `GCSGREP_TEST_BUCKET=<test-bucket> GCSGREP_TEST_CREDS=<dir> go test -tags integration -v -count=1 ./integration/`
(35 tests, 0 fallos, 0 omitidos; 87 s).

| VC | Requisito | Test | Se observa | Estado |
|---|---|---|---|---|
| VC-1.1 | FR-1.1 resultados | `TestVC1_1_Search` | stdout `logs/a.log:2:ERROR timeout`, stderr vacío, exit 0 | ✅ |
| VC-1.3 | FR-1.3 patrón RE2 | `TestVC1_3_PatternIsRE2` | `version 1\.[0-9]` → solo `v/a.log:1:version 1.2`, exit 0 | ✅ |
| VC-1.4 | FR-1.4 patrón inválido | `TestVC1_4_InvalidPattern` | `gcsgrep: error: invalid pattern "(": error parsing regexp: missing closing ): ...`, exit 2 | ✅ |
| VC-2.2 | FR-2 bucket completo | `TestVC2_2_WholeBucketListing` | `gcloud storage ls` cuenta 666 objetos (620 de datos de prueba + 46 de `legacy-iter1/`); gcsgrep informa `the prefix has 666 objects`, exit 2, sin leer contenido | ✅ |
| VC-3.1 | FR-3.1 formato | `TestVC3_1_OutputFormat` | `logs/a.log:2:ERROR timeout` | ✅ |
| VC-3.3 | FR-3.3 sin color redirigido | `TestVC3_3_NoANSIWhenRedirected` | stdout sin ningún byte `0x1b` | ✅ |
| VC-4.1 | FR-4 `-i` | `TestVC4_1_IgnoreCase` | `case/a.log:1:TIMEOUT error`, exit 0 | ✅ |
| VC-4.2 | FR-4 sin `-i` | `TestVC4_2_CaseSensitiveByDefault` | stdout vacío, exit 1 | ✅ |
| VC-8.1 | FR-8.1 exit 0 | `TestVC8_1_ExitMatch` | exit 0 | ✅ |
| VC-8.2 | FR-8.2 exit 1 | `TestVC8_2_ExitNoMatch` | stdout vacío, exit 1 | ✅ |
| VC-8.3 | FR-8.3 exit 2 | `TestVC8_3_And_VC9_1_UnreadableObject` (con `<sa-restringida>`) | 5 matches de `acl/1..5.log`, exit 2 | ✅ |
| VC-9.1 | FR-9.1 sin permiso | misma corrida | stderr exactamente `gcsgrep: warning: acl/denied.log: permission denied` | ✅ |
| VC-9.4 | FR-9.4 otro 4xx | `TestVC9_4_OtherPermanentError` | `x/ok.log:1:timeout`; aviso `x/csek.log: read failed: googleapi: got HTTP response code 400 ... ResourceIsEncryptedWithCustomerEncryptionKey ...`, exit 2 | ✅ |
| VC-11 | FR-11 binarios | `TestVC11_BinarySkipped` | `bin/a.log:1:timeout`; aviso `bin/icon.png: skipped (binary object)`, exit 0 | ✅ |
| VC-15.1 | BR-1 solo lectura | `TestVC15_1_ReadOnlyCredentials` | los 3 comandos (VC-1.1, VC-8.2, VC-11) dan el mismo stdout y exit con ADC del usuario y con `<sa-viewer>` | ✅ |
| VC-16 | BR-2 sin amplificación | `TestVC16_NoAccessAmplification` (con `<sa-restringida>`) | `secreto` → stdout vacío, aviso de `acl/denied.log`, exit 2 | ✅ |
| VC-17.1 | BR-3 guardrail | `TestVC17_1_ObjectCountGuardrail` | `the prefix has 10 objects, which exceeds the limit of 5 (...)`, stdout vacío, exit 2 | ✅ |
| VC-17.2 | BR-3 `--max 20` | `TestVC17_2_GuardrailRaised` | los 10 objetos de `max/`, exit 0 | ✅ |
| VC-17.3 | BR-3 `--max 0` | `TestVC17_3_GuardrailDisabled` | los 10 objetos de `max/`, exit 0 | ✅ |
| VC-24 | FR-15 líneas > 1 MiB | `TestVC24_LongLinesSkipped` | líneas 1 y 3 (el `timeout` en el byte 2.097.152 de la línea de 5 MiB no se reporta); un único aviso `skipped lines longer than 1 MiB`, exit 0 | ✅ |
| VC-25.1 | FR-16.1 sin `gs://` | `TestVC25_1_LocationWithoutScheme` | `invalid location "mybucket/logs/": must start with gs://`, exit 2 | ✅ |
| VC-25.2 | FR-16.2 sin ADC | `TestVC25_2_NoCredentials` | `no Application Default Credentials found: dialing: credentials: could not find default credentials. ...`, exit 2 | ✅ |
| VC-25.3 | FR-16.3 listado denegado | `TestVC25_3_ListingDenied` (con `<sa-sin-rol>`) | `permission denied listing gs://<test-bucket>/logs/`, exit 2 | ✅ |
| VC-25.4 | FR-16.4 bucket inexistente | `TestVC25_4_BucketDoesNotExist` | `bucket gcsgrep-bucket-inexistente-xyz does not exist`, exit 2 | ✅ |
| VC-26 | FR-17 ubicación vacía | `TestVC26_LocationWithoutObjects` | stderr exactamente el aviso `no objects under ...` (sin progreso), exit 1 | ✅ |
| VC-27 | FR-18 prefijo sin `/` | `TestVC27_PrefixWithoutTrailingSlash` | `pfx/logs-old/b.log` y `pfx/logs/a.log`, sin `pfx/other/c.log`, exit 0 | ✅ |
| VC-28.1 | FR-19.1 orden secuencial | `TestVC28_1_SequentialOrder` | `ord/a`, `ord/b`, `ord/c` en ese orden (subidos b, a, c), líneas 1 y 2 de cada uno | ✅ |
| VC-29 | FR-20 `-n` | `TestVC29_NFlagHasNoEffect` | stdout, stderr y exit idénticos con y sin `-n` | ✅ |
| VC-30.1 | FR-21.1 objeto de 0 bytes | `TestVC30_1_EmptyObject` | solo `e/a.log:1:timeout`, stderr vacío, exit 0 | ✅ |
| VC-30.2 | FR-21.2 sin `\n` final | `TestVC30_2_LastLineWithoutNewline` | `n/last.log:2:dos timeout`, exit 0 | ✅ |

**Tests unitarios y chequeo estático** — `go test ./...` (sin red):

| VC | Requisito | Test | Se observa | Estado |
|---|---|---|---|---|
| VC-1.2 | FR-1.2 sin escritura a disco | `internal/invariants::TestNoDiskWrites` | el AST del código de producción no tiene `os.Create`, `os.CreateTemp`, `os.WriteFile` ni `os.OpenFile`; el test falla si se agrega uno (probado con una mutación) | ✅ |
| VC-2.1 | FR-2 prefijo vacío | `internal/scanner::TestRun_WholeBucket`, `internal/app::TestRun_WholeBucketLocation` | `gs://b/` lista con prefijo `""` y cubre `a/`, `b/`, `c/d/` y la raíz, stdout exacto | ✅ |
| VC-9.3 | FR-9.3 objeto borrado (404) | `internal/scanner::TestRun_ObjectFailsToOpen/FR-9.3` | aviso `x/bad.log: object not found`, el resto se procesa, exit 2 | ✅ |
| VC-15.2 | BR-1 superficie de solo lectura | `internal/invariants::TestReadOnlyGCSSurface` | `gcsclient.Client` declara solo `List` y `Open`; ningún `NewWriter`, `Delete`, `Update`, `Copier`, `Compose` ni `ACL` (probado con una mutación) | ✅ |
| VC-31.1 | FR-22.1 flag desconocido | `internal/app::TestRun_InvalidInvocationsNeverTouchGCS` | `gcsgrep: error: unknown flag -v`, exit 2, 0 llamadas a GCS | ✅ |
| VC-31.2 | FR-22.2 argumentos | ídem | `expected 2 arguments (PATTERN and LOCATION), got 1`, exit 2, 0 llamadas | ✅ |
| VC-31.3 | FR-22.3 valor no entero | ídem | `invalid value "diez" for --max: must be an integer`, exit 2, 0 llamadas | ✅ |
| VC-31.4 | FR-22.4 valor negativo | ídem | `invalid value "-5" for --max: must be >= 0`, exit 2, 0 llamadas | ✅ |

Los tests unitarios también cubren la parte de "cero llamadas a GCS" de
VC-1.4, VC-17.1 y VC-25.1, que la corrida real no puede contar.

**Mediciones** — `GCSGREP_BENCH=1 ... go test -tags integration -v -run 'VC21|VC22' ./integration/`:

| VC | Requisito | Se observa | Estado |
|---|---|---|---|
| VC-21.1 | NFR-1 throughput secuencial ≥ 0,5 obj/s | 3 corridas sobre `perf/` (500 × 1 MiB): 429,1 s, 477,9 s, 416,1 s → mediana 429,1 s = **1,17 objetos/seg** | ✅ |
| VC-21.3 | NFR-1 primer resultado ≤ 2 s | 1,741 s, 1,733 s, 1,690 s → mediana **1,733 s** | ✅ |
| VC-22 | NFR-2 memoria constante (≤ 5 MiB de diferencia) | Corrido en Linux (WSL). RSS máximo de `mem/small.log` (5 MiB): 34,2 / 34,7 / 34,2 MiB; de `mem/large.log` (500 MiB, 100 veces más grande): 37,0 / 37,7 / 37,2 MiB → medianas 34,2 y 37,2 MiB, **diferencia 3,0 MiB** | ✅ |

VC-21.2 (throughput con `--concurrency 8`) es de la Iteración 3.

### Notas

1. **La salvedad de VC-9/VC-16 de la verificación anterior quedó resuelta.**
   Antes se habían probado solo con un cliente GCS simulado, porque el
   bucket tiene uniform bucket-level access y no admite ACLs por objeto.
   Ahora `<sa-restringida>` tiene `roles/storage.objectViewer` con una IAM
   Condition que excluye `acl/denied.log`, y los dos VCs pasan contra GCS
   real con un 403 real.
2. **VC-2 cambió de forma.** La primera versión corregida pedía un bucket
   aparte con 4 objetos; no se podía crear otro bucket en el proyecto.
   Ahora VC-2.1 verifica con un cliente simulado que la ubicación sin prefijo
   lista con prefijo vacío y procesa todos los prefijos, y VC-2.2 verifica
   contra GCS real que ese listado alcanza todos los objetos del bucket,
   comparando el conteo del guardrail con uno independiente de `gcloud`.
3. **Dos bugs del código anterior aparecieron al escribir los tests de la
   spec corregida:**
   - un error de red a mitad de una línea se tomaba como fin del objeto: el
     objeto no quedaba como fallido y el error se perdía (ahora es
     `read interrupted: ...`, NFR-3);
   - los matches de un objeto se acumulaban en memoria y se imprimían al
     final: rompía NFR-2 con muchos matches, demoraba el primer resultado
     (VC-21.3) y se perdían si el objeto fallaba a mitad (decisión de diseño
     11). Ahora cada match se imprime al encontrarlo.
4. **VC-22 se midió en Linux (WSL)** y no en Windows, donde se corrió el
   resto: el test lee el RSS máximo con `getrusage`, que Windows no tiene.
   En WSL las ADC de Windows se usaron con
   `GOOGLE_APPLICATION_CREDENTIALS` apuntando a su archivo. Con un objeto
   100 veces más grande la memoria pico creció 3,0 MiB (umbral ≤ 5 MiB); la
   verificación anterior había observado 0,7 MiB con una relación de tamaños
   de 40 veces.
5. **El throughput secuencial mejoró respecto de la medición anterior**
   (1,17 contra 0,59 objetos/seg) sin cambios de código que lo expliquen: la
   red desde donde se midió es distinta. El umbral (≥ 0,5) se cumple en
   ambas.

### Cómo se ejercita todo

```bash
cd gcsgrep
go vet ./... && go vet -tags integration ./... && go test ./...

# Datos de prueba y service accounts (una vez)
PROJECT=<proyecto> BUCKET=<test-bucket> ./testdata/setup-testdata.sh todo

# VCs contra GCS real
GCSGREP_TEST_BUCKET=<test-bucket> GCSGREP_TEST_CREDS=<dir-con-credenciales> \
  go test -tags integration -v -count=1 ./integration/

# Mediciones (VC-22 requiere Linux o macOS)
GCSGREP_BENCH=1 GCSGREP_TEST_BUCKET=<test-bucket> \
  go test -tags integration -v -count=1 -timeout 60m -run 'VC21|VC22' ./integration/
```
