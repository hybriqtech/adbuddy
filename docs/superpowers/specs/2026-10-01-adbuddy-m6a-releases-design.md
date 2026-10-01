# ADBuddy · M6a · Pipeline de releases · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario; las decisiones están en §9
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§12 y la fila M6 de §13)
- **Parte de:** línea B (M6 sin protocolo), rama `evaluar-agente-app-paralelo`, que sale de `main` (0ec286f)
- **En paralelo con:** la línea A (M3, sesión del worktree `tern`) y M6b, skills de contexto (sesión del worktree `nautilus`)

## 1. Por qué

Hoy no hay forma de publicar ADBuddy. El host se compila a mano (`go build`) y su versión es `dev`. La app se compila en debug con `versionName = "0.2.0"` y `versionCode = 1` fijos. `adbuddy setup` descarga el APK de `releases/latest/download/adbuddy.apk` y comprueba su `.sha256`, pero esa release no existe todavía (`ErrNoRelease`).

Hay además dos pendientes de seguridad apuntados para M6 (`plans/pendientes-m3-m5.md`):

- el `.sha256` de la release solo protege la integridad del APK, no que lo haya firmado hybriqtech: una release comprometida pasaría en la primera instalación;
- `ADBUDDY_RELEASE_URL` cambia a la vez el origen del APK y el de su checksum, así que es un gancho de test que vive en producción.

Fijar en el host el certificado de firma cierra los dos: con el certificado comprobado, el origen de la descarga deja de importar.

## 2. Alcance

M6a deja hecho:

1. **Versión desde la etiqueta.** Una etiqueta `vX.Y.Z` (o `vX.Y.Z-rc.N`) fija la versión del host y la de la app.
2. **Binarios del host con goreleaser:** Windows x64 y arm64, macOS universal y Linux x64 y arm64, con `checksums.txt`.
3. **APK de release firmado** con la clave de los secretos del repo y publicado como `adbuddy.apk` y `adbuddy.apk.sha256` en la misma release.
4. **El host fija el certificado de firma del APK** y rechaza un APK descargado que no esté firmado con él (§5).
5. **Script de instalación** `curl … | sh` para Linux y macOS (§6).
6. **Comprobación en la CI** de todo lo anterior en cada PR que lo toque, sin publicar nada (§7).
7. **Documentación para el mantenedor** (`docs/releasing.md`): cómo generar la clave, qué secretos crear y cómo sacar una release.

Fuera de M6a, en un M6c cuando exista la primera release pública, porque necesitan repos o cuentas externas que crea el usuario:

- `brew install hybriqtech/tap/adbuddy` (repo `hybriqtech/homebrew-tap` y su token);
- `winget install hybriqtech.adbuddy` (manifiesto en `microsoft/winget-pkgs`, que pide una URL pública);
- IzzyOnDroid (petición al mantenedor y APK publicado);
- firma de binarios de Windows con SignPath Foundation (solicitud del usuario).

Las skills de contexto, la i18n de la app y el README completo son M6b y siguientes.

### 2.1 Criterios de éxito

- `git push` de una etiqueta `v0.3.0` en un fork con los secretos crea una release **en borrador** con los cinco binarios, `checksums.txt`, `adbuddy.apk` y `adbuddy.apk.sha256`. `adbuddy --version` dice `adbuddy 0.3.0` y la app, `versionName` 0.3.0.
- El host instala ese APK y rechaza otro idéntico firmado con otra clave, aunque su `.sha256` cuadre.
- La CI de un PR que toque el pipeline compila todo en modo snapshot, firma un APK con una clave de usar y tirar y comprueba el certificado con el código del host, sin secretos.
- Nada se publica sin que el usuario pulse "Publish release".

## 3. Versión

**Fuente única: la etiqueta git.** Formato `vX.Y.Z` o `vX.Y.Z-rc.N`, con X, Y y Z enteros de 0 a 999 (X hasta 20) y N de 1 a 98. Cualquier otra etiqueta `v*` hace fallar el workflow antes de compilar nada.

