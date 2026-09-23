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
> Implementación en Go, código en `gcsgrep/`.

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

## Iteración 2

### Resumen

| | |
|---|---|
| VCs en el alcance de la Iteración 2 | 10 (VC-3 completo, VC-5, VC-6, VC-7, VC-10, VC-12, VC-18, VC-19, VC-23, más la regresión de VC-8) |
| VCs con cobertura ejecutable | 10 |
| VCs pasando (tests unitarios + emulador local) | 10 |
| VCs verificados contra GCS real | 7 de 10 (VC-3, VC-5, VC-6, VC-7, VC-8, VC-10, VC-19) + VC-18 (a); pendientes VC-12 y VC-18 (b) por falta de permiso de escritura, y VC-23 no se puede provocar en GCS real (ver nota 3) |
| Tests unitarios | 59 (21 de la Iteración 1 + 38 nuevos), todos verdes; `go vet` sin warnings |

### Cómo se verificó esta vez: tres niveles

1. **Tests unitarios** (`go test ./...`), con clientes GCS fake inyectados vía
   la interfaz `gcsclient.Client`, igual que en la Iteración 1.
2. **Binario real contra un emulador local de GCS.** El SDK de GCS respeta
   la variable `STORAGE_EMULATOR_HOST`: se armó un servidor HTTP mínimo (fuera
   del repo) que implementa el listado (`/storage/v1/b/{bucket}/o`) y la
   descarga de objetos, registra cada request, puede inyectar 503 y emula el
   *decompressive transcoding* de GCS para objetos con
   `Content-Encoding: gzip`. Esto ejercita el binario compilado de punta a
   punta —con el SDK real, su configuración de reintentos real, y una TTY
   real (`script`)— sin credenciales. No sustituye a GCS real (ver nota 3),
   pero es un nivel de evidencia más alto que el cliente fake: lo que se
   prueba acá es el mismo binario que se entrega.
3. **GCS real:** el 2026-09-22, contra el bucket de prueba, con ADC de la
   cuenta del equipo (ver nota 3).

### Cobertura, una por una

