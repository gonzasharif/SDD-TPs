# Spec — `new-ssh-window`: cliente SSH nativo en `tmux` (solo Linux)

> Spec **brownfield** construida sobre [`notas-exploracion.md`](./notas-exploracion.md).
> **No se implementa nada**: es el contrato desde el cual otro equipo puede construir sin
> hablar con nosotros y sin romper los builds de macOS y BSD.

**Repo:** [`tmux/tmux`](https://github.com/tmux/tmux) · commit base **`5a820e63`** (2026-09-30)
**Entrega:** el commit de la rama entregada se declara al entregar (`git rev-parse HEAD`).

## Propósito

Agregar un comando `new-ssh-window` que abre una ventana cuyo pane es una sesión SSH remota,
hablada por un cliente SSH integrado a `tmux`, sin invocar nunca el binario `ssh`.

## El límite solo-Linux

Es el eje de la spec. Se aplica en **tres capas**, de modo que ninguna depende de la otra:

| Capa | Mecanismo | Resultado fuera de Linux | FR |
|---|---|---|---|
| **Configure** | `--enable-native-ssh`, **opt-in** (apagado por default). Si `PLATFORM` (`configure.ac:1006-1118`) no es `linux`, `configure` **falla con un mensaje claro** | el flag no se puede activar por accidente | FR-1 … FR-7 |
| **Make** | `AM_CONDITIONAL(ENABLE_NATIVE_SSH, …)`; las fuentes nuevas se agregan solo en esa rama (patrón de `ENABLE_SIXEL`, `Makefile.am:253-255`) | las fuentes nuevas no se compilan ni se enlazan | FR-8, FR-9 |
| **Código** | `AC_DEFINE(ENABLE_NATIVE_SSH)`; todo código nuevo en archivos existentes va dentro de `#ifdef ENABLE_NATIVE_SSH` | el binario es el mismo que hoy | FR-10, FR-30 |

Un build sin el flag es **idéntico** al actual, en cualquier plataforma, incluido Linux.
`libssh` **nunca** es dependencia obligatoria.

## Glosario

Un término por concepto; el resto de la spec lo usa sin variantes.

| Término | Significado |
|---|---|
| **destino** | El argumento posicional `[usuario@]host` |
| **destino válido** | `[usuario@]host` con usuario y host no vacíos, sin `@` ni espacios internos |
| **host** | La parte del destino posterior a `@` (o el destino entero si no hay `@`) |
| **usuario** | La parte anterior a `@`; si falta, el usuario del proceso del server |
| **ventana** / **pane** | La ventana que crea el comando y su único pane |
| **error de uso** | Rechazo del comando antes de crear la ventana (FR-12, FR-13, FR-21 … FR-28). Sale por stderr del cliente `tmux` con estado `1` y **no** crea ventana |
| **falla del cliente** | El hijo del pane termina por una condición de FR-34, FR-36, FR-37, FR-45 … FR-49, FR-58, FR-60 … FR-62. Imprime una línea `new-ssh-window: …` en el pane y sale con `1` (BR-3) |
| **mensaje** | La línea `new-ssh-window: …` de una falla del cliente, en el pane |
| **`<ruta>`** | Ruta del archivo nombrado, tal como se escribió (los archivos que nombra la spec son siempre absolutos) |
| **se autentica** | `$SSHD_LOG` contiene `Accepted publickey for <usuario>` |
| **llega a `sshd`** | `$SSHD_LOG` contiene `Connection from 127.0.0.1 port <n> on 127.0.0.1 port $PORT` |
| **llega al prompt remoto** | Tras `send-keys 'echo R-$((6*7))' Enter`, `capture-pane -p` contiene la línea `R-42` |

## Alcance

### Dentro

| Archivo | Cambio |
|---|---|
| `cmd-new-ssh-window.c` (**nuevo**) | `cmd_new_ssh_window_entry`: parsea y valida argumentos, arma el `spawn_context` y llama a `spawn_window()` |
| `ssh-client.c` (**nuevo**) | El cliente nativo: conectar, verificar host, autenticar, abrir canal con pty, copiar datos. Una sola entrada pública, `ssh_client_run()` |
| `spawn.c` | **Un** bloque en el camino del hijo de `spawn_pane()`, después de `environ_push` (`spawn.c:544`) y antes de los `exec` (`:552-574`) |
| `tmux.h` | Flag `SPAWN_SSH 0x2000` (el `0x1000` ya es `SPAWN_FLOATOVERZOOM`, `tmux.h:2531`) y prototipo de `ssh_client_run()` |
| `cmd.c` | `extern` + entrada en `cmd_table[]` (`cmd.c:123`), ambos bajo `#ifdef` |
| `configure.ac` | Flag, detección de `libssh >= 0.9.0` con `pkg-config`, guarda de plataforma, línea en el resumen final |
| `Makefile.am` | `if ENABLE_NATIVE_SSH` agregando las dos fuentes nuevas |
| `tmux.1` | Entrada del comando |
| `regress/new-ssh-window.sh` (**nuevo**) | Prueba end-to-end con el harness descripto abajo. Se saltea si `list-commands` no lista el comando; si además `TEST_REQUIRE_SSHD=1` y falta `sshd`, **falla** en vez de saltear |
| `regress/native-ssh-guard.sh` (**nuevo**) | Prueba de la guarda, corre en **todas** las plataformas (lo levanta `TESTS != echo *.sh`, `regress/Makefile`). Elige qué VC ejecutar con `uname -s` y con `$TEST_TMUX list-commands`: fuera de Linux, VC-INV2.1 y VC-5.1; en Linux, si el comando no está listado (build sin flag), VC-INV1.1; si está listado, no hace nada (los VCs con flag son de `new-ssh-window.sh`) |
| `regress/list-commands.base` (**nuevo**) | La salida de `list-commands` del build sin flag de `5a820e63` (92 líneas), **versionada**: la leen VC-INV1.1 y VC-INV3.1 |
| `.github/workflows/regress.yml` | Entrada **nueva** de matriz solo Linux con `--enable-native-ssh` y `TEST_REQUIRE_SSHD=1`; en el step de dependencias de **Linux**, `libssh-dev`, `openssh-server`, `openssh-client`, `netcat-openbsd`, `procps`, `strace` y `man-db` |

### Fuera de alcance

Explícito, por path:

- **`window.c`, `server.c`, `server-fn.c`, `window-*.c`** — el modelo de pane (fd + bufferevent
  + pid) no cambia. Ese es el motivo de enganchar en el hijo (decisión 2).
- **`job.c`, `input*.c`, `screen*.c`, `tty*.c`, `layout*.c`** — sin cambios.
- **`format.c`, `window-tree.c`, `osdep-*.c`, `cmd-find.c`** — `pane_current_command`,
  `pane_current_path` y búsqueda por tty siguen igual; no se parchan.
- **`cmd-split-window.c`** — **no hay variante con split** (`split-window` tiene 424 líneas
  contra 196 de `new-window` por el manejo de layout).
- **`cmd-respawn-pane.c`** — `respawn-pane` sobre un pane SSH **no se soporta** (ver
  "Limitaciones conocidas").
- **`options-table.c`, `key-bindings.c`** — sin opciones nuevas ni binding por default.
- **`compat/`** — no se agrega nada: esto es una feature, no un reemplazo de portabilidad.
- **El job de macOS de `regress.yml`** — no se toca: ni su `configure`, ni sus dependencias.
  Fuera de Linux las fuentes nuevas no se compilan y el comando no existe en el binario.
- **Autenticación por password o keyboard-interactive, TOFU de host keys, agent forwarding,
  port forwarding, SFTP, ProxyJump, `~/.ssh/config`, `known_hosts` global, líneas
  malformadas de `known_hosts`** — fuera de alcance.

## Invariantes

Cosas que tienen que seguir siendo verdad **después** del cambio. Cada una tiene sus VCs en
la sección "VCs de invariantes".

| # | Invariante | VC |
|---|---|---|
| **INV-1** | Sin `--enable-native-ssh`, el binario es el de hoy | VC-INV1.1 |
| **INV-2** | Los builds no-Linux siguen compilando, y el comando nuevo no está en su binario | VC-INV2.1 |
| **INV-3** | Los comandos existentes no cambian | VC-INV3.1, VC-INV3.2 |
| **INV-4** | El modelo de PTY/panes no cambia | VC-INV4.1, VC-INV4.2 |
| **INV-5** | No se invoca el binario `ssh` (ni ningún otro) para el pane SSH | VC-INV5.1, VC-INV5.2 |
| **INV-6** | El server no gana hilos | VC-INV6.1, VC-INV6.2 |
| **INV-7** | El bit `SPAWN_SSH` no pisa uno existente | VC-INV7.1 |

## Línea de base de regresión

Medida **antes** de tocar una línea, sobre `5a820e63` y **sin** el flag:

```bash
sh autogen.sh && ./configure --enable-utf8proc && make
./tmux -f/dev/null list-commands > regress/list-commands.base   # 92 líneas
(cd regress && make)                                    # 172 scripts: 171 PASS, 1 FAIL previo
```

**Medido** (Ubuntu 24.04, `make -j8` en `regress/`): el build compila (`make` sale con `0`),
`list-commands` imprime **92** líneas (coincide con las 92 entradas de `cmd_table[]`,
`cmd.c:124-215`) y `regress/` contiene **172** scripts `*.sh`, de los cuales **171 dan `PASS`
y 1 da `FAIL` antes de tocar nada**: `prompt-words-history.sh` (`prompt result is 'show-r',
expected 'history-command'`, determinístico en ese entorno). La línea de base es entonces
"esos 171 en `PASS`": un script que falla antes del cambio no cuenta como regresión, y quien
implemente mide la suya en su entorno y la guarda junto con `regress/list-commands.base`
(`list-commands` sin flag, 92 líneas): VC-INV1.1 y VC-INV3.1 comparan contra ese archivo.
Al cerrar cada iteración, la misma corrida **sin** el flag tiene que dar lo mismo.

Con el cambio completo, `regress/` tiene **174** scripts (los 172 más los dos nuevos). Un
script que estaba en `PASS` en la línea de base y se pone rojo es una regresión: no se "actualiza" para que pase.

## Harness de prueba

Lo usan los VCs de red. Vive en `regress/new-ssh-window.sh`; el feature nunca lo invoca. Las
pruebas corren como usuario **no root** (los VCs de permisos de archivo lo necesitan).

- **`sshd` de prueba:** `sshd -D -e -f <conf>` en `127.0.0.1`, puerto alto `$PORT`, host key
  propia, `LogLevel VERBOSE`, log en `$SSHD_LOG`, pid en `$SSHD_PID`. Acepta solo `publickey`, salvo en VC-46.1.
- **`HOME` temporal** `$H`, con `$H/.ssh/known_hosts` y las claves que pida cada VC.
- **Destino** `$D` = `$USER@127.0.0.1`.
- **`tmux` de prueba** con `-f/dev/null`, `remain-on-exit on` (para leer la pantalla y
  `#{pane_dead_status}` de un pane que ya terminó), `history-limit 250000` y el entorno de sesión con `HOME=$H`.
  Por defecto el server corre **sin** `strace`. Los VCs que piden el trace (VC-33.1, VC-INV5.1, VC-INV5.2) lo lanzan
  **bajo `strace`**: `strace -f -o $TRACE -e trace=execve,connect tmux … new-session -d`. Los VCs de tiempo
  (VC-62.1, VC-NFR1.1, VC-NFR3.1) corren siempre sin `strace`.
- **Espera:** "esperar al pane" es consultar `#{pane_dead}` cada 100 ms hasta `1`, con tope
  de 20 s.
- **Listener mudo:** `nc -lk 127.0.0.1 $MUTE`, que acepta conexiones TCP y nunca manda el
  banner SSH. Deja una conexión colgada de forma determinista: el hijo del pane queda vivo
  hasta el timeout de FR-62 (15 s). `$M` = `$USER@127.0.0.1` con `-p $MUTE`.
- **Puerto libre** `$FREE`: `python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'`, sin ningún listener.
- **Herramientas:** OpenSSH ≥ 8.0 (`sshd`, `ssh-keygen`, `ssh-agent`), `nc` de `netcat-openbsd` (con `-lk`), `procps` (`ps`, `pgrep`), `strace`, `python3` y `man-db` (`man -l`).

## Requerimientos

Un Cuando y un resultado observable por FR. Cada FR tiene al menos un VC `VC-<n>.<k>`
debajo, con su mismo número. Cada VC fija datos, comando y salida esperada; las fallas del
cliente además cumplen BR-3.

### Guarda de build

#### FR-1 · El flag está apagado por defecto

**Dado** el `configure.ac` modificado (flag declarado con `AC_ARG_ENABLE`, como `sixel` en
`configure.ac:545-548`),
**cuando** se corre `./configure` sin `--enable-native-ssh`,
**entonces** `ENABLE_NATIVE_SSH` no queda definido.

**VC-1.1:** `./configure` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da `0` (`tmux` no genera `config.h`: los `AC_DEFINE` viajan en `DEFS` del `Makefile`).

#### FR-2 · En Linux con `libssh`, el flag activa el feature

**Dado** Linux con `libssh >= 0.9.0` detectable por `pkg-config` (`PKG_CHECK_MODULES`, como
`libsystemd` en `configure.ac:508`),
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `ENABLE_NATIVE_SSH` queda definido.

**VC-2.1:** `./configure --enable-native-ssh` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da `1`.

#### FR-3 · Sin `libssh`, el flag falla en `configure`

**Dado** Linux **sin** `libssh` instalado,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con un error que nombra `libssh`.

**VC-3.1:** en un entorno sin `libssh-dev`, el comando sale con `1` y `./configure --enable-native-ssh 2>&1 | grep -c libssh` da al menos `1`; `ls Makefile` falla (no se generó).

#### FR-4 · Con `libssh` anterior a 0.9.0, el flag falla en `configure`

**Dado** Linux con `libssh` **0.8.x** instalado,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con un error que contiene `libssh >= 0.9.0`.

**VC-4.1:** con un `libssh.pc` de prueba que declara `Version: 0.8.9` en `PKG_CONFIG_PATH`, el comando sale con `1` y su salida contiene `libssh >= 0.9.0`.

#### FR-5 · Fuera de Linux, el flag falla en `configure`

**Dado** una plataforma donde `PLATFORM` (`configure.ac:1006-1118`) no es `linux`,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con el error `native SSH is only supported on Linux`.

**VC-5.1:** `regress/native-ssh-guard.sh`, cuando `uname -s` no es `Linux`, copia el árbol (`SRC` = directorio padre de `regress/`) con `d=$(mktemp -d) && cp -R "$SRC/." "$d"` y corre `(cd "$d" && ./configure --enable-native-ssh)` (el `configure` del job de macOS no se toca); sale con `1` y su salida contiene `native SSH is only supported on Linux`.

#### FR-6 · Sin el flag, el resumen de `configure` informa `off`

**Dado** el bloque de resumen de `configure.ac` (los `AC_MSG_NOTICE` que terminan en `configure.ac:1166`),
**cuando** `configure` termina bien sin `--enable-native-ssh`,
**entonces** imprime la línea `native SSH: off`.

**VC-6.1:** la salida de `./configure` contiene exactamente una línea `native SSH: off`.

#### FR-7 · Con el flag, el resumen de `configure` informa `on`

**Dado** el mismo bloque de resumen de `configure.ac`,
**cuando** `configure` termina bien con `--enable-native-ssh`,
**entonces** imprime la línea `native SSH: on`.

**VC-7.1:** la salida de `./configure --enable-native-ssh` contiene exactamente una línea `native SSH: on`.
#### FR-8 · Con el flag, las fuentes nuevas quedan enlazadas

**Dado** `Makefile.am` con `if ENABLE_NATIVE_SSH … dist_tmux_SOURCES += cmd-new-ssh-window.c ssh-client.c` (patrón de `Makefile.am:253-255`),
**cuando** se corre `make` en un build con `--enable-native-ssh`,
**entonces** los dos objetos quedan enlazados en `tmux`.

**VC-8.1:** tras `make`, existen `cmd-new-ssh-window.o` y `ssh-client.o`, y `nm tmux | grep -c ' T ssh_client_run'` da `1`.

#### FR-9 · Sin el flag, las fuentes nuevas no se compilan

**Dado** el mismo `Makefile.am`,
**cuando** se corre `make` en un build sin el flag,
**entonces** ninguno de los dos objetos se construye.

**VC-9.1:** tras `make`, `ls cmd-new-ssh-window.o ssh-client.o` falla (no existe ninguno) y `nm tmux | grep -c ssh_client_run` da `0`.
### El comando

#### FR-10 · El comando está registrado en la tabla de comandos

**Dado** un build con el flag,
**cuando** el server arma `cmd_table[]` (`cmd.c:123`), con la entrada
`&cmd_new_ssh_window_entry` y su `extern` dentro de `#ifdef ENABLE_NATIVE_SSH`,
**entonces** `new-ssh-window` es un comando válido.

**VC-10.1:** `tmux list-commands | grep -c '^new-ssh-window'` da `1`.

#### FR-11 · El alias es `sshw`

**Dado** `cmd_new_ssh_window_entry` con `.alias = "sshw"`,
**cuando** se invoca `tmux sshw …`,
**entonces** se ejecuta `new-ssh-window`.

**VC-11.1:** `tmux sshw -d -P -F '#{window_index}' -t :9 $D` imprime `9`.

#### FR-12 · Sin destino, el comando falla con el uso

**Dado** `.args = { "dF:i:n:p:Pt:", 1, 1, NULL }` y `.usage = "[-dP] [-F format] [-i identity-file] [-n window-name] [-p port] [-t target-window] destination"`,
**cuando** se invoca sin argumento posicional,
**entonces** es un error de uso con el mensaje de `cmd.c:530`.

**VC-12.1:** `tmux new-ssh-window` sale con `1`, su stderr es `usage: new-ssh-window [-dP] [-F format] [-i identity-file] [-n window-name] [-p port] [-t target-window] destination` y `list-windows | wc -l` no cambia.

#### FR-13 · Con más de un destino, el comando falla con el uso

**Dado** la misma entrada de FR-12,
**cuando** se invoca con dos argumentos posicionales,
**entonces** es un error de uso con el mensaje de `cmd.c:530`.

**VC-13.1:** `tmux new-ssh-window a b` sale con `1`, su stderr es el mismo `usage: new-ssh-window …` de VC-12.1 y `list-windows | wc -l` no cambia.

#### FR-14 · `-t` elige la ventana destino como en `new-window`

**Dado** `.target = { 't', CMD_FIND_WINDOW, CMD_FIND_WINDOW_INDEX }` (igual que `cmd-new-window.c:46`),
**cuando** se pasa `-t :N` con `N` libre,
**entonces** la ventana se crea en el índice `N`.

**VC-14.1:** tras `tmux new-ssh-window -d -t :7 $D`, `list-windows -F '#{window_index}' | grep -cx 7` da `1`.

#### FR-15 · `-d` no cambia la ventana actual

**Dado** el flag `-d`, que pone `SPAWN_DETACHED` (como `cmd-new-window.c:156-157`),
**cuando** se crea la ventana,
**entonces** la ventana actual de la sesión sigue siendo la misma.

**VC-15.1:** `display-message -p '#{window_index}'` da el mismo valor antes y después de `new-ssh-window -d $D`.

#### FR-16 · `-P` sin `-F` imprime el formato por defecto de `new-window`

**Dado** el flag `-P` y el template `NEW_WINDOW_TEMPLATE` (`cmd-new-window.c:33`, `:174`),
**cuando** se crea la ventana con `-P` y sin `-F`,
**entonces** imprime `#{session_name}:#{window_index}.#{pane_index}` expandido.

**VC-16.1:** en la sesión `s`, `tmux new-ssh-window -d -P -t :6 $D` imprime `s:6.0`.

#### FR-17 · `-F` define el formato que imprime `-P`

**Dado** los flags `-P` y `-F` (como `cmd-new-window.c:172-173`),
**cuando** se crea la ventana con `-P -F 'W=#{window_index}'`,
**entonces** imprime ese formato expandido.

**VC-17.1:** `tmux new-ssh-window -d -P -F 'W=#{window_index}' -t :9 $D` imprime `W=9`.

#### FR-18 · `-n` fija el nombre de la ventana

**Dado** el flag `-n nombre`, que se pasa en `sc.name` (como `cmd-new-window.c:83`, `:140`),
**cuando** se crea la ventana,
**entonces** la ventana se llama `nombre`.

**VC-18.1:** `tmux new-ssh-window -d -n prod -P -F '#{window_name}' $D` imprime `prod`.

#### FR-19 · Sin `-n`, la ventana se llama como el destino

**Dado** que el hijo es un fork del server, y `osdep_get_name` (`osdep-linux.c:30`) leería
`/proc/<pgrp>/cmdline` y devolvería el título del server,
**cuando** no se pasa `-n`,
**entonces** el nombre de la ventana es el destino tal como se escribió.

**VC-19.1:** `tmux new-ssh-window -d -P -F '#{window_name}' $D` imprime `$USER@127.0.0.1`.

#### FR-20 · El nombre de la ventana no cambia solo

**Dado** que fijar `sc.name` apaga `automatic-rename` (`spawn.c:216-224`),
**cuando** pasan 5 s desde que se creó la ventana,
**entonces** el nombre sigue siendo el que tenía.

**VC-20.1:** tras `new-ssh-window -d -P -F '#{window_id}' $D` y `sleep 5`, `display-message -p -t <window_id> '#{window_name}'` da `$USER@127.0.0.1`.

#### FR-21 · `-c` no existe

**Dado** que `-c` no figura en `.args` (un directorio local no aplica a un destino remoto),
**cuando** se pasa `-c`,
**entonces** es un error de uso: el parser lo rechaza (`arguments.c:240`).

**VC-21.1:** `tmux new-ssh-window -c /tmp $D` sale con `1`, su stderr es `command new-ssh-window: unknown flag -c` y `list-windows | wc -l` no cambia.

#### FR-22 · `-e` no existe

**Dado** que `-e` no figura en `.args` (un entorno local no aplica a un destino remoto),
**cuando** se pasa `-e`,
**entonces** es un error de uso: el parser lo rechaza (`arguments.c:240`).

**VC-22.1:** `tmux new-ssh-window -e A=b $D` sale con `1`, su stderr es `command new-ssh-window: unknown flag -e` y `list-windows | wc -l` no cambia.

#### FR-23 · Un destino mal formado es un error de uso

**Dado** un destino que no cumple `[usuario@]host` (usuario y host no vacíos, sin `@` ni espacios internos),
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `invalid destination '<destino>'`.

**VC-23.1:** `tmux new-ssh-window 'u@'` sale con `1`, su stderr es `invalid destination 'u@'` y `list-windows | wc -l` no cambia.
**VC-23.2:** `tmux new-ssh-window ''` sale con `1` y su stderr es `invalid destination ''`.
**VC-23.3:** `tmux new-ssh-window '@h'` sale con `1` y su stderr es `invalid destination '@h'`.
**VC-23.4:** `tmux new-ssh-window 'a@b@c'` sale con `1` y su stderr es `invalid destination 'a@b@c'`.
**VC-23.5:** `tmux new-ssh-window 'a b'` sale con `1` y su stderr es `invalid destination 'a b'`.
#### FR-24 · Un puerto inválido es un error de uso

**Dado** un valor de `-p` que no es un entero entre 1 y 65535,
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `invalid port '<valor>'`.

**VC-24.1:** `tmux new-ssh-window -p abc $D` sale con `1` y su stderr es `invalid port 'abc'`.
**VC-24.2:** con `-p 0` el stderr es `invalid port '0'`; con `-p 65536`, `invalid port '65536'`; con `-p 65535` el comando no falla.


#### FR-25 · Una ruta de clave no absoluta es un error de uso

**Dado** un valor de `-i` que no empieza con `/` (incluye `~/k`, que no se expande),
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `key file path must be absolute: '<ruta>'`.

**VC-25.1:** `tmux new-ssh-window -i rel/k $D` sale con `1` y su stderr es `key file path must be absolute: 'rel/k'`.
**VC-25.2:** `tmux new-ssh-window -i '~/k' $D` sale con `1` y su stderr es `key file path must be absolute: '~/k'`.
#### FR-26 · Un archivo de clave inexistente es un error de uso

**Dado** un valor de `-i` que no existe,
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `key file '<ruta>' does not exist`.

**VC-26.1:** `tmux new-ssh-window -i /nonexistent/k $D` sale con `1`, su stderr es `key file '/nonexistent/k' does not exist` y `list-windows | wc -l` no cambia.

#### FR-27 · Un archivo de clave sin permiso de lectura es un error de uso

**Dado** un valor de `-i` que existe con modo `000`,
**cuando** se invoca el comando (como usuario no root),
**entonces** es un error de uso con el mensaje `cannot read key file '<ruta>'`.

**VC-27.1:** con `touch $H/k && chmod 000 $H/k`, `tmux new-ssh-window -i $H/k $D` sale con `1` y su stderr es `cannot read key file '<H>/k'`.

#### FR-28 · Con varios errores de uso, se informa solo el primero del orden fijo

**Dado** el orden fijo: (1) flag desconocido, (2) cantidad de destinos, (3) destino mal formado, (4) puerto inválido, (5) ruta de clave no absoluta, (6) archivo de clave inexistente, (7) archivo de clave ilegible,
**cuando** el comando tiene más de un error de uso a la vez,
**entonces** informa únicamente el de menor número.

**VC-28.1:** `tmux new-ssh-window -c /tmp -p abc 'u@'` imprime en stderr solo `command new-ssh-window: unknown flag -c`.
**VC-28.2:** `tmux new-ssh-window -p abc -i /nonexistent/k 'u@'` imprime en stderr solo `invalid destination 'u@'`.
**VC-28.3:** `tmux new-ssh-window -p abc -i /nonexistent/k $D` imprime en stderr solo `invalid port 'abc'`.
**VC-28.4:** `tmux new-ssh-window -i rel/k $D` imprime en stderr solo `key file path must be absolute: 'rel/k'`, y `grep -c 'does not exist'` sobre esa salida da `0`.
### Enganche en el spawn

#### FR-29 · El pane SSH es un hijo directo del server con un pty

**Dado** el comando de FR-10, que llama a `spawn_window()` (`tmux.h:4195`) con `SPAWN_SSH` en `sc.flags` y crea el pane con el mismo `fdforkpty` (`spawn.c:478`) que cualquier otro,
**cuando** se ejecuta con argumentos válidos,
**entonces** el pane es un hijo directo del server con un tty `/dev/pts/N`.

**VC-29.1:** tras `new-ssh-window -d -P -F '#{pane_pid} #{pane_tty}' $M` (el hijo queda vivo contra el listener mudo), el primer campo es un pid cuyo padre es el server (`ps -o ppid= -p <pane_pid>` igual a `display-message -p '#{pid}'`) y el segundo es un `/dev/pts/N` que existe.

#### FR-30 · En el hijo, `ssh_client_run()` ocupa el lugar de los `exec`

**Dado** un pane creado con `SPAWN_SSH`,
**cuando** el hijo llega al punto posterior a `environ_push` (`spawn.c:544`),
**entonces** el proceso del pane sigue siendo el binario `tmux`: los `exec` de `spawn.c:552-574` no se ejecutan.

**VC-30.1:** con el hijo colgado contra el listener mudo (`new-ssh-window -d $M`), `readlink /proc/<pane_pid>/exe` es igual a `readlink /proc/<pid del server>/exe`.

#### FR-31 · Contrato de `argv` entre el comando y el cliente

**Dado** el `argv` que el comando deja en el `spawn_context` (y que `spawn_pane` copia a `wp->argv`),
**cuando** se arma con destino, `-p` e `-i`,
**entonces** `argv` es `destino [-p PUERTO] [-i ARCHIVO]`, en ese orden.

**VC-31.1:** `tmux new-ssh-window -d -P -F '#{pane_start_command}' -i /tmp/k -p 2222 u@h` imprime `u@h -p 2222 -i /tmp/k` (`pane_start_command` stringifica `wp->argv`, `format.c:901-908`). *(Con `/tmp/k` existente y legible; si no, el comando falla antes por FR-26.)*

#### FR-32 · El puerto es el de `-p`

**Dado** el `argv` de FR-31 con `-p $PORT`,
**cuando** el cliente abre la conexión TCP,
**entonces** conecta a `$PORT`.

**VC-32.1:** con el `sshd` de prueba en `$PORT` distinto de 22, `new-ssh-window -d -p $PORT $D` llega a `sshd`.

#### FR-33 · Sin `-p`, el puerto es 22

**Dado** el `argv` de FR-31 sin `-p`,
**cuando** el cliente abre la conexión TCP,
**entonces** conecta al puerto 22.

**VC-33.1:** tras `new-ssh-window -d 127.0.0.1`, `$TRACE` contiene un `connect` del pid del pane con `sin_port=htons(22)`.

### Verificación del host

#### FR-34 · Un host desconocido se rechaza

**Dado** un destino sin entrada en `$HOME/.ssh/known_hosts` (un archivo inexistente cuenta como un `known_hosts` sin entradas),
**cuando** se intenta conectar,
**entonces** el pane muestra el mensaje `new-ssh-window: host key for <host> not found in known_hosts`.

**VC-34.1:** con `$H/.ssh/known_hosts` vacío, `new-ssh-window -d -p $PORT $D` y espera al pane: `capture-pane -p | grep -c 'new-ssh-window: host key for 127.0.0.1 not found in known_hosts'` da `1`.
**VC-34.2:** con `rm $H/.ssh/known_hosts`, el mismo comando muestra el mismo mensaje.
**VC-34.3:** con `printf 'otrohost ssh-ed25519 AAAA' > $H/.ssh/known_hosts` (una línea de otro host, **sin** `\n` final), el mismo comando muestra el mismo mensaje.

#### FR-35 · Un host desconocido no modifica `known_hosts`

**Dado** el mismo caso de FR-34,
**cuando** el hijo termina,
**entonces** `$HOME/.ssh/known_hosts` queda sin cambios.

**VC-35.1:** `stat -c %s $H/.ssh/known_hosts` da `0` antes y después, y su `sha256sum` es el mismo.

#### FR-36 · Un host con la clave cambiada se rechaza

**Dado** un destino con una host key **distinta** de la registrada en `$HOME/.ssh/known_hosts`,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: host key for <host> has changed`.

**VC-36.1:** con otra clave registrada para `[127.0.0.1]:$PORT`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p | grep -c 'new-ssh-window: host key for 127.0.0.1 has changed'` da `1`.

#### FR-37 · Un `known_hosts` ilegible se informa

**Dado** un `$HOME/.ssh/known_hosts` que existe con modo `000` (único sentido de "corrupto" en esta spec; las líneas malformadas están fuera de alcance),
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot read <ruta>`.

**VC-37.1:** con `chmod 000 $H/.ssh/known_hosts`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p | grep -c "new-ssh-window: cannot read $H/.ssh/known_hosts"` da `1`.

### Autenticación

#### FR-38 · Autenticación por agent

**Dado** un host conocido y `SSH_AUTH_SOCK` en el entorno de la **sesión** (llega al hijo
por `environ_for_session`, `environ.c:253`, y `environ_push`, `spawn.c:544`),
**cuando** se conecta sin `-i`,
**entonces** se autentica con una clave del agent.

**VC-38.1:** con `set-environment SSH_AUTH_SOCK <socket>` de un agent que tiene la clave autorizada, y `$H/.ssh` sin ninguna clave, `new-ssh-window -d -p $PORT $D` se autentica.

#### FR-39 · Autenticación con la clave de `-i`

**Dado** un host conocido, sin agent,
**cuando** se pasa `-i ARCHIVO`,
**entonces** se autentica con esa clave.

**VC-39.1:** sin `SSH_AUTH_SOCK` en el entorno de la sesión, con la clave autorizada en `/tmp/<dir>/k` (fuera de `$H/.ssh`), `new-ssh-window -d -p $PORT -i /tmp/<dir>/k $D` se autentica.

#### FR-40 · Autenticación con las claves por defecto

**Dado** un host conocido, sin agent y sin `-i`,
**cuando** se conecta,
**entonces** se prueban, en este orden, `$HOME/.ssh/id_ed25519`, `$HOME/.ssh/id_rsa`.

**VC-40.1:** con la clave autorizada en `$H/.ssh/id_ed25519`, sin `SSH_AUTH_SOCK` y sin `-i`, `new-ssh-window -d -p $PORT $D` se autentica.
**VC-40.2:** con la clave autorizada solo en `$H/.ssh/id_rsa` (sin `id_ed25519`), sin `SSH_AUTH_SOCK` y sin `-i`, se autentica y `$SSHD_LOG` contiene `Accepted publickey` con el fingerprint de `id_rsa`.


#### FR-41 · Una clave por defecto ilegible se salta

**Dado** un host conocido, sin agent ni `-i`, con `$HOME/.ssh/id_ed25519` de modo `000` y una `$HOME/.ssh/id_rsa` autorizada,
**cuando** se conecta (como usuario no root),
**entonces** se autentica con `id_rsa`.

**VC-41.1:** con `chmod 000 $H/.ssh/id_ed25519`, `new-ssh-window -d -p $PORT $D` se autentica y `$SSHD_LOG` contiene `Accepted publickey` con el fingerprint de `id_rsa`.
#### FR-42 · El agent se prueba antes que los archivos de clave

**Dado** un agent con la clave A y un `-i` con la clave B, ambas autorizadas,
**cuando** se conecta,
**entonces** la sesión se autentica con A.

**VC-42.1:** `$SSHD_LOG` contiene `Accepted publickey` con el fingerprint de A y ninguna línea `Accepted` con el de B.

#### FR-43 · La passphrase se pide en el tty del pane

**Dado** una clave protegida con passphrase y sin agent,
**cuando** el cliente la necesita,
**entonces** escribe `Enter passphrase for key '<ruta>': ` en el tty del pane (el pty de `fdforkpty`, `spawn.c:478`).

**VC-43.1:** `capture-pane -p | grep -c "Enter passphrase for key '$H/.ssh/id_ed25519': "` da `1`; tras `send-keys '<passphrase>' Enter` la sesión se autentica.

#### FR-44 · La passphrase tecleada no se muestra

**Dado** el prompt de FR-43,
**cuando** se teclea la passphrase,
**entonces** no se escribe en el pane (sin eco).

**VC-44.1:** tras `send-keys 'S3cretoX' Enter`, `capture-pane -p | grep -c S3cretoX` da `0`.

#### FR-45 · Una passphrase incorrecta se informa

**Dado** una clave protegida con passphrase,
**cuando** se teclea una passphrase incorrecta,
**entonces** el pane muestra `new-ssh-window: wrong passphrase for key '<ruta>'`.

**VC-45.1:** tras `send-keys 'mala' Enter`, `capture-pane -p | grep -c "new-ssh-window: wrong passphrase for key '$H/.ssh/id_ed25519'"` da `1`.

#### FR-46 · No se ofrece autenticación por password

**Dado** un servidor que solo acepta `password`,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: no supported authentication method`.

**VC-46.1:** con el `sshd` de prueba en `AuthenticationMethods password`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p | grep -c 'new-ssh-window: no supported authentication method'` da `1` y `capture-pane -p | grep -ci assword` da `0` (no se pidió nada).

#### FR-47 · Sin ninguna clave disponible se informa

**Dado** un host conocido, sin agent, sin `-i` y sin `id_ed25519` ni `id_rsa` en `$HOME/.ssh`,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: no authentication key found`.

**VC-47.1:** con `$H/.ssh` solo con `known_hosts`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p | grep -c 'new-ssh-window: no authentication key found'` da `1`.

#### FR-48 · Una clave rechazada por el servidor se informa

**Dado** una clave legible que el servidor **no** tiene autorizada,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: authentication failed for <usuario>@<host>`.

**VC-48.1:** con `-i` apuntando a una clave no listada en `authorized_keys`, `capture-pane -p | grep -c "new-ssh-window: authentication failed for $USER@127.0.0.1"` da `1`.


#### FR-49 · Un archivo que no es una clave se informa

**Dado** un valor de `-i` que existe, es legible y no es una clave privada (por ejemplo, un archivo de 0 bytes),
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: cannot load key '<ruta>'`.

**VC-49.1:** con `: > $H/empty`, tras `new-ssh-window -d -p $PORT -i $H/empty $D` y esperar al pane, `capture-pane -p | grep -c "new-ssh-window: cannot load key '$H/empty'"` da `1`.
### Sesión interactiva

#### FR-50 · El pty remoto usa el `TERM` del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con el `TERM` del entorno del hijo (el `default-terminal` que pone
`environ_for_session`, `environ.c:265`).

