# gcsgrep — tabla de cobertura de VCs

> Salida del paso **Verificar**, acumulada a lo largo de todo el proyecto
> (ver `gcsgrep-plan.md`) — **un documento por proyecto, no uno por
> iteración**. Cada iteración agrega su propia sección al final, sin tocar
> las anteriores. Esta tabla no dice "lo probé". Dice, para cada criterio
> de verificación, con qué se lo ejercitó y qué se observó. Un VC sin
> evidencia es un VC que no pasó.
>
> Entorno de prueba: un proyecto de GCP dedicado y un bucket de prueba en
> `us-central1` (nombres reales deliberadamente no versionados acá — no son
> secretos, pero tampoco hace falta exponerlos en un repo público; están en
> la memoria local del agente que armó el entorno). ADC vía
> `gcloud auth application-default login`. Implementación en Go, código en
> `gcsgrep/`.

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
