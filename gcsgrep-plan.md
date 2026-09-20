# gcsgrep — Plan de iteraciones

> Deriva de [`gcsgrep-spec.md`](./gcsgrep-spec.md). La spec define **qué**
> tiene que hacer `gcsgrep` en su versión completa (v1); este documento
> define **en qué orden** se construye y **qué queda deliberadamente afuera**
> de cada iteración, y por qué. El alcance diferido vive acá — la spec no
> tiene iteraciones, tiene un contrato final.

## Dónde vive el código

La implementación está en el subdirectorio `gcsgrep/` de este repo, en Go
(módulo `gcsgrep`). Estructura: `cmd/gcsgrep/main.go` es el entry point;
`internal/{cli,scanner,reader,match,gcsclient,output}` son los módulos
descritos en el "Esquema de arquitectura" de `gcsgrep-requirements.md`.

```bash
cd gcsgrep
go build -o /tmp/gcsgrep ./cmd/gcsgrep   # binario
go vet ./...                              # sin warnings
go test ./...                             # tests unitarios
```

El entorno de prueba real (proyecto de GCP, bucket, credenciales ADC) ya
está armado — los nombres concretos no están versionados en el repo a
propósito. Están en `gcsgrep/testenv.local.md` (gitignoreado); si no lo
tenés, pedíselo a quien armó el entorno en vez de crear uno nuevo.

## Resumen

| Iteración | Foco | FRs | BRs | NFRs |
|---|---|---|---|---|
| 1 | Búsqueda mínima viable, segura por diseño | FR-1, FR-2, FR-3 (sin color), FR-4, FR-8, FR-9, FR-11, FR-15 | BR-1, BR-2, BR-3 | NFR-2, NFR-1 (parcial: umbral secuencial) |
| 2 | Costo en objetos comprimidos/grandes, UX, confiabilidad de red | FR-3 (color), FR-5, FR-6, FR-7, FR-10, FR-12 | BR-4, BR-5 | NFR-3 |
| 3 | Concurrencia y rendimiento | FR-13, FR-14 | BR-6 | NFR-1 (completo, con concurrencia) |

Las 15 FRs, 6 BRs y 3 NFRs de la spec quedan cubiertas entre las tres
iteraciones — ninguna queda sin asignar.

## Iteración 1 — Búsqueda mínima viable, segura por diseño

**Objetivo:** una `gcsgrep` que ya resuelve el caso de uso central —buscar un
patrón en objetos de texto bajo un prefijo, por streaming, sin bajar nada a
disco— y que es segura por diseño desde el primer commit: no amplía acceso,
no escribe nunca, no se cae por un objeto problemático, y no puede reventar
memoria ni escanear un bucket entero por accidente.

**Entra:**
- **FR-1, FR-2** — búsqueda sobre prefijo o bucket completo, literal/regex
  básica, streaming.
- **FR-3** (versión simplificada) — formato `objeto:línea:texto`, **siempre
  en texto plano** (el resaltado condicional a TTY se difiere a la Iteración
  2 — es una mejora de UX, no afecta la corrección del resultado).
- **FR-4** — `-i`. El flag `-n` se acepta como no-op silencioso, ya que el
  formato por defecto siempre incluye el número de línea.
- **FR-8** — exit codes estilo `grep` (0/1/2).
- **FR-9** — un objeto ilegible no aborta la corrida.
- **FR-11** — binarios se saltean (heurística de byte nulo). Se incluye ya en
  la Iteración 1, y no se difiere, porque sin esto un solo objeto binario en
  el prefijo puede volcar basura sobre la terminal del usuario — es barato de
  implementar y protege la experiencia básica.
- **FR-15** — líneas que exceden el buffer se saltean (no se truncan para
  matchear). Entra en la Iteración 1 porque es lo que sostiene la garantía de
  memoria constante de NFR-2 — sin esto, el diseño de streaming no cumple lo
  que promete desde el día uno.
