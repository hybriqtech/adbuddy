# ADBuddy · M6c · Internacionalización EN/ES · Diseño

- **Fecha:** 2026-10-02
- **Estado:** borrador escrito por delegación del usuario, sin revisión todavía. Las decisiones están en §12; no queda ninguna pregunta que bloquee (§13).
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§6 «Idiomas: inglés y español desde el día 1», fila M6 de §13)
- **Protocolo:** [2026-10-01-adbuddy-protocolo-m3-m5-design.md](2026-10-01-adbuddy-protocolo-m3-m5-design.md) (§4.1, el `text` de `error`). M6c **no cambia el protocolo**
- **Parte de:** línea B (M6 sin protocolo), rama `m6c`, que sale de `main`
- **En paralelo con:** M5c (`nautilus`, pantallas de la app), M6b (skills) y lo que quede de M4, M5a y M5b (§7)
- **Medido sobre:** la rama `m4` (todo `main` hasta f9f4422 más M4), el 2026-10-02. Las cifras de §1 salen de `grep` y `wc` sobre ese árbol

## 1. Por qué y qué hay hoy

La v1 promete inglés y español desde el día 1, y la app ya lo cumple casi entera por haberlo hecho así desde M2. El host solo lo cumple en lo que escribe para la **confirmación**. El resto de lo que ve la persona en el PC está en inglés y sin mecanismo. M6c cierra eso y, sobre todo, **lo hace comprobable**, para que ni M5c ni nadie vuelva a meter texto suelto.

### 1.1 App Android (`android/app/src/main`, 79 ficheros `.kt`, 7 562 líneas)

| Qué | Medido |
|---|---|
| Recursos | `values/strings.xml`: 120 `<string>` y 1 `<plurals>`. `values-es/strings.xml`: 118 y 1. Las dos que faltan en es son `app_name` y `home_title`, ambas `translatable="false"` (el nombre propio). Los marcadores (`%1$s`, `%1$d`) coinciden en las 118. Solo `perm_deny` ("No") es igual en los dos idiomas |
| Uso | 155 usos de `stringResource`, `getString`, `pluralStringResource` o `R.string` en 20 ficheros |
| Texto suelto en composables y notificaciones | **0** en `ui/`, `notify/`, `confirm/`, `control/` y `files/` fuera de los `@Preview` (buscado: `Text("`, `text =`, `contentDescription =`, `label =`, `title =`, `setContentTitle`, `setContentText`, `setSubText`, `Toast`, `NotificationChannel(`, `setName`, `setLabel`, `addAction`, y cualquier literal con dos palabras). Los únicos que salen son los de los `@Preview` (~10) y un `IOException("cannot open")` interno de `files/UriSource.kt` |
| Texto que no pasa por recursos y sí se ve | (a) etiquetas de las teclas rápidas (`term/QuickKeys.kt`: Esc, Ctrl+C, ↑, ↓, Tab, Enter, 1, 2, 3); (b) `sizeText` en `ui/ChatParts.kt` (`"%.1f MB"`, `"%d KB"`, `"$n B"`); (c) las horas con patrón fijo `HH:mm:ss` (`confirm/ConfirmNotifier.kt`) y `HH:mm` (`ui/RemoteCard.kt`); (d) los iconos de nivel 🟢🟡🔴 (`ActivityScreen.levelIcon`) |
| Texto que no se ve | 25 mensajes de error de `a11y/UiExecutor.kt` hacia el host (los lee el agente); trazas de `Log`; ~10 literales de `@Preview`; el texto de `PROPERTY_SPECIAL_USE_FGS_SUBTYPE` del manifest (para la revisión de Play) |
| `ErrorReply.text` (inglés, protocolo §4.1) mostrado tal cual a la persona | 6 sitios: `AdbuddyApp.attachUri`/`files/Drafts.kt` (vía `UploadFailed`), `files/Downloads.kt`, `chat/ChatController.kt` (2), `ui/RemoteCard.kt`. Son los únicos huecos reales |
| `Notice.text` del host | `AdbuddyApp.notices` lo enseña en un snackbar «tal como lo escribió el host». Hoy en inglés |
| Idioma | La app **no tiene código de idioma** (cero `Locale`/`LocaleManager`), ni `localeConfig`, ni `lint.xml`. `minSdk` 30, `targetSdk` y `compileSdk` 37, sin AppCompat |
| Controles que ya existen | `lintDebug` corre en la CI (`android.yml`) y `MissingTranslation` es un error por defecto, así que ya no se puede perder una clave. No hay nada que compare marcadores ni plurales, ni nada que busque texto suelto. 46 ficheros de test JVM, sin Robolectric; 1 instrumentado (`TokenProviderTest`) |
| Canales de notificación | Se crean en `AdbuddyApp.onCreate` con el nombre traducido **en ese momento**: un cambio de idioma con el proceso vivo los deja con el nombre viejo |

### 1.2 Host Go (`host/`, 25 936 líneas sin tests, 102 ficheros de test)

| Qué | Medido |
|---|---|
| Lo que ya está en los dos idiomas | Las reglas del clasificador: 156 reglas YAML con `es:` (`adb.yaml` 34, `shell-change.yaml` 34, `shell-read.yaml` 88) y 26 `Text{EN, ES}` en Go (`classify/*.go`), con `Text.In(lang)`; el título y el detalle de `adbuddy remote on` (`remoteConfirmTitle/Detail`); el prefijo «Deshacer: » (`undoPrefix`); 3 títulos de ayudas en `daemon/ui.go`. `config.language` ("en" o "es") y `Config.Lang(getenv)`, que mira `LC_ALL`, `LC_MESSAGES` y `LANG` |
| Hueco de detección | **En Windows `Lang` da siempre "en"** (no hay `LANG`) salvo que se escriba `language` en `config.toml`. No se lee el idioma de la interfaz |
| Lo que no está | Todo lo demás. Literales con frase (≥ 2 palabras) fuera de tests: `cli` 554 (en 30 ficheros; el mayor, `doctor.go` 84, `setup.go` 62, `remotecmd.go` 57, `phone_files.go` 45), `classify` 219 (casi todo ya bilingüe), `daemon` 103, `phoneapp` 64, `platformtools` 24, `pairing` 18, `precheck` 14, `claudecode` 14, `ipc` 12, `device` 10, `bridge` 10, `session` 9 y unos 40 más repartidos. Total ≈ 1 100, de los que **≈ 470 son frases para la persona que irán al catálogo** (más ≈ 70 razones del clasificador, que van en `Text`, §6.5). El resto: causas técnicas, argv, regex, trazas, datos de test. Es una estimación por `grep`: el plan de cada fase la fija contando |
| CLI | 49 líneas `Short`/`Long`/`Example` (15 ficheros), 31 ayudas de flags, 116 llamadas a `NewError`/`ConfigError`/`TransientError`/`UsageError` (cada una con mensaje y arreglo), 151 `Fprint*`, 31 `ipc.Exit{Error:…}`, 27 `fail(…)` en `daemon/ui.go`, 17 `OnNotice`/`OnWarning`, y 22 lecturas de `ADBUDDY_SOURCE` |
| Texto que va al móvil | `Notice.text` (`phone.go`, `phone_agent.go`, `bridge/confirm.go` con `StaleReplyText`), `ErrorReply.text` (`phone_chat.go`, `phone_remote.go`, `phone.go`), y el título y el detalle de `confirm.request` y `permission.request` |
| Texto que sale del host a la persona en el PC | `confirm/confirm.go` (la pregunta 🔴 de la terminal, 6 frases), `session/attach.go` (`AgentEnded`) y la salida de todos los comandos para personas |
| Tests | 34 ficheros de test en `cli` con 259 aserciones `Contains(` sobre texto en inglés; el motor de tests no fija el idioma: la salida de `Config.Lang(os.Getenv)` depende de la máquina |
| Dependencias | Ninguna de i18n. Ya están `BurntSushi/toml`, `yaml.v3` y `golang.org/x/sys`, que trae `windows.GetUserPreferredUILanguages`: **M6c no añade dependencias** |

### 1.3 Qué texto ve la persona y quién lo escribe hoy