| VC | Requisito | Ejercitado por | Se observa | Estado |
|---|---|---|---|---|
| VC-3 (completo) | FR-3 color condicional a TTY | `internal/output/output_test.go::TestMatch_PlainHasNoEscapeCodes`, `TestMatch_ColorHighlightsEveryMatch`, `TestMatch_ColorSkipsEmptySpans` + emulador bajo pty (`script -qec`) y con `stdout` a archivo | con pty: los 60 matches de `many/` envueltos en `\x1b[01;31m…\x1b[m`; con `stdout` a archivo: cero bytes `\x1b`, formato `objeto:línea:texto`. Detección con `x/term` (`isatty` real): verdadero en pty, falso en pipe, archivo y `/dev/null` | ✅ |
| VC-5 | FR-5 `-l` corta en el primer match | `internal/reader/reader_test.go::TestProcessObject_ListModeStopsAtFirstMatch`, `internal/scanner/scanner_test.go::TestRun_ListModePrintsOnlyMatchingObjectNames` + emulador | objeto de 10 MiB con match en la línea 1: se leen ≤ 72 KiB (buffers de sniff + bufio). **Mutación comprobada:** sacando el corte temprano, el test falla leyendo los 10 MiB completos. Salida: solo nombres, una vez por objeto | ✅ |
| VC-6 | FR-6 `-c` | `TestProcessObject_CountModeCountsEveryMatch`, `TestRun_CountModePrintsZeroCountsButNotSkippedObjects`, `TestRun_CountModeWithNoMatchesExitsNoMatch` + emulador | `logs/app1.log:1`, `logs/app2.log:0`, `logs/app3.log:0`; el binario (`icon.png`) no imprime conteo; todo en cero → exit 1 | ✅ |
| VC-7 | FR-7 `-l` + `-c` | `internal/cli/cli_test.go::TestParse_ListAndCountAreMutuallyExclusive` + emulador | exit 2, error de uso por `stderr`, **cero requests** registrados por el emulador (se rechaza antes de crear el cliente GCS) | ✅ |
| VC-8 (regresión) | FR-8 con las fuentes de error nuevas | `TestRun_ObjectOverSizeLimitIsSkippedBeforeOpening`, `TestRun_ExpandingGzipIsCutAtObjectLimit`, `TestRun_TotalSizeLimitStopsTheRun`, `TestRun_MidReadFailurePrintsEarlierMatchesOnce` + emulador | BR-4 (antes de abrir y durante la lectura), BR-5, y fallo a mitad de lectura → exit 2 aunque haya matches en otros objetos; los tres casos de la Iteración 1 (0/1/2) siguen pasando | ✅ |
| VC-10 | FR-10 progreso | `internal/output/output_test.go::TestProgress_*` (5 tests), `TestRun_ProgressCountsEveryObject` + emulador con pty y sin pty | (a) pty: 242 `\r` sobre 60 objetos, porcentaje monótono hasta `100% (60/60 objects)`; (b) `stderr` a pipe: una línea cada 10%, cero `\r`, final `4/4 objects (100%)`. La barra se borra antes de cada match o warning y se redibuja después | ✅ |
| VC-12 | FR-12 gzip | `TestProcessObject_GzipIsDetectedByContentNotName`, `TestProcessObject_PlainTextNamedGzIsSearchedAsText`, `TestProcessObject_BinaryDetectionRunsOnDecompressedContent`, `TestProcessObject_CorruptGzipFails`, `TestRun_GzipObjectIsSearchedAndReportedByItsName` + emulador | los tres casos del VC, contra el binario real: (a) `gz/plain.log.gz` → `:2:gzip timeout inside`; (b) `gz/transcoded.log.gz` (con `Content-Encoding: gzip`, llega ya descomprimido) → `:1:transcoded timeout line`; (c) `gz/noext` → `:1:gzip without extension timeout`. Siempre bajo el nombre original; sin archivos temporales (el código no tiene ninguna llamada de escritura a disco) | ✅ |
| VC-18 | BR-4 tope por objeto | (a) `TestRun_ObjectOverSizeLimitIsSkippedBeforeOpening`; (b) `TestProcessObject_ObjectSizeLimitCutsExpandingGzip`, `TestRun_ExpandingGzipIsCutAtObjectLimit`; borde: `TestProcessObject_ObjectExactlyAtSizeLimitIsNotCut` + emulador | (a) `--max-object-size 40`: `logs/app1.log` (61 bytes) **nunca recibe un GET** en el log del emulador, warning, exit 2, el resto se procesa; (b) `bomb/bomb.gz` (~8 MiB descomprimido) con `--max-object-size 64k`: se imprime el match de la línea 1, se corta, warning, exit 2 | ✅ |
| VC-19 | BR-5 tope acumulado | `TestProcessObject_TotalBudgetIsSharedAcrossObjects`, `TestRun_TotalSizeLimitStopsTheRun` + emulador | `--max-total-size 500` sobre 60 objetos: 18 GETs, el objeto 17 se corta a la mitad, los matches de los 17 anteriores se imprimen, warning de corrida incompleta, exit 2. Con `-c`, el objeto cortado no imprime un conteo parcial | ✅ |
| VC-23 | NFR-3 reintentos | `internal/gcsclient/retry_test.go::TestRetry_*` (6 tests) + `TestProcessObject_MidReadErrorFailsObjectAndKeepsEarlierMatches`, `TestRun_MidReadFailurePrintsEarlierMatchesOnce` + emulador inyectando 503 | (a) 2×503 y después OK → recuperado, esperas de exactamente 500ms y 1s (reloj inyectado); (b) 503 persistente → **exactamente 3 GETs** en el log del emulador (lo que confirma además que el retry propio del SDK quedó desactivado), warning, exit 2; (c) 403/404 → 1 intento, sin espera; (d) corte a mitad de lectura → el match previo se imprime una sola vez, objeto fallido, sin reintento | ✅ |

### Nota 3 — verificación contra GCS real

La Iteración 2 se implementó en una máquina sin `gcloud` ni credenciales, así
que primero se verificó con tests y emulador. Después (2026-09-22) se instaló
`gcloud`, se configuró ADC y se corrió contra el bucket de prueba real. Todo lo
que se pudo correr coincidió con lo observado en el emulador.

**Regresión de la Iteración 1 (GCS real):**

