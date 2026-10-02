# ADBuddy M6d · Documentación de usuario y donaciones

Fecha: 2026-10-02 · Estado: borrador para plan · Rama: `m6d` (desde main 0072dc8)

Parte de la spec v1 (§12 "Publicación y proyecto": README en inglés con traducción al español, guía de instalación con capturas, modelo de seguridad explicado y FAQ; donaciones) y del hito M6 de §13. Complementa M6a (releases, mergeado), M6b (skills, mergeado), M6c (i18n del producto, otra sesión) y M5c (pantallas de la app, otra sesión): **M6d no toca cadenas del host ni de la app, ni pantallas**.

## 1. Alcance

La documentación que hoy falta para que un usuario que llega al repositorio entienda qué es ADBuddy, lo instale, lo use, confíe en él y sepa qué hacer cuando algo falla. Hoy `README.md` tiene tres líneas; `docs/` solo tiene la guía de preparación del móvil (`docs/setup/`, EN y ES, con capturas pendientes) y los documentos de mantenimiento.

Entregables:

| Fichero | Contenido |
|---|---|
| `README.md` (EN) | Qué es, por qué, cómo funciona, instalación y primeros pasos en 5 líneas, agentes compatibles, modelo de seguridad resumido con su aviso honesto, ventajas (sin claves de API; seguridad por defecto), requisitos, tabla de comandos, enlaces a la documentación, apoyo al proyecto, licencia |
| `README.es.md` | Traducción al español del README |
| `docs/index.md` | Portada del sitio de GitHub Pages con los enlaces a todas las guías |
| `docs/install.md` | Instalar el host (Linux y macOS con el script; Windows con el archivo de la release), comprobación de checksums, actualizar, desinstalar, qué hace `adbuddy setup` |
| `docs/usage.md` | Día a día: `serve` y `run`, el chat desde el móvil, diario y `undo`, `send`, las ayudas de pantalla, pausar y cortar acceso, modo remoto |
| `docs/safety.md` | Modelo de seguridad explicado: niveles 🟢🟡🔴, shim, diario e inversos, confirmaciones, apps protegidas, confianza en la app (certificado de release), privacidad, y los **límites honestos** |
| `docs/troubleshooting.md` | `adbuddy doctor` y cómo leerlo; tabla de códigos de error con su arreglo; problemas habituales por síntoma |
| `docs/faq.md` | Preguntas frecuentes |
| `docs/setup/index.md` y `docs/setup/es/index.md` | Se revisan frente al flujo real de `adbuddy setup` (hoy llegan solo hasta el emparejamiento): instalación de la app, consentimiento, elección del agente, y qué hacer después. Las capturas siguen siendo marcadores `pending.svg` (§4.3) |
| `.github/FUNDING.yml`, `docs/pre-public-checklist.md` | Donaciones (§5) |
| `host/internal/cli/docs_test.go` | Guarda de las guías: enlaces relativos que existen y comandos de `adbuddy` que existen, con flags válidos (§6) |

**Fuera de M6d:** i18n del host y de la app (M6c); pantallas de la app y "Acerca de" (M5c); Homebrew, winget, IzzyOnDroid y SignPath (pendientes de publicación de `pendientes-m3-m5.md`, "M6c, tras la primera release pública"; la documentación **no promete** esas vías hasta que existan); traducción al español de las guías nuevas (solo el README y la guía de preparación del móvil están en español en la v1; el resto, a pendientes); capturas reales (las pone el usuario, §4.3); `SECURITY.md` (a pendientes).

### 1.1 Criterios de éxito

- Alguien que no ha visto el proyecto puede, solo con el README y las guías enlazadas, instalar el host, preparar el móvil, ejecutar `adbuddy setup`, `serve` y `run`, y entender qué hace ADBuddy con su móvil y qué no puede garantizar.
- Todo comando `adbuddy …` que aparece en un bloque de código de las guías (y del README) existe, con flags que existen; todo enlace relativo entre `.md` apunta a un fichero que existe. Un test lo comprueba.
- Ninguna guía afirma algo que el producto no hace hoy: lo no mergeado o no publicado (pantallas de M5c, Homebrew, winget, firma de Windows, i18n) no se documenta como disponible.
- El aviso honesto de §4.6 de la spec v1 está en el README y en `docs/safety.md`, con las mismas palabras de fondo.
- Cada código de error del host (`NewError("CÓDIGO", …)`) tiene su fila en `docs/troubleshooting.md` con el arreglo que el propio host sugiere.