Hay cinco clases, y cada una pide un trato distinto (§3):

1. **Marco de la interfaz** (botones, títulos, estados): lo escribe cada interfaz. App: recursos, ya en/es. Host: sin traducir.
2. **Lo que va a pasar** (título y detalle de una pregunta 🔴, de un permiso, entradas del diario): lo escribe el host a partir de reglas. Ya bilingüe el título; **las razones (`reason`) están solo en inglés** y entran en el `detail` de la pregunta (§4.1).
3. **Mensajes de estado del host al móvil** (`notice`, `error.text`): hoy en inglés.
4. **Mensajes de máquina** (códigos de error, claves JSON, `--json`, trazas, argv, causas técnicas): no se traducen.
5. **Lo que viene del agente o del comando** (burbujas, resúmenes de herramientas, `rawCommand`, salida de adb): nunca se traduce.

## 2. Alcance

M6c deja hecho:

1. **App:** cero texto visible suelto; los 6 sitios de `ErrorReply.text` salen de recursos por `code`; horas y tamaños según el idioma; plurales completos; canales y notificación al cambiar de idioma; idioma por app en Android 13+ (`localeConfig`); fallback EN (§5).
2. **Host:** paquete `host/internal/i18n` (catálogos embebidos, sin dependencias), detección de idioma completa con Windows y `ADBUDDY_LANG`, y traducción de todo lo que ve una persona: ayuda de cobra, errores y mensajes de `setup`, `serve`, `connect`, `doctor`, `attach`/`new`/`stop`, `journal`, `undo`, `env`, `remote`, `run`, la pregunta 🔴 de la terminal y las razones de las confirmaciones (§6).
3. **Controles que impiden la regresión:** en la app, un test de paridad (claves, marcadores y plurales) y un test de texto suelto, más un subconjunto de reglas de lint; en el host, un test de paridad de catálogos, otro que comprueba que cada clave usada existe, y otro que pasa por el clasificador los corpus en es (§5.4, §6.6).
4. **Pruebas en español** de las superficies de seguridad (§9) y las validaciones manuales (§10).
5. **Contrato de añadir texto** para M5c y los demás (§7).

### 2.1 Fuera de M6c

- **Las pantallas de M5c** (apps protegidas, acceso a notificaciones, Ajustes): son de la sesión `nautilus`. **Su i18n se hace después**: M5c escribe sus cadenas en `values/` y `values-es/` desde el primer commit, con la convención de §7.1, y M6c no toca esos ficheros. Cuando M5c esté en `main` se pasan los controles de §5.4 sobre ellas y se revisa el castellano (§8, fase V). La fila «Idioma» de Ajustes (v1 §6) es de M5c; M6c solo le deja el ayudante `LanguageSettings` (§5.2).
- Las **skills y el contexto del agente** (M6b: `AGENTS.md`, `CLAUDE.md`, `NOTES.md` de `workspace/templates`): se quedan en inglés, que es para quien los lee, el agente.
- **Documentación y donaciones:** el README bilingüe y la guía de instalación son de M6; solo se añade a los README del host y de la app la receta de §7.2.
- **El idioma de los modelos y de los agentes:** M6c no les pide que contesten en un idioma.
- **Lo que viene del agente o del comando:** mensajes del chat, resúmenes de herramientas (la parte del agente), `rawCommand`, salida de adb, nombres de ficheros, texto de la pantalla del móvil. Nunca se traduce ni se reescribe.
- **El reconocimiento de los diálogos de Claude Code en inglés** (`session.mgr.Busy()`, `session/session.go`): no es i18n de ADBuddy, depende de lo que pinta Claude Code. No se toca (ya tiene su cierre en M4 §6.2 por otra vía).
- **Más idiomas** que en y es (§13) y plataformas de traducción.
- **Cambios de protocolo.** `hello.lang`, un `code` en `notice` y similares quedan para v1.1 si hace falta (§12, ruling 1).

### 2.2 Criterios de éxito

- Un móvil con el sistema en español muestra **toda** la app en español: pantallas, notificaciones, canales, acciones, tarjetas de confirmación y de permiso, errores. En inglés, todo en inglés. En cualquier otro idioma, en inglés.
- En Android 13+ se puede poner la app en un idioma distinto del del sistema desde Ajustes del sistema (Apps → ADBuddy → Idioma), y la app y sus notificaciones siguen el cambio sin reiniciar.
- Con el PC en español (variable, `config.toml` o idioma de interfaz de Windows) `adbuddy --help`, `setup`, `doctor`, `serve`, los errores y la pregunta 🔴 de la terminal salen en español. `ADBUDDY_LANG=en` lo fuerza a inglés.
- **Nada de lo que decide la persona cambia con el idioma** (§4): la misma tarjeta 🔴 dice lo mismo en en y en es, con el mismo comando, nivel y aviso `special`, y una clave que falte en es sale en inglés, nunca vacía ni como nombre de clave.
- `--json`, los códigos de error, las claves y la salida para agentes (`ADBUDDY_SOURCE=agent`) son idénticos a los de hoy, byte a byte, en cualquier idioma.
- Los tests existentes pasan **sin tocar sus aserciones** (el idioma de test se fija en inglés).
- Un PR que añada una cadena solo en un idioma, con marcadores distintos, o texto suelto en `ui/`, `notify/` o `confirm/`, **falla la CI** (la app) o `go test` (el host).

## 3. Quién habla qué idioma

**Regla general:** cada lado escribe en su propio idioma lo que es suyo y no se envían idiomas por el cable. La app usa el idioma del móvil (o el de la app, en Android 13+). El host usa el idioma del PC. Cuando el host escribe algo que sale en el móvil (el título de una confirmación, un `notice`), sale **en el idioma del host**, como hasta ahora; la app solo le pone el marco.

| Texto | Quién lo escribe | Idioma | Se traduce | Dónde |
|---|---|---|---|---|
| Botones, títulos, estados, notificaciones, canales de la app | La app | Móvil | Sí | `res/values*/strings.xml` |
| Marco de la tarjeta 🔴 y de permiso, aviso `special` | La app (y la terminal en el PC) | Móvil / PC | Sí | Recursos / catálogo del host |
| Título y detalle (incluidas las razones) de una confirmación 🔴 | El host (reglas) | PC | Sí, completo (§4.1) | `classify` (`Text`), `precheck`, `daemon/pcwrite.go` |
| `description` del diario | El host, al escribir la entrada | PC, en ese momento | Sí; no se reescribe después (la entrada está firmada) | `classify` |
| `reason` del diario | El host | Inglés | **No** (persistido y leído por el agente) | |
| `notice.text` | El host | PC | Sí | Catálogo |
| `error.text` | El host o la app | Inglés (protocolo) | **No** en el cable; la app muestra su propio texto por `code` | `ErrorText` (§5.3) |
| Resumen de herramienta (`tool.activity`, `permission.request.summary`) | El host (verbo) más el agente (objeto) | PC (el verbo) | Solo el verbo («Leer», «Editar»…); lo del agente, tal cual | `claudecode/rule.go` |
| Salida de los comandos **para personas** | El host | PC | Sí | Catálogo |
| Salida de los comandos **para agentes** y de la shim | El host | Inglés | **No** | |
| `--json`, códigos, claves, `ipc.Exit`, mensajes de la shim | El host | Inglés | **No** | |
| Causas técnicas de paquetes internos (`pairing`, `platformtools`, `phoneapp`, `device`, `precheck` internos, errores de `adb`) | El host | Inglés | **No**; se citan como causa dentro de una frase traducida | |
| `AGENTS.md`, `CLAUDE.md`, `NOTES.md` | El host (plantillas) | Inglés | **No** (M6b) | |

**Comandos para personas** (se traducen; salida en inglés si `ADBUDDY_SOURCE=agent` o `--json`): `setup`, `serve`, `connect`, `doctor`, `attach`, `new`, `stop`, `run` (lo anterior a lanzar al agente), `journal`, `undo`, `env`, `remote`, `version`, la ayuda de cada comando.
**Comandos para agentes** (la ayuda se traduce, lo que hacen y sus errores en inglés siempre): `screen`, `tap`, `type`, `foreground`, `screenshot`, `send`, `hook` y la shim de `adb`. Un agente puede ejecutar también los de personas, y por eso la marca `ADBUDDY_SOURCE=agent` (ya existe, 22 usos) pasa a ser «habla en inglés».

