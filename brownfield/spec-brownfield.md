# Spec — `new-ssh-window`: cliente SSH nativo en `tmux` (solo Linux)

> Spec **brownfield**, construida sobre [`notas-exploracion.md`](./notas-exploracion.md).
> **No se implementa nada**: esta spec es el contrato desde el cual otro equipo podría
> construir sin hablar con nosotros y sin romper los builds de macOS y BSD.
>
> **Iteración 2 de la spec**, tras la corrección de la cátedra: FR atómicos (un
> comportamiento por FR), un VC numerado por cada invariante y decisiones fundadas en
> código de `tmux`. El registro del cambio está en [`revision-spec.md`](./revision-spec.md).

**Repo:** [`tmux/tmux`](https://github.com/tmux/tmux) · commit base **`5a820e63`** (2026-09-30)

## Propósito

Agregar un comando `new-ssh-window` que abre una ventana cuyo pane es una sesión SSH remota,
hablada por un cliente **nativo** (`libssh`), sin invocar nunca el binario `ssh`.

## El límite solo-Linux

Es el eje de la spec. Se aplica en **tres capas**, de modo que ninguna depende de la otra:

| Capa | Mecanismo | Resultado fuera de Linux | FR |
|---|---|---|---|
| **Configure** | `--enable-native-ssh`, **opt-in** (apagado por default). Si `PLATFORM` (`configure.ac:1006-1118`) no es `linux`, `configure` **falla con un mensaje claro** | el flag no se puede activar por accidente | FR-1 … FR-5 |
| **Make** | `AM_CONDITIONAL(ENABLE_NATIVE_SSH, …)`; las fuentes nuevas se agregan solo en esa rama (patrón de `ENABLE_SIXEL`, `Makefile.am:253-255`) | las fuentes nuevas no se compilan ni se enlazan | FR-6 |
| **Código** | `AC_DEFINE(ENABLE_NATIVE_SSH)`; todo código nuevo en archivos existentes va dentro de `#ifdef ENABLE_NATIVE_SSH` | el binario es el mismo que hoy | FR-7, FR-17 |

Un build sin el flag es **idéntico** al actual, en cualquier plataforma, incluido Linux.
`libssh` **nunca** es dependencia obligatoria.

## Alcance

### Dentro

| Archivo | Cambio |
|---|---|
| `cmd-new-ssh-window.c` (**nuevo**) | `cmd_new_ssh_window_entry`: parsea argumentos, arma el `spawn_context` y llama a `spawn_window()` |
| `ssh-client.c` (**nuevo**) | El cliente nativo: conectar, verificar host, autenticar, abrir canal con pty, copiar datos. Una sola entrada pública, `ssh_client_run()` |
| `spawn.c` | **Un** bloque en el camino del hijo de `spawn_pane()`, después de `environ_push` (`spawn.c:544`) y antes de los `exec` (`:552-574`) |
| `tmux.h` | Flag `SPAWN_SSH 0x2000` (el `0x1000` ya es `SPAWN_FLOATOVERZOOM`, `tmux.h:2531`) y prototipo de `ssh_client_run()` |
| `cmd.c` | `extern` + entrada en `cmd_table[]` (`cmd.c:123`), ambos bajo `#ifdef` |
| `configure.ac` | Flag, detección de `libssh` con `pkg-config`, guarda de plataforma, línea en el resumen final |
| `Makefile.am` | `if ENABLE_NATIVE_SSH` agregando las dos fuentes nuevas |
| `tmux.1` | Entrada del comando |
| `regress/new-ssh-window.sh` (**nuevo**) | Prueba end-to-end con el harness descripto abajo. Se saltea si `list-commands` no lista el comando; si además `TEST_REQUIRE_SSHD=1` y falta `sshd`, **falla** en vez de saltear |
| `regress/native-ssh-guard.sh` (**nuevo**) | Prueba de la guarda, corre en **todas** las plataformas (lo levanta `TESTS != echo *.sh`, `regress/Makefile`): es el VC-46 y el VC-47 |
| `.github/workflows/regress.yml` | Entrada **nueva** de matriz solo Linux con `--enable-native-ssh` y `TEST_REQUIRE_SSHD=1`; en el step de dependencias de **Linux**, `libssh-dev`, `openssh-server` y `strace` |

### Fuera de alcance

Explícito, por path:

- **`window.c`, `server.c`, `server-fn.c`, `window-*.c`** — el modelo de pane (fd + bufferevent
  + pid) no cambia. Ese es el motivo de enganchar en el hijo (decisión 2).
- **`job.c`, `input*.c`, `screen*.c`, `tty*.c`, `layout*.c`** — sin cambios.
- **`format.c`, `window-tree.c`, `osdep-*.c`, `cmd-find.c`** — `pane_current_command`,
  `pane_current_path` y búsqueda por tty siguen igual; no se parchan.
- **`cmd-split-window.c`** — **no hay variante con split** (`split-window` tiene 424 líneas
  contra 196 de `new-window` por el manejo de layout). Es una spec posterior.
- **`cmd-respawn-pane.c`** — `respawn-pane` sobre un pane SSH **no se soporta** (ver
  "Limitaciones conocidas").
- **`options-table.c`, `key-bindings.c`** — sin opciones nuevas ni binding por default.
- **`compat/`** — no se agrega nada: esto es una feature, no un reemplazo de portabilidad.
- **El job de macOS de `regress.yml`** — no se toca: ni su `configure`, ni sus dependencias.
  Fuera de Linux las fuentes nuevas no se compilan y el comando no existe en el binario.
- **Autenticación por password o keyboard-interactive, TOFU de host keys, agent forwarding,
  port forwarding, SFTP, ProxyJump, `~/.ssh/config`, `known_hosts` global** — fuera de alcance.

## Invariantes

Cosas que tienen que seguir siendo verdad **después** del cambio. Cada una tiene su VC en
la sección "VCs de invariantes".

| # | Invariante | VC |
|---|---|---|
| **INV-1** | Sin `--enable-native-ssh`, el binario es el de hoy | VC-46 |
| **INV-2** | Los builds no-Linux siguen compilando, y el comando nuevo no está en su binario | VC-47 |
| **INV-3** | Los comandos existentes no cambian | VC-48, VC-49 |
| **INV-4** | El modelo de PTY/panes no cambia | VC-50, VC-51 |
| **INV-5** | No se invoca el binario `ssh` (ni ningún otro) para el pane SSH | VC-52 |
| **INV-6** | El server no gana hilos | VC-53 |
| **INV-7** | El bit `SPAWN_SSH` no pisa uno existente | VC-54 |

## Línea de base de regresión

Medida **antes** de tocar una línea, sobre `5a820e63` y **sin** el flag:

```bash
sh autogen.sh && ./configure --enable-utf8proc && make
(cd regress && make)                                   # 172 scripts: todos PASS
./tmux -f/dev/null list-commands > list-commands.base  # 92 líneas esperadas
```

Valores ya medidos por lectura estática: **92** entradas en `cmd_table[]` (`cmd.c:124-215`)
y **172** scripts en `regress/`. El resultado de la corrida lo registra quien implemente,
antes de empezar, y guarda `list-commands.base`: los VC-46 y VC-48 comparan contra ese
archivo. Al cerrar cada iteración, la misma corrida **sin** el flag tiene que dar lo mismo.

Con el cambio completo, `regress/` tiene **174** scripts (los 172 más los dos nuevos). Un
script viejo que se pone rojo es una regresión: no se "actualiza" para que pase.

## Harness de prueba

Lo usan los VCs de red. Vive en `regress/new-ssh-window.sh`; el feature nunca lo invoca.

- **`sshd` de prueba:** `sshd -D -e -f <conf>` en `127.0.0.1`, puerto alto `$PORT`, host key
  propia, `LogLevel VERBOSE`, log en `$SSHD_LOG`. Acepta solo `publickey`, salvo en VC-28.
- **`HOME` temporal** `$H`, con `$H/.ssh/known_hosts` y las claves que pida cada VC.
- **Destino** `$D` = `$USER@127.0.0.1`.
- **`tmux` de prueba** con `-f/dev/null`, `remain-on-exit on` (para leer la pantalla y
  `#{pane_dead_status}` de un pane que ya terminó) y el entorno de sesión con `HOME=$H`.
  El server se lanza **bajo `strace`**: `strace -f -o $TRACE -e trace=execve,connect tmux … new-session -d`.
- **Listener mudo:** `nc -l 127.0.0.1 $MUTE`, que acepta la conexión TCP y nunca manda el
  banner SSH. Sirve para tener una conexión colgada de forma determinista.
- "**Llega al prompt remoto**" significa: tras `send-keys 'echo R-$((6*7))' Enter`,
  `capture-pane -p` contiene la línea `R-42`.

## Requerimientos

Un comportamiento por FR. Cada FR tiene al menos un VC debajo.

### Guarda de build

#### FR-1 · El flag está apagado por defecto

**Dado** el `configure.ac` modificado (flag declarado con `AC_ARG_ENABLE`, como `sixel` en
`configure.ac:545-548`),
**cuando** se corre `./configure` sin `--enable-native-ssh`,
**entonces** termina bien y **no** define `ENABLE_NATIVE_SSH`.

**VC-1:** `./configure` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da `0` (`tmux` no genera `config.h`: los `AC_DEFINE` viajan en `DEFS` del `Makefile`).

#### FR-2 · En Linux con `libssh`, el flag activa el feature

**Dado** Linux con `libssh` detectable por `pkg-config` (`PKG_CHECK_MODULES`, como
`libsystemd` en `configure.ac:508`),
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** termina bien y define `ENABLE_NATIVE_SSH`.

**VC-2:** `./configure --enable-native-ssh` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da al menos `1`.

#### FR-3 · Sin `libssh`, el flag falla en `configure`

**Dado** Linux **sin** `libssh` instalado,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con error y el mensaje nombra a `libssh`.

**VC-3:** el comando sale con estado distinto de `0`, su salida contiene `libssh` y no se generó `Makefile`.

#### FR-4 · Fuera de Linux, el flag falla en `configure`

**Dado** una plataforma donde `PLATFORM` (`configure.ac:1006-1118`) no es `linux`,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con el error `native SSH is only supported on Linux`.

**VC-4:** en macOS, el comando sale con estado distinto de `0` y su salida contiene ese texto. VC **manual**: ningún job existente pasa el flag fuera de Linux.

#### FR-5 · El resumen de `configure` informa el estado

**Dado** el bloque de resumen de `configure.ac` (los `AC_MSG_NOTICE` que terminan en `configure.ac:1166`),
**cuando** `configure` termina bien,
**entonces** imprime `native SSH: on` u `native SSH: off`.

**VC-5:** la salida de `./configure` contiene `native SSH: off`; la de `./configure --enable-native-ssh` contiene `native SSH: on`.

#### FR-6 · Las fuentes nuevas se compilan solo con el flag

**Dado** `Makefile.am` con `if ENABLE_NATIVE_SSH … dist_tmux_SOURCES += cmd-new-ssh-window.c ssh-client.c` (patrón de `Makefile.am:253-255`),
**cuando** se corre `make`,
**entonces** los dos objetos se construyen y se enlazan solo si el flag está activo.

**VC-6:** con el flag, tras `make` existen `cmd-new-ssh-window.o` y `ssh-client.o` y `nm tmux | grep -c ' T ssh_client_run'` da `1`; sin el flag, no existe ninguno de los dos `.o`.

### El comando

#### FR-7 · El comando está registrado en la tabla de comandos

**Dado** un build con el flag,
**cuando** el server arma `cmd_table[]` (`cmd.c:123`), con la entrada
`&cmd_new_ssh_window_entry` y su `extern` dentro de `#ifdef ENABLE_NATIVE_SSH`,
**entonces** `new-ssh-window` es un comando válido.

**VC-7:** `tmux list-commands | grep -c '^new-ssh-window'` da `1`.

#### FR-8 · El alias es `sshw`

**Dado** `cmd_new_ssh_window_entry` con `.alias = "sshw"`,
**cuando** se invoca `tmux sshw …`,
**entonces** se ejecuta `new-ssh-window`.

**VC-8:** `tmux sshw -d -P -F '#{window_name}' $D` imprime `$D`, y `grep -n '\.alias = "sshw"' cmd-*.c` devuelve una sola línea.

#### FR-9 · Exige exactamente un destino

**Dado** `.args = { "dF:i:n:p:Pt:", 1, 1, NULL }` en la entrada,
**cuando** se invoca sin argumento posicional o con más de uno,
**entonces** falla con el mensaje de uso (`cmd.c:530`) y no crea ninguna ventana.

**VC-9:** `tmux new-ssh-window` y `tmux new-ssh-window a b` salen con estado distinto de `0`, su salida empieza con `usage: new-ssh-window`, y `list-windows | wc -l` no cambia.

#### FR-10 · `-t` elige la ventana destino como en `new-window`

**Dado** `.target = { 't', CMD_FIND_WINDOW, CMD_FIND_WINDOW_INDEX }` (igual que `cmd-new-window.c:46`),
**cuando** se pasa `-t :N` con `N` libre,
**entonces** la ventana se crea en el índice `N`.

**VC-10:** tras `tmux new-ssh-window -d -t :7 $D`, `list-windows -F '#{window_index}' | grep -cx 7` da `1`.

#### FR-11 · `-d` no cambia la ventana actual

**Dado** el flag `-d`, que pone `SPAWN_DETACHED` (como `cmd-new-window.c:156-157`),
**cuando** se crea la ventana,
**entonces** la ventana actual de la sesión sigue siendo la misma.

**VC-11:** `display-message -p '#{window_index}'` da el mismo valor antes y después de `new-ssh-window -d $D`; después de `new-ssh-window $D` (sin `-d`) da el índice de la ventana nueva.

#### FR-12 · `-P` imprime información de la ventana, con el formato de `-F`

**Dado** los flags `-P` y `-F` (como `cmd-new-window.c:172-173`),
**cuando** se crea la ventana,
**entonces** el comando imprime el formato expandido para la ventana nueva.

**VC-12:** `tmux new-ssh-window -d -P -F 'W=#{window_index}' -t :9 $D` imprime `W=9`; sin `-P` no imprime nada.

#### FR-13 · `-n` fija el nombre de la ventana

**Dado** el flag `-n nombre`, que se pasa en `sc.name` (como `cmd-new-window.c:83`, `:140`),
**cuando** se crea la ventana,
**entonces** la ventana se llama `nombre`.

**VC-13:** `tmux new-ssh-window -d -n prod -P -F '#{window_name}' $D` imprime `prod`.

#### FR-14 · Sin `-n`, la ventana se llama como el destino

**Dado** que el hijo es un fork del server, y `osdep_get_name` (`osdep-linux.c:30`) leería
`/proc/<pgrp>/cmdline` y devolvería el título del server,
**cuando** no se pasa `-n`,
**entonces** el comando pone el **destino** en `sc.name`, lo que además apaga
`automatic-rename` (`spawn.c:216-224`).

**VC-14:** `tmux new-ssh-window -d -P -F '#{window_name}' $D` imprime `$D`, y 5 s después `display-message -p -t <ventana> '#{window_name}'` sigue dando `$D`.

#### FR-15 · `-c` y `-e` no existen

**Dado** que no figuran en `.args` (un directorio o un entorno locales no aplican a un destino remoto),
**cuando** se pasa `-c` o `-e`,
**entonces** el parser los rechaza (`arguments.c:240`) y no se crea ninguna ventana.

**VC-15:** `tmux new-ssh-window -c /tmp $D` y `tmux new-ssh-window -e A=b $D` salen con estado distinto de `0`, su salida contiene `unknown flag`, y `list-windows | wc -l` no cambia.

### Enganche en el spawn

#### FR-16 · La ventana se crea por `spawn_window()` con `SPAWN_SSH`

**Dado** el comando de FR-7,
**cuando** se ejecuta,
**entonces** llama a `spawn_window()` (`tmux.h:4195`) con `SPAWN_SSH` en `sc.flags`, y el
pane se crea con el mismo `fdforkpty` (`spawn.c:478`) que cualquier otro.

**VC-16:** tras `new-ssh-window -d -P -F '#{pane_pid} #{pane_tty}' $D`, el primer campo es un pid cuyo padre es el server (`ps -o ppid= -p <pane_pid>` igual a `display-message -p '#{pid}'`) y el segundo es un `/dev/pts/N` que existe.

#### FR-17 · En el hijo, `ssh_client_run()` ocupa el lugar de los `exec`

**Dado** un pane creado con `SPAWN_SSH`,
**cuando** el hijo llega al punto posterior a `environ_push` (`spawn.c:544`),
**entonces** un bloque `#ifdef ENABLE_NATIVE_SSH` llama a `ssh_client_run(argc, argv)` y hace
`_exit` con su valor de retorno, sin llegar a `execvp`/`execl` (`spawn.c:552`, `:567`, `:574`).

**VC-17:** con la sesión abierta, `readlink /proc/<pane_pid>/exe` es igual a `readlink /proc/<pid del server>/exe`: el proceso del pane es el binario `tmux`, no uno ejecutado con `exec`.

#### FR-18 · Contrato de `argv` entre el comando y el cliente

**Dado** el `argv` que el comando deja en el `spawn_context` (y que `spawn_pane` copia a `wp->argv`),
**cuando** se arma,
**entonces** `argv[0]` es el destino `[usuario@]host`, seguido por `-p PUERTO` si se pasó
`-p` y por `-i ARCHIVO` si se pasó `-i`, en ese orden.

**VC-18:** `tmux new-ssh-window -d -P -F '#{pane_start_command}' -i /tmp/k -p 2222 u@h` imprime `u@h -p 2222 -i /tmp/k` (`pane_start_command` stringifica `wp->argv`, `format.c:901-908`).

#### FR-19 · Puerto: el de `-p`, o 22

**Dado** el `argv` de FR-18,
**cuando** el cliente abre la conexión TCP,
**entonces** usa el puerto de `-p`; si no hay `-p`, el 22.

**VC-19:** con el `sshd` de prueba en `$PORT` distinto de 22, `new-ssh-window -d -p $PORT -i $H/.ssh/k $D` llega al prompt remoto.
**VC-20:** tras `new-ssh-window -d 127.0.0.1` (sin `-p`), `$TRACE` contiene un `connect` del pid del pane con `sin_port=htons(22)`.

### Verificación del host

#### FR-20 · Un host desconocido se rechaza

**Dado** un destino cuya host key no está en `$HOME/.ssh/known_hosts`,
**cuando** se intenta conectar,
**entonces** el hijo imprime `new-ssh-window: host key for <host> not found in known_hosts`
y sale con `1`, sin preguntar y sin escribir `known_hosts`.

**VC-21:** con `$H/.ssh/known_hosts` vacío, `#{pane_dead_status}` es `1`, `capture-pane -p` contiene `not found in known_hosts` y el archivo sigue con 0 bytes.

#### FR-21 · Un host con la clave cambiada se rechaza

**Dado** un destino con una host key **distinta** de la registrada en `$HOME/.ssh/known_hosts`,
**cuando** se intenta conectar,
**entonces** el hijo imprime `new-ssh-window: host key for <host> has changed` y sale con `1`.

**VC-22:** con otra clave registrada para `[127.0.0.1]:$PORT`, `#{pane_dead_status}` es `1` y `capture-pane -p` contiene `has changed`.

### Autenticación

#### FR-22 · Autenticación por agent

**Dado** un host conocido y `SSH_AUTH_SOCK` en el entorno de la **sesión** (llega al hijo
por `environ_for_session`, `environ.c:253`, y `environ_push`, `spawn.c:544`),
**cuando** se conecta sin `-i`,
**entonces** se autentica con una clave del agent.

**VC-23:** con `set-environment SSH_AUTH_SOCK <socket>` de un agent que tiene la clave autorizada, y `$H/.ssh` sin ninguna clave, `new-ssh-window -d -p $PORT $D` llega al prompt remoto.

#### FR-23 · Autenticación con la clave de `-i`

**Dado** un host conocido, sin agent,
**cuando** se pasa `-i ARCHIVO`,
**entonces** se autentica con esa clave.

**VC-24:** sin `SSH_AUTH_SOCK` en el entorno de la sesión, con la clave autorizada en `/tmp/<dir>/k` (fuera de `$H/.ssh`), `new-ssh-window -d -p $PORT -i /tmp/<dir>/k $D` llega al prompt remoto.

#### FR-24 · Autenticación con las claves por defecto

**Dado** un host conocido, sin agent y sin `-i`,
**cuando** se conecta,
**entonces** se prueba `$HOME/.ssh/id_ed25519` y después `$HOME/.ssh/id_rsa`.

**VC-25:** con la clave autorizada en `$H/.ssh/id_ed25519`, sin `SSH_AUTH_SOCK` y sin `-i`, `new-ssh-window -d -p $PORT $D` llega al prompt remoto.

#### FR-25 · El agent se prueba antes que los archivos de clave

**Dado** un agent con la clave A y un `-i` con la clave B, ambas autorizadas,
**cuando** se conecta,
**entonces** la sesión se autentica con A.

**VC-26:** `$SSHD_LOG` contiene `Accepted publickey` con el fingerprint de A y ninguna línea `Accepted` con el de B.

#### FR-26 · La passphrase se pide en el tty del pane

**Dado** una clave protegida con passphrase y sin agent,
**cuando** el cliente la necesita,
**entonces** escribe `Enter passphrase for key '<ruta>': ` en el tty del pane (el pty de
`fdforkpty`, `spawn.c:478`) y lee la respuesta sin eco.

**VC-27:** `capture-pane -p` contiene `Enter passphrase for key`; tras `send-keys '<passphrase>' Enter` se llega al prompt remoto, y `capture-pane -p` no contiene la passphrase.

#### FR-27 · No se ofrece autenticación por password

**Dado** un servidor que solo acepta `password`,
**cuando** se conecta,
**entonces** el hijo imprime `new-ssh-window: no supported authentication method` y sale
con `1`, sin pedir nada.

**VC-28:** con el `sshd` de prueba en `AuthenticationMethods password`, `#{pane_dead_status}` es `1`, `capture-pane -p` contiene `no supported authentication method` y no contiene `assword`.

### Sesión interactiva

#### FR-28 · El pty remoto usa el `TERM` del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con el `TERM` del entorno del hijo (el `default-terminal` que pone
`environ_for_session`, `environ.c:265`).