## 2. Principios

1. **Verdad antes que marketing.** Se documenta lo que el código hace; cada afirmación comprobable (comandos, flags, rutas, códigos, nombres de ficheros de la release, requisitos de versión de Android y de adb) se comprueba contra el código o el pipeline de release. Lo que depende de un móvil real y no está validado se dice con cautela ("probado en Pixel con Android 17") o se omite.
2. **Idioma.** Las guías están en inglés (publico internacional, lo que ve GitHub Pages); el README y la guía de preparación del móvil, además en español. Ningún texto traducible nuevo en el host ni en la app.
3. **Corto y por tareas.** Cada guía responde a "quiero hacer X" con pasos numerados y comandos copiables; el detalle de diseño vive en `docs/superpowers/` y no se publica (`exclude` de `_config.yml`).
4. **Una sola fuente por dato.** Los comandos y flags se describen por su `--help` real; las guías no copian tablas largas que se desfasen: enlazan o resumen. La tabla de códigos de error sí se copia del código, y el test de §6 no puede protegerla, así que lleva el comentario de dónde sale y un paso en la lista de revisión de releases (`docs/releasing.md`).
5. **Honestidad sobre la seguridad.** `docs/safety.md` empieza por lo que ADBuddy hace, y termina por lo que no garantiza: el shim protege contra errores y contra inyecciones de prompts, no contra un agente decidido a buscar otro adb; la garantía real es "Cortar acceso" en la app (§4.6 de la spec v1). Sin superlativos.
6. **Sin datos privados.** Ni IPs, ni seriales, ni rutas de usuario, ni nombres de equipos en ninguna guía (los ejemplos usan `192.0.2.x`/`100.64.0.x` documentales o marcadores `<ip:port>`).
7. **Compatible con GitHub Pages.** Jekyll `jekyll-theme-minimal` con `layout: default` y front matter (`title`, `lang`); los enlaces entre páginas son relativos y acaban en `/` o `.md` según el patrón de `docs/setup/`; todo `.md` nuevo bajo `docs/` se publica salvo que esté en `exclude` (los de mantenimiento ya lo están).

## 3. Contenido por documento

### 3.1 README.md

Secciones, en este orden: título y una frase; **What it does** (tres viñetas: el agente gestiona el Android por adb Wi-Fi; chat desde el móvil; seguro por defecto); **How it works** (diagrama de texto: móvil ↔ host ↔ agente ↔ adb real; qué corre dónde); **Quick start** (cinco pasos: instalar el host → `adbuddy setup` → `adbuddy serve` en una terminal → `adbuddy run` en otra → hablar con el agente, y el enlace a la guía de preparación del móvil); **Supported agents** (Claude Code recomendado, Codex, Gemini CLI, OpenCode; el host no instala agentes: `adbuddy run` dice cómo); **Safety** (resumen de seis líneas con enlace a `docs/safety.md` y el aviso honesto); **Why ADBuddy** (usa el agente que ya tienes y su suscripción: sin claves de API ni LLM propio; comparación breve y verídica con servidores MCP de QA como ARTEMIS: sin superlativos, citando lo de la spec v1 §1); **Requirements** (Android 11+, PC Windows/macOS/Linux, el agente que se elija, misma red Wi-Fi o Tailscale; adb 37+ lo descarga el host); **Commands** (tabla corta con el `--help` real); **Documentation** (enlaces); **Support the project** (§5); **License** (Apache-2.0, enlace a `LICENSE`); **Status** (la versión actual de la release más reciente del repo, y una nota de que el proyecto está en desarrollo activo).

`README.es.md`: la misma estructura traducida; enlace cruzado en la primera línea de cada uno.

### 3.2 docs/install.md