## 4. Seguridad

M6c toca texto que acompaña a decisiones del usuario. Los principios:

### 4.1 Lo que decide la persona no depende de un idioma

- **La decisión viaja como código** (`allow`, `deny`, `allow_session`; `allow`, `allow_always`, `deny` en permisos; el código de dos cifras de la terminal). Ninguna se lee en lenguaje natural ni depende de una frase traducida. M6c no cambia ninguno.
- **Estos datos de una pregunta 🔴 no tienen versión traducida:** `rawCommand` (siempre entero y tal cual lo escribió el agente, como hoy), `level`, `special`, `expiresAt`, `allowSession`. La app los pinta igual en cualquier idioma.
- **Quién escribe qué frase:** el **qué** (título y detalle de la pregunta, razones) lo escribe el host, a partir de reglas, **completo y en un solo idioma**, el del PC (§3). El **marco** (el rótulo «El agente quiere:», los botones, el aviso de `special`) lo pone cada interfaz desde sus recursos. Esto no cambia la arquitectura de M2; M6c solo hace bilingüe lo que faltaba.
- **Una traducción incompleta nunca esconde información.** Orden de caída, siempre: idioma pedido → inglés. Nunca vacío y nunca el nombre de la clave. En la app lo hace la plataforma (los `values/` son el predeterminado). En el host, `Lang.T` y `Text.In` caen a EN, y las claves que no estén en EN dan un test rojo (§6.6). La pregunta se enseña **completa** en el idioma que sea, y en todos con el `rawCommand`.
- **El detalle de una pregunta 🔴 (`Detail`) incluye las razones** (`classify.Finding.Reason`, `precheck.Prepared.Reason`, `daemon/pcwrite.go`): hoy están solo en inglés y se cuelan en una frase en español. En M6c pasan a ser `Text` bilingües (§6.5) y los corpus del clasificador comprueban que **todas** tengan es. El `reason` del diario sigue siendo la versión en inglés.

### 4.2 Lo que sigue genérico o intacto

- **Pantalla de bloqueo:** la notificación de conversación y la de «Listo» siguen con versión pública genérica (`notif_public_connected`: «Connected»/«Conectado», `notif_done_title`), en los dos idiomas. M6c no cambia la visibilidad de ninguna notificación: la pregunta 🔴 sigue como está en M2 (acciones que exigen desbloquear para permitir). Un test (§9) comprueba que el texto público no lleva argumentos.
- **Los códigos y claves máquina** (`error.code`, `UNKNOWN_REQUEST`, códigos de `Error`, claves de `confirm.request`, `--json`) no se traducen nunca. En `--json` el `message` y el `fix` de los errores siguen en inglés (un agente los lee).
- **Salida para agentes en inglés** (§3), incluidos los mensajes de bloqueo de la shim («Blocked by ADBuddy: …») y las razones de la guarda de pantalla: el agente decide qué hacer con ellas, y una frase en otro idioma no le sirve más.
- **`classify.Printable`** (escapado de lo que viene del agente o del móvil) se sigue aplicando **a los argumentos antes de pasarlos a `T`**. El catálogo no lleva marcado ni escapes; un argumento nunca se interpreta como plantilla (`T` usa `fmt.Sprintf` con el patrón del catálogo, jamás con el argumento como patrón).

### 4.3 Reconocimiento de diálogos de Claude Code

`mgr.Busy()` busca frases en inglés de Claude Code («Do you want to proceed?»…). Eso es el idioma de Claude Code, no el de ADBuddy: **no se toca**. M4 ya lo cubre en lo importante (con adaptador, `chat.send` da `busy` si hay permisos pendientes o atención, aunque la pantalla no se reconozca).

## 5. Android

### 5.1 Mecanismo

- **Recursos estándar de Android:** `values/` (inglés, es el predeterminado y el fallback), `values-es/`. Nada de catálogos propios ni de bibliotecas.
- **Idioma de la app:** el del sistema por defecto. En **Android 13+** además, idioma por app: `res/xml/locales_config.xml` (`<locale android:name="en"/>`, `<locale android:name="es"/>`) y `android:localeConfig="@xml/locales_config"` en `<application>` (el atributo se ignora por debajo de la API 33, sin `tools:targetApi`). Con eso, Ajustes del sistema → Apps → ADBuddy → Idioma ya funciona.
- **Android 11 y 12 (`minSdk` 30):** solo el idioma del sistema. No se añade AppCompat (la app es `ComponentActivity` y `AppCompatDelegate.setApplicationLocales` no sirve ahí) ni un selector propio. `es-ES`, `es-MX`, `es-419`… caen en `values-es`.
- **`androidResources.localeFilters`** a `en` y `es` en el `build.gradle.kts` quita de la app los recursos de otros idiomas de Compose y Material, que ADBuddy no ofrece. Si AGP 9 lo rechaza, se omite (el coste es un APK algo mayor). Se mide el tamaño antes y después.
- **Sin cambios de idioma propios en el código:** ningún `Locale`, `Configuration` ni `Resources` manual; la plataforma aplica el idioma de la app al contexto de todos los componentes (actividad, servicio, receptor).

### 5.2 Convención de claves (también la de M5c)

`<área>_<qué>`, en minúsculas con guion bajo, **como las 120 de hoy**. Áreas ya en uso: `action_`, `chat_`, `channel_`, `confirm_`, `consent_`, `file_`, `filter_`, `home_`, `menu_`, `notif_`, `perm_`, `remote_`, `session_`, `status_`, `tool_`, `a11y_`. Áreas reservadas para M5c: `protected_`, `access_` (acceso a notificaciones), `onboarding_`, `settings_`. Reservadas para M6c: `error_` (§5.3) y `settings_language*`.

- **Una clave, un texto.** No se reutilizan claves de una pantalla en otra con otro sentido, ni se componen frases con trozos («Hay » + n + « esperando»).
- **Marcadores posicionales siempre** (`%1$s`, `%2$d`), aunque haya uno solo, para que es pueda cambiar el orden.
- **Plurales con `<plurals>`** y `pluralStringResource`. Todo texto con un número que cambie la frase lleva `one` y `other` en los dos idiomas (en y es solo usan esos dos). `perm_more` ya es así.
- **`translatable="false"`** solo para el nombre propio y lo que no es texto (`app_name`, `home_title`).
- **Los textos de recursos no incluyen datos del agente más que como argumento** (`%1$s`), y los argumentos nunca se interpretan como formato.
- **Tono (es):** tú, «móvil» (no «teléfono»), «PC», «agente», «diario», «depuración inalámbrica», «ficheros», sin «usted» ni voseo, igual que las 118 de hoy. Glosario completo en §7.3.
- **Helper `LanguageSettings`** (`ui/LanguageSettings.kt`): en API 33+ abre `Settings.ACTION_APP_LOCALE_SETTINGS` con el paquete de la app; por debajo, no hace nada y devuelve `false`. Claves `settings_language` («Language»/«Idioma») y `settings_language_hint`. **No añade pantalla ni fila.** La pone M5c en Ajustes (v1 §6), y si Ajustes no existe cuando acabe M6c, queda en `pendientes-m3-m5.md` (en Android 13+ el usuario ya puede cambiarlo desde el sistema).

### 5.3 Qué cambia en el código