**VC-29:** tras `send-keys 'echo T=$TERM' Enter`, `capture-pane -p` contiene `T=` seguido del valor de `show-options -gv default-terminal`.

#### FR-29 · El pty remoto arranca con el tamaño del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con las filas y columnas del tty del pane (`TIOCGWINSZ` sobre el tty local).

**VC-30:** tras `send-keys 'stty size' Enter`, `capture-pane -p` contiene `<pane_height> <pane_width>` con los valores de `display-message -p '#{pane_height} #{pane_width}'`.

#### FR-30 · Se abre una shell y los bytes se copian en ambos sentidos

**Dado** el pty remoto,
**cuando** se abre la sesión,
**entonces** se pide una shell y el hijo copia tty local → canal y canal → tty local hasta
que el canal se cierra.

**VC-31:** `send-keys 'echo R-$((6*7))' Enter` seguido de `capture-pane -p` muestra `R-42`, producido por el host remoto.

#### FR-31 · El tty local queda en modo raw durante la sesión

**Dado** que la disciplina de línea local duplicaría el eco y atraparía `Ctrl-C`,
**cuando** la sesión se abre,
**entonces** el hijo pone su tty en raw con `cfmakeraw` (`compat.h:426`, usado igual en `client.c:349`).

**VC-32:** tras `send-keys 'echo ok' Enter`, `capture-pane -p | grep -c 'echo ok'` da `1`.
**VC-33:** `send-keys 'sleep 100' Enter` y luego `send-keys C-c`: el prompt remoto vuelve en menos de 2 s y `kill -0 <pane_pid>` sigue dando `0`.