Requisitos; **Linux y macOS**: la orden `curl … | sh` de `scripts/install.sh` tal cual la documenta el script (con `ADBUDDY_VERSION` y `ADBUDDY_INSTALL_DIR`), qué comprueba (SHA-256) y a dónde instala; leer el script antes de ejecutarlo es una opción que se menciona; **Windows**: descargar el archivo de la release (los nombres reales los define `.goreleaser.yaml`; el implementador los comprueba) y descomprimir en una carpeta del `PATH`, comprobar el checksum con `checksums.txt` (la orden de PowerShell correcta, probada); aviso honesto de SmartScreen (los binarios de Windows aún no están firmados: SignPath está en los pendientes de publicación); **la app del móvil** la instala `adbuddy setup` (y qué ocurre con `--apk`/`--no-app`); **actualizar** (cómo, hoy: volver a ejecutar el script / descargar la nueva release; `adbuddy setup` actualiza la app si procede, solo si el código lo hace) y **desinstalar** (qué borrar en el PC: el binario, la carpeta de configuración y de datos con sus rutas reales por sistema (§8 de la spec v1, comprobadas con `host/internal/paths`), el entorno `~/ADBuddy/…`; en el móvil: desinstalar la app y revocar la depuración inalámbrica).

### 3.3 docs/usage.md

Cómo se usa cada día, con ejemplos: `adbuddy serve` (qué muestra, qué es 🔴, confirmar en la terminal o en el móvil), `adbuddy run [agente]`, el chat desde el móvil (lo que hace hoy la app mergeada: no se documentan pantallas de M5c), `adbuddy new`/`attach`/`stop`, el diario (`adbuddy journal`, `adbuddy undo`), `adbuddy send`, las ayudas de pantalla (`screen`, `tap`, `type`, `foreground`, `screenshot`), pausar y cortar acceso desde la app, modo remoto (`adbuddy remote`, Tailscale, `adbuddy connect`, el riesgo del puerto 5555 y su caducidad), varios móviles solo si el código ya lo soporta. El entorno del agente (`~/ADBuddy/<móvil>/`, `AGENTS.md`, `NOTES.md`, skills) en un apartado corto con enlace a las skills.

### 3.4 docs/safety.md

Qué hace ADBuddy: el shim `adb` primero en el `PATH` del agente; la clasificación (🟢 solo lectura, 🟡 reversible, 🔴 irreversible o no entendido) con ejemplos reales de cada nivel; el diario con su inverso y `undo` (y qué NO tiene inverso); las confirmaciones (en el móvil con desbloqueo, o en la terminal; caducan a los 2 minutos; `Denied by user` / `Blocked by ADBuddy`); las protecciones fijas (apps protegidas, PIN nunca, botón Pausar y Cortar acceso, ADBuddy no actúa sobre sí mismo); la confianza en la app (el host solo entrega el token a una app firmada con el certificado de release o con el APK de desarrollo que el usuario eligió, y de versión mínima: M6a); el contexto contra la inyección de prompts (`AGENTS.md`: lo leído es dato); privacidad (cero telemetría, cero servidores propios; qué habla con internet); **qué no garantiza** (el aviso honesto; modo remoto y 5555; un agente decidido a buscar otro adb; el clasificador no es una caja fuerte: lista blanca con su corpus, pero la ejecución real del móvil la hace mksh; las apps protegidas por heurística de nombre; Advanced Protection de Android; lo que ve el proveedor del modelo del agente: lo que el agente lee del móvil (pantalla, ficheros) va a su proveedor). Termina con "Cómo informar de un problema de seguridad" mientras no haya `SECURITY.md`: abrir una *security advisory* privada en GitHub si está habilitada, o un issue sin detalles de explotación (el implementador no inventa canales: lo que no exista se pone en la lista de acciones del usuario).

### 3.5 docs/troubleshooting.md