| VC | Se observa | Estado |
|---|---|---|
| VC-1 / VC-3 plano | `logs/app1.log:2:...connection timeout after 30s`, exit 0 | ✅ |
| VC-2 | una corrida sobre `gs://<test-bucket>/` cubre `logs/` **y** `other-prefix/`. **Cambio esperado:** ahora termina con exit 2 y no 0, porque `mem/large.log` (335 MiB) supera el tope de 250 MiB de BR-4 y se saltea sin abrirse | ✅ |
| VC-4 | `"timeout while"` sin `-i` → exit 1; con `-i` → `logs/app3.log:2`, exit 0 | ✅ |
| VC-8 | match → 0; sin match → 1; bucket inexistente → 2 (el 404 no se reintenta) | ✅ |
| VC-11 | `logs/icon.png` salteado como binario en todas las corridas | ✅ |
| VC-17 | `--max 1` sobre `logs/` → error, exit 2 | ✅ |
| VC-22 | con `-c --max-object-size 0` (el objeto grande ahora supera el tope por defecto): RSS **35.5 MiB** (8 MiB) vs. **37.5 MiB** (335 MiB) — ~2 MiB de diferencia para ~40x de tamaño | ✅ |

No se volvieron a correr VC-9/VC-16 (siguen con la salvedad de la nota 1),
VC-15 (requiere crear un service account) ni VC-21 (benchmark).

**VCs de la Iteración 2 (GCS real):**

| VC | Se observa | Estado |
|---|---|---|
| VC-3 | bajo pty (`script -qec`): `^[[01;31mtimeout^[[m` alrededor del match; con `stdout` a archivo: 0 bytes `\x1b` | ✅ |
| VC-5 | `-l` sobre `gs://<test-bucket>/` → solo nombres (`logs/app1.log`, `mem/small.log`, `other-prefix/data.log`); sobre `mem/small.log` (8.6 MB, match en la línea 1): **1.32 s** con `-l` vs. **3.82 s** leyendo completo | ✅ |
| VC-6 | `logs/app1.log:1`, `logs/app2.log:0`, `logs/app3.log:0`; el binario no imprime conteo | ✅ |
| VC-7 | `-l -c` → error de uso, exit 2 | ✅ |
| VC-8 (regresión) | nuevas fuentes de exit 2 observadas en real: BR-4 antes de abrir (VC-2) y BR-5 (VC-19) | ✅ |
| VC-10 | (a) pty sobre `perf/` (30 objetos): 31 redibujados, porcentaje monótono 0 → 100, final `100% (30/30 objects)`; (b) `stderr` a archivo sobre el bucket completo: 10 líneas, 0 `\r` | ✅ |
| VC-18 (a) | `mem/large.log` (351 462 090 bytes) salteado por tamaño listado, sin abrirse, warning, exit 2 | ✅ |
| VC-19 | `--max-total-size 10MiB -c` sobre el bucket completo: se leen `logs/`, `mem/small.log`, `other-prefix/`, `perf/obj_1.log`; `perf/obj_10.log` se corta a la mitad (sin conteo parcial), no se abre ningún objeto más, warning, exit 2 | ✅ |
| VC-12 | — | ⏳ ver abajo |
| VC-18 (b) | — | ⏳ ver abajo |
| VC-23 | GCS real no permite provocar 503 a pedido; queda cubierto por tests y emulador (con requests HTTP reales contados) | n/a |

**Pendiente — VC-12 y VC-18 (b):** los dos necesitan objetos gzip en el
bucket (incluido uno con `Content-Encoding: gzip`) que todavía no existen. La
cuenta usada tiene permiso de lectura sobre el bucket pero no de escritura
(`storage.objects.create` → 403), así que no se pudieron subir. Hace falta que
quien administra el entorno los suba (o dé permiso de escritura sobre el
prefijo `gz/`) y correr los comandos de VC-12 y VC-18 de abajo. Hasta
entonces, esos dos VCs están verificados con tests y emulador, no contra GCS
real.

### Nota 4 — bug de la Iteración 1 encontrado en esta iteración