#### FR-32 · El resize llega al remoto

**Dado** una sesión abierta,
**cuando** cambia el tamaño del pane (`window_pane_send_resize`, `window.c:597`, hace el
`ioctl(TIOCSWINSZ)` y el hijo recibe `SIGWINCH`),
**entonces** el cliente actualiza el tamaño del pty remoto.

**VC-34:** tras `resize-window -x 100 -y 30`, `send-keys 'stty size' Enter` muestra `30 100`.

### Fin de sesión

#### FR-33 · El hijo sale con el código de salida remoto

**Dado** una sesión abierta,
**cuando** la shell remota termina,
**entonces** `ssh_client_run()` devuelve su código de salida y el pane sigue el camino
habitual: `SIGCHLD` → `server_child_signal` (`server.c:468`) → `server_child_exited` →
`PANE_EXITED`. No se agrega ninguna ruta de destrucción nueva.

**VC-35:** `send-keys 'exit 3' Enter` deja `#{pane_dead_status}` en `3`.

#### FR-34 · Si la conexión se corta, el hijo sale con `1`

**Dado** una sesión abierta,
**cuando** la conexión se pierde sin código de salida remoto,
**entonces** el hijo sale con `1`.

**VC-36:** tras matar el proceso `sshd` de la sesión (el hijo del `sshd` de prueba), `#{pane_dead_status}` es `1` y `tmux display-message -p ok` sigue respondiendo `ok`.