**`adbuddy doctor`**: qué comprueba y en qué orden (configuración, adb propio y su servidor, `serve`, el móvil, la app, el entorno), cómo leer las líneas `Run:` y `Guidance:` y el código de salida (0 listo o degradado, 1 roto, 130), `--json` y `--fix`; **tabla de códigos de error**: todos los `NewError("CÓDIGO", mensaje, arreglo)` del host (se extraen con `grep` del código y se comprueban a mano), con su causa y arreglo; **por síntoma**: no encuentra el móvil, el puerto de depuración inalámbrica cambia, "Wrong password" y `protocol fault` al emparejar, el móvil bloqueado, `device offline`, `Denied: nobody could confirm`, `Blocked by ADBuddy: waiting for…`, la app no conecta, la accesibilidad no se activa (ajustes restringidos), Windows y el cortafuegos, el agente no instalado, otro `adb` en el `PATH`, el puerto 5038, `WORKSPACE_FAILED`. Cómo reunir información para un issue (`adbuddy doctor --json`, versión) sin datos privados.

### 3.6 docs/faq.md

¿Necesito USB? ¿Funciona con mi agente? ¿Qué ve el agente de mi móvil? ¿Puede borrar mis fotos? ¿Cómo deshago algo? ¿Funciona fuera de casa? ¿Qué pasa si pierdo el móvil o el PC? ¿Por qué no pide mi PIN? ¿Cuánto cuesta (nada; el agente puede tener coste propio)? ¿Es seguro? (remite a `safety.md`) ¿Telemetría? ¿Es oficial de Google/Anthropic/OpenAI? (no, proyecto independiente). Cada respuesta corta y con enlace a la guía.

### 3.7 docs/setup/ (EN y ES)

La guía llega hoy hasta el emparejamiento. Se completa con lo que `adbuddy setup` hace después (según `host/internal/cli/setup.go`: conexión, instalación de la app y comprobación del certificado, perfil del móvil, elección del agente y creación del entorno; consentimiento en la app) y el siguiente paso ("run `adbuddy serve`, then `adbuddy run`"). Los marcadores de captura `![Pending screenshot…](img/pending.svg)` se mantienen y se añade una lista de las capturas pendientes con su nombre de fichero propuesto, en `docs/pre-public-checklist.md`, no en la guía.

## 4. Hecho y por hacer del usuario

### 4.1 Cómo se comprueba la verdad

Cada documento lo escribe un implementador con el código delante y lo revisa otro agente frente al código y a esta spec: comandos y flags (también por el test de §6), mensajes literales entre comillas (se buscan en el código), rutas, nombres de ficheros de la release, y las afirmaciones de seguridad (revisor opus para `safety.md`).

### 4.2 Sitio

`docs/_config.yml` no cambia salvo que un fichero nuevo no deba publicarse. La URL de la guía de preparación (`https://hybriqtech.github.io/adbuddy/setup/`) está en el código del host (QR de `adbuddy setup`): **no se mueve**.

### 4.3 Capturas

Las capturas reales las hace el usuario con su móvil. M6d deja la lista de las que faltan y mantiene los marcadores; no inventa imágenes.

## 5. Donaciones

La spec v1 (§12) dice: GitHub Sponsors en hybriqtech y Ko-fi, enlazados en el README y en "Acerca de", sin avisos insistentes. "Acerca de" es de la app (M5c): fuera de M6d.

- No se conoce el identificador de Ko-fi ni consta que GitHub Sponsors esté habilitado: **una página de donaciones que apunta a una cuenta que no es del usuario es un riesgo** (alguien podría registrar ese nombre). Por eso M6d **no publica enlaces de donación que no existan**: añade `.github/FUNDING.yml` con las claves comentadas y deja en el README una sección "Support the project" cuyo enlace se activa cuando el usuario cree las cuentas.
- `docs/pre-public-checklist.md` gana: crear el perfil de GitHub Sponsors y la página de Ko-fi; poner sus identificadores reales en `FUNDING.yml` (descomentar) y en la sección "Support the project" del README y de `README.es.md` (los marcadores están señalados); enlazarlos en "Acerca de" de la app (M5c).
- El tono: una frase, sin insistencia ni cuenta atrás ("ADBuddy is free and open source; if it saves you time, you can support its development").

## 6. Guarda de las guías (`host/internal/cli/docs_test.go`)

Un test del paquete `cli`, junto al de las skills, recorre `README.md`, `README.es.md` y `docs/**/*.md` publicados (excluidos los de `exclude` de `_config.yml` y `docs/superpowers`, `docs/spikes`, `docs/pre-public-checklist.md` y `docs/releasing.md`):