**VC-50.1:** tras `send-keys 'echo T=$TERM' Enter`, `capture-pane -p` contiene `T=` seguido del valor de `show-options -gv default-terminal`.

#### FR-51 · El pty remoto arranca con el tamaño del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con el tamaño (filas por columnas) del tty del pane (`TIOCGWINSZ` sobre el tty local).

**VC-51.1:** tras `send-keys 'stty size' Enter`, `capture-pane -p` contiene `<pane_height> <pane_width>` con los valores de `display-message -p '#{pane_height} #{pane_width}'`.

#### FR-52 · Lo tecleado en el pane llega a la shell remota

**Dado** el pty remoto con una shell,
**cuando** se teclea en el pane,
**entonces** los bytes llegan a la shell remota.

**VC-52.1:** `send-keys 'echo L > $H/sent' Enter` crea `$H/sent` con contenido `L` (la shell remota corre en la misma máquina de prueba).

#### FR-53 · La salida de la shell remota aparece en el pane

**Dado** el pty remoto con una shell,
**cuando** la shell remota escribe,
**entonces** el pane muestra esos bytes.

**VC-53.1:** `send-keys 'echo R-$((6*7))' Enter` seguido de `capture-pane -p` muestra `R-42`, producido por el host remoto (prompt remoto alcanzado).