#### FR-35 · Cerrar el pane desconecta al hijo

**Dado** una sesión abierta,
**cuando** el pane se cierra desde `tmux` (el cierre del master le llega al hijo como `SIGHUP`),
**entonces** el hijo cierra la conexión y termina.

**VC-37:** tras `kill-pane`, `kill -0 <pane_pid>` falla en menos de 2 s y `pgrep -P <pid del sshd de prueba>` no devuelve nada.

### Fallas de conexión

#### FR-36 · Un error de conexión se informa en el pane

**Dado** un host que no resuelve o un puerto cerrado,
**cuando** se intenta conectar,
**entonces** el hijo imprime `new-ssh-window: cannot connect to <host>: <motivo>` y sale con `1`.

**VC-38:** `new-ssh-window -d nohost.invalid` deja `#{pane_dead_status}` en `1` y `capture-pane -p` contiene `cannot connect to nohost.invalid`.
**VC-39:** `new-ssh-window -d -p <puerto libre> 127.0.0.1` da el mismo resultado con `cannot connect to 127.0.0.1`.

#### FR-37 · La conexión tiene un timeout de 15 s

**Dado** un destino que acepta la conexión TCP pero no completa el intercambio de claves,
**cuando** pasan **15 s** desde que el cliente arrancó,
**entonces** el hijo imprime `new-ssh-window: connection to <host> timed out` y sale con `1`.