- **Host:** goreleaser pone `-X github.com/hybriqtech/adbuddy/host/internal/cli.Version={{.Version}}` (sin la `v`), con `-s -w -trimpath` y `CGO_ENABLED=0`.
- **App:** `build.gradle.kts` lee dos propiedades de Gradle y, si faltan, usa los valores actuales (`0.2.0` y 1), así que los builds de desarrollo no cambian:

  ```kotlin
  versionCode = providers.gradleProperty("adbuddy.versionCode").map(String::toInt).getOrElse(1)
  versionName = providers.gradleProperty("adbuddy.versionName").getOrElse("0.2.0")
  ```

- **`versionCode`** = `((X·1000 + Y)·1000 + Z)·100 + S`, donde S es N para una `-rc.N` y 99 para la final. Así una rc siempre es menor que su final y mayor que la versión anterior, y el máximo (X = 20) cabe en el `int` de Android (2 100 000 000). Lo calcula un script del workflow (`scripts/release-version.sh`), con su test.
- Las `-rc.N` se publican como *prerelease*: `releases/latest` las ignora, así que `adbuddy setup` nunca descarga una rc.

## 4. Workflow de release (`.github/workflows/release.yml`)

Se dispara con `push` de etiquetas `v*`. Tiene `permissions: contents: write` solo en los jobs que suben a la release, y `concurrency` por etiqueta.

| Job | Hace | Necesita |
|---|---|---|
| `version` | valida la etiqueta y calcula `versionName` y `versionCode` (§3) | — |
| `test` | `go vet`, `go test` y `go test -race` del host (Linux) y los tests unitarios de la app | `version` |
| `host` | `goreleaser release --clean` con la release en **borrador** | `test` |
| `apk` | compila, firma y comprueba el APK y lo sube al borrador | `host` |

**El job `apk`:**

1. `gradle :app:assembleRelease -Padbuddy.versionName=… -Padbuddy.versionCode=…`. Sin `signingConfig`, AGP deja `app-release-unsigned.apk` ya alineado.
2. Restaura el keystore desde el secreto `ADBUDDY_KEYSTORE_B64` a un fichero temporal del runner y firma con `apksigner sign` (`--ks-pass env:…`, nunca la contraseña en la línea de órdenes), con v2 y v3 activos y v1 apagado (`minSdk` 30).
3. `apksigner verify --print-certs` y compara el SHA-256 del certificado con **la constante fijada en el host** (§5), leída con `go run ./host/cmd/apkcert` sobre el mismo APK. Si no coinciden, el job falla: nunca sale una release cuyo APK rechace su propio host.
4. Escribe `adbuddy.apk.sha256` (`sha256sum`, formato `<hex>  adbuddy.apk`, el que ya lee `phoneapp.Download`) y sube los dos ficheros con `gh release upload` al borrador de la etiqueta.
5. Borra el keystore temporal (`if: always()`).

Sin los secretos (`ADBUDDY_KEYSTORE_B64`, `ADBUDDY_KEYSTORE_PASSWORD`, `ADBUDDY_KEY_ALIAS` y `ADBUDDY_KEY_PASSWORD`), el job `apk` falla con un mensaje que apunta a `docs/releasing.md`. El borrador se queda sin APK y el usuario lo borra.

**goreleaser (`.goreleaser.yaml`, versión 2):** `project_name: adbuddy`; `dir: host`, `main: ./cmd/adbuddy`; destinos `windows/amd64`, `windows/arm64`, `darwin/amd64`, `darwin/arm64` (unidos con `universal_binaries`, `replace: true`), `linux/amd64` y `linux/arm64`. Archivos `.zip` en Windows y `.tar.gz` en el resto, con nombre `adbuddy_<versión>_<os>_<arch>` (`darwin_all` para el universal), más `LICENSE` y `README.md`. `checksum.name_template: checksums.txt`. `release.draft: true`, `prerelease: auto` y `mode: keep-existing`. `mod_timestamp: {{ .CommitTimestamp }}` para que los binarios sean reproducibles. Sin brew, winget ni nfpm (M6c).

## 5. El host fija el certificado de firma