#### FR-54 · El tty local en raw no duplica el eco

**Dado** que la disciplina de línea local duplicaría el eco, y que `cfmakeraw` (`compat.h:426`, usado igual en `client.c:349`) la apaga,
**cuando** se teclea un comando,
**entonces** el pane muestra el eco una sola vez.

**VC-54.1:** tras `send-keys 'echo ok' Enter`, `capture-pane -p | grep -c 'echo ok'` da `1`.

#### FR-55 · `Ctrl-C` llega al proceso remoto

**Dado** el tty local en raw y un `sleep 100` remoto en curso,
**cuando** se envía `Ctrl-C`,
**entonces** el prompt remoto vuelve en menos de **2 s**.

**VC-55.1:** `send-keys 'sleep 100' Enter`, luego `send-keys C-c` y `send-keys 'echo P-$((6*7))' Enter`: la línea `P-42` aparece en `capture-pane -p` menos de 2 s después de `C-c`.
#### FR-56 · El resize llega al remoto

**Dado** una sesión abierta,
**cuando** cambia el tamaño del pane (`window_pane_send_resize`, `window.c:597`, hace el
`ioctl(TIOCSWINSZ)` y el hijo recibe `SIGWINCH`),
**entonces** el cliente actualiza el tamaño del pty remoto.