**VC-40:** contra el listener mudo, `#{pane_dead_status}` sigue vacío a los 14 s y es `1` antes de los 20 s; `capture-pane -p` contiene `timed out`.

### Reglas y no funcionales

#### BR-1 · No se registran secretos

Passphrases, contenido de claves y respuestas del agent **nunca** llegan al log de `tmux`.
El hijo ya cerró el log (`log_close`, `spawn.c:543`) antes del bloque de FR-17, y
`ssh-client.c` no vuelve a abrirlo ni llama a `log_debug`.

**VC-41:** con `tmux -vv`, tras la sesión con passphrase del VC-27, `grep -c '<passphrase>' tmux-server-*.log` da `0`, y `grep -c 'log_' ssh-client.c` da `0`.

#### BR-2 · Sin binding por default

El comando no se asocia a ninguna tecla.

**VC-42:** `tmux -f/dev/null list-keys | grep -c new-ssh-window` da `0`.

#### NFR-1 · El server responde mientras una conexión está pendiente

El server no hace I/O de red: toda la conexión ocurre en el hijo (FR-17).

**VC-43:** con un `new-ssh-window -d` colgado contra el listener mudo, `tmux display-message -p ok` desde otro cliente responde en menos de **1 s**.

#### NFR-2 · Documentación