1. **`ErrorText`** (`bridge/ErrorText.kt`): `ErrorReply` → texto, por `code`. Claves `error_unsupported`, `error_invalid`, `error_not_found`, `error_no_agent`, `error_busy`, `error_too_large`, `error_checksum`, `error_failed`, `error_unavailable`, `error_protected`, `error_expired`, `error_ambiguous` y `error_unknown` (12 códigos de `ErrorCode` más uno por defecto). Los 6 sitios de §1.1 lo usan. Para `failed` se añade el `text` del host, en inglés, detrás del texto propio y entre paréntesis; la regla se extiende a `busy`, `unavailable` y `too_large` porque el host explica en ellos la causa accionable (una pregunta abierta en el PC, Tailscale apagado en el móvil, la bandeja llena), y a un código desconocido, que sin `text` muestra su código; para el resto, `text` se ignora. Cuesta 13 cadenas y quita el único inglés que veía una persona con la app en español.
2. **Horas:** `DateTimeFormatter.ofLocalizedTime(FormatStyle.MEDIUM)` (notificación de confirmación) y `SHORT` (`RemoteCard`), con el `Locale` por defecto: «14:05:09» o «2:05:09 PM» según el idioma. Los tests de `HostClockTest` y `RemoteUiTest` fijan `Locale.ROOT`/`ENGLISH`.
3. **Tamaños:** `android.text.format.Formatter.formatShortFileSize(context, n)` en lugar de `sizeText`.
4. **Canales y notificación al cambiar de idioma:** `AdbuddyApp.onConfigurationChanged(newConfig)` (la plataforma lo llama también para el idioma por app) vuelve a llamar a `createNotificationChannels()` (crear un canal que ya existe solo actualiza su nombre) y pide a `BridgeService` que repinte su notificación fija. Sin receptor de `ACTION_LOCALE_CHANGED` y sin permisos nuevos.
5. **Teclas rápidas:** `Esc`, `Ctrl+C`, `Tab`, `Enter` y las flechas son nombres de teclas de terminal, iguales en todos los idiomas; quedan como están y entran en la lista de excepciones del test de §5.4 con ese motivo.
6. **`UiExecutor`:** sus 25 mensajes van al agente y se quedan en inglés; excepción explícita y comentada en el test.

### 5.4 Cómo se comprueba, sin texto suelto ni claves a medias

Todo corre en `testDebugUnitTest` (JVM, sin Robolectric, igual que los demás tests) y `lintDebug`, que ya están en `android.yml`.