**Qué se comprueba.** `phoneapp.Download`, después del `.sha256`, comprueba que el APK esté firmado con el certificado de hybriqtech. Para ello lee el *APK Signing Block* (la estructura que precede al directorio central del zip):

- busca los bloques de los esquemas v2 (`0x7109871a`), v3 (`0xf05368c0`) y v3.1 (`0x1b93ad61`);
- de cada firmante de cada bloque toma el primer certificado (el del firmante) y calcula el SHA-256 de su DER;
- acepta el APK solo si hay al menos un bloque de esos tres, **todos** los firmantes de **todos** los bloques presentes tienen el certificado fijado y en v3 y v3.1 no hay prueba de rotación de clave (atributo `0x3ba06f8c`);
- cualquier otro ID del Signing Block rechaza el APK, salvo una lista cerrada de bloques que no son de firma: relleno (`0x42726577`), sello de origen v1 y v2 (`0x2b09189e` y `0x6dff800d`), información de dependencias de AGP (`0x504b4453`) y *frosting* de Google Play (`0x2146444e`). Los APK los produce nuestro propio pipeline, y el job `apk` comprueba cada uno con este mismo código (§4, paso 3), así que un bloque nuevo de AGP sale en la CI y nunca en el móvil de un usuario.

**Por qué basta con mirar el certificado sin verificar la firma.** La firma la verifica Android al instalar: un APK con el bloque auténtico pegado sobre contenido cambiado no se instala. Lo que Android no comprueba es *quién* firmó. Hay que mirar todos los esquemas porque Android usa el más alto que encuentre. Si el host mirara solo v2, un atacante podría dejar el v2 auténtico y añadir un v3 válido con su clave, y Android instalaría lo que diga el v3. Por eso un firmante desconocido en cualquier esquema rechaza el APK.

**La constante.** `phoneapp.SigningCertSHA256` es el SHA-256 en hexadecimal del certificado de la clave de release. Mientras el usuario no haya generado la clave (§8), vale `""`, y entonces `Download` rechaza todo APK con `ErrUnpinned` ("this build of adbuddy has no pinned signing certificate: pass --apk <file>"). Así un host compilado sin la constante no instala nada descargado sin comprobarlo. El job `apk` (§4, paso 3) falla si la constante está vacía.

**Errores.** `ErrSigner` ("the downloaded APK is not signed by hybriqtech: not installing it") se trata en `setupApp` como hoy `ErrChecksum`: aviso, setup sigue sin la app y las confirmaciones 🔴 van a la terminal. Un bloque mal formado también es `ErrSigner`, con la causa en el mensaje: el parser no entra en pánico ni lee fuera del búfer con ninguna entrada (test de *fuzzing*).

**`--apk <fichero>`** no se comprueba: es la vía del desarrollador para instalar su build de debug, y el fichero lo elige el usuario. Se documenta en la ayuda de la opción.

**`ADBUDDY_RELEASE_URL`** se mantiene para los tests e2e. Con el certificado fijado ya no es un riesgo: un origen distinto solo puede servir APK firmados por hybriqtech. Se dice en su comentario y en `docs/releasing.md`, y se cierra así el pendiente de M6.

**Herramienta `host/cmd/apkcert`.** Un `main` de pocas líneas que imprime el SHA-256 del certificado de cada firmante de un APK, usando el mismo parser. Lo usan el job `apk` y `scripts/release-key.sh`, y no se distribuye: goreleaser solo compila `./cmd/adbuddy`.

**Paso de un build de desarrollo a la release.** Android no deja actualizar una app firmada con otra clave (`INSTALL_FAILED_UPDATE_INCOMPATIBLE`). El Pixel del usuario tiene hoy un build de debug, así que la primera release pide desinstalarla, y eso borra sus datos (token y emparejamiento con la app; el host vuelve a emparejar en `setup`). `setupApp` reconoce ese error de `adb install` y dice qué hacer en vez de mostrar el texto crudo. **No desinstala nada por su cuenta.**

## 6. Script de instalación (`scripts/install.sh`)

Para Linux y macOS: `curl -fsSL https://raw.githubusercontent.com/hybriqtech/adbuddy/main/scripts/install.sh | sh`.