El comando figura en `tmux.1` con su sinopsis, las limitaciones y la aclaración de que solo
existe si se compiló con `--enable-native-ssh` (el man page se instala siempre).

**VC-44:** `grep -c 'new-ssh-window' tmux.1` da al menos `1` y `grep -c 'enable-native-ssh' tmux.1` da al menos `1`.

#### NFR-3 · La salida masiva no se atasca

**VC-45:** `send-keys 'seq 1 200000' Enter`: en menos de **30 s** `capture-pane -p` muestra la línea `200000` exactamente una vez y el prompt remoto vuelve.

## VCs de invariantes

Cada invariante tiene acá su VC, con el mismo peso que los de los FR.

**VC-46 (INV-1):** en Linux, build **sin** el flag: `ldd ./tmux | grep -c libssh` da `0`;
`nm ./tmux | grep -c ssh_client_run` da `0`; y
`./tmux -f/dev/null list-commands | diff - list-commands.base` no imprime nada. Lo ejecuta
`regress/native-ssh-guard.sh` en los jobs de Linux sin flag.

**VC-47 (INV-2):** en el job `macos-26-arm64` de `regress.yml`, **sin tocarlo** (sigue con
`--enable-utf8proc --enable-asan` y sin `libssh`): el step `build` termina con `0`, y
`regress/native-ssh-guard.sh` comprueba, cuando `uname -s` no es `Linux`, que
`$TEST_TMUX list-commands | grep -c new-ssh-window` da `0` y que
`nm $TEST_TMUX | grep -c ssh_client_run` da `0`. Es el caso "build no-Linux que compila y
no trae el comando", automatizado.

**VC-48 (INV-3):** en Linux, build **con** el flag:
`./tmux -f/dev/null list-commands | grep -v '^new-ssh-window' | diff - list-commands.base`
no imprime nada (los 92 comandos existentes conservan nombre, alias y uso), y
`list-commands | wc -l` da `93`.

**VC-49 (INV-3):** `(cd regress && make)` termina con `0` con y sin el flag: los 172 scripts
originales en `PASS`, incluido `new-window-command.sh` (crea cuatro ventanas con `new-window`).

**VC-50 (INV-4):** `git diff --stat 5a820e63` **no lista** `window.c`, `server.c`,
`server-fn.c`, `job.c`, `format.c`, `input.c`, `cmd-find.c` ni `osdep-*.c`, y
`git diff --numstat 5a820e63 -- spawn.c cmd.c tmux.h` muestra **0 líneas eliminadas** en los tres.

**VC-51 (INV-4):** en el build **con** el flag, un pane común sigue por el camino de siempre:
tras `new-window -d -P -F '#{pane_pid} #{pane_tty}' sleep 100`,
`readlink /proc/<pane_pid>/exe` termina en `/sleep` (pasó por `execvp`, `spawn.c:552`), el
tty es un `/dev/pts/N`, y `kill <pane_pid>` deja `#{pane_dead_signal}` en `15`.