1. **Enlaces relativos** entre ficheros `.md` (`](ruta.md)`, `](ruta/)`, `](ruta/#ancla)` resueltos con la convención de Jekyll: `x/` → `x/index.md`, `x.md` o `x`): el destino existe. Los enlaces `http(s)://` y `mailto:` se ignoran. Las anclas no se comprueban.
2. **Comandos**: en bloques de código con lenguaje `sh`, `bash`, `shell`, `console`, `powershell` o sin lenguaje, las líneas que empiezan por `adbuddy ` o `$ adbuddy ` (tras quitar el prompt) se validan con el árbol de cobra: el subcomando existe y los flags son válidos (`ParseFlags`). Los marcadores entre `<…>` valen como argumentos posicionales; las líneas con `|`, `&&` o `;` se validan por tramos que empiezan por `adbuddy `. Los comandos en prosa (`` `adbuddy doctor` `` en línea) también: se extraen y validan igual si empiezan por `adbuddy ` y su primer campo es un subcomando conocido; si el primer campo no es un subcomando (p. ej. el nombre del producto en una frase), se ignoran.
3. Falla si un `.md` publicado contiene una IP privada, un `emulator-N`, una ruta de usuario (`C:\Users`, `/home/`, `/Users/`) o la cadena `ANDROID_SERIAL=`.

Los comandos que no son de `adbuddy` (`curl`, `sh`, `certutil`, `winget`…) no se comprueban: los revisa el revisor frente a `scripts/install.sh`, `.goreleaser.yaml` y `docs/releasing.md`.

## 7. Riesgos y decisiones

- **Documentación desfasada:** el producto cambia (M5c, M6c). La guarda de §6 caza comandos y enlaces rotos, no afirmaciones; por eso el principio 4 y que `docs/releasing.md` gane el paso "revisar las guías si cambió la CLI".
- **Afirmar de más sobre seguridad:** el riesgo mayor de este hito. `docs/safety.md` lo revisa opus frente al código y a la spec §4, y el README solo resume lo que `safety.md` dice.
- **Donaciones sin cuentas:** decisión de §5 (marcadores, sin enlaces inventados).
- **Traducciones:** solo README y la guía de preparación; el resto de guías, en inglés (se apunta a pendientes).
- **Solape con otras sesiones:** M6c (strings) y M5c (pantallas) no se tocan; si una guía necesita nombrar una pantalla de la app, describe solo lo que ya está en main.

## 8. Correcciones tras la revisión opus del plan (2026-10-02)

La revisión del plan (réplica de la guarda y lectura del código) cambió lo siguiente respecto a lo escrito arriba; **mandan estas líneas**:

- **Hoy no hay release pública ni certificado fijado:** `docs/install.md` incluye "Build from source" (cierto hoy) y presenta el script `curl … | sh` y la descarga de la app por `adbuddy setup` como "cuando haya release"; el README dice "no public release yet" sin número de versión (§3.1, §3.2).
- **El código manda, no la spec v1:** lo que la spec v1 describe y el código no hace no se documenta (p. ej. la lista editable de apps protegidas llega con M5c; "Mantener la depuración activa" no existe; el desbloqueo para confirmar es solo de Android 12+).
- **`host/README.md`** (documento para desarrolladores, desfasado) se reduce a una nota de desarrollo que enlaza a las guías (§1).
- **Donaciones:** toda la sección "Support the project" del README va dentro de un comentario HTML hasta que existan las cuentas (§5).
- **Guarda (§6):** acepta `<…>` y `[…]` como marcadores, `--help`/`-h`/`help`, comillas, comentarios ` #`, `adbuddy.exe`, bloques `console` solo con prompt; construye un árbol de comandos por comando; un enlace desde un `.md` de `docs/` no puede salir de `docs/` ni caer en un fichero excluido; los enlaces a páginas sueltas llevan `.md`; `releasing.md` pasa a `exclude`; los esqueletos de las guías los crea T1.
- **Revisión:** opus revisa también el README (resumen de seguridad y comparación con ARTEMIS) y la revisión final cubre todas las afirmaciones de seguridad de todas las guías.