- **`StringsParityTest`** (`i18n/`): también exige que exista `error_<code>` por cada constante de `ErrorCode` (por reflexión sobre el `object`) y `error_unknown`. Lee `values/strings.xml` y `values-es/strings.xml` con el XML de la JVM (la ruta llega por una propiedad de sistema desde `build.gradle.kts`, como las fixtures) y comprueba, por clave: mismas claves (salvo las `translatable="false"`); mismo conjunto de marcadores con el mismo índice y tipo (`%1$s`≠`%1$d`); **sin marcadores sin índice** cuando hay más de uno; mismo conjunto de `quantity` y que cada `plurals` tenga al menos `one` y `other` en los dos; ningún texto vacío; ningún texto igual en en y es salvo una lista corta explícita (`perm_deny`, y lo que M5c justifique); y que el apóstrofo y las comillas estén escapados como Android pide (un `'` sin `\` rompe el `aapt` en silencio).
- **`NoHardcodedTextTest`** (`i18n/`): recorre `src/main/kotlin/**` con dos reglas.
  1. **Sumideros de texto visible, en cualquier fichero:** un literal como argumento de `Text(`, `text =`, `contentDescription =`, `label =`, `title =`, `placeholder =`, `setContentTitle(`, `setContentText(`, `setSubText(`, `setTicker(`, `Toast.makeText(`, `NotificationChannel(`, `setName(`, `setLabel(`, `addAction(` o `Action.Builder(`.
  2. **Literales con frase** (dos o más palabras de letras, sin contar lo que va dentro de `${…}`) en `ui/`, `notify/`, `confirm/`, `control/`, `files/`, `AdbuddyApp.kt`, `MainActivity.kt`, `ChatBubbleActivity.kt`, `bridge/BridgeService.kt` y `a11y/Overlay.kt`.

  Se saltan: lo que esté dentro de una función con `@Preview` (el recorrido salta el cuerpo hasta cerrar la llave), los comentarios, las líneas con `Log.`, `throw`, `require(`, `check(` o `error(`, y las líneas de `i18n-exceptions.txt`, una por línea con su motivo (`a11y/UiExecutor.kt`: mensajes al agente; `term/QuickKeys.kt`: nombres de teclas; `ActivityScreen.kt` `levelIcon`: emojis). La lista es corta y va en el mismo árbol de test: añadir una excepción se ve en la revisión. No es un parser de Kotlin, es una regex con lista de excepciones; el riesgo y el motivo están en §11.
- **Lint:** `lint { checkOnly += ["MissingTranslation", "ExtraTranslation", "StringFormatCount", "StringFormatMatches", "StringFormatInvalid", "MissingQuantity", "ImpliedQuantity", "PluralsCandidate"]; abortOnError = true }` mantiene los controles de la plataforma como segunda red, sin sustituir a los dos tests.
- **Instrumentado** (`LocalesTest` en `androidTest`, sin móvil del usuario, en el emulador de `emulator.yml`): con `createConfigurationContext` en `en` y en `es`, `confirm_title`, `perm_more` (1 y 3) y `notif_working_on` dan lo esperado, y una clave que solo esté en `values/` sale en inglés en es.

### 5.5 Notificaciones

- Las notificaciones usan `ctx.getString` en el momento de construirse; con el idioma de la app aplicado al contexto, salen en el idioma vigente y se repintan con §5.3.4.
- Los textos que construyen las notificaciones (`ConversationView`, `ConfirmUi`) son ya testeables en JVM con un `getString` falso; M6c no cambia su estructura.
- Texto público (pantalla de bloqueo): §4.2.

## 6. Host Go

### 6.1 Mecanismo: `host/internal/i18n`

Un paquete pequeño, sin dependencias nuevas, sin estado global:

- **`type Lang string`** (`"en"`, `"es"`), con `Lang.T(key string, args ...any) string` y `Lang.TN(key string, n int, args ...any) string`. **Nada de variable global de idioma:** cada llamador recibe su `Lang` (como ya recibe `daemon.Options.Lang`), así dos tests o dos goroutines en idiomas distintos no se pisan, y `-race` no tiene nada que mirar.
- **Catálogos embebidos** (`//go:embed catalog/en.toml catalog/es.toml`), TOML plano de claves con puntos entre comillas, parseado una vez con `BurntSushi/toml` (ya está). Se eligió TOML y no un mapa de Go porque admite textos largos de varias líneas (los `Long` de cobra, las guías de `setup`) legibles en el diff y revisables por quien sabe español pero no Go:

  ```toml
  "doctor.short" = "Check that ADBuddy, adb, the phone and the app are ready, and say how to fix what is not"
  "undo.done.one"   = "Undid %[1]d change."
  "undo.done.other" = "Undid %[1]d changes."
  ```

- **Claves:** `<comando o paquete>.<qué>[.<variante>]`, en minúsculas: `setup.step.pair`, `doctor.check.adb.missing`, `confirm.prompt.code`, `error.not_running.message`, `error.not_running.fix`.
- **Marcadores `fmt`** con **índice explícito** siempre que haya más de uno (`%[2]s`) para poder reordenar en es. Los argumentos que vienen del agente o del móvil se escapan **antes** con `classify.Printable` (§4.2).
- **Plurales:** `TN` busca `<clave>.one` si `n == 1` y `<clave>.other` si no, con `n` como primer argumento. En y es no hay más formas.
- **Caída:** `T` busca en el idioma pedido, luego en `en`, y si tampoco, devuelve la clave. Eso último solo pasa con un error de programación que el test de §6.6 caza antes.
- **`Text` de `classify`** (EN, ES en reglas y en Go) sigue como está y gana un tercer uso, el de las razones (§6.5). `Lang.In(Text)` es un atajo; `Text.In(lang string)` se mantiene.

### 6.2 Detección

`i18n.Resolve(getenv func(string) string, configLang string) Lang`. Se mira en este orden y **la primera fuente que esté definida decide** (lo que no sea español da `en`):

1. **`ADBUDDY_LANG`** (`en`, `es`, o con región: `es_ES`): lo escribe el usuario para una sola ejecución, y es lo que fijan los tests.
2. **`language` de `config.toml`** (ya existe y ya se valida como `"en"`/`"es"`).
3. **`LC_ALL`, `LC_MESSAGES`, `LANG`**, el primero que no esté vacío decide, como dice POSIX. `es`, `es_ES.UTF-8`, `es-ES`, `es_419` → `es` (prefijo `es` seguido de fin o de `_`, `-`, `.`, `@`); `C`, `POSIX` y cualquier otro → `en` y **no se sigue buscando**. `LANGUAGE` no se lee.
4. **Windows:** `windows.GetUserPreferredUILanguages(MUI_LANGUAGE_NAME)` (con `x/sys`, que ya es dependencia), el primero de la lista. En `detect_windows.go`; el resto de plataformas tienen un `detect_other.go` que no devuelve nada. En macOS manda `LANG`, que Terminal pone según el idioma del sistema; no se ejecuta `defaults`.
5. **`en`.**

**Dónde se llama:** una sola vez, al principio de `cli.mainContext` y de `serve`, con el `getenv` de la `App` y, para (2), un `config.Load` que **ignora los errores** (una configuración rota no debe impedir enseñar `--help` ni el propio error en el idioma correcto). El resultado va a un campo nuevo `App.Lang` y a `daemon.Options.Lang` (ya existe). `config.Config.Lang(getenv)` pasa a delegar en `i18n.Resolve`, así que el daemon y la ayuda de `remote` no se pueden desincronizar.

**Tests:** una `App` construida a mano en un test tiene `Lang == ""`, que se entiende como `en`. La detección real (con el idioma de Windows) solo corre desde `mainContext`, que además lee `ADBUDDY_LANG` primero; los tests que llaman a `Main` fijan `ADBUDDY_LANG=en` (§9.1).

**Consola de Windows:** para escribir acentos y `ñ` hay que estar en UTF-8. Los comandos para personas llaman, en Windows y solo si la salida es una consola, a `SetConsoleOutputCP(65001)` y lo restauran al salir (hoy ya se escriben emojis UTF-8, así que esto solo cierra el caso de la consola antigua con página 850 o 1252).

### 6.3 Qué se traduce

| Bloque (fase) | Qué | Claves de catálogo (estimadas con §1.2) |
|---|---|---|
| Ayuda de cobra (H1) | `Short`, `Long`, `Example` (49 líneas), ayuda de los 31 flags, las plantillas de uso y de ayuda (cabeceras «Usage:», «Flags:», «Available Commands:»…), la plantilla de la versión y la tabla de códigos de salida de `adbuddy --help` | ≈ 90 |
| Errores (H2) | Los 116 `NewError`/`ConfigError`/`TransientError`/`UsageError` (mensaje y arreglo; algunos arreglos se repiten) | ≈ 190 |
| Mensajes de comandos para personas (H3) | `Fprint*` (151 llamadas) de `setup` (incluida la guía del móvil), `doctor`, `serve`, `appsetup`, `connect`, `remotecmd`, `undo`, `journal`, `attach`/`new`/`stop`, `env`, `run`, `phone*.go` (avisos al terminal), `invariant`, `doctor_claude`; sin contar las ayudas ni los errores de las filas anteriores | ≈ 150 |
| Confirmación, avisos y estado (H4b) | `confirm/confirm.go` (6 frases), `session.AgentEnded`, `daemon` (17 `OnNotice`/`OnWarning`, `undoPrefix`), `remoteConfirmTitle/Detail`, los 7 verbos de `ToolSummary`, `Notice.text` al móvil (≈ 8, con `StaleReplyText`) | ≈ 45 |
| **Total del catálogo** | | **≈ 475** |
| Razones de las confirmaciones (H4a) | `classify` (53 sitios: 12 en `shell.go`, 27 en `wrappers.go`, 8 en `classify.go`, 2 en `arith.go`, 4 en `vars.go`, más `special.go`), `precheck` (≈ 10), `daemon/pcwrite.go` (≈ 6). Van en `Text{EN, ES}`, no en el catálogo (§6.5) | ≈ 70 |

El plan de cada fase fija el número exacto contándolo con el test de §6.6.

**No se traduce** (§3): `Error.Code`, `Error.Message`/`Fix` en `--json`, `ipc.Exit.ErrorCode`, claves JSON, nombres de comandos y de flags, valores de flags, las líneas `Run:` de `doctor` (son órdenes), los textos de la shim y de las ayudas para agentes, las causas técnicas (`fmt.Errorf` de paquetes internos), `Entry.Reason` y los mensajes de bloqueo («Blocked by ADBuddy: …»).

### 6.4 Errores con el mensaje en dos formas

`cli.Error` (hoy `Code`, `Message`, `Fix`) gana campos internos `key`, `fixKey` y `args`. Un constructor nuevo `NewErrorT(code, key, fixKey string, args ...any)` calcula **`Message` y `Fix` en inglés al construir** (son lo que ven `--json`, el agente y los 259 `Contains` de los tests, que no cambian) y guarda las claves. `WriteFailure(…)`, solo en modo humano y sin `ADBUDDY_SOURCE=agent`, vuelve a pintar el error con `app.Lang`. Así el cambio por llamada es mecánico (`NewError("X", "…", "…")` → `NewErrorT("X", "x.message", "x.fix")`), los errores globales (`notRunning`, `alreadyRunning`…) siguen siendo variables de paquete, y un error que no tenga clave sigue saliendo (en inglés).

La misma idea para los mensajes de `Fprint*`: `app.T(key, args...)` (usa `app.Lang` y devuelve inglés si es agente o `--json`).

### 6.5 Razones bilingües (la parte de seguridad)

`classify.Finding.Reason` y `PCWrite.Reason` pasan de `string` a `Text` (EN, ES), y `joinReason` une las dos formas. El `Entry.Reason` del diario guarda `.EN`; `Result.Detail(lang)` usa `.In(lang)`. `precheck.Prepared.Reason` y las de `pcwrite` siguen el mismo camino. Las razones con una parte dinámica (la palabra concreta, la ruta) la meten como `{word}` o `{path}` con el mismo `render` que los títulos, que ya escapa con `Printable`. El resultado: el `detail` de una pregunta 🔴 de un PC en español es **íntegramente** español salvo el `rawCommand` y lo que cite del móvil.

### 6.6 Cómo se evita la regresión

- **`TestCatalogParity`:** las claves de en y es son las mismas; mismos marcadores con el mismo índice y tipo (un analizador de `%[n]verbo`); ningún texto vacío; los `.one`/`.other` están en pareja; y ningún texto igual en en y es salvo una lista explícita (`"OK"`, nombres de órdenes…), para cazar el copia-y-pega sin traducir.
- **`TestKeysExist`:** recorre con `go/ast` los paquetes de `host/` y comprueba que **todo** `T("…")`, `TN("…")`, `NewErrorT("…","…")` con literal tiene su clave en `en.toml`, y que no hay claves en el catálogo que no use nadie (para que los catálogos no acumulen basura). Las claves construidas en tiempo de ejecución se declaran en una lista corta del propio test.
- **`TestRulesBilingual`** (clasificador): las 156 reglas YAML, los `Text{}` de Go y todas las razones tienen es no vacío, con los mismos `{marcadores}` que en. Y un test que pasa por el clasificador `testdata/habitual.jsonl` y `differential.jsonl` y comprueba que **ningún `Title` ni `Reason` de ningún resultado** cae a EN (`ES == ""` o `ES == EN` fuera de una lista).
- **Linter de texto suelto del host:** un test `go/ast` que falla si un `fmt.Fprint*(app.Stderr|app.Stdout, "literal con letras")` o un `cobra.Command{Short|Long|Example: "literal"}` aparece en `host/internal/cli` fuera de la lista de excepciones (agente, `--json`, depuración). Es el equivalente de `NoHardcodedTextTest` y sale de la medida de §1.2.
- Todos corren en el `go test ./...` de `host.yml`, en Linux, macOS y Windows.

## 7. Coordinación

### 7.1 Con M5c (`nautilus`) y las demás sesiones

Para pasar al supervisor y a `nautilus` tal cual:

1. **Desde ya, antes de que exista M6c:** cada cadena nueva de la app va en **los dos** ficheros (`values/strings.xml` y `values-es/strings.xml`) en el mismo commit; con la convención de §5.2 (`protected_*`, `access_*`, `onboarding_*`, `settings_*`); con marcadores posicionales; con `plurals` cuando haya números; y **cero literales** en composables, notificaciones y `contentDescription`. M5a ya lo hace así (`overlay_pill` está en los dos).
2. **Cuando M6c llegue a `main`**, los tests de §5.4 se aplican a todo el árbol, M5c incluido. Quien integre segundo rebasa y arregla lo que salga (cabe en minutos: una clave que falta o un literal).
3. **M6c no toca las pantallas de M5c ni sus cadenas.** Cuando M5c esté en `main`, la fase V de M6c las revisa (castellano, glosario, controles) y corrige solo texto.
4. **El host:** M5b/M5c que añadan mensajes para personas al host usan `app.T`/`NewErrorT` si M6c ya está; si no, escriben el inglés en el sitio y M6c lo migra. Nada de `Text{ES:…}` nuevos fuera de reglas.

### 7.2 Receta de añadir una cadena (va en `host/README.md` y en `android/README.md`)

- **App:** añade la clave a los dos ficheros; `./gradlew :app:testDebugUnitTest` dice si falta algo o los marcadores no cuadran.
- **Host:** añade la clave a `catalog/en.toml` y `catalog/es.toml`, úsala con `app.T("…")` y ejecuta `go test ./internal/i18n/... ./internal/cli/...`.

### 7.3 Glosario y tono (es)

móvil (no «teléfono»); PC; agente (de código); diario; confirmación; depuración inalámbrica; emparejar; ficheros (no «archivos», como los de hoy); notificaciones; «Pausar», «Cortar acceso», «Restaurar acceso»; tuteo; sin anglicismos que tengan palabra («pull request» sí se queda), sin el punto final en botones y rótulos. Los nombres de órdenes, flags y claves (`adbuddy serve`, `--json`, `ADBUDDY_LANG`) no se traducen nunca.

### 7.4 Ficheros y dueños

| Fichero | Dueño | M6c toca | Cuándo |
|---|---|---|---|
| `host/internal/i18n/**`, `host/internal/cli/lang.go` | M6c | todo (nuevo) | Fase H0 |
| `host/internal/config/config.go` | M1 | `Lang` pasa a delegar en `i18n.Resolve` | H0 |
| `host/internal/classify/**` (`rules.go`, `classify.go`, `shell.go`, `wrappers.go`, `special.go`, `rules/*.yaml`, `testdata/`) | M2.5 | razones bilingües (§6.5) y sus tests | H4a |
| `host/internal/precheck/**`, `host/internal/daemon/pcwrite.go` | M1/M2.5 | las razones | H4a |
| `host/internal/confirm/confirm.go`, `session/attach.go`, `daemon/daemon.go` (`OnNotice`, `OnWarning`, `undoPrefix`), `claudecode/rule.go` | M1/M3/M4 | texto para personas | H4b |
| `host/internal/cli/*.go` (todos) | compartido | texto de comandos para personas, ayuda y errores | H1-H3 |
| `host/internal/bridge/confirm.go` | tern (M3) | `StaleReplyText` pasa a opción | H4b |
| `host/cmd/adbuddy/*_test.go`, `host/internal/cli/main_test.go` | compartido | solo el `TestMain` que fija `ADBUDDY_LANG=en` | H0 |
| `android/app/src/main/res/**`, `AndroidManifest.xml` (solo `localeConfig`) | M2 / compartido | cadenas `error_*` y `settings_language*`, `locales_config.xml` | A2 |
| `android/app/src/main/kotlin/**` | compartido | `ErrorText`, horas y tamaños, `AdbuddyApp.onConfigurationChanged`, `LanguageSettings` | A2 |
| `android/app/build.gradle.kts` | tern | `lint {}`, `localeFilters`, propiedad de la ruta de recursos para los tests | A1/A2 |
| `android/app/src/test/**`, `src/androidTest/**` | compartido | tests de §5.4 | A1/A2 |
| `docs/superpowers/plans/pendientes-m3-m5.md` | cada uno su sección | solo M6c | cada fase |
| `docs/superpowers/specs/…protocolo…` | supervisor | una nota en §4.1: «el `text` de `error` es detalle para el agente y el registro; la app muestra su texto por `code`» | V (con ok del supervisor) |

**Conflictos esperados:** `host/internal/cli/*.go` lo tocan M5b, M6a (`appsetup.go`, `phone.go`, `setup.go`) y M4. Por eso **H1, H2 y H3 empiezan cuando M4, M5b y M6a estén en `main`**, y cada fase rebasa antes de empezar. H0, A1, A2, H4a y H4b no esperan a nadie (H4b toca ficheros que no tocan los demás salvo `daemon.go`, una línea por función, que se rebasa sin dolor).

## 8. Fases, orden y esfuerzo

Cada fase compila y pasa todos sus tests por sí sola, con una réplica acumulada tarea a tarea al escribir el plan. Los números de claves salen de §6.3 y se fijan al contarlos en cada plan.

| Fase | Contenido | Se verifica con | Tareas | Revisor |
|---|---|---|---|---|
| **H0** Base del host | `i18n` (catálogo vacío más `T`, `TN`, `Resolve`, Windows), `App.Lang`, `config.Lang` delega, `TestMain` que fija `ADBUDDY_LANG=en` en `cli`, `daemon`, `cmd/adbuddy`, `confirm`, `session`; `SetConsoleOutputCP`; tests de detección con tabla (`es_ES.UTF-8`, `C`, `LC_ALL` sobre `LANG`…); `TestCatalogParity` y `TestKeysExist` (vacíos pero activos) | `go test ./...` igual que antes, más los nuevos | 1 | `adbuddy-reviewer` |
| **A1** Controles de la app | `StringsParityTest` (sin la parte de `error_*` hasta A2), `NoHardcodedTextTest` con su lista de excepciones, `lint {}`, propiedad de la ruta. Sin cambios de comportamiento | `testDebugUnitTest` y `lintDebug` verdes sobre el árbol actual (hoy pasan tal cual, con las excepciones de `UiExecutor`, `QuickKeys` y `levelIcon`). Se comprueba además que **fallan**: se añade a mano un literal y una clave a medias y se ve el rojo, antes de quitarlos | 1 | `adbuddy-reviewer` |
| **A2** Arreglos de la app | `ErrorText` y las 13 `error_*`, horas y tamaños según idioma, `locales_config` y manifest, `localeFilters`, `onConfigurationChanged`, `LanguageSettings`, `LocalesTest` instrumentado; se vacía la lista de excepciones de A1 de lo que ya no hace falta | los de A1 más `LocalesTest` en el emulador | 1 (2 si el instrumentado se atasca) | `adbuddy-reviewer-opus` (muestra texto de errores y notificaciones) |
| **H4a** Razones bilingües | `Reason` → `Text` en `classify`, `precheck`, `pcwrite`; `Detail(lang)`; `TestRulesBilingual` y el test de corpus en es; el diario sigue con `.EN` | `go test ./internal/classify/... ./internal/daemon/... ./internal/precheck/...` y los corpus de `habitual.jsonl`/`differential.jsonl` **sin cambiar ni una línea** (el nivel no depende del idioma; sale un test que lo comprueba: mismo `Level`, `Special` y `Op` en en y es para todo el corpus) | 2 (clasificador; precheck y pcwrite) | `adbuddy-reviewer-opus` |
| **H4b** Confirmación y avisos | `confirm.go` (terminal), `session.AgentEnded`, `OnNotice`/`OnWarning`, `undoPrefix`, `remoteConfirm*` a catálogo, `ToolSummary`, `Notice.text`, `StaleReplyText` como opción de `bridge` | tests de seguridad de §9.2 | 1 | `adbuddy-reviewer-opus` |
| **H1** Ayuda de cobra | `Short`/`Long`/`Example`/flags/plantillas, tabla de códigos de salida | `--help` de cada comando en en (idéntico) y en es (sin claves a la vista); `TestKeysExist` | 1 | `adbuddy-reviewer` |
| **H2** Errores | `NewErrorT`, `WriteFailure` con idioma, los 116 sitios; `--json` idéntico byte a byte | test que ejecuta cada error en `--json` y en agente y lo compara con una grabación del inglés actual | 1 | `adbuddy-reviewer` |
| **H3** Mensajes de comandos | `setup`/`appsetup`/`connect`, `doctor`, `serve`/`phone*`/`remotecmd`/`attach`/`undo`/`journal`/`env`/`run`; el linter de texto suelto del host | tests existentes sin cambios (en); tests en es de §9.3; el linter | 3 (uno por bloque) | `adbuddy-reviewer` (el de `doctor` y `setup` mira que `Run:` y las órdenes no se traduzcan) |
| **V** Cierre | `README` del host y de la app (§7.2), `pendientes`, nota al protocolo, revisión del castellano de lo de M5c (si ya está), validaciones de §10 apuntadas en el plan | CI verde, revisión final de toda la rama | 1 | `adbuddy-reviewer-opus` |

**Orden:** H0, A1, A2, H4a y H4b (en cualquier orden entre A* y H4*, en paralelo porque son árboles disjuntos: Android y `host/`) → H1 → H2 → H3 → V. **H1, H2 y H3 esperan a M4, M5b y M6a** (§7.4); H0 y H4 empiezan ya.

**Esfuerzo (medido sobre §1):** **12 tareas** de implementador (Sonnet 5.5: H0 1, A1 1, A2 1, H4a 2, H4b 1, H1 1, H2 1, H3 3, V 1) más el plan y su revisión opus previa, que es obligatoria. Por volumen pesan H2 (≈ 190 claves) y H3 (≈ 150, en tres tareas de ≈ 50: `setup`+`appsetup`+`connect`; `doctor`+`doctor_claude`; `serve`+`phone*`+`remotecmd`+`attach`+`undo`+`journal`+`env`+`run`); cada texto es una clave en dos ficheros y un cambio mecánico en el sitio. Por riesgo, H4a (≈ 70 razones con partes dinámicas, tipo `string` → `Text`) y H4b (lo que decide el usuario), que llevan revisor opus. La app es pequeña: 13 claves nuevas (más `settings_language*`), 4 arreglos de código y 3 tests. Ficheros tocados: unos 60 en `host/` (casi los 34 de `cli`, más `classify`, `precheck`, `daemon`, `confirm`, `session`, `claudecode`, `bridge`, `config`), 3 nuevos en `host/internal/i18n/` (más 2 catálogos), unos 12 en `android/` más 4 de test. Si hay que recortar, se recorta H3 (`env`, `run` y lo menos visto), no H4.

## 9. Tests

### 9.1 Pruebas con el idioma fijado

- **Los tests existentes siguen en inglés sin cambios:** `TestMain` de `cli`, `daemon`, `confirm`, `session`, `claudecode` y `cmd/adbuddy` hace `os.Setenv("ADBUDDY_LANG", "en")` (gana a `config.language`, a `LANG` y al idioma de Windows, así que da igual la máquina; un binario lanzado por los tests de `cmd/adbuddy` lo hereda). Un test de `i18n` comprueba que `ADBUDDY_LANG` está primero.
- Ayudante `i18ntest.Lang(t, "es")` (`t.Setenv("ADBUDDY_LANG", …)`) para los tests en español.
- Los tests de la app que dependen del idioma fijan `Locale` explícito.

### 9.2 Seguridad (H4)

- **El nivel no depende del idioma:** para todo `habitual.jsonl` y `differential.jsonl`, `Classify` da el mismo `Level`, `Special`, `Op` y `Findings[].Rule` con `lang` en y es, y `Detail("es")` no contiene ninguna de las palabras de las razones en inglés de una lista.
- **La pregunta de la terminal en es:** `confirm.Terminal` en es contiene el `rawCommand` íntegro, el título en español, el aviso `special`, el código de dos cifras y las mismas decisiones: el código permite, cualquier otra cosa deniega, `eof` es «nadie»; con el mismo flujo en en y en es las decisiones salen iguales.
- **Un texto sin traducir cae a inglés, nunca vacío:** se carga un catálogo es al que se le quita una clave de la pregunta 🔴 y se comprueba que sale en inglés y que la pregunta se puede responder.
- **`ErrorText`:** para cada `ErrorCode`, hay una clave `error_<code>` en en y es (`StringsParityTest`), y un código desconocido da `error_unknown`; en `LocalesTest` (instrumentado) los textos salen en el idioma pedido.
- **Texto público:** `ConversationView` con `notif_public_connected` no usa argumentos (el test existente, ampliado a es).
- **Salida de agentes en inglés:** con `ADBUDDY_SOURCE=agent` y `ADBUDDY_LANG=es`, `doctor`, `undo`, `journal` y un error cualquiera salen idénticos a los grabados en inglés, y `--json` también.

### 9.3 Superficie en español (H1-H3)

Un test por bloque con `Lang=es` que ejecuta el comando contra el adb falso y comprueba, sin pegarse a la frase entera: que no aparece ninguna clave a la vista (`^[a-z]+\.[a-z_.]+$`), que las órdenes de `Run:` siguen intactas, que los argumentos (rutas, seriales) están, y que ningún texto del catálogo EN sale en la salida salvo los de la lista de excepciones. Hay un test «grabado» de `adbuddy doctor` y de la ayuda raíz en es para que un cambio de redacción se vea en el diff.

## 10. Validaciones manuales pendientes (no las corre el agente)

Todas necesitan el móvil o el PC del usuario, y ninguna modifica el móvil más allá de cambiar el idioma de la app:

1. **Pixel con Android 17, sistema en español:** instalar, abrir; onboarding, chat, actividad, tarjetas de permiso (`Sí`/`Sí, siempre`/`No`) y de confirmación 🔴, consentimiento, estado vacío, errores forzados (adjunto demasiado grande, modo remoto sin PC); revisar el castellano y que nada salga cortado (textos más largos que en inglés: botones de la notificación, «Permitir durante la sesión»).
2. **Idioma por app (Android 13+):** con el sistema en español, poner ADBuddy en inglés desde Ajustes → Apps → ADBuddy → Idioma, volver sin reiniciar el proceso: pantallas, notificación fija y canales (pulsación larga en la notificación → ajustes del canal) pasan a inglés; y vuelta a español.
3. **Notificaciones:** pregunta 🔴 y notificación de conversación en español, con la pantalla bloqueada (texto público genérico en español) y desbloqueada; hora de caducidad en 24 horas.
4. **Plurales:** 1 y 2 permisos esperando («1 más esperando»/«2 más esperando»).
5. **PC con Windows en español:** `adbuddy doctor`, `setup`, `serve` y una pregunta 🔴 en la terminal en español **sin** poner `language` ni `LANG`; acentos y `ñ` bien en Windows Terminal y en la consola clásica (`cmd.exe`); `ADBUDDY_LANG=en` lo pasa a inglés.
6. **PC y móvil en idiomas distintos** (PC en inglés, móvil en español): la tarjeta 🔴 sale con el marco en español y el título en inglés, es decir, mezcla pero completa (§12, ruling 1); revisar si molesta lo bastante para pedir `hello.lang` en v1.1.
7. **Una pregunta 🔴 con razón** (por ejemplo `adb push` que sobrescribe un fichero) en el móvil y en la terminal, en es: la razón sale en español.

## 11. Riesgos

- **Texto suelto que se cuela entre medias (M5c, M5b).** Mitigación: los controles de §5.4 y §6.6 en la CI, la convención desde ya (§7.1) y la fase V de repaso. Coste: un literal por PR.
- **`NoHardcodedTextTest` por regex.** Puede dar un falso negativo con una construcción rara (`val s = "…"; Text(s)`). Es una red, no una prueba: la revisión y el ES visto en el móvil siguen. El motivo de no usar el PSI de Kotlin: arrastraría el compilador al test y a la CI. Coste si me equivoco: algún literal pasa; se arregla al verlo.
- **Conflictos de `host/internal/cli/**`.** H1-H3 tocan casi todos sus ficheros. Mitigación: esperan a M4, M5b y M6a, cada fase rebasa y cambia cadenas, no lógica; los commits son por fichero.
- **Mezcla de idiomas PC/móvil** (§12, ruling 1). Aceptada; no es una pérdida de información.
- **Traducir de más.** Una orden traducida («Run:» → «Ejecuta:») rompería a un agente. Mitigación: lista de «no se traduce» en §6.3, test de §9.2 con `ADBUDDY_SOURCE=agent`, y el revisor de H3 la mira.
- **Cambiar la salida en inglés sin querer.** Mitigación: el inglés **es** el catálogo en.toml, extraído literalmente, y los 259 `Contains` de `cli` más la grabación de §9.2 lo cubren.
- **Un cambio de idioma con el servicio vivo en la app.** Mitigación: §5.3.4; si `onConfigurationChanged` no se llamara en alguna versión, la notificación se repinta en el siguiente evento y el canal en el siguiente arranque.
- **Windows y UTF-8** (§6.2).
- **`localeFilters` en AGP 9** (§5.1).
- **Calidad del español.** Lo escribe un implementador y lo revisa el revisor; el usuario lo verá en §10. Un texto mal traducido es un fallo menor, no de seguridad (§4.1).

## 12. Decisiones

- **Ruling 1: el host escribe en el idioma del host y la app en el del móvil, sin enviar el idioma por el protocolo.** Por qué: el protocolo ya está en uso por tres sesiones, un cambio necesita el ok del supervisor y toca los reductores de la app y el diario (que va firmado y no se reescribe); y lo que el host escribe para el móvil ya sale en el idioma del PC desde M2. Si me equivoco: con el PC y el móvil en idiomas distintos, la tarjeta sale con marco y título en idiomas distintos, completa; arreglarlo es un campo opcional `lang` en `hello` y renderizar por conexión (aditivo, v1.1).
- **Ruling 2: solo en y es; EN es la fuente y la caída; ES completo.** Por qué: lo dice la v1; con dos idiomas los controles de paridad son triviales y una clave sin es no puede esconder información (§4.1). Si me equivoco: añadir un idioma es añadir un fichero y relajar el test de paridad.
- **Ruling 3: en Android, recursos de plataforma más `localeConfig`, sin AppCompat ni selector propio; en 11-12 solo el idioma del sistema.** Por qué: es lo que hace Android desde la 13, sin dependencias, y la app es Compose sobre `ComponentActivity`. Si me equivoco: un usuario con Android 11-12 que quiera la app en otro idioma que el sistema no puede; es una molestia, no un fallo.
- **Ruling 4: la app muestra los errores por `code`, no por `error.text`.** Por qué: el `text` es inglés por protocolo y es lo único que una persona con la app en español veía en inglés. Si me equivoco: un `code` nuevo sin clave sale como «error desconocido» más el `text` en inglés; el test de §9.2 obliga a tener clave para cada `ErrorCode`.
- **Ruling 5: un paquete `i18n` propio con catálogos TOML embebidos y `Lang` explícito, sin dependencias ni estado global.** Por qué: `go-i18n` y `x/text/message` traen bastante más de lo que hace falta para dos idiomas y plurales de dos formas, y un idioma global haría que los tests y `-race` dependieran del orden. Si me equivoco: migrar a otra biblioteca cambia solo el cuerpo de `T`.
- **Ruling 6: precedencia `ADBUDDY_LANG` → `config.language` → `LC_ALL`/`LC_MESSAGES`/`LANG` → idioma de Windows → `en`.** Por qué: lo explícito gana a lo ambiente y `ADBUDDY_LANG` da a los tests el control total. Si me equivoco: quien quiera otra cosa pone `language` en `config.toml` o `ADBUDDY_LANG`.
- **Ruling 7: el agente y `--json` siempre en inglés (`ADBUDDY_SOURCE=agent`).** Por qué: la salida para agentes es un contrato y lo lee un modelo, no una persona. Si me equivoco: el agente de una persona que quisiera su salida en español no la tiene; basta quitar la regla en un sitio.
- **Ruling 8: `cli.Error` calcula `Message` y `Fix` en inglés al construirse y guarda las claves.** Por qué: `--json`, los tests y los agentes siguen idénticos, y el cambio por llamada es mecánico. Si me equivoco: un `NewError` sin migrar sale en inglés, nunca roto.
- **Ruling 9: las razones de las confirmaciones pasan a `Text` bilingüe; el `reason` del diario se queda en inglés.** Por qué: el detalle de una pregunta 🔴 debe leerse entero en un idioma, y el diario es un registro firmado que lee el agente. Si me equivoco: una razón nueva sin es la ve el usuario en inglés (el test de corpus la caza).
- **Ruling 10: las causas técnicas de los paquetes internos (adb, red, disco) quedan en inglés y se citan dentro de una frase traducida.** Por qué: son cientos de frases de bajo valor que además trae adb en inglés. Si me equivoco: un error en un PC en español trae una causa en inglés.
- **Ruling 11: lo que decide el usuario viaja como código y `rawCommand`, `level` y `special` no tienen traducción.** Por qué: ninguna traducción incompleta puede cambiar qué se aprueba. Si me equivoco: nada, ya es así; el test de §9.2 lo cierra.
- **Ruling 12: el texto de la pantalla de bloqueo no cambia de visibilidad ni gana argumentos.** Por qué: es de M2/M3 y se revisó. Si me equivoco: un texto público con datos, que el test de §9.2 caza.
- **Ruling 13: los controles de la app son dos tests JVM más un subconjunto de lint, y los del host un test de paridad, uno de claves y un linter `go/ast`.** Por qué: corren en los mismos comandos de CI que ya hay y con la misma regla («cada tarea compila y pasa sus tests»); no dependen de una herramienta externa. Si me equivoco: falsos positivos o negativos, que se arreglan en la lista de excepciones.
- **Ruling 14: H1-H3 esperan a que M4, M5b y M6a estén en `main`; H0, A1, A2, H4a y H4b no.** Por qué: `cli/*.go` es el árbol más disputado. Si me equivoco: M6c se retrasa unos días y no queda nada a medias.
- **Ruling 15: M6c no toca las pantallas de M5c y deja `LanguageSettings` para su Ajustes.** Por qué: son de otra sesión y de otro árbol de ficheros; una fila de ajustes sin pantalla de ajustes sería código muerto. Si me equivoco: sin fila «Idioma» hasta M5c; en Android 13+ se cambia desde el sistema.
- **Ruling 16: `mgr.Busy()` y el reconocimiento de diálogos de Claude Code no se tocan.** Por qué: es el idioma de otra herramienta, no el de ADBuddy. Si me equivoco: nada que M6c arregle; es una limitación de v1.1 para Codex y Gemini.
- **Ruling 17: horas y tamaños de la app según el idioma, y fecha y hora de la terminal del host en formato fijo.** Por qué: la app tiene `Locale` y `Formatter` gratis; en la terminal, el diario ya usa `15:04:05`, que es lo que quien lee `journal` espera en cualquier idioma. Si me equivoco: una hora en 24 horas en una terminal en inglés.
- **Ruling 18: español con tuteo, «móvil» y «ficheros», como el resto de la app.** Por qué: coherencia con las 118 de hoy. Si me equivoco: un barrido de texto en los dos catálogos.
- **Ruling 19: los errores propios de cobra («unknown command», «unknown flag») quedan en inglés.** Por qué: cobra los trae con la palabra de la causa y los convertimos en `USAGE` con una línea de arreglo, que sí se traduce. Si me equivoco: en un PC en español un flag mal escrito se explica en inglés con una línea de arreglo en español.

## 13. Preguntas del usuario

**Ninguna bloquea.** Dos son suyas, con el valor por defecto ya decidido:

1. **Validar el español en su móvil y en su PC** (§10). Es suyo porque es quien lo lee. Mientras tanto vale lo escrito.
2. **¿Se aceptan más idiomas después de v1.0** (por PR o con una plataforma como Weblate)? Por defecto, no: v1.0 es en/es (v1 §6), y añadir uno es un fichero por lado (ruling 2). Una cuenta en una plataforma de traducción sería suya.

## 14. Revisiones

Pendiente. Antes de escribir el plan: revisión de esta spec y del plan por `adbuddy-reviewer-opus` (obligatoria, ver `CLAUDE.md`), con atención a §4, §6.5 y el reparto de §3.