**VC-52 (INV-5):** tras una sesión completa del VC-31, `$TRACE` no contiene ninguna línea
`execve` del pid del pane, y durante la sesión `pgrep -P <pane_pid>` no devuelve nada (el
hijo no lanza procesos).

**VC-53 (INV-6):** `ls /proc/<pid del server>/task | wc -l` da el mismo número antes de
crear el pane SSH, con la conexión colgada del VC-43 y con la sesión abierta del VC-31.

**VC-54 (INV-7):** `grep -c 'define SPAWN_SSH 0x2000' tmux.h` da `1`, y
`grep -o 'SPAWN_[A-Z]* 0x[0-9a-f]*' tmux.h | awk '{print $2}' | sort | uniq -d` no imprime nada.

## Matriz de cobertura

| Requisito | VC | Caso de falla incluido |
|---|---|---|
| FR-1 … FR-6 | VC-1 … VC-6 | VC-3 (sin `libssh`), VC-4 (no-Linux) |
| FR-7, FR-8 | VC-7, VC-8 | — |
| FR-9 | VC-9 | sin destino, dos destinos |
| FR-10 … FR-14 | VC-10 … VC-14 | — |
| FR-15 | VC-15 | flags rechazados |
| FR-16, FR-17, FR-18 | VC-16, VC-17, VC-18 | — |
| FR-19 | VC-19, VC-20 | — |
| FR-20, FR-21 | VC-21, VC-22 | host desconocido, host cambiado |
| FR-22 … FR-26 | VC-23 … VC-27 | — |
| FR-27 | VC-28 | autenticación rechazada |
| FR-28 … FR-30 | VC-29 … VC-31 | — |
| FR-31 | VC-32, VC-33 | — |
| FR-32, FR-33 | VC-34, VC-35 | — |
| FR-34, FR-35 | VC-36, VC-37 | conexión cortada, pane cerrado |
| FR-36 | VC-38, VC-39 | host inexistente, puerto cerrado |
| FR-37 | VC-40 | sesión que no abre |
| BR-1, BR-2 | VC-41, VC-42 | — |
| NFR-1, NFR-2, NFR-3 | VC-43, VC-44, VC-45 | — |
| INV-1 | VC-46 | — |
| INV-2 | VC-47 | build no-Linux |
| INV-3 | VC-48, VC-49 | — |
| INV-4 | VC-50, VC-51 | — |
| INV-5, INV-6, INV-7 | VC-52, VC-53, VC-54 | — |

37 FR, 2 BR, 3 NFR y 7 invariantes; 54 VCs, sin requisito sin VC y sin VC huérfano.

## Plan de iteraciones

Cada iteración termina con la línea de base **sin** el flag igual a la inicial (VC-46, VC-49).

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Guardas de build: `configure.ac`, `Makefile.am` y `regress/native-ssh-guard.sh`; dos fuentes nuevas vacías | FR-1 … FR-6 (VC-1 … VC-6), INV-1 (VC-46), INV-2 (VC-47) |
| **2** | Comando, `SPAWN_SSH`, bloque en `spawn.c`, `ssh_client_run()` que solo conecta; **harness con `sshd` de prueba** | FR-7 … FR-19 (VC-7 … VC-20), FR-36, FR-37 (VC-38 … VC-40), NFR-1 (VC-43), INV-3 … INV-7 (VC-48 … VC-54, con VC-52 y VC-53 repetidos al cerrar la 3) |
| **3** | Host key y autenticación | FR-20 … FR-27 (VC-21 … VC-28), BR-1 (VC-41) |
| **4** | Sesión interactiva, resize y fin de sesión | FR-28 … FR-35 (VC-29 … VC-37), NFR-3 (VC-45) |
| **5** | `tmux.1` y entrada de CI | NFR-2 (VC-44), BR-2 (VC-42) |

La Iteración 1 es el camino más angosto que se puede verificar solo: no hay código C con
lógica y prueba el límite solo-Linux completo. El `sshd` de prueba aparece en la Iteración
2, no al final, para que ningún VC de red quede sin forma de ejercitarse.

## Decisiones

Cada decisión cita el código de `tmux` (`5a820e63`) que la funda y el FR donde queda escrita.