**VC-56.1:** tras `resize-window -x 100 -y 30`, `send-keys 'stty size' Enter` muestra `30 100`.

### Fin de sesión

#### FR-57 · El hijo sale con el código de salida remoto

**Dado** una sesión abierta,
**cuando** la shell remota termina,
**entonces** el hijo sale con ese código.

El pane sigue el camino habitual (`SIGCHLD` → `server_child_signal`, `server.c:468` → `server_child_exited` → `PANE_EXITED`); no se agrega ninguna ruta de destrucción.

**VC-57.1:** `send-keys 'exit 3' Enter` deja `#{pane_dead_status}` en `3`.

#### FR-58 · Si la conexión se corta, el pane lo informa

**Dado** una sesión abierta,
**cuando** la conexión se pierde sin código de salida remoto,
**entonces** el pane muestra `new-ssh-window: connection lost`.

**VC-58.1:** tras `kill -9 $(pgrep -P $SSHD_PID)` (el proceso `sshd` de la sesión, hijo del `sshd` de prueba), `capture-pane -p | grep -c 'new-ssh-window: connection lost'` da `1`.

#### FR-59 · Cerrar el pane termina al hijo

**Dado** una sesión abierta,
**cuando** el pane se cierra desde `tmux` (el cierre del master le llega al hijo como `SIGHUP`),
**entonces** el hijo termina en menos de **2 s**.