- POSIX `sh`, con `set -eu` y todo dentro de una función `main` que se llama en la última línea, para que una descarga cortada no ejecute medio script.
- Detecta el sistema (`uname -s`: Linux, Darwin) y la arquitectura (`x86_64`/`amd64`, `aarch64`/`arm64`; en macOS siempre `darwin_all`). Cualquier otra combinación falla con un mensaje claro.
- Descarga `checksums.txt` y el archivo de la última release (`releases/latest/download/`), comprueba el SHA-256 con `sha256sum` o `shasum -a 256` y falla si no cuadra o si no encuentra la línea.
- Instala en `${ADBUDDY_INSTALL_DIR:-$HOME/.local/bin}` sin `sudo` y avisa si ese directorio no está en el `PATH`.
- `ADBUDDY_VERSION=v0.3.0` instala una versión concreta (`releases/download/v0.3.0/`). `ADBUDDY_DOWNLOAD_URL` cambia la base para los tests. Como el checksum viene del mismo origen, se documenta como opción de pruebas.
- Necesita `curl` o `wget`, `tar` y una herramienta de SHA-256, y comprueba que existan antes de descargar nada.

En Windows el camino es winget (M6c). Mientras tanto, el `.zip` de la release, con los pasos en `host/README.md`.

## 7. Comprobación en la CI (`.github/workflows/release-check.yml`)

En PR y pushes a `main` que toquen `.goreleaser.yaml`, `.github/workflows/release*.yml`, `scripts/install.sh`, `scripts/release-*.sh`, `host/internal/phoneapp/**`, `host/cmd/apkcert/**` o `android/app/build.gradle.kts`. Solo Linux.

- `goreleaser check` y `goreleaser release --snapshot --clean --skip=publish`: compila los seis destinos y el universal sin publicar nada.
- `scripts/release-version.sh` con su tabla de casos (etiquetas válidas e inválidas y el orden de los `versionCode`).
- `shellcheck` de los scripts y una prueba de `install.sh` contra los archivos del snapshot servidos con `python3 -m http.server` (`ADBUDDY_DOWNLOAD_URL`), comprobando que el binario instalado diga `adbuddy … --version`. También comprueba que falle con un `checksums.txt` alterado.
- Firma de prueba: genera con `keytool` una clave de usar y tirar, compila `assembleRelease` con una versión de prueba, firma con `apksigner` y comprueba que `go run ./host/cmd/apkcert` dé el mismo SHA-256 que `apksigner verify --print-certs`. Esto prueba el parser de Go contra un APK real de `apksigner`, sin secretos. No compara con la constante fijada, porque la clave es otra.

`host.yml` ya ejecuta los tests unitarios del parser, incluido un corpus de *fuzzing* corto.

## 8. Lo que hace el usuario (una vez)

Está en `docs/releasing.md`. El plan lo apunta en "Validaciones manuales pendientes", porque genera secretos y publica.

1. Ejecuta `scripts/release-key.sh`, que necesita `keytool` (el JDK o el de Android Studio). Genera un keystore PKCS12 RSA 4096 con 30 años de validez fuera del repo, en `~/.adbuddy-release/`, e imprime el base64 para el secreto y el SHA-256 del certificado. Nunca escribe dentro del worktree, y se niega a sobrescribir un keystore que ya exista.
2. Guarda el keystore y su contraseña en un sitio seguro: si se pierde, ninguna release futura podrá actualizar la app instalada.
3. Crea los cuatro secretos en el repo (`gh secret set`).
4. Pasa el SHA-256 del certificado, que es información pública: una tarea del plan lo pone en `phoneapp.SigningCertSHA256`.
5. Saca la primera release: `git tag v0.3.0 && git push origin v0.3.0`. Revisa el borrador y pulsa "Publish release".

## 9. Decisiones