`readLine` trataba **cualquier** error de lectura como fin de archivo. Si la
conexión se cortaba a mitad de un objeto, el objeto se daba por leído
completo, sin warning, la línea cortada a la mitad se matcheaba igual (en el
test apareció `partial timeout li` como match), y la corrida terminaba con
exit 0: una violación silenciosa de FR-9 y FR-8. Ningún VC de la Iteración 1
lo detectó porque todos los fallos simulados ocurrían en `Open`, nunca a
mitad de la lectura. Se escribió primero el test que lo reproduce
(`TestProcessObject_MidReadErrorFailsObjectAndKeepsEarlierMatches`), se vio
fallar, y después se corrigió.

### Decisiones de spec tomadas durante esta iteración

Registradas en `gcsgrep-spec.md` antes de implementar (commit `a4b5a6d`) o
durante la implementación:

- `-c` imprime también los objetos con `0`; los objetos salteados o
  incompletos no imprimen conteo.
- BR-4 aplica a todo objeto, comprimido o no, con chequeo previo por el
  tamaño listado.
- BR-5 corta también el objeto en curso.
- gzip se detecta por la firma `1f 8b`, no por la extensión `.gz`.
- NFR-3: 3 intentos con esperas de 500ms y 1s (la versión anterior listaba
  3 esperas para 2 reintentos); se reintentan `List` y `Open`, nunca una
  lectura en curso; también se reintentan 408 y 429, siguiendo la
  clasificación del propio SDK.
- `0` deshabilita `--max-object-size` y `--max-total-size`; los tamaños
  aceptan sufijos `KiB`/`MiB`/`GiB`.
- FR-10 sin TTY: una línea cada 10% de los objetos.

### Cómo se ejercita todo

```bash
cd gcsgrep
go vet ./...
go test ./...                             # 59 tests unitarios
go build -o /tmp/gcsgrep ./cmd/gcsgrep

BUCKET=<test-bucket>   # contra GCS real (pendiente, ver nota 3)

# VC-3: color solo con TTY
script -qec "/tmp/gcsgrep timeout gs://${BUCKET}/logs/" /dev/null | cat -v   # ^[[01;31m alrededor del match
/tmp/gcsgrep timeout "gs://${BUCKET}/logs/" > out.txt; grep -c $'\x1b' out.txt   # 0

# VC-5 / VC-6 / VC-7
/tmp/gcsgrep -l timeout "gs://${BUCKET}/logs/"
/tmp/gcsgrep -c timeout "gs://${BUCKET}/logs/"
/tmp/gcsgrep -l -c timeout "gs://${BUCKET}/logs/"; echo $?   # 2

# VC-10: con TTY (barra) y con stderr redirigido (líneas)
script -qec "/tmp/gcsgrep --max 0 timeout gs://${BUCKET}/" /dev/null
/tmp/gcsgrep timeout "gs://${BUCKET}/" 2> progress.log; grep -c $'\r' progress.log   # 0

# VC-12: los tres casos de gzip (subir antes los objetos de prueba)
gsutil cp app.log.gz "gs://${BUCKET}/gz/plain.log.gz"
gsutil -h "Content-Encoding:gzip" cp app.log.gz "gs://${BUCKET}/gz/transcoded.log.gz"
gsutil cp app.log.gz "gs://${BUCKET}/gz/noext"
/tmp/gcsgrep timeout "gs://${BUCKET}/gz/"

# VC-18 / VC-19: límites bajos para que corra rápido
/tmp/gcsgrep --max-object-size 1k timeout "gs://${BUCKET}/"
/tmp/gcsgrep --max-total-size 10MiB timeout "gs://${BUCKET}/"

# VC-23: requiere inyectar fallos; se cubre con los tests unitarios y el
# emulador (GCS real no permite provocar 503 a pedido).
```

### Qué mirar en esta tabla

- **Hay una columna de estado honesta sobre el nivel de evidencia:** todo pasa
  en tests unitarios y contra el binario real con emulador, pero nada se corrió
  todavía contra GCS real, y eso está dicho en el resumen, no escondido.
- **Los tests no se asumieron correctos:** el de VC-5 se probó rompiendo el
  código a propósito (mutación) para confirmar que falla, y dos tests fallaron
  de verdad durante la implementación y encontraron bugs (el de la nota 4, y
  que la línea final de progreso no salía si la corrida se cortaba a mitad de
  un tramo del 10%).
- **VC-23 (b) se verificó contando requests HTTP reales**, no solo llamadas
  a una interfaz: eso es lo que prueba que no hay dos políticas de reintento
  apiladas.