**VC-59.1:** tras `kill-pane`, `kill -0 <pane_pid>` falla en menos de 2 s.
### Fallas de conexión

#### FR-60 · Un host que no resuelve se informa

**Dado** un host que no resuelve,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot connect to <host>: name resolution failed`.

**VC-60.1:** `new-ssh-window -d nohost.invalid` y espera al pane: `capture-pane -p | grep -c 'new-ssh-window: cannot connect to nohost.invalid: name resolution failed'` da `1`.

#### FR-61 · Un puerto cerrado se informa

**Dado** un host que resuelve y un puerto sin listener,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot connect to <host>: connection refused`.

**VC-61.1:** `new-ssh-window -d -p $FREE 127.0.0.1`: `capture-pane -p | grep -c 'new-ssh-window: cannot connect to 127.0.0.1: connection refused'` da `1`.

#### FR-62 · La conexión tiene un timeout de 15 s

**Dado** un destino que acepta la conexión TCP pero no completa el intercambio de claves,
**cuando** pasan **15 s** desde que el cliente arrancó,
**entonces** el pane muestra `new-ssh-window: connection to <host> timed out`.

**VC-62.1:** contra el listener mudo, `#{pane_dead}` sigue en `0` a los 14 s y es `1` antes de los 20 s; `capture-pane -p | grep -c 'new-ssh-window: connection to 127.0.0.1 timed out'` da `1`.

