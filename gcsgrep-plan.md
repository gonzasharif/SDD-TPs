# gcsgrep — Plan de iteraciones

> Deriva de [`gcsgrep-spec.md`](./gcsgrep-spec.md). La spec define **qué**
> tiene que hacer `gcsgrep` en su versión completa (v1); este documento
> define **en qué orden** se construye y **qué queda deliberadamente afuera**
> de cada iteración, y por qué. El alcance diferido vive acá — la spec no
> tiene iteraciones, tiene un contrato final.

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
- **NFR-1**, parcial — solo el umbral en modo secuencial (≥ 3 objetos/seg).
  El umbral con concurrencia se valida en la Iteración 3, cuando existe la
  concurrencia.

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

**Criterio de salida (VCs que deben pasar):** VC-1, VC-2, VC-3 (solo la rama
sin color), VC-4, VC-8, VC-9, VC-11, VC-15, VC-16, VC-17, VC-22, VC-24, y la
mitad secuencial de VC-21 (throughput ≥ 3 objetos/seg).

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

**Criterio de salida (VCs que deben pasar):** todos los de la Iteración 1
siguen pasando, más VC-3 completo (con y sin color), VC-5, VC-6, VC-7,
VC-10, VC-12, VC-18, VC-19, VC-23.

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

**Criterio de salida (VCs que deben pasar):** todos los de las Iteraciones 1
y 2 siguen pasando (en particular, correr la suite completa con
`--concurrency 8` y no solo en modo secuencial, para confirmar que la
concurrencia no rompe ningún comportamiento ya validado), más VC-13, VC-14,
VC-20, VC-21 completo.

## Cómo se usa este plan

Cada iteración es un incremento entregable y verificable por sí mismo: al
final de la Iteración 1 ya hay una `gcsgrep` usable en el caso de uso
central, no un prototipo descartable. Las iteraciones 2 y 3 no reabren
decisiones de la spec — solo agregan comportamiento que la spec ya
contempla, en el orden que minimiza la superficie de riesgo por iteración
(primero correctitud y seguridad, después cobertura de formatos reales,
después rendimiento).
