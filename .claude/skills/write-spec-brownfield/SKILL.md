---
name: write-spec-brownfield
description: Usar cuando la persona quiere especificar un cambio sobre un codebase que ya existe — "quiero agregar X a <proyecto>", "especificá este cambio", "armá la spec para modificar…", "qué hay que tocar para soportar Y". Produce una spec con alcance dentro/fuera por path, invariantes con VC propio, línea de base de regresión, FRs atómicos con VCs literales, decisiones con fundamento en código y plan de iteraciones. No usar para un proyecto desde cero.
---

# write-spec-brownfield

Convierte un pedido de cambio sobre código existente en una spec desde la que otro
equipo puede construir **sin romper lo que ya anda**.

**La regla que este skill hace imposible violar:** no hay spec brownfield sin alcance
*fuera* nombrado por path, sin un VC numerado por cada invariante y sin decisiones con
fundamento en código del sistema. Lo que no está acotado, se toca; lo que no se observa,
se rompe en silencio; lo que no se funda, es opinión.

## Cuándo NO usarlo

- Proyecto desde cero: no hay nada que preservar, no hay invariantes ni línea de base.
- Bugfix de una línea con test que lo cubre: no amerita spec.

## Pasos

1. **Descubrí antes de especificar.** Si no hay notas de exploración, lanzá un subagent
   `Explore` en modo solo lectura: punto de enganche, módulos vecinos, capa de
   portabilidad, cómo se testea hoy. Declará el **commit** explorado y citá **archivo y
   símbolo** (la línea sola se corre; el símbolo se verifica). Verificá cada símbolo contra
   ese checkout antes de citarlo.
2. **Escribí el propósito** en un párrafo: qué cambia y qué límite lo encierra.
3. **Glosario.** Un término por concepto (destino, host, falla, error de uso…) y úsalo sin
   variantes en toda la spec.
4. **Alcance dentro y fuera, por path.** Una fila por archivo; en "fuera", también las
   features vecinas que alguien podría suponer incluidas.
5. **Invariantes.** Qué sigue siendo verdad después del cambio. **Cada INV tiene al menos un
   `VC-INVn.k` definido en la spec**; una columna "cómo se comprueba" no cuenta. Mínimo:
   un build sin el feature compila, el comando nuevo no está en el binario, y un comando
   existente se comporta igual que antes.
6. **Línea de base de regresión.** Qué se corre *antes* de tocar código, con el número que
   da, medido (no leído).
7. **FR/BR/NFR atómicos** (ver reglas) y, debajo de cada uno, sus VCs `VC-<n>.<k>`.
8. **Decisiones** entre las reglas y el plan: cada una con *elegido / fundamento (código
   real) / descartado con motivo / FR donde queda escrita*.
9. **Plan de iteraciones.** Cada FR, BR, NFR e INV cierra en **exactamente una**
   iteración; la primera es la más angosta que se verifica sola; el harness llega en la
   iteración que lo necesita.
10. **Chequeo mecánico:** `scripts/check-vc-coverage.sh --strict <spec.md>` desde la raíz
    del skill. Corregí hasta que salga 0 y sin `WARN`.
11. **Revisión independiente** con el subagent `spec-reviewer`, pasándole el path de la spec
    y la salida del paso 10. No escribas el veredicto vos: quien escribe la spec no la
    aprueba.

## Reglas (salen de las tres correcciones de la cátedra)

**Atomicidad.** Un FR tiene un solo *Cuando* y un solo resultado observable en una sola
ejecución. Si el *Entonces* lleva "y", "o" o "si… en cambio…", partilo. Un *Dado* con
alternativas ("no existe o es ilegible") también cuenta como no atómico: un FR por caso.
Mensaje y exit code son dos resultados: el exit code va en una BR que lo fija para todas las
fallas.

**Caminos de falla: siempre los cuatro**, cada uno con su FR y su VC, y listados en lo que
define "falla": (1) el caso vacío o sin coincidencia del feature; (2) un recurso ilegible y
qué pasa con el resto; (3) sin credenciales o sin permiso; (4) entrada inválida o vacía.
Además, la precedencia cuando hay varios errores de uso a la vez.

**VC concreto.** Cada VC lleva datos de entrada, el comando, el stdout literal, el stderr
literal, el exit code y números con unidad (bytes, ms, RSS). Nada de "se comporta
correctamente" ni "funciona igual que antes". Un VC solo cubre un requisito si lo que manda
observar lo ejercita de verdad (`pane_pid != 0` también lo cumple un pane común). Evitá VCs
atados al lenguaje o a nombres de símbolos propios.

**IDs alineados.** `VC-<n>.<k>` observa a `FR-n`; `VC-BRn.k`, `VC-NFRn.k`, `VC-INVn.k`.
`#VC ≥ #FR + #BR + #NFR + #INV`.

**Decisiones.** Elegido, fundamento, descartado con motivo. En brownfield el fundamento cita
código del sistema (archivo y símbolo) que sostiene la decisión: "cubre el uso real" o "lo
pide la consigna" no alcanza. Si no hay código que la funde (un timeout, un umbral), decilo y
dá el criterio. Decidí: autenticación, flags, formatos, exit codes y valores de guardarraíles.