### Reglas y no funcionales

#### BR-1 · No se registran secretos

Passphrases, contenido de claves y respuestas del agent **nunca** llegan al log de `tmux`.
El hijo ya cerró el log (`log_close`, `spawn.c:543`) antes del bloque de FR-30.

**VC-BR1.1:** con `tmux -vv` (el log registra los argumentos de cada comando, `cmd.c:249`: por eso la passphrase no se teclea con `send-keys`), en el prompt de VC-43.1 se inyecta `S3cretoX` con `printf 'S3cretoX\n' > $H/pw; tmux load-buffer $H/pw; tmux paste-buffer -t <pane>`; `grep -c S3cretoX tmux-server-*.log tmux-client-*.log` da `0` en cada archivo.
#### BR-2 · Sin binding por default

El comando no se asocia a ninguna tecla.

**VC-BR2.1:** `tmux -f/dev/null list-keys | grep -c new-ssh-window` da `0`.

#### BR-3 · Toda falla del cliente sale con estado 1

Cada falla del cliente (ver glosario) termina el hijo con estado `1`, visible en
`#{pane_dead_status}` (`format_cb_pane_dead_status`, `format.c:2288`, devuelve `WEXITSTATUS(wp->status)`, que `server_child_exited` guarda en `wp->status`, `server.c:499`).

**VC-BR3.1:** `new-ssh-window -d nohost.invalid` deja `#{pane_dead_status}` en `1` (FR-60).
**VC-BR3.2:** con `$H/.ssh/known_hosts` vacío, `#{pane_dead_status}` es `1` (FR-34).
**VC-BR3.3:** con `-i` no autorizada, `#{pane_dead_status}` es `1` (FR-48).
**VC-BR3.4:** contra el listener mudo, a los 16 s `#{pane_dead_status}` es `1` (FR-62).


#### BR-4 · Los VCs con el flag corren en el CI de Linux

`.github/workflows/regress.yml` tiene una entrada de matriz solo Linux con `--enable-native-ssh` y `TEST_REQUIRE_SSHD=1`, de modo que los VCs de red no se saltean en silencio.

**VC-BR4.1:** `grep -c -- '--enable-native-ssh' .github/workflows/regress.yml` da `1` y `grep -c 'TEST_REQUIRE_SSHD=1' .github/workflows/regress.yml` da `1`.
#### NFR-1 · El server responde mientras una conexión está pendiente

Con **un** pane SSH colgado contra el listener mudo, `display-message -p ok` desde otro cliente responde en menos de **1 s**, medido con el reloj de pared (`date +%s%N` antes y después), en loopback y con el server **sin** `strace`. El server no hace I/O de red: toda la conexión ocurre en el hijo (FR-30).

**VC-NFR1.1:** con un `new-ssh-window -d $M` colgado y el server sin `strace`, `t0=$(date +%s%N); tmux display-message -p ok; t1=$(date +%s%N)` imprime `ok` y `t1 - t0` es menor que `1000000000` ns.
#### NFR-2 · Documentación

`tmux.1`, renderizado con `man -l tmux.1`, contiene **exactamente una** sinopsis de `new-ssh-window` igual al `.usage` de FR-12 y **al menos una** mención de `--enable-native-ssh` (el man page se instala siempre, así que aclara que el comando solo existe con ese flag).

**VC-NFR2.1:** `man -l tmux.1 | col -b | grep -c 'new-ssh-window \[-dP\] \[-F format\] \[-i identity-file\] \[-n window-name\] \[-p port\] \[-t target-window\] destination'` da `1` y `man -l tmux.1 | col -b | grep -c 'enable-native-ssh'` da al menos `1`.
#### NFR-3 · La salida masiva no se atasca

Con `history-limit 250000`, una sesión SSH en loopback, un pane y el server **sin** `strace`, `seq 1 200000` termina en menos de **30 s** de reloj de pared (`date +%s%N`) y el pane muestra los 200000 números, cada uno **una sola vez**.

**VC-NFR3.1:** `t0=$(date +%s%N)`, `send-keys 'seq 1 200000' Enter` y espera de la línea `200000`: `t1 - t0` es menor que `30000000000` ns; `capture-pane -p -S - | grep -cx '[0-9]\+'` da `200000` y `capture-pane -p -S - | grep -x '[0-9]\+' | sort -n | uniq -d | wc -l` da `0`.
## VCs de invariantes

Cada invariante tiene acá su VC, con el mismo peso que los de los FR.

**VC-INV1.1:** en Linux, build **sin** el flag: `ldd ./tmux | grep -c libssh` da `0`;
`nm ./tmux | grep -c ssh_client_run` da `0`; y
`./tmux -f/dev/null list-commands | diff - regress/list-commands.base` no imprime nada. Lo ejecuta
`regress/native-ssh-guard.sh` en los jobs de Linux sin flag.

**VC-INV2.1:** en el job `macos-26-arm64` de `regress.yml`, **sin tocarlo** (sigue con
`--enable-utf8proc --enable-asan` y sin `libssh`): el step `build` termina con `0`, y
`regress/native-ssh-guard.sh` comprueba, cuando `uname -s` no es `Linux`, que
`$TEST_TMUX list-commands | grep -c new-ssh-window` da `0` y que
`nm $TEST_TMUX | grep -c ssh_client_run` da `0`.

**VC-INV3.1:** en Linux, build **con** el flag:
`./tmux -f/dev/null list-commands | grep -v '^new-ssh-window' | diff - regress/list-commands.base`
no imprime nada (los 92 comandos existentes conservan nombre, alias y uso), y
`list-commands | wc -l` da `93`.

**VC-INV3.2:** `(cd regress && make)` termina con `0` con y sin el flag: los scripts
que estaban en `PASS` en la línea de base (171 en el entorno medido), incluido `new-window-command.sh` (crea cuatro ventanas con `new-window`).

