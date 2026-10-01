# Notas de exploración — cliente SSH nativo en `tmux` (solo Linux)

> Salida del paso **Descubrir**. Producidas en modo **solo lectura**: no se editó ningún
> archivo de `tmux`, no se compiló, no se escribió C.
>
> **Repo explorado:** [`tmux/tmux`](https://github.com/tmux/tmux), commit
> **`5a820e63`** (2026-09-30, "Update CHANGES."). Los números de línea pueden correrse en
> versiones posteriores; los nombres de archivo y de función son lo estable.
>
> Para verificar: `git clone https://github.com/tmux/tmux.git && git checkout 5a820e63`.
>
> **Convención:** lo que dice *"verificado"* se leyó en el código de ese commit. Lo que dice
> *"no verificado"* es conocimiento externo (sobre `libssh`) que **no** está en este repo y
> hay que confirmarlo antes de construir.

## Qué se quiere lograr

Un comando nuevo que abra un pane cuyo contenido sea una sesión SSH remota, hablada por un
cliente **nativo** (`libssh`) en vez de correr el binario `ssh` dentro de una shell. Solo
Linux: en macOS y BSD el build tiene que seguir compilando como hoy.

## El terreno

Los `.c`, `.h` y `.y` de la raíz suman ~108.000 líneas (`wc -l`), más que las ~60.000 del
enunciado; no hace falta entenderlas. Hace falta entender **un camino**: del comando al
`fork`/`exec`, y del pane vivo al event loop.

```
new-window / split-window / respawn-pane / display-menu / new-session
        │   (cada uno arma un struct spawn_context)
        ▼
  spawn.c   spawn_window() → spawn_pane()
        │
        ▼
  fdforkpty()  →  hijo: execvp / execl $SHELL -c        padre: window_pane_set_event()
                                                                   │
                                                                   ▼
                                                  libevent: bufferevent sobre wp->fd
```

## Hallazgos

### 1 · De la tabla de comandos al spawn

Cada comando es un `struct cmd_entry` (**`tmux.h:2056`**: `name`, `alias`, `args`, `usage`,
`target`, `flags`, `exec`). Se registra en dos lugares de **`cmd.c`**: un
`extern const struct cmd_entry` arriba y un puntero en `cmd_table[]` (**`cmd.c:123`**; por
ejemplo `cmd_new_window_entry` en `:170`, `cmd_respawn_pane_entry` en `:182`,
`cmd_split_window_entry` en `:206`). Ejemplo de entrada completa: **`cmd-split-window.c:58-73`**.

Los comandos que crean un pane llenan un **`struct spawn_context`** (**`tmux.h:2500`**) y
llaman a `spawn_window()` o `spawn_pane()` (prototipos en **`tmux.h:4195-4196`**):

| Comando | Llamada |
|---|---|
| `new-window` | `spawn_window` — `cmd-new-window.c:161` |
| `new-session` | `spawn_window` — `cmd-new-session.c:305` |
| `split-window` | `spawn_pane` — `cmd-split-window.c:208` |
| `respawn-pane` | `spawn_pane` — `cmd-respawn-pane.c:83` |
| `display-menu` | `spawn_pane` — `cmd-display-menu.c:498` |

El contexto lleva `argv`/`argc` (el comando a correr), `environ`, `cwd` y `flags`
(`SPAWN_KILL`, `SPAWN_DETACHED`, `SPAWN_RESPAWN`, `SPAWN_EMPTY`, `SPAWN_FLOATING`, etc.).
**No hay ningún campo "tipo de pane"**: hoy todo pane es un proceso hijo en un pty.

### 2 · El punto exacto donde se lanza el proceso

**`spawn.c:243`**, `spawn_pane()`. La secuencia, en orden:

1. Arma el entorno del hijo (`environ_for_session`, `TMUX_PANE`, `PATH`, `SHELL`).
2. Bloquea señales (`sigprocmask`) y arma el `winsize`.
3. Si `SPAWN_EMPTY`, **no hace fork** (`spawn.c:458`): el pane queda vacío.
4. **`spawn.c:478`**: `new_wp->pid = fdforkpty(ptm_fd, &new_wp->fd, new_wp->tty, NULL, &ws)`.
   Esto crea el pty y el hijo de una vez; llena `wp->pid`, `wp->fd` (lado master) y
   `wp->tty` (nombre).
5. En el hijo: ajusta `termios`, limpia señales, `closefrom(STDERR_FILENO + 1)`,
   `environ_push`, y recién ahí ejecuta:
   - varios argumentos → `execvp(argvp[0], argvp)` (**`spawn.c:552`**);
   - un argumento → `execl(shell, argv0, "-c", cmd)` (**`spawn.c:567`**);
   - ninguno → `execl(shell, "-nombre")`, shell de login (**`spawn.c:574`**).
6. En el padre (`complete:`): `window_pane_set_event(new_wp)` y `spawn_fire_pane_created`.

**Este es el único lugar donde `tmux` convierte "un comando" en "un proceso con terminal".**
Cualquier pane SSH tiene que decidir qué hace en este punto.

### 3 · Cómo vive un pane: fd + bufferevent

**`window.c:1673`**, `window_pane_set_event()`: pone `wp->fd` en no bloqueante y crea un
`bufferevent` sobre él (`window.c:1677`), con `window_pane_read_callback` (`:1632`, que
termina en `input_parse_pane`) y `window_pane_error_callback` (`:1660`, que marca
`PANE_EXITED` y destruye el pane si `window_pane_destroy_ready`).

Lo que *escribe* al pane (teclas, pegado) usa `bufferevent_write(wp->event, …)`
(p. ej. `window.c:2064`). O sea: **todo el resto de `tmux` ve al pane como un fd con un
bufferevent**. Eso es el "modelo de PTY/panes" que el enunciado manda no cambiar.

### 4 · Quién más depende de que ese fd sea un pty

Cada uno de estos sitios asume `wp->fd` pty y `wp->pid` hijo real:

| Sitio | Qué hace | Si el fd no es un pty |
|---|---|---|
| `window.c:612` (`window_pane_send_resize`) | `ioctl(wp->fd, TIOCSWINSZ)` y **`fatal("ioctl failed")` si falla** (`:622`) | **tira abajo el server entero** |
| `window.c:501` (`window_pane_destroy_ready`) | `ioctl(wp->fd, FIONREAD)` | falla en silencio, sigue |
| `server.c:491` (`server_child_exited`) | busca el pane por `wp->pid == pid` (`:498`) | sin hijo, nunca dispara |
| `format.c:966`, `:990`, `window-tree.c:948` | `osdep_get_name(fd, tty)` / `osdep_get_cwd(fd)` | en Linux usan `tcgetpgrp(fd)` + `/proc/<pgrp>/…` (`osdep-linux.c:30`, `:64`) |
| `format.c:2578` | `pane_pid` imprime `wp->pid` | valor sin sentido |
| `cmd-find.c:89` | `strcmp(wp->tty, c->ttyname)` | `tty` vacío |

El punto de la primera fila es el más delicado: **si el pane SSH usara un `socketpair` en
lugar de un pty, el primer resize mata el servidor.**

### 5 · Cómo termina un pane

El hijo muere → `SIGCHLD` → `server_child_signal` (**`server.c:466`**, `waitpid`) →
`server_child_exited` → `PANE_EXITED` → `server_destroy_pane`. Alternativamente, EOF en el
fd dispara `window_pane_error_callback` (el mismo flag). Cerrar el fd:
**`window.c:1579-1587`** y **`server-fn.c:365-373`** (`bufferevent_free` + `close`).

### 6 · El event loop

`tmux` es **de un solo hilo, con libevent** (`event_init` vía `osdep_event_init()`,
llamado desde `tmux.c:624`; en Linux fija `EVENT_NOEPOLL=1` porque "epoll doesn't work on
/dev/null", `osdep-linux.c:92`). Consecuencia: **cualquier llamada bloqueante en el
proceso server congela a todos los clientes**. Un `ssh_connect()` / handshake bloqueante
dentro del server es inaceptable.

Hay un precedente de "I/O que no es un pane": **`job.c`**. `job_run` (`job.c:72`) elige
entre `fdforkpty` (si `JOB_PTY`, `:116`) y `socketpair` (`:118`), y arma un `bufferevent`
(`:225`). Muestra que ya existen dos formas de lanzar un hijo atado al loop.

### 7 · La capa de portabilidad: se elige por archivo, no por `#ifdef`

Hallazgo contraintuitivo: **no hay ningún `#ifdef __linux__` en el código** (verificado con
`grep`). `tmux` aísla plataforma así:

- **Un archivo por plataforma**: `osdep-linux.c`, `osdep-darwin.c`, `osdep-freebsd.c`, … y
  `Makefile.am:235` agrega `osdep-@PLATFORM@.c`. `configure.ac:1006-1118` calcula
  `PLATFORM` con un `case "$host_os"` (Linux en ~`:1062`).
- **`compat/`**: reemplazos de funciones que faltan (`fdforkpty.c`, `closefrom.c`,
  `strlcpy.c`, …), anunciados en **`compat.h`** (`fdforkpty` en `:396-404`). `configure.ac`
  decide si hace falta cada uno (p. ej. `:828-839` busca `fdforkpty`/`forkpty` en `libutil`
  y si no existe agrega `compat/fdforkpty.c` con `AC_LIBOBJ`).
- **`AM_CONDITIONAL(IS_LINUX, …)` ya existe** (`configure.ac:1122`) pero **ningún
  `Makefile.am` lo usa todavía**.

### 8 · El precedente exacto de feature opcional: systemd

`--enable-systemd` es, casi uno a uno, el patrón que necesita el feature SSH:

| Pieza | Dónde |
|---|---|
| Flag de configure y detección con `pkg-config` | `configure.ac:501-527` |
| `AC_DEFINE(HAVE_SYSTEMD)` + `AM_CONDITIONAL(HAVE_SYSTEMD, …)` | `configure.ac:522,527` |
| Fuente que se agrega solo si está activo | `Makefile.am:243-245` → `compat/systemd.c` |
| Prototipos condicionales | `compat.h:444-449` (`#ifdef HAVE_SYSTEMD`) |
| Uso en el camino de spawn, **en el hijo** | `spawn.c:504` (`#if defined(HAVE_SYSTEMD) && defined(ENABLE_CGROUPS)`) |
| Otros usos | `server.c:223`, `client.c:283`, `environ.c:272` |

Hay un **segundo precedente**, más cercano a una feature con código propio: `ENABLE_SIXEL`
(`configure.ac:549-552`, `AC_DEFINE` + `AM_CONDITIONAL`). Agrega **fuentes propias** con
`if ENABLE_SIXEL … dist_tmux_SOURCES += image.c image-sixel.c` (`Makefile.am:252-255`) y se usa con
`#ifdef ENABLE_SIXEL` en `format.c`, `input.c`, `screen-write.c` y `screen-redraw.c`. O sea: `tmux`
**sí** usa `#ifdef` para features opcionales; lo que no usa es `#ifdef` por plataforma.

El `spawn.c:504` es especialmente relevante: ya hay código opcional que corre **en el hijo,
justo después de `fdforkpty`**.

### 9 · Historia de compat/build (por qué está así)

Útil para no pelearse con decisiones viejas (`git log -S` / `--follow`):

| Commit | Fecha | Qué cuenta |
|---|---|---|
| `91241f14` | 2009-04-29 | "Apply the make magic wand to pick an osdep-*.c file rather than using ifdefs": el origen de la elección por archivo |
| `436f3b35` | 2010-12-30 | epoll roto con `/dev/null` en Linux (el `EVENT_NOEPOLL`) |
| `4273c1b8` | 2014-02-24 | utempter opcional detectado en configure |
| `94207581` | 2017-04-20 | `getptmfd()` + `fdforkpty()`: el pty se pide a un fd abierto al inicio (`tmux.c:538`, `ptm_fd`) |
| `1c69a91c`, `fc7f1e7a` | 2022-03-28 | systemd: activación por socket, `--enable-systemd` |
| `b9524f5b` | 2023-04-03 | systemd: cgroup por pane, **en el spawn** |
| `6a5745c7` | 2026-09-28 | refinamiento reciente del mismo camino de cgroups |

`6a5745c7` es de hace tres días: **el camino de `spawn_pane` se sigue tocando**; hay riesgo
de conflicto de merge.

### 10 · Entorno y autenticación

- `update-environment` (**`options-table.c:1211`**) incluye `SSH_AUTH_SOCK` y
  `SSH_AGENT_PID`: el entorno **de la sesión** se actualiza cuando un cliente se conecta,
  pero el entorno **del proceso server** no. Para auth por agent, el socket correcto está
  en `environ_for_session(s, …)` (`environ.c:253`), no en `getenv` del server.
- No hay en el repo **ninguna referencia a `libssh`** ni a un cliente SSH; solo menciones
  incidentales a `ssh` en `options-table.c` y comentarios (verificado).

### 11 · La línea de base de regresión

- **Tests:** `regress/*.sh` (scripts de shell que manejan un `tmux` real con
  `TEST_TMUX`, p. ej. `regress/new-window-command.sh`). Se corren con `make` dentro de
  `regress/` (`regress/Makefile`).
- **CI:** `.github/workflows/regress.yml` — matriz **ubuntu-24.04 x64**, **ubuntu-24.04
  arm64** y **macos-26 arm64**, con `--enable-utf8proc --enable-asan`. Las dependencias de
  Linux se instalan con `apt-get` (`libevent-dev`, `libncurses-dev`, …): **sumar `libssh`
  tiene que ser opcional o rompe el job de macOS**.
- No hay tests que mencionen un cliente SSH; hay fuzzers (`fuzz/`) que no tocan spawn.

## Implicaciones para el diseño (análisis, no decisiones)

Dos formas de enchufar un cliente nativo en `spawn_pane`:

- **A · Cliente en el proceso del hijo.** Mantener `fdforkpty` intacto (mismo `pid`,
  `fd`, `tty`, `SIGCHLD`, `TIOCSWINSZ`) y que el hijo, en vez de `execvp`/`execl`
  (`spawn.c:552-574`), corra un bucle `libssh` propio. El handshake bloquea **al hijo**, no
  al server. Toca un solo punto del camino y deja intactos los hallazgos de la tabla 4.
- **B · Cliente dentro del server, en el event loop.** Sin hijo: `wp->fd` pasa a ser un
  extremo de un canal propio. Hay que resolver todo lo de la tabla 4 (resize, destroy,
  pid, name/cwd) y usar modo no bloqueante de `libssh`. Mucho más superficie.

A es la que mejor respeta el invariante "el modelo de PTY/panes no cambia". La decisión
final va en la spec.

## Riesgos

| Riesgo | Por qué |
|---|---|
| **Pane sin pty tira abajo el server** | `window_pane_send_resize` hace `fatal` si falla el `ioctl` (`window.c:612-622`) |
| **Handshake bloqueante en el server** | Un solo hilo con libevent: congela a todos los clientes |
| **Romper el build no-Linux** | Si `cmd.c` referencia el comando sin guarda, macOS/BSD no linkea: `cmd.c` **no tiene ningún `#if`** hoy, así que habría que introducir el primero |
| **Dependencia obligatoria nueva** | CI de macOS no instala `libssh`; tiene que ser `--enable-*` opcional |
| **Colisión con `SPAWN_*` / `PANE_*`** | Los flags son bits fijos (`tmux.h:2500+`, `:1323+`); agregar uno exige elegir bit libre |
| **Auth por agent con el entorno equivocado** | El server no tiene el `SSH_AUTH_SOCK` del cliente actual (hallazgo 10) |
| **`known_hosts` y prompts** | Verificar host keys o pedir passphrase requiere interacción; no hay prompt dentro de un pane que todavía no tiene proceso (**no verificado** cómo lo resuelve `libssh`) |
| **Merge** | `spawn.c` y el sistema de cgroups se tocaron el 2026-09-28 (`6a5745c7`) |
| **`libssh` vs OpenSSH** | Decisión de dependencia: `libssh` es una librería enlazada; "OpenSSH" como librería no existe como API estable (**no verificado**) |

## Lo que NO hace falta entender

`input*.c`, `screen*.c`, `grid*.c`, `tty*.c`, `format-draw.c` (el render y el parseo de
terminal), `layout*.c`, los modos (`window-copy.c`, `window-tree.c`, …) y `control*.c`.
Acotar también es decidir qué no leer.

## Qué falta verificar antes de construir

- Que `libssh` ofrezca `SSH_OPTIONS_IDENTITY_AGENT` y un modo no bloqueante adecuado
  (conocimiento externo, no verificado acá).
- Comportamiento exacto de `format_cb_pane_current_command` cuando `osdep_get_name` devuelve
  `NULL` (`format.c:956-978`): no se leyó el manejo del `NULL`.