**Sin ambigüedad.** Nombrá la sintaxis exacta (sabor de regex), la forma del nombre en la
salida, el orden de salida, el texto literal de cada mensaje y qué cuenta como "ilegible" o
"corrupto". "A intervalos regulares" no es un valor. Un flag nombrado, aunque sea entre
paréntesis, tiene FR y VC; si se acepta sin efecto, decilo y probalo.

**Una sola fuente.** Todo comportamiento que un VC verifica (un aviso por stderr, un
reintento silencioso) está escrito en el FR o la BR, no solo en el VC ni solo en notas. No
repitas una regla en dos lugares; la lista de "qué es una falla" vive una vez (glosario).

**NFR.** Umbral numérico congelado: nunca "pendiente", "a validar" ni "queda para la
iteración N". El VC repite las condiciones del NFR (carga, cantidad de objetos, entorno), no
cambia el umbral y nombra cómo se mide (`/usr/bin/time -v`, `date +%s%N`, `getrusage`).
Reintentos: el VC prueba intentos, espera y exit code final. Un umbral que discrimina
(500 MiB contra 5 MiB distingue streaming de descarga completa) es mejor que uno laxo.

**Bordes que siempre se cubren.** Recurso de 0 bytes; última línea sin `\n`; binarios,
`.gz` y `.gz` corrupto; guardarraíl alcanzado (exit code y mensaje); corte de red a mitad de
una lectura; precedencia de errores de uso; permisos mínimos de quien invoca declarados como
requisito. Los que no apliquen al cambio se descartan por escrito, en "fuera de alcance".

**Simplicidad.** Nada de capacidades sin fundamento en el diseño (color, alias, flags
extra): se difieren al plan. Ningún mecanismo dentro de un FR (un FR describe lo observable).
Sin historia del proceso en la spec ("versión anterior", "tras la corrección…"). Fijá un
valor por defecto en vez de especificar un flag que nadie ejercita.

**Brownfield.** Declará el commit explorado y el commit de la entrega. Superficie de cambio
por path y guarda de build opt-in (`AM_CONDITIONAL` + `#ifdef`) con las invariantes escritas
como restricciones.

## Plantilla

```md
# Spec — <cambio>

**Repo:** <url> · commit base **`<hash>`** (<fecha>)

## Propósito
<qué cambia, en un párrafo, y qué límite lo encierra>

## Glosario
| Término | Significado |
|---|---|

## Alcance
### Dentro
| Archivo | Cambio |
|---|---|
| `<path>` (**nuevo**) | <qué> |
| `<path>` | <qué, y dónde: `archivo:símbolo`> |

### Fuera de alcance
- **`<path o glob>`** — <por qué no se toca>
- **<feature vecina>** — fuera de alcance.

## Invariantes
| # | Invariante | VC |
|---|---|---|
| **INV-1** | <lo que sigue siendo verdad> | VC-INV1.1 |

## Línea de base de regresión
<comando> → <resultado medido antes del cambio>

## Harness de prueba
<lo que necesitan los VCs: servidor de prueba, listener, variables, cómo se espera>

## Requerimientos
### FR-1 · <título>
**Dado** …,
**cuando** …,
**entonces** <un resultado observable>.

**VC-1.1:** `<comando>` con <datos> → stdout `<literal>`, stderr `<literal>`, exit `<n>`.
**VC-1.2:** <camino de falla> → <mensaje literal>, <qué NO pasa>.

### BR-1 · <regla>
<texto>

**VC-BR1.1:** …

### NFR-1 · <métrica, número y condición de carga>
<texto con umbral y cómo se mide>

**VC-NFR1.1:** `<herramienta>` … → <valor> < <umbral con unidad>.

## VCs de invariantes
**VC-INV1.1:** `<comando>` → `<salida esperada>`.

## Matriz de cobertura
| Requisito | VC | Camino de falla incluido |
|---|---|---|

## Plan de iteraciones
| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | <lo más angosto verificable> | FR-1 … FR-3, INV-1 |

## Decisiones
| # | Pregunta | Elegido | Fundamento (código del sistema) | Descartado y por qué | FR |
|---|---|---|---|---|---|

## Limitaciones conocidas
```

No hay sección de preguntas abiertas: cada pregunta se decide en "Decisiones" o se difiere
al plan con su motivo.

## Antes de entregar

- [ ] Un *Cuando* y un resultado observable por FR; sin "y"/"o" en el *Entonces*.
- [ ] Los cuatro caminos de falla, cada uno con FR y VC; precedencia fijada.
- [ ] Cada VC con datos, comando, stdout/stderr literales, exit code y números con unidad.
- [ ] Cada INV con su `VC-INVn.k`; `#VC ≥ #FR + #BR + #NFR + #INV`.
- [ ] Cada decisión con elegido / fundamento en código / descartado.
- [ ] Cero TBD, "a confirmar", "pendiente", "provisorio".
- [ ] Todo comportamiento de un VC está en un FR o BR; glosario usado sin variantes.
- [ ] Bordes cubiertos o descartados por escrito; sin mecanismos en FRs ni historia del proceso.
- [ ] Hash del commit explorado y símbolos citados verificados contra ese checkout.
- [ ] `check-vc-coverage.sh --strict` en 0 y revisión del `spec-reviewer` sin Issues.