| # | Pregunta | Decisión | Código de `tmux` que la funda | FR |
|---|---|---|---|---|
| 1 | ¿Comando nuevo, y dónde se registra? | **Sí**: `new-ssh-window`, alias `sshw`, en `cmd-new-ssh-window.c`, registrado en `cmd_table[]` de `cmd.c` bajo `#ifdef` | `struct spawn_context` (`tmux.h:2500-2532`) no tiene campo "tipo de pane" y `new-window` trata todo posicional como comando a ejecutar (`.args` con `0, -1`, `cmd-new-window.c:41`): no hay flag libre que cambie eso sin tocar un comando existente. Todo comando se registra en `cmd_table[]` (`cmd.c:123`), que hoy no tiene ningún `#if` | FR-7, FR-8 |
| 2 | ¿Dónde engancha en el spawn? | **En el hijo**, tras `fdforkpty` y `environ_push`, donde hoy están los `exec` | `spawn.c:478` deja `pid`, `fd` y `tty` reales; `window_pane_send_resize` hace `fatal` si el `ioctl(TIOCSWINSZ)` falla (`window.c:612-622`), así que el fd tiene que seguir siendo un pty; `server_child_exited` busca el pane por `wp->pid` (`server.c:491-498`). Ya hay código opcional en ese tramo del hijo (`spawn.c:504`) | FR-16, FR-17 |
| 3 | ¿`libssh` u OpenSSH? | **`libssh`**, enlazada | (a) La única forma que tiene `tmux` de usar OpenSSH es ejecutar su binario por los tres `exec` de `spawn.c:552`, `:567` y `:574`: es lo que hoy hace `new-window ssh host`, y lo que INV-5 prohíbe. (b) `configure.ac` detecta cada librería opcional con `PKG_CHECK_MODULES` sobre un `.pc` (`libsystemd` en `:508`, `libutf8proc` en `:463`, `jemalloc` en `:670`): la dependencia tiene que ser una librería con `pkg-config`, que es como se detecta `libssh` en FR-2/FR-3. (c) El cliente corre en un hijo al que `closefrom(STDERR_FILENO + 1)` (`spawn.c:541`) le dejó solo los fds 0-2: la librería tiene que abrir su propio socket y convivir en un bucle con el tty, sin apoyarse en el libevent del server. (d) No hay código SSH que reusar: `grep -il libssh` sobre el árbol no devuelve nada | FR-2, FR-3, FR-17 |
| 4 | ¿Cómo se integra con el event loop? | **No se integra**: el cliente corre en el hijo con su propio bucle | El server es de un solo hilo: la base de libevent se crea en `osdep_event_init` (`osdep-linux.c:92`, llamada en `tmux.c:624`), el server la reusa con `event_reinit` (`server.c:198`) y entra a `proc_loop` (`server.c:258`), que llama a `event_loop(EVLOOP_ONCE)` (`proc.c:227`). Un handshake bloqueante ahí congela a todos los clientes | FR-17, NFR-1 |
| 5 | ¿Auth por claves o por agent? | **Ambas**, agent primero; sin password | `tmux` ya transporta el agent: el default de `update-environment` incluye `SSH_AUTH_SOCK` y `SSH_AGENT_PID` (`options-table.c:1207-1214`), `environ_for_session` copia el entorno de la sesión (`environ.c:253-261`) y `environ_push` lo instala en el hijo antes del bloque nuevo (`spawn.c:544`): el agent del cliente adjunto llega sin código nuevo, y por eso va primero. Las claves en archivo usan el `HOME` de ese mismo entorno y la passphrase se puede pedir porque el hijo tiene el pty de `spawn.c:478` como stdin. `tmux` no propaga ningún mecanismo de password, y dejarlo fuera acota los secretos que pasan por el pane a uno solo | FR-22 … FR-27 |
| 6 | ¿Qué guarda deja afuera a no-Linux? | **Opt-in** `--enable-native-ssh`, solo Linux, con `AM_CONDITIONAL` y `#ifdef` | `tmux` no usa `#ifdef __linux__` (cero ocurrencias) y sí usa flags de feature: `ENABLE_SIXEL` (`configure.ac:545-552`, `Makefile.am:253-255`) agrega fuentes propias solo con el flag, y `HAVE_SYSTEMD` (`configure.ac:503-527`, `Makefile.am:243-245`) hace lo mismo con detección por `pkg-config`. `PLATFORM` sale del `case "$host_os"` de `configure.ac:1006-1118` | FR-1 … FR-6 |

Decisiones menores:

| Pregunta | Decisión | Por qué |
|---|---|---|
| ¿Ventana o split? | **Ventana** | reusa `spawn_window` (`spawn.c:209` llama a `spawn_pane`) y evita el manejo de layout de `cmd-split-window.c`; el split es una spec posterior |
| ¿Host desconocido? | **Se rechaza** (FR-20) | un pane puede crearse con `-d`, sin nadie mirando: una pregunta interactiva quedaría colgada |
| ¿Se restaura el tty al salir? | **No se especifica** | cuando el hijo termina el pane muere (`PANE_EXITED`) y nadie más usa ese tty: no hay nada observable que restaurar |

## Limitaciones conocidas

- **`respawn-pane` sobre un pane SSH** relanza `argv` como un comando común: como `argv[0]`
  es el destino, ejecutaría el destino como programa y fallaría. Es inofensivo, pero
  confuso. Soportarlo exige guardar en el pane que es SSH (`struct window_pane`,
  `tmux.h:1306`), fuera del alcance.
- `pane_current_command` / `pane_current_path` no reflejan al host remoto: muestran el
  proceso local (hallazgo 4 de las notas).
- **Licencia:** `libssh` se distribuye bajo LGPL (**a confirmar**), distinta de la licencia
  de `tmux` (`COPYING`: "Permission to use, copy, modify, and distribute…"). Al ser opcional
  y enlazada solo con el flag, el riesgo queda acotado, pero es una decisión de empaquetado
  para el proyecto, no de esta spec.
- **Versión de `libssh`**: los nombres exactos de las funciones y la versión mínima **no se
  verificaron** (no están en el repo de `tmux`); los fija quien implemente en la Iteración 2.
  Los textos de los mensajes de error de esta spec son contrato de `ssh-client.c`, no de la
  librería.
