# Brownfield — cliente SSH nativo en `tmux` (Lección 2)

TP de Spec-Driven Development sobre un codebase que **ya existe**: [`tmux`](https://github.com/tmux/tmux),
un proyecto C maduro. El cambio pedido es agregar un cliente SSH nativo (`libssh`) para que
un pane abra una sesión remota **sin invocar el binario `ssh`**. Solo Linux.

> **No hay implementación, por consigna.** El entregable es el descubrimiento y el
> acotamiento: una spec desde la que otro equipo podría construir sin romper los builds de
> macOS y BSD. Acá no hay nada que compilar ni correr.

## Qué hay en esta carpeta

Los documentos, en orden de pipeline:

| # | Archivo | Paso SDD | Qué contiene |
|---|---|---|---|
| 1 | [`notas-exploracion.md`](notas-exploracion.md) | Descubrir | Cómo un pane lanza su proceso hoy, cómo se aísla lo específico de plataforma, el event loop, riesgos y la historia de compat/build, con archivos y funciones reales |
| 2 | [`spec-brownfield.md`](spec-brownfield.md) | Especificar | El comando `new-ssh-window`, el límite solo-Linux, alcance dentro y fuera por path, glosario, 7 invariantes, 75 FR atómicos más 4 BR y 3 NFR, 131 VCs (`VC-<n>.<k>` alineados con cada requisito, e invariantes con VC propio), decisiones con elegido / fundamento en código / descartado y un plan de 5 iteraciones |
| 3 | [`revision-spec.md`](revision-spec.md) | Revisar | Los 14 huecos de la primera revisión, la segunda iteración tras la corrección de la cátedra (VE-3, VE-4, VE-5) y la tercera, contra el resumen de correcciones, y la cuarta, tras la revisión independiente. Sin veredicto propio: lo da el `spec-reviewer` |

La consigna original no se copia acá: es el enunciado de la Lección 2 de la cátedra.

## El cambio en una página

- **Qué:** un comando `new-ssh-window` que crea una ventana cuyo pane es una sesión SSH,
  hablada por `libssh` y sin ejecutar `ssh`.
- **Dónde engancha:** en `spawn_pane()` (`spawn.c`), en el proceso hijo, justo después de
  `fdforkpty` y en el lugar donde hoy están los `exec`. El pane sigue siendo un pty con
  `pid`, `fd` y `tty` reales, así que el modelo de PTY/panes **no cambia**.
- **Event loop:** no se toca. El cliente corre en el hijo con su propio bucle, porque el
  server de `tmux` es de un solo hilo y una conexión bloqueante congelaría a todos los
  clientes.
- **Autenticación:** agent y claves; sin passwords. Los hosts desconocidos se rechazan.

### El límite solo-Linux

Se aplica en tres capas independientes, y un build sin el flag es idéntico al actual:

| Capa | Mecanismo |
|---|---|
| Configure | `--enable-native-ssh`, opt-in; falla en configure si la plataforma no es Linux |
| Make | `AM_CONDITIONAL`: las fuentes nuevas se agregan solo con el flag |
| Código | Todo lo nuevo en archivos existentes va bajo `#ifdef ENABLE_NATIVE_SSH` |

### Invariantes

Los builds no-Linux siguen compilando, los comandos existentes no cambian y el modelo de
PTY/panes tampoco. Cada invariante tiene en la spec su VC numerado (`VC-INV1.1` a `VC-INV7.1`).

## Cómo verificar las notas

Las notas se escribieron sobre `tmux` en el commit **`5a820e63`** (2026-09-30). Podés
comprobar cualquier afirmación vos mismo:

```bash
git clone https://github.com/tmux/tmux.git && cd tmux
git checkout 5a820e63

grep -n "fdforkpty" spawn.c                  # el fork del pane: spawn.c:478
sed -n 612,622p window.c                     # ioctl(TIOCSWINSZ) y fatal() si falla
grep -c "__linux__" *.c *.h | grep -v ":0"   # sin salida: tmux no usa #ifdef por plataforma
grep -n "ENABLE_SIXEL" Makefile.am           # el patrón de feature opcional que se reusa
```

Los números de línea pueden correrse en versiones posteriores; los nombres de archivo y de
función son lo estable.

## Estado

- **Descubrir y Especificar: hechos**, con cuatro iteraciones de la spec (la primera obtuvo
  5,5/10). Ver `revision-spec.md`.
- **Revisión independiente:** la pasa el `spec-reviewer`, no quien escribió la spec.
- **Medido sobre `5a820e63`:** el build compila, `list-commands` da 92 líneas y `regress/` tiene
  172 scripts: 171 `PASS` y 1 `FAIL` previo (`prompt-words-history.sh`).
- **Fuera del alcance de la spec:** los VCs de red necesitan la implementación y el harness
  (`sshd`, `strace`); no se ejecutaron.

Tag de esta carpeta: `02-Brownfield`.