- **BR-1, BR-2** — solo lectura y sin amplificación de acceso. No son
  features que se "agreguen" después: son propiedades de cómo está
  construida la herramienta desde el primer commit (uso de ADC sin ninguna
  llamada de escritura en el código).
- **BR-3** — guardrail de cantidad de objetos (1000 por defecto, `--max`).
  Entra en la Iteración 1 y no se difiere porque la consigna lo marca como
  restricción dura de entrega ("no escanees un bucket enorme sin guardrail de
  costo"), no como una mejora incremental.
- **NFR-2** — memoria constante (streaming + FR-15).
- **NFR-1**, parcial — solo el umbral en modo secuencial (≥ 0.5 objetos/seg,
  válido incluso en redes de latencia alta). El umbral con concurrencia se
  valida en la Iteración 3, cuando existe la concurrencia.

**Explícitamente afuera (y por qué):**
- Color en la salida (FR-3 completo) — cosmético, no bloquea el caso de uso.
- `-l`, `-c` (FR-5, FR-6, FR-7) — el flujo de streaming ya funciona; agregar
  dos modos de salida distintos y su validación de mutua exclusión es trabajo
  incremental, no fundacional.
- Indicador de progreso (FR-10) — relevante recién con prefijos grandes; con
  el guardrail de 1000 objetos de BR-3, una corrida de Iteración 1 nunca es
  tan larga como para "parecer colgada".
- `.gz` (FR-12) y sus guardrails de tamaño (BR-4, BR-5) — la descompresión
  agrega una fuente de complejidad (el guardrail de bytes descomprimidos no
  tiene sentido sin descompresión primero).
- Reintentos de red (NFR-3) — sin reintentos, un blip transitorio marca un
  objeto como fallido (vía FR-9), lo cual es correcto pero pesimista. Aceptable
  para una primera versión funcional.
- Concurrencia (FR-13, FR-14, BR-6) — el modo secuencial ya es correcto y
  verificable; la concurrencia es una optimización de rendimiento, no una
  corrección funcional.

**Criterios de éxito**

- [ ] VC-1 pasa — búsqueda básica sobre un prefijo
- [ ] VC-2 pasa — búsqueda sobre bucket completo
- [ ] VC-3 pasa (solo la rama sin color) — formato `objeto:línea:texto` en
      texto plano
- [ ] VC-4 pasa — `-i` case-insensitive
- [ ] VC-8 pasa — exit codes 0/1/2 en los tres escenarios controlados
- [ ] VC-9 pasa — un objeto ilegible no aborta la corrida
- [ ] VC-11 pasa — binarios se saltean sin volcar basura a la terminal
- [ ] VC-15 pasa — la suite funciona igual con credenciales de solo lectura
- [ ] VC-16 pasa — no se expone contenido fuera del acceso del usuario
- [ ] VC-17 pasa — guardrail de cantidad de objetos (1000 por defecto)
- [ ] VC-22 pasa — memoria constante entre un objeto chico y uno grande
- [ ] VC-24 pasa — una línea que excede el buffer se saltea, no se trunca
- [ ] VC-21 pasa (solo la mitad secuencial) — throughput ≥ 0.5 objetos/seg

**Demostrable así:**

```bash
# Sobre un bucket de prueba gs://gcsgrep-test/logs/ con algunos objetos
# de texto conocidos y un objeto binario mezclado.

gcsgrep "timeout" gs://gcsgrep-test/logs/
# imprime objeto:línea:texto por cada match, exit 0

gcsgrep "patron_inexistente_xyz" gs://gcsgrep-test/logs/
# sin resultados, exit 1

gcsgrep -i "TIMEOUT" gs://gcsgrep-test/logs/
# matchea sin distinguir mayúsculas, exit 0

gcsgrep "timeout" gs://gcsgrep-test/
# sin prefijo, cubre todo el bucket

gcsgrep "x" gs://bucket-sin-permiso/
# objeto/bucket no accesible con las credenciales del usuario, exit 2
```

## Iteración 2 — Costo en objetos grandes/comprimidos, UX y confiabilidad de red

**Objetivo:** cubrir el caso de uso real de logs (que suelen estar en `.gz`),
cerrar los guardrails de costo que solo importan una vez que hay
descompresión de por medio, sumar los modos de salida que reducen el volumen
de resultados (`-l`, `-c`), mejorar la experiencia interactiva (color,
progreso), y dejar de ser pesimista ante blips de red transitorios.

**Entra:**
- **FR-12** — descompresión de `.gz` al vuelo.
- **BR-4, BR-5** — guardrails de tamaño descomprimido, por objeto y
  acumulado. Entran junto con FR-12 porque no tienen sentido antes: sin
  descompresión, el tamaño leído de un objeto ya es su tamaño real (no hay
  "expansión" que necesite guardrail aparte del listado de BR-3).
- **FR-5, FR-6, FR-7** — `-l`, `-c`, y su mutua exclusión.
- **FR-3** (completar) — color condicional a TTY.
- **FR-10** — indicador de progreso (barra con porcentaje si hay TTY en
  `stderr`, líneas periódicas si no). Entra acá porque recién con `.gz` y
  guardrails más permisivos las corridas empiezan a ser lo bastante largas
  como para que el usuario necesite la señal de que sigue viva.
- **NFR-3** — reintentos con backoff exponencial ante fallos de red
  transitorios.

**Explícitamente afuera (y por qué):**
- Concurrencia (FR-13, FR-14, BR-6) — todavía no es necesaria para que la
  herramienta sea correcta; se dejan para la iteración de rendimiento, así
  la introducción de paralelismo no se mezcla con la introducción de `.gz` y
  sus guardrails (dos fuentes de bugs a la vez es peor que una por vez).

**Criterios de éxito**

- [ ] Todos los VCs de la Iteración 1 siguen pasando
- [ ] VC-3 pasa completo (rama con color TTY, además de la rama plana ya
      cubierta en la Iteración 1)
- [ ] VC-5 pasa — `-l` corta la lectura en el primer match
- [ ] VC-6 pasa — `-c` cuenta matches leyendo el objeto completo
- [ ] VC-7 pasa — `-l` y `-c` juntas se rechazan con exit 2
- [ ] VC-10 pasa — progreso con barra (TTY) y con líneas simples (no TTY)
- [ ] VC-12 pasa — `.gz` se descomprime y se busca dentro del contenido
- [ ] VC-18 pasa — guardrail de tamaño por objeto (250 MiB descomprimidos)
- [ ] VC-19 pasa — guardrail acumulado de la corrida (2 GiB descomprimidos)
- [ ] VC-23 pasa — reintentos ante fallos de red transitorios

**Nota de regresión:** VC-3 se verificó en la Iteración 1 solo en su rama
sin color (porque el color todavía no existía); acá hay que volver a
correrlo agregando la rama con TTY, no alcanza con no romper la rama plana
que ya pasaba. Además, VC-8 (exit codes) hay que re-ejercitarlo con las
nuevas fuentes de error que aparecen en esta iteración —un guardrail de
tamaño alcanzado (BR-4/BR-5) también tiene que producir exit 2, igual que
un objeto sin permisos ya lo hacía en la Iteración 1.

## Iteración 3 — Concurrencia y validación de rendimiento

**Objetivo:** acelerar la corrida sobre prefijos con muchos objetos mediante
lectura paralela, con un tope explícito para no convertir el flag de
concurrencia en una forma de saltear los guardrails de costo, y validar
formalmente los umbrales de NFR-1 con un benchmark real.

**Entra:**
- **FR-13, FR-14** — `--concurrency N` / `-j N`, con tope de 32.
- **BR-6** — fundamento y rechazo explícito de valores fuera de rango.
- **NFR-1**, completo — correr el benchmark con `--concurrency 8` y comparar
  contra el umbral (≥ 15 objetos/seg) y la latencia al primer resultado.
- Revisión de thread-safety de los contadores compartidos entre workers:
  bytes acumulados de BR-5 y contador de progreso de FR-10, que hasta esta
  iteración corrían en un solo hilo y ahora necesitan sincronización
  (atomic/lock) para no producir una condición de carrera que deje pasar más
  bytes de los que BR-5 permite.

**Explícitamente afuera:** nada — con esta iteración se cierra la spec
completa (15 FRs, 6 BRs, 3 NFRs).

**Criterios de éxito**

- [ ] Todos los VCs de las Iteraciones 1 y 2 siguen pasando, corridos además
      con `--concurrency 8` (no solo en modo secuencial)
- [ ] VC-13 pasa — reducción de tiempo total con concurrencia, mismo
      conjunto de resultados que en modo secuencial
- [ ] VC-14 pasa — `--concurrency` fuera de rango se rechaza
- [ ] VC-20 pasa — idéntico a VC-14
- [ ] VC-21 pasa completo — throughput con concurrencia y latencia al
      primer resultado, además del umbral secuencial ya validado

**Nota de regresión:** esta es la iteración de mayor riesgo de regresión
silenciosa, porque introduce concurrencia sobre comportamiento que hasta acá
solo corrió en un único hilo. En particular, BR-5 (guardrail acumulado) y
FR-10 (progreso) dependen de contadores compartidos — si no se sincronizan
correctamente, VC-19 y VC-10 podrían seguir "pasando" en una corrida
puntual y fallar de forma intermitente bajo carga. No alcanza con correr la
suite una vez con `--concurrency 8`: conviene correrla varias veces seguidas
para exponer condiciones de carrera que no aparecen siempre.

## Lo que quedó afuera del plan entero

Esto no es "todavía no lo hicimos": es alcance rechazado en la spec (sección
Alcance → Fuera), y vive acá también para que no vuelva a aparecer en cada
conversación de implementación.

| Idea | Decisión |
|---|---|
| Regex completa tipo PCRE, o elegible por flag | Descartado en v1 |
| Autenticación por archivo de service account key | Descartado en v1 |
| Sintaxis de ubicación sin esquema (`bucket/prefijo`) | Descartado en v1 |
| Flags `-v`, `-r`, `--include` | Descartado en v1 |
| Modo de salida JSON | Descartado en v1 |
| Cualquier escritura/copia/borrado sobre GCS | Descartado — viola BR-1 |
| Interfaz web, API HTTP, librería importable | Descartado en v1 |
| Soporte para S3, Azure Blob u otro proveedor | Descartado en v1 |
| Detección de generación/versión de objeto ante escritura concurrente | Riesgo conocido, aceptado (ver decisión de FR original sobre objetos que cambian mid-lectura) |

## Cómo se usa este plan

Cada iteración es un incremento entregable y verificable por sí mismo: al
final de la Iteración 1 ya hay una `gcsgrep` usable en el caso de uso
central, no un prototipo descartable. Las iteraciones 2 y 3 no reabren
decisiones de la spec — solo agregan comportamiento que la spec ya
contempla, en el orden que minimiza la superficie de riesgo por iteración
(primero correctitud y seguridad, después cobertura de formatos reales,
después rendimiento).

## Qué sigue

Tras implementar y verificar cada iteración, la evidencia va en
**`gcsgrep-cobertura-vc.md`** — un único documento acumulativo para todo el
proyecto, no uno nuevo por iteración. Cada iteración agrega su propia
sección (ej. "Cobertura de VCs — Iteración 2") con, para cada VC de su
lista de criterios de éxito, con qué se lo ejercitó y qué se observó, sin
borrar ni reescribir las secciones de iteraciones anteriores: el documento
completo es el historial de qué se verificó y cuándo, no solo el estado
actual.