- **Ruling: release en borrador, que publica el usuario.** Por qué: publicar es irreversible hacia fuera (la URL `latest` la usan los hosts instalados), y CLAUDE.md reserva al usuario las acciones hacia fuera. Si me equivoco: un clic más por release.
- **Ruling: firma con `apksigner` en el workflow, sin `signingConfig` en Gradle.** Por qué: la clave nunca toca la configuración de Gradle ni su caché, y `android/` (que es de M3) cambia solo en dos líneas de versión. Si me equivoco: mover la firma a Gradle es un cambio local del job.
- **Ruling: comprobar el certificado de todos los esquemas (v2, v3 y v3.1) sin verificar la criptografía.** Por qué: Android verifica la firma al instalar y elige el esquema más alto, así que lo que falta es saber quién firmó en cada esquema que Android pueda usar. Si me equivoco: un esquema futuro con otro ID que Android prefiera abriría un hueco. Por eso cualquier ID que no esté en la lista cerrada de §5 rechaza el APK. El coste es que un bloque nuevo de AGP o de `apksigner` rompe el job `apk` hasta que se añada a la lista, y eso se ve antes de publicar.
- **Ruling: sin rotación de clave.** Por qué: no la necesitamos y su linaje complica el parser. Si me equivoco: el día que haya que rotar la clave, M6 o una versión posterior añaden el linaje (lo dice `docs/releasing.md`).
- **Ruling: constante vacía = rechazar las descargas.** Por qué: un host sin certificado fijado que instalara cualquier APK con checksum sería el agujero de hoy. Si me equivoco: hasta la primera release nadie puede descargar la app, como hoy (`ErrNoRelease`).
- **Ruling: `versionCode` derivado de la etiqueta y no de un contador.** Por qué: reproducible y sin estado. Si me equivoco: las versiones mayores de 20 no caben, y eso queda muy lejos.
- **Ruling: brew, winget, IzzyOnDroid y SignPath, a M6c.** Por qué: necesitan repos, cuentas o URLs públicas que no existen hasta la primera release. Si me equivoco: goreleaser los añade con unas líneas.
- **Ruling: `CGO_ENABLED=0`.** Por qué: compilación cruzada desde Linux y binarios estáticos. Si me equivoco: si M3 necesita cgo para el PTY de algún sistema, ese destino pasa a compilarse en su runner nativo (aviso a la línea A).

## 10. Tests

- **Parser del bloque de firma** (`phoneapp/apksig_test.go`): APK sintéticos construidos en el test (zip mínimo, bloque y firmantes con certificados autofirmados de `crypto/x509`):
  - v2 bueno, v3 bueno y v2 + v3 buenos;
  - v2 bueno + v3 con otra clave, v3 con dos firmantes (uno ajeno), v3.1 ajeno;
  - prueba de rotación en v3;
  - sin bloque, ID de esquema desconocido, relleno y sello de origen (aceptados);
  - tamaños y offsets falsos, bloque truncado y EOCD con comentario.

  Además, `FuzzSigners` con ese corpus.
- **`Download`:** checksum bueno con firmante ajeno (`ErrSigner`), constante vacía (`ErrUnpinned`) y caso bueno, con la constante cambiada por el test (variable del paquete, no exportada).
- **`setupApp`:** mensaje de `ErrSigner` y de `INSTALL_FAILED_UPDATE_INCOMPATIBLE` (adb falso).
- **Fixture real:** un APK mínimo firmado con `apksigner` (unos KB, en `phoneapp/testdata/`) y su SHA-256 esperado, para que el parser no dependa solo de la estructura que construye el propio test. La CI lo vuelve a comprobar con un APK nuevo (§7).
- **Scripts:** casos de `release-version.sh`, `shellcheck` e instalación contra el snapshot (§7).

## 11. Coordinación con las otras líneas

- **Línea A (M3, worktree `tern`):** M6a solo toca en `android/` las dos líneas de versión de `build.gradle.kts`, y en el host, `host/internal/phoneapp` y `host/cmd/apkcert`, que M3 no toca. Si M3 necesita cgo, se revisa `CGO_ENABLED` (§9).
- **M6b (skills, worktree `nautilus`):** sin ficheros en común.
- **`plans/pendientes-m3-m5.md`:** M6a solo cambia la sección M6. Marca hechos el certificado fijado y `ADBUDDY_RELEASE_URL`, y añade M6c.