**VC-INV4.1:** `git diff --stat 5a820e63` **no lista** `window.c`, `server.c`,
`server-fn.c`, `job.c`, `format.c`, `input.c`, `cmd-find.c` ni `osdep-*.c`, y
`git diff --numstat 5a820e63 -- spawn.c cmd.c tmux.h` muestra **0 líneas eliminadas** en los tres.

**VC-INV4.2:** en el build **con** el flag, un pane común sigue por el camino de siempre:
tras `new-window -d -P -F '#{pane_pid} #{pane_tty}' sleep 100`,
`readlink /proc/<pane_pid>/exe` termina en `/sleep` (pasó por `execvp`, `spawn.c:552`), el
tty es un `/dev/pts/N`, y `kill <pane_pid>` deja `#{pane_dead_signal}` en `15`.

**VC-INV5.1:** con el hijo colgado contra el listener mudo (`new-ssh-window -d $M`), `$TRACE` no contiene ninguna línea `execve` del pid del pane y `pgrep -P <pane_pid>` no devuelve nada.

**VC-INV5.2:** con la sesión abierta de VC-53.1 (el prompt remoto alcanzado), `$TRACE` sigue sin ninguna línea `execve` del pid del pane y `pgrep -P <pane_pid>` sigue sin devolver nada.

**VC-INV6.1:** `ls /proc/<pid del server>/task | wc -l` da el mismo número antes de crear el pane SSH y con la conexión colgada de VC-NFR1.1.

**VC-INV6.2:** `ls /proc/<pid del server>/task | wc -l` da el mismo número con la sesión abierta de VC-53.1.

**VC-INV7.1:** `grep -c 'define SPAWN_SSH 0x2000' tmux.h` da `1`, y
`grep -o 'SPAWN_[A-Z]* 0x[0-9a-f]*' tmux.h | awk '{print $2}' | sort | uniq -d` no imprime nada.

## Matriz de cobertura

| Requisito | VC | Camino de falla incluido |
|---|---|---|
| FR-1 … FR-9 | VC-1.1 … VC-9.1 | VC-3.1 (sin `libssh`), VC-4.1 (versión vieja), VC-5.1 (no-Linux) |
| FR-10, FR-11 | VC-10.1, VC-11.1 | — |
| FR-12, FR-13 | VC-12.1, VC-13.1 | sin destino, dos destinos |
| FR-14 … FR-20 | VC-14.1 … VC-20.1 | — |
| FR-21, FR-22 | VC-21.1, VC-22.1 | flags rechazados |
| FR-23 … FR-27 | VC-23.1 … VC-27.1 | destino mal formado, puerto inválido, ruta de clave no absoluta, clave inexistente o ilegible |
| FR-28 | VC-28.1 … VC-28.4 | precedencia de errores de uso |
| FR-29 … FR-33 | VC-29.1 … VC-33.1 | — |
| FR-34 … FR-37 | VC-34.1 … VC-37.1 | host desconocido (también sin archivo y sin `\n` final), host cambiado, `known_hosts` ilegible |
| FR-38 … FR-44 | VC-38.1 … VC-44.1 | FR-41: clave por defecto ilegible, la siguiente se usa |
| FR-45 … FR-49 | VC-45.1 … VC-49.1 | passphrase incorrecta, sin password, sin clave, clave rechazada, archivo que no es clave |
| FR-50 … FR-57 | VC-50.1 … VC-57.1 | — |
| FR-58, FR-59 | VC-58.1, VC-59.1 | conexión cortada, pane cerrado |
| FR-60 … FR-62 | VC-60.1 … VC-62.1 | host inexistente, puerto cerrado, sesión que no abre |
| BR-1 … BR-4 | VC-BR1.1 … VC-BR4.1 | exit `1` de cuatro fallas (BR-3) |
| NFR-1 … NFR-3 | VC-NFR1.1 … VC-NFR3.1 | — |
| INV-1 … INV-7 | VC-INV1.1 … VC-INV7.1 | build no-Linux (INV-2) |

## Plan de iteraciones

Cada iteración termina con la línea de base **sin** el flag igual a la inicial (VC-INV1.1, VC-INV3.2).

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Guardas de build: `configure.ac`, `Makefile.am` y `regress/native-ssh-guard.sh`; las dos fuentes nuevas con `ssh_client_run()` como esqueleto que devuelve `1` | FR-1 … FR-9, INV-1, INV-2 |
| **2** | Comando, validación de argumentos, `SPAWN_SSH`, bloque en `spawn.c`, `ssh_client_run()` que solo conecta; **harness con `sshd` de prueba y listener mudo** | FR-10 … FR-33, FR-60 … FR-62, NFR-1, INV-3, INV-4, INV-7 |
| **3** | Host key y autenticación | FR-34 … FR-49, BR-1, BR-3 |
| **4** | Sesión interactiva, resize y fin de sesión | FR-50 … FR-59, NFR-3, INV-5, INV-6 |
| **5** | `tmux.1` y entrada de CI | NFR-2, BR-2, BR-4 |

La Iteración 1 es el camino más angosto que se puede verificar solo: el esqueleto de
`ssh_client_run()` basta para VC-7.1 y la prueba del límite solo-Linux completo no necesita
lógica de red. El `sshd` de prueba aparece en la Iteración 2, no al final, para que ningún VC
de red quede sin forma de ejercitarse.

Cada VC se puede correr en la iteración que lo cierra. Los de la Iteración 2 no dependen de
autenticar: observan la llegada a `sshd` en su log (VC-32.1) o un hijo que sigue vivo contra
el listener mudo (VC-29.1, VC-30.1, VC-NFR1.1). Los de la Iteración 3 observan
`Accepted publickey` en el log de `sshd` o un mensaje del pane, no el prompt remoto, que
necesita la shell de la Iteración 4. VC-INV5.1 y VC-INV6.1 pueden correrse ya en la
Iteración 2, con la conexión colgada; INV-5 e INV-6 cierran en la 4 porque VC-INV5.2 y
VC-INV6.2 piden la sesión abierta. VC-BR3.1 y VC-BR3.4 pueden correrse en la 2; VC-BR3.2 y
VC-BR3.3 se cierran en la 3.

## Decisiones

Cada decisión dice qué se eligió, qué código de `tmux` (`5a820e63`) la funda, qué se
descartó y por qué, y en qué FR queda escrita.

| # | Pregunta | Elegido | Fundamento (código de `tmux`) | Descartado y por qué | FR |
|---|---|---|---|---|---|
| 1 | ¿Comando nuevo, y dónde se registra? | `new-ssh-window`, alias `sshw`, en `cmd-new-ssh-window.c`, registrado en `cmd_table[]` de `cmd.c` bajo `#ifdef` | `struct spawn_context` (`tmux.h:2500-2532`) no tiene campo "tipo de pane" y `new-window` trata todo posicional como comando a ejecutar (`.args` con `0, -1`, `cmd-new-window.c:41`). Todo comando se registra en `cmd_table[]` (`cmd.c:123`), que hoy no tiene ningún `#if` | Un flag `-S` en `new-window`: obliga a tocar un comando existente y su `.args` (INV-3). Un prefijo `ssh:` en el comando de `new-window`: mezcla significados en un argumento que hoy es "ejecutar" | FR-10, FR-11 |
| 2 | ¿Dónde engancha en el spawn? | **En el hijo**, tras `fdforkpty` y `environ_push`, donde hoy están los `exec` | `spawn.c:478` deja `pid`, `fd` y `tty` reales; `window_pane_send_resize` hace `fatal` si el `ioctl(TIOCSWINSZ)` falla (`window.c:612-622`), así que el fd tiene que seguir siendo un pty; `server_child_exited` busca el pane por `wp->pid` (`server.c:491-498`). Ya hay código opcional en ese tramo del hijo (`spawn.c:504`) | Un pane con fd de socket y sin hijo: rompe `window_pane_send_resize` y `server_child_exited` (modelo de pane, INV-4). Engancharse en el server: bloquea (decisión 4) | FR-29, FR-30 |
| 3 | ¿`libssh` u OpenSSH? | **`libssh >= 0.9.0`**, enlazada | (a) La única forma que tiene `tmux` de usar OpenSSH es ejecutar su binario por los tres `exec` de `spawn.c:552`, `:567` y `:574`: lo que hoy hace `new-window ssh host` y lo que INV-5 prohíbe. (b) `configure.ac` detecta cada librería opcional con `PKG_CHECK_MODULES` sobre un `.pc` (`libsystemd` en `:508`, `libutf8proc` en `:463`, `jemalloc` en `:670`). (c) El hijo recibe solo los fds 0-2 (`closefrom(STDERR_FILENO + 1)`, `spawn.c:541`): la librería abre su propio socket y convive con el tty en un bucle propio. (d) No hay código SSH que reusar en el árbol. El mínimo 0.9.0 es el de la API de lectura de `known_hosts` `ssh_session_is_known_server`, que en 0.9.0 reemplaza a la ya deprecada `ssh_is_server_known` (la spec nunca escribe `known_hosts`, FR-35) | OpenSSH por `exec`: viola INV-5. Escribir el protocolo a mano: miles de líneas sin base en el árbol. `libssh` sin mínimo: obligaría a soportar la API deprecada | FR-2, FR-3, FR-4, FR-30 |
| 4 | ¿Cómo se integra con el event loop? | **No se integra**: el cliente corre en el hijo con su propio bucle | El server es de un solo hilo: la base de libevent se crea en `osdep_event_init` (`osdep-linux.c:92`, llamada en `tmux.c:624`), el server la reusa con `event_reinit` (`server.c:198`) y entra a `proc_loop` (`server.c:258`), que llama a `event_loop(EVLOOP_ONCE)` (`proc.c:227`) | Handshake dentro del bucle del server: congela a todos los clientes. Un hilo en el server: viola INV-6 y `tmux` no usa hilos | FR-30, NFR-1 |
| 5 | ¿Auth por claves o por agent? | **Ambas**, agent primero; sin password | `update-environment` incluye `SSH_AUTH_SOCK` y `SSH_AGENT_PID` por default (`options-table.c:1207-1214`), `environ_for_session` copia el entorno de la sesión (`environ.c:253-261`) y `environ_push` lo instala en el hijo antes del bloque nuevo (`spawn.c:544`): el agent llega sin código nuevo, y por eso va primero. La passphrase se puede pedir porque el hijo tiene el pty de `spawn.c:478` como stdin | Password y keyboard-interactive: `tmux` no propaga ningún mecanismo de password, y dejarlos fuera acota los secretos que pasan por el pane a uno solo. Solo agent: no cubre a quien usa clave en archivo | FR-38 … FR-49 |
| 6 | ¿Qué guarda deja afuera a no-Linux? | **Opt-in** `--enable-native-ssh`, solo Linux, con `AM_CONDITIONAL` y `#ifdef` | `tmux` no usa `#ifdef __linux__` (cero ocurrencias) y sí usa flags de feature: `ENABLE_SIXEL` (`configure.ac:545-552`, `Makefile.am:253-255`) agrega fuentes propias solo con el flag, y `HAVE_SYSTEMD` (`configure.ac:503-527`, `Makefile.am:243-245`) hace lo mismo con detección por `pkg-config`. `PLATFORM` sale del `case "$host_os"` de `configure.ac:1006-1118` | `#ifdef __linux__`: no hay precedente en el árbol. Activar el feature solo por detectar `libssh`: un Linux con `libssh-dev` instalado cambiaría de binario sin pedirlo (viola INV-1) | FR-1 … FR-9 |

Decisiones menores:

| Pregunta | Elegido | Fundamento | Descartado y por qué | FR |
|---|---|---|---|---|
| ¿Ventana o split? | **Ventana** | reusa `spawn_window` (`spawn.c:209` llama a `spawn_pane`) y evita el manejo de layout de `cmd-split-window.c` | Split: 424 líneas contra 196 de `new-window` por el layout | FR-29 |
| ¿Qué flags acepta? | `-d -F -n -P -t` como `new-window`, más `-i` y `-p` | `.args` de `cmd-new-window.c:41` y sus usos en `:46`, `:83`, `:140`, `:156-157`, `:172-173`; `-i` y `-p` son los únicos datos propios de SSH | `-c` y `-e`: un directorio o entorno locales no aplican a un destino remoto (`arguments.c:240` los rechaza). `-l usuario`, `-o`: `usuario@host` ya lo cubre | FR-14 … FR-22 |
| ¿Host desconocido? | **Se rechaza** | un pane puede crearse con `-d`, sin nadie mirando. El hijo recibe solo los fds 0-2 (`closefrom(STDERR_FILENO + 1)`, `spawn.c:541`): su único canal con el usuario es el tty del pane, y una pregunta ahí quedaría colgada sin un cliente adjunto | TOFU con pregunta: cuelga el pane. TOFU silencioso: acepta man-in-the-middle sin que nadie lo vea | FR-34 |
| ¿Qué estado de salida tienen las fallas? | **`1`** para todas | `server_child_exited` guarda el status en `wp->status` (`server.c:499`) y `format_cb_pane_dead_status` lo expone (`format.c:2288`): ningún otro componente interpreta el valor | Un código por causa: nadie lo consume en ningún FR ni VC. `255` como `ssh`: el código remoto `255` se confundiría con una falla del cliente | BR-3 |
| ¿Timeout de conexión? | **15 s** | `tmux` no tiene ningún timeout de red que reusar: el único `connect()` del árbol es el del socket Unix del cliente (`client.c:126`). El valor es de usabilidad: más corto que el timeout TCP del sistema (en Linux ~2 minutos con `tcp_syn_retries=6`), más largo que un handshake normal en LAN | Sin timeout: el pane queda vivo sin información. El timeout del sistema: ~2 minutos parecen un cuelgue | FR-62 |
| ¿Umbrales de NFR-1 y NFR-3? | **1 s** y **30 s** para 200000 líneas | el bucle del server hace `event_loop(EVLOOP_ONCE)` (`proc.c:227`): una iteración bloqueada congela a todos los clientes, y 1 s es el límite en que un cliente percibe el server congelado; 200000 líneas ≈ 1,3 MB en 30 s exige ≥ 44 KB/s, un orden de magnitud bajo lo que da un canal SSH en loopback. `history-limit` vale 2000 por default (`options-table.c:845-851`): NFR-3 lo sube a 250000 para poder contar las 200000 líneas | 100 ms: ruido de la máquina de CI. Sin umbral: no discrimina un bucle bloqueante | NFR-1, NFR-3 |
| ¿Se restaura el tty al salir? | **No se especifica** | cuando el hijo termina, `server_child_exited` marca el pane como terminado por `wp->pid` (`server.c:491-499`) y nadie más lee ese tty: no hay nada observable que restaurar |
| ¿Alias del comando? | **`sshw`** | `new-window` ya usa `.alias = "neww"` (`cmd-new-window.c:39`): el alias abreviado es el mecanismo de `cmd_entry` para los comandos de ventana | Sin alias: nombre de 14 caracteres que se escribe seguido. `ssh`: se confundiría con el binario que INV-5 prohíbe | FR-11 | Restaurarlo: código sin ningún VC posible | — |

## Limitaciones conocidas

- **`respawn-pane` sobre un pane SSH** relanza `argv` como un comando común: como `argv[0]`
  es el destino, ejecutaría el destino como programa y fallaría. Es inofensivo, pero
  confuso. Soportarlo exige guardar en el pane que es SSH (`struct window_pane`,
  `tmux.h:1306`), fuera del alcance.
- `pane_current_command` / `pane_current_path` no reflejan al host remoto: muestran el
  proceso local (hallazgo 4 de las notas).
- **Licencia:** `libssh` se distribuye bajo LGPL, distinta de la licencia de `tmux`
  (`COPYING`: "Permission to use, copy, modify, and distribute…"). Al ser opcional y
  enlazada solo con el flag, el riesgo queda acotado; el empaquetado es una decisión del
  proyecto `tmux`, no de esta spec.
- **Funciones de `libssh`:** las elige `ssh-client.c`; la spec fija solo la versión mínima
  (`>= 0.9.0`, FR-2, FR-4) y el comportamiento observable. Los textos de los mensajes de esta
  spec son contrato de `ssh-client.c`, no de la librería.
