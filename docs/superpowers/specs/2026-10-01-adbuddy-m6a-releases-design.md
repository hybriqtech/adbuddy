# ADBuddy · M6a · Pipeline de releases y app de confianza · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario. Revisión de seguridad 1 (nautilus-19 con `adbuddy-reviewer-opus`): 2 críticos, 4 importantes y 6 menores, todos incorporados (§12). Las decisiones están en §11.
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§12, §4.6 y la fila M6 de §13)
- **Parte de:** línea B (M6 sin protocolo), rama `m6a`, que sale de `main` (0ec286f)
- **En paralelo con:** M3 (tern-51) y M5 (nautilus-19), con el supervisor adbuddy-e5

## 1. Por qué

Hoy no hay forma de publicar ADBuddy. El host se compila a mano (`go build`) y su versión es `dev`. La app se compila en debug con `versionName = "0.2.0"` y `versionCode = 1` fijos. `adbuddy setup` descarga el APK de `releases/latest/download/adbuddy.apk` y comprueba su `.sha256`, pero esa release no existe todavía (`ErrNoRelease`).

Hay además un hueco de seguridad mayor que el que apuntaba `plans/pendientes-m3-m5.md`:

- el host **nunca comprueba quién firmó la app**: ni el APK que descarga (el `.sha256` solo protege la integridad), ni la app ya instalada con el nombre `tech.hybriq.adbuddy`;
- en particular, `adbuddy serve` entrega el token del canal a **cualquier** app instalada con ese nombre, y `adbuddy setup` le concede WRITE_SECURE_SETTINGS, la exención de batería y el servicio de accesibilidad;
- un agente con una inyección de prompt puede instalar una app falsa con ese nombre cuando la real no está (antes de la primera release, tras `setup --no-app` o tras un fallo de descarga). Para eso basta `adb install x.apk`, que es 🟡, sin confirmación. Con el token, la app falsa responde ella misma a las preguntas 🔴: **salta todas las confirmaciones**. En M5 recibiría además los `ui.request` (leer y tocar la pantalla).

Por último, `ADBUDDY_RELEASE_URL` cambia a la vez el origen del APK y el de su checksum: es un gancho de test que vive en producción.

M6a cierra todo esto fijando en el host el certificado de firma y comprobándolo **en la app instalada antes de cada entrega del token y de cada concesión**, y añade el pipeline que produce APK firmados con ese certificado.

## 2. Alcance

M6a deja hecho:

1. **Versión desde la etiqueta.** Una etiqueta `vX.Y.Z` (o `vX.Y.Z-rc.N`) fija la versión del host y la de la app (§3).
2. **Binarios del host con goreleaser:** Windows x64 y arm64, macOS universal y Linux x64 y arm64, con `checksums.txt`.
3. **APK de release firmado** en un job aislado, publicado como `adbuddy.apk` y `adbuddy.apk.sha256` en la misma release (§4).
4. **App de confianza:** el host solo trata con una app `tech.hybriq.adbuddy` firmada con un certificado de confianza, de versión mínima, tanto al descargarla como ya instalada (§5 y §6).
5. **Defensa en el clasificador:** desinstalar, borrar, desactivar o parar la app de ADBuddy es 🔴 especial, con casos en el corpus (§7).
6. **Script de instalación** `curl … | sh` para Linux y macOS (§8).
7. **Comprobación en la CI** de todo lo anterior en cada PR que lo toque, sin publicar nada (§9).
8. **Documentación** (`docs/releasing.md`): las acciones que solo puede hacer el usuario, cómo sacar una release y qué hacer si se filtra la clave.

Fuera de M6a, en M6c, cuando exista la primera release pública, porque necesitan repos o cuentas externas:

- Homebrew (`hybriqtech/homebrew-tap`);
- winget (`microsoft/winget-pkgs`);
- IzzyOnDroid;
- SignPath Foundation para firmar los binarios de Windows;
- procedencia firmada de los binarios (`actions/attest-build-provenance`).

Las skills de contexto (M6b), la i18n y el README completo van después.

### 2.1 Criterios de éxito

- Una etiqueta `v0.3.0` en un fork con el environment y los secretos crea, tras aprobar el despliegue, una release **en borrador** con cinco archivos del host, `checksums.txt`, `adbuddy.apk` y `adbuddy.apk.sha256`. `adbuddy --version` dice `adbuddy 0.3.0`, y la app tiene `versionName` 0.3.0.
- `adbuddy setup` instala ese APK y rechaza uno idéntico firmado con otra clave, aunque su `.sha256` cuadre.
- `adbuddy serve` no entrega el token a una app `tech.hybriq.adbuddy` firmada con otra clave, ni a una de versión menor que la mínima. Las 🔴 siguen en la terminal y se dice por qué.
- Un build de desarrollo solo recibe el token si el usuario lo eligió con `setup --apk`, y se le avisa en cada `serve`.
- La CI de un PR que toque el pipeline lo compila todo en modo snapshot, firma con una clave de usar y tirar y compara el parser del host con `apksigner`, sin secretos.
- Nada se publica sin que el usuario apruebe el despliegue y pulse "Publish release".

## 3. Versión

**Fuente única: la etiqueta git.** Formato `vX.Y.Z` o `vX.Y.Z-rc.N`, con X de 0 a 20, Y y Z de 0 a 999 (sin ceros a la izquierda) y N de 1 a 98. Cualquier otra etiqueta `v*` hace fallar el workflow antes de compilar nada.

- **Host:** goreleaser pone `-X github.com/hybriqtech/adbuddy/host/internal/cli.Version={{.Version}}` (sin la `v`), con `-s -w -trimpath` y `CGO_ENABLED=0`.
- **App:** `build.gradle.kts` lee dos propiedades de Gradle y, si faltan, usa los valores actuales (`0.2.0` y 1), así que los builds de desarrollo no cambian:

  ```kotlin
  versionCode = providers.gradleProperty("adbuddy.versionCode").map(String::toInt).getOrElse(1)
  versionName = providers.gradleProperty("adbuddy.versionName").getOrElse("0.2.0")
  ```

- **`versionCode`** = `((X·1000 + Y)·1000 + Z)·100 + S`, donde S es N en una `-rc.N` y 99 en la final. Una rc siempre queda por debajo de su final y por encima de la versión anterior, y el máximo (20.999.999, 2 099 999 999) cabe en el `int` de Android. Lo calcula `scripts/release-version.sh`, que tiene su test.
- Las `-rc.N` se publican como *prerelease*: `releases/latest` las ignora, así que `adbuddy setup` e `install.sh` nunca toman una rc por defecto.

## 4. Workflow de release (`.github/workflows/release.yml`)

Se dispara con `push` de etiquetas `v*`. `permissions: {}` arriba, y cada job pide solo lo que usa. `concurrency` por etiqueta, sin cancelar. **Todas las acciones van fijadas por SHA**, con la versión en un comentario.

| Job | Hace | Permisos y secretos |
|---|---|---|
| `version` | valida la etiqueta (§3), comprueba que su commit desciende de `origin/main` y calcula `name`, `code` y `prerelease` | `contents: read` |
| `test` | `go vet` y `go test -race` del host; tests unitarios de la app | `contents: read` |
| `host` | `goreleaser release --clean`, con la release en **borrador** | `contents: write` |
| `build-apk` | `assembleRelease` con la versión de la etiqueta; sube `app-release-unsigned.apk` como artefacto | `contents: read`, **sin secretos** |
| `sign` | en una VM limpia: descarga el artefacto, `zipalign` y `apksigner sign`; sube el APK firmado como artefacto | `environment: release`, sin `contents`, **solo aquí los secretos** |
| `publish-apk` | descarga el APK firmado, comprueba su certificado contra el fijado (`scripts/release-cert.sh --pinned`), escribe `adbuddy.apk.sha256` y lo sube al borrador | `contents: write`, sin secretos |

**El job `sign` (spec §12, I2):**

- **Sin código de terceros:** ni Gradle, ni Go, ni cachés (`actions/cache`, `setup-gradle` o `setup-go` con caché), ni acciones que no sean de `actions/*`. Solo `actions/download-artifact` y `actions/upload-artifact`, el `sdkmanager` de Google para instalar `build-tools;37.0.0`, y `zipalign` y `apksigner` de esas build-tools, con el Java que trae el runner.
- **Los secretos, solo en el `env:` del paso que firma.** Ese paso restaura el keystore en `$RUNNER_TEMP` con `umask 077`, firma con `--ks-pass env:KS_PASS` (alias `adbuddy`, v1 apagada y v2 y v3 activas) y borra el keystore al final del mismo paso. Otro paso con `if: always()` lo borra también si el primero falla.
- El job `sign` no comprueba el certificado con el código del host: eso hace falta Go y sus dependencias. Lo hace `publish-apk`, que no tiene secretos, sobre el APK firmado, que es público de todos modos.

**Environment `release` (spec §12, I1):** lo crea el usuario (§10):

- solo admite etiquetas `v*`;
- tiene un revisor obligatorio, el propio usuario;
- guarda los dos secretos, `ADBUDDY_KEYSTORE_B64` y `ADBUDDY_KEYSTORE_PASSWORD`. Son secretos del environment, **no del repo**: un workflow de otra rama no los ve.

Un *ruleset* de etiquetas reserva la creación de `v*` a los administradores. Sin el environment o sin los secretos, `sign` falla con un mensaje que apunta a `docs/releasing.md`, y el borrador se queda sin APK.

**`version`:** comprueba la etiqueta con `scripts/release-version.sh "$TAG"`, donde `TAG` viene de `env:` y nunca se interpola en `run:`. Después, `git merge-base --is-ancestor "$GITHUB_SHA" origin/main` (con `fetch-depth: 0`). Una etiqueta sobre un commit que no está en `main` no publica nada.

**goreleaser (`.goreleaser.yaml`, versión 2):**

- `project_name: adbuddy` y `dist: host/dist` (ya está en `.gitignore`);
- build en `dir: host`, `main: ./cmd/adbuddy`, para `linux`, `windows` y `darwin` en `amd64` y `arm64`, con `universal_binaries` (`replace: true`) en macOS;
- archivos `.zip` en Windows y `.tar.gz` en el resto, llamados `adbuddy_<versión>_<os>_<arch>` (`darwin_all` para el universal), con `LICENSE` y `README.md` dentro;
- `checksums.txt`;
- `release.draft: true`, `prerelease: auto` y `mode: keep-existing`;
- `mod_timestamp: {{ .CommitTimestamp }}`, para builds reproducibles;
- sin brew, winget ni nfpm (M6c).

## 5. Leer quién firmó un APK

**Qué lee.** `phoneapp.Signers(apk)` devuelve el SHA-256 del certificado de cada firmante de los bloques v2 (`0x7109871a`), v3 (`0xf05368c0`) y v3.1 (`0x1b93ad61`) del *APK Signing Block*. No verifica ninguna firma criptográfica: eso lo hace Android al instalar, y un APK cuyas firmas no casan con su contenido no se instala. Lo que Android no comprueba es *quién* firmó, y eso es lo que da este parser.

**Encuentra exactamente el bloque que encuentra Android, o rechaza (spec §12, C2).** Si el parser leyera un bloque que Android no ve, un atacante podría pegar el bloque auténtico de una release sobre su propio APK firmado con v1, con `targetSdk` 29 y un hueco entre el directorio central y su registro final: Android no encontraría el bloque, volvería a la firma v1 e instalaría la app del atacante. Así que estas condiciones, que siguen a `ApkSigningBlockUtils` de AOSP y son a veces más estrictas, se exigen todas:

1. El registro final del directorio central (EOCD, firma `0x06054b50`) está exactamente en `len-22`, con el comentario vacío. Nuestros APK no llevan comentario: así no hay que elegir entre varios EOCD posibles.
2. No hay localizador ZIP64 (firma `0x07064b50` en `eocd-20`), y los campos de offset y tamaño del directorio central no valen `0xffffffff`.
3. El directorio central acaba justo donde empieza el EOCD: `cdOffset + cdSize == eocd`.
4. Los 16 bytes antes de `cdOffset` son `APK Sig Block 42`. El tamaño del pie está entre 24 y `cdOffset-8`, y el tamaño de la cabecera es igual. Todas las comparaciones se hacen en `uint64`, antes de convertir nada a `int`.
5. Los pares ID-valor ocupan el bloque entero, cada uno con longitud ≥ 4 y dentro del bloque.
6. **Un ID repetido rechaza el APK.** Android usa el primero, y aceptar repetidos obligaría a imitarlo.
7. Cualquier ID que no sea v2, v3 ni v3.1 rechaza el APK, salvo esta lista cerrada de bloques que no son de firma: relleno (`0x42726577`), sello de origen v1 y v2 (`0x2b09189e` y `0x6dff800d`), dependencias de AGP (`0x504b4453`) y *frosting* de Play (`0x2146444e`). Android elige el esquema más alto que conoce, así que un esquema que el host no sabe leer podría esconder otro firmante.
8. Hay al menos uno de los tres bloques de firma; cada bloque tiene al menos un firmante y cada firmante, al menos un certificado no vacío. Ningún dato sobra al final de un bloque.
9. En v3 y v3.1 ningún firmante lleva prueba de rotación de clave (atributo `0x3ba06f8c`): la rotación no está soportada.
10. **El APK no tiene firma v1:** ninguna entrada del directorio central se llama `META-INF/*.SF`, `.RSA`, `.DSA` o `.EC` (sin distinguir mayúsculas). Los nombres se leen con `archive/zip` sobre el mismo directorio central de las condiciones 1-3. Nuestros APK se firman sin v1 (`minSdk` 30), así que una firma v1 solo puede venir de otro.

Cualquier fallo es un error con la causa; nunca hay pánico ni lecturas fuera del búfer, con ninguna entrada (*fuzzing*).

**`phoneapp.CheckSigner(apk, trusted...)`** acepta el APK solo si `Signers` no da error y el certificado de **cada** firmante está en `trusted`. Si no, devuelve un error que envuelve `ErrSigner`.

**Herramienta `host/cmd/apkcert`:** imprime los certificados distintos de un APK (con todas las comprobaciones de arriba) o, con `-pinned`, el certificado fijado. Lo usa la CI y no se distribuye: goreleaser solo compila `./cmd/adbuddy`.

## 6. App de confianza

### 6.1 Certificados de confianza

- **El de release:** `phoneapp.SigningCertSHA256`, una constante. Vale `""` hasta que el usuario genere la clave (§10). Mientras esté vacía, ningún APK descargado se acepta (`ErrUnpinned`, sin tocar la red) y `publish-apk` falla.
- **Uno de desarrollo, solo si el usuario lo elige.** `adbuddy setup --apk <fichero>`, la vía para instalar un build propio, lee el certificado de ese fichero con `Signers`:
  - si no es el de release, lo guarda en `<data>/app/trusted-dev-cert` con un aviso explícito: `⚠ Trusting the development signing certificate <sha> of <file>: adbuddy serve will hand the phone app's token to an app signed with it. Delete <path> to stop.`;
  - si el fichero no tiene un bloque de firma válido, setup no lo instala.

  Nunca se confía en nada en silencio: `serve` repite el aviso cada vez que la app instalada lleva el certificado de desarrollo.
- `phoneapp.TrustedCerts(dataDir)` devuelve los dos (los que no estén vacíos).

### 6.2 Comprobar la app instalada

`phoneapp.VerifyInstalled(ctx, a, trusted, minCode)` devuelve un `phoneapp.Verified`. Es un valor que solo se puede obtener así: sus campos no son exportados, y el paquete no lo construye en ningún otro sitio. Hace esto:

1. `pm path tech.hybriq.adbuddy` ha de dar exactamente una línea `package:<ruta>`, y la ruta ha de casar con `^/data/app/[A-Za-z0-9._~=+/-]+/base\.apk$` y no tener `..`. Si no, devuelve `ErrNoTrustedApp`: no se puede leer, o la app no es la nuestra (la nuestra no tiene *splits*).
2. `stat -c %s <ruta>` ha de dar como mucho `maxAPK` (200 MiB).
3. `base64 <ruta>` (toybox, Android 7+; la app pide API 30) devuelve el APK, que se decodifica en el PC. Así no hace falta escribir ningún fichero temporal, y el adb falso de los tests puede servirlo como texto.
4. `CheckSigner(apk, trusted...)`. Es seguro aunque el parser no verifique la criptografía: ese APK lo verificó Android al instalarlo, y las condiciones de §5 garantizan que el host lee el mismo bloque.
5. `versionCode` de `dumpsys package` ≥ `minCode` (`phoneapp.MinAppVersionCode`, hoy 0). Si no, devuelve `ErrAppTooOld`. Es la defensa contra volver a una release vieja con fallos firmada por hybriqtech (spec §12, I4). Cuando una release arregle un fallo de seguridad de la app, se sube esta constante; lo dice `docs/releasing.md`.

Falla **cerrado**: cualquier error (un adb que no responde, una salida rara) devuelve un error que envuelve `ErrNoTrustedApp` o `ErrAppTooOld`, y no hay `Verified`.

### 6.3 Dónde se exige

Las funciones de `phoneapp` que dan algo a la app **piden un `Verified` como argumento**. Así un llamador nuevo (M5 incluido) no puede saltarse la comprobación sin que falle la compilación:

- `Handshake(ctx, a, v Verified, token, port, host)`: la entrega del token;
- `AskConsent(ctx, a, v, …)`, `Grant(ctx, a, v)`, `AllowBackground(ctx, a, v)` y `EnableAccessibility(ctx, a, v)`.

`Verified` lleva el SHA-256 del certificado y la ruta comprobada. `Handshake` y las concesiones vuelven a llamar a `pm path` justo antes de actuar y fallan si la ruta cambió (la app se reinstaló entre medias). La ventana restante, entre ese `pm path` y la orden misma, es de milisegundos y exige reinstalar la app, lo que pasa por una desinstalación 🔴 (§7).

**En `setup` (`appsetup.go`):** tras instalar (o si ya estaba), `VerifyInstalled` y después, con su `Verified`, el consentimiento y las concesiones. Si falla:

- con la app firmada con otra clave: `⚠ The ADBuddy app on the phone is not signed by hybriqtech (a development build?): ADBuddy will not trust it. Uninstall it on the phone and run adbuddy setup again, or pass --apk <that APK> to trust a development build.`;
- con una app demasiado vieja: `⚠ The ADBuddy app on the phone is too old for this host: run adbuddy setup to update it.`;
- en ambos casos setup sigue sin la app, y las 🔴 van a la terminal.

**En `serve` (`phone.go`, `connect`):** antes de cada `Handshake`, que se repite en cada reconexión, `VerifyInstalled`. Si falla, no se entrega el token, se avisa una vez (como hoy los fallos de conexión) y las 🔴 siguen en la terminal.

**`setup --apk` y `INSTALL_FAILED_UPDATE_INCOMPATIBLE`:** Android no actualiza una app firmada con otra clave. `Install` lo reconoce (`ErrOtherSigner`) y setup explica qué hacer: `Uninstall it on the phone (it forgets its pairing with this PC) and run adbuddy setup again.` **ADBuddy no desinstala nada por su cuenta.** El Pixel del usuario tiene hoy un build de debug: la primera release lo pide.

**`Install`** pasa `--no-incremental`: la instalación incremental leería el APK del PC después de comprobarlo.

### 6.4 Descarga

`phoneapp.Download(ctx, client, base, dir, cert)`:

- sin `cert`, `ErrUnpinned` antes de tocar la red;
- tras el `.sha256`, `CheckSigner(data, cert)` con el certificado de release **solamente**: un APK descargado nunca vale por el certificado de desarrollo;
- un fallo es `ErrSigner`, y setup lo dice: `⚠ The downloaded ADBuddy app is not signed by hybriqtech, so nothing was installed. Please report it at https://github.com/hybriqtech/adbuddy/issues, or pass --apk <file>.`

El cambio del APK en disco entre `Download` e `Install` (TOCTOU) lo cubre §6.2: lo que se instale y no sea nuestro no recibe nada.

**`ADBUDDY_RELEASE_URL`** se queda para los tests e2e, con dos límites:

- solo se acepta `https://` o `http://` a `127.0.0.1`, `::1` o `localhost`; cualquier otra cosa es un error de setup;
- aunque sirva una release vieja firmada por hybriqtech, `MinAppVersionCode` la frena.

Con esto se cierra el pendiente de M6.

**`--apk <fichero>`** no se comprueba contra el certificado de release, porque es el fichero que elige el usuario. Sí se lee su certificado (§6.1), y la ayuda de la opción lo dice.

## 7. Defensa en el clasificador

Ruling del supervisor. Un caso de `host/internal/classify`, con revisión opus. Estas órdenes sobre `tech.hybriq.adbuddy` han de ser 🔴 especiales:

- `pm uninstall` (con o sin `-k` y `--user`);
- `cmd package uninstall`;
- `pm clear`;
- `pm disable` y `pm disable-user`;
- `pm suspend`;
- `am force-stop` y `am kill`;
- `adb uninstall` (también con `-k`).

Si alguna se quita, la app real deja de estar y entra en juego §6. Hoy `mentionsADBuddy` ya debería cubrirlas, por nombre de paquete en las palabras (en `shell` solo cuando el nivel no es 🟢). M6a lo deja escrito en `testdata/habitual.jsonl`, una línea por orden con `"level":"red","special":true`, y añade a `special.go` lo que no salga.

`adb install <apk>` sigue siendo 🟡: el host no sabe qué paquete lleva el APK, y §6 es lo que impide que una app falsa reciba nada.

## 8. Script de instalación (`scripts/install.sh`)

Para Linux y macOS: `curl -fsSL https://raw.githubusercontent.com/hybriqtech/adbuddy/main/scripts/install.sh | sh`.

- POSIX `sh`, con `set -eu`, todo dentro de `main` y llamado en la última línea: si la descarga se corta, no se ejecuta nada.
- Sistema y arquitectura: `Linux` con `x86_64`/`amd64` o `aarch64`/`arm64`, y `Darwin` (siempre `darwin_all`). Cualquier otra combinación falla con un mensaje claro, y Windows apunta al `.zip`.
- Descargas con `curl --proto '=https' --tlsv1.2 -fsSL` o `wget --https-only`. `ADBUDDY_DOWNLOAD_URL`, que es solo para tests, admite otros esquemas, y en ese caso se avisa de que el origen no es la release oficial.
- `ADBUDDY_VERSION` ha de casar con la expresión regular de las etiquetas (§3) o valer `latest`.
- Descarga `checksums.txt` y busca la línea cuyo **segundo campo sea exactamente** `adbuddy_<v>_<target>.tar.gz` (`awk '$2 == f'`). Tiene que haber una y solo una. Descarga el archivo, compara el SHA-256 (`sha256sum` o `shasum -a 256`) y falla si no cuadra.
- Trabaja en un `mktemp -d` que borra un `trap`, extrae solo `adbuddy` e instala en `${ADBUDDY_INSTALL_DIR:-$HOME/.local/bin}` con `mv` atómico, sin `sudo`. Avisa si esa carpeta no está en el `PATH`.

## 9. Comprobación en la CI (`.github/workflows/release-check.yml`)

Solo `on: pull_request` y `push` a `main`, **nunca `pull_request_target`**, con `permissions: contents: read`, en Linux y con las acciones fijadas por SHA. Rutas:

- `.goreleaser.yaml` y `.github/workflows/release*.yml`;
- `scripts/install*.sh` y `scripts/release-*.sh`;
- `host/internal/phoneapp/**` y `host/cmd/apkcert/**`;
- `android/app/build.gradle.kts` y `LICENSE`.

Jobs:

- **`scripts`:** `shellcheck -s sh` de los scripts, `release-version_test.sh` e `install_test.sh`.
- **`goreleaser`:** `goreleaser check` y `goreleaser release --snapshot --clean`. Comprueba que están los cinco archivos y `checksums.txt`, y que `install.sh` instala el snapshot (con `file://`) y el binario dice su versión.
- **`sign`:** `assembleRelease` con una versión de prueba. Firma con una clave de usar y tirar (`keytool -storepass:env`) y `apksigner`. `scripts/release-cert.sh` (sin `--pinned`) comprueba que `apksigner` y `apkcert` dan el mismo certificado. Como `apkcert` aplica todas las condiciones de §5, un bloque nuevo de AGP sale en el PR. Al final comprueba la versión con `aapt2 dump badging`.

`host.yml` ya ejecuta los tests unitarios del parser y de `VerifyInstalled`, con un corpus de *fuzzing* corto.

## 10. Acciones del usuario

Están en `docs/releasing.md`, en "User actions", y en "Validaciones manuales pendientes" del plan. **No bloquean el resto de M6a.** El script de la clave lo ejecuta el usuario en su terminal, **nunca el agente**: así el keystore no pasa por el transcript.

1. **Clave:** `sh scripts/release-key.sh`. Necesita `keytool`, de un JDK o del `jbr` de Android Studio. Genera un keystore PKCS12 RSA 4096, válido 30 años:
   - lo deja en `~/.adbuddy-release/` (carpeta 0700, fichero 0600, `umask 077`), y se niega si esa ruta está dentro del repo o si el keystore ya existe;
   - la contraseña la pide `keytool`; nunca pasa por la línea de órdenes;
   - imprime el SHA-256 del certificado y las órdenes para los secretos, **no** el base64.
2. **Copia de seguridad** del keystore y su contraseña. Si se pierden, ninguna release podrá actualizar la app instalada.
3. **Environment `release`** en GitHub, solo para etiquetas `v*` y con el propio usuario como revisor obligatorio. Dentro, sus dos secretos: `gh secret set --env release ADBUDDY_KEYSTORE_B64 < <(base64 …)` y `gh secret set --env release ADBUDDY_KEYSTORE_PASSWORD`, que pide el valor sin mostrarlo.
4. **Ruleset de etiquetas:** solo los administradores crean `v*`.
5. **Fijar el certificado:** pasar su SHA-256 (es público) para ponerlo en `SigningCertSHA256`, en un commit y un PR propios.
6. **Su móvil:** desinstalar el build de debug antes de la primera release. Hasta entonces, `adbuddy setup --apk <su build>` para seguir confiando en él (§6.1).
7. **Primera release:** etiqueta, aprobar el despliegue de `sign`, revisar el borrador y "Publish release".

**Si se filtra la clave** (lo explica `docs/releasing.md`):

- revocar los secretos;
- generar otra clave;
- sacar una release del host con el certificado nuevo fijado y `MinAppVersionCode` subido, para que ningún host nuevo acepte nada firmado con la vieja;
- en cada móvil, desinstalar la app y volver a ejecutar `adbuddy setup`;
- anunciarlo en el README y en las notas de la release.

## 11. Decisiones

- **Ruling: comprobar la app instalada antes de cada token y de cada concesión, con un tipo `Verified` como prueba.** Por qué: el token y las concesiones van a cualquier app con nuestro nombre, y una app falsa salta todas las 🔴. Pedir el tipo en la firma de cada función hace que un llamador nuevo no pueda olvidarlo. Si me equivoco: el coste es leer el APK por adb en cada conexión (unos segundos por Wi-Fi con un APK de unos 10 MB), solo al arrancar `serve` y al reconectar.
- **Ruling: leer el certificado de la app instalada con `pm path` y `base64`, y el mismo parser.** Por qué: Android no da el SHA-256 del certificado a la shell. `dumpsys package` solo muestra `signatures:[xxxxxxxx]`, un `hashCode()` de 32 bits que un atacante puede igualar. Si me equivoco: si una versión de Android cambia la salida de `pm path` o quita `base64`, la comprobación falla cerrada y la app no recibe el token, que se ve en `serve`. No queda abierta.
- **Ruling: condiciones del parser más estrictas que AOSP (sin comentario en el EOCD, sin IDs repetidos, sin v1).** Por qué: ante la duda, rechazar. Los APK los produce nuestro pipeline, y `release-check` prueba cada cambio. Si me equivoco: un cambio de AGP rompe `release-check` antes de publicar.
- **Ruling: un certificado de desarrollo solo con `setup --apk` y con aviso en cada `serve`.** Por qué: hay que poder desarrollar la app sin la clave de release, sin que la confianza se dé en silencio. Si me equivoco: quien tenga acceso al PC puede ejecutar `setup --apk`, pero ese mismo acceso ya le deja cambiar el binario del host.
- **Ruling: compilar y firmar en jobs distintos, y el de firma sin código de terceros, con environment y revisor.** Por qué: Gradle ejecuta plugins y dependencias de terceros que podrían leer el keystore o la contraseña. Y unos secretos del repo los lee cualquier workflow de cualquier rama. Si me equivoco: un artefacto y un job más por release.
- **Ruling: release en borrador, que publica el usuario.** Por qué: publicar es irreversible hacia fuera (la URL `latest` la usan todos los hosts instalados). Si me equivoco: un clic más por release.
- **Ruling: `MinAppVersionCode` en el host, hoy 0.** Por qué: con el certificado fijado, toda release vieja firmada por hybriqtech sigue siendo "nuestra". Si me equivoco: olvidar subirla deja la vuelta atrás abierta hasta la siguiente; `docs/releasing.md` lo pone en la lista de cada release de seguridad.
- **Ruling: firma con `apksigner` en el workflow, sin `signingConfig` en Gradle.** Por qué: la clave nunca toca Gradle ni su caché, y `android/` cambia solo en dos líneas de versión. Si me equivoco: mover la firma a Gradle es un cambio del job.
- **Ruling: sin rotación de clave.** Por qué: no la necesitamos y su linaje complica el parser. Si me equivoco: rotar la clave pide primero una release del host que lea el linaje.
- **Ruling: `versionCode` derivado de la etiqueta.** Por qué: reproducible y sin estado. Si me equivoco: solo caben versiones mayores hasta la 20.
- **Ruling: brew, winget, IzzyOnDroid, SignPath y la procedencia firmada, a M6c.** Por qué: necesitan cuentas, repos o una release pública. Si me equivoco: goreleaser los añade con pocas líneas.
- **Ruling: `CGO_ENABLED=0`.** Por qué: compilación cruzada y binarios estáticos. M3 confirma que su PTY es Go puro (conpty, creack/pty y charmbracelet/x/vt). Si me equivoco: si algún destino necesita cgo, pasa a su runner nativo.

## 12. Revisión de seguridad 1 y cómo se resolvió

| Hallazgo | Resuelto en |
|---|---|
| C1: el certificado solo se comprueba al descargar, no en la app que recibe el token y las concesiones | §6.2 y §6.3 (`VerifyInstalled` y el tipo `Verified`) y §7 |
| C2: el parser puede leer un bloque que Android no ve (vuelta a v1); IDs repetidos | §5, condiciones 1-10 |
| I1: los secretos se pueden leer desde cualquier rama | §4 (environment `release`, ruleset y la etiqueta, que ha de descender de `main`) |
| I2: compilar y firmar en el mismo job | §4 (`build-apk`, `sign` y `publish-apk`; acciones fijadas por SHA; `permissions: {}`) |
| I3: el script de la clave imprime el base64 y no dice cómo pasa las contraseñas | §10.1 |
| I4: `ADBUDDY_RELEASE_URL` permite volver a una release vieja | §6.2, paso 5, y §6.4 |
| M1: "todos los firmantes" se cumple sin firmantes; enteros | §5, condiciones 4 y 8 |
| M2: TOCTOU entre comprobar e instalar; instalación incremental | §6.3 y §6.4 |
| M3: el chequeo completo, también en la CI | §5 (`apkcert`) y §9 (`sign`) |
| M4: interpolar `ref_name` en `run:`; `pull_request_target` | §4 (`version`) y §9 |
| M5: endurecer `install.sh` | §8 |
| M6: qué hacer si se filtra la clave | §10 |

## 13. Tests

- **Parser** (`phoneapp/apksig_test.go`), con APK sintéticos de `phoneapp/apktest`:
  - aceptados: v2, v3 y v3.1 buenos, v2 + v3 y los bloques permitidos que no son de firma;
  - rechazados por quién firma o por los firmantes: v2 nuestro + v3 ajeno, dos firmantes con uno ajeno, rotación, sin firmantes y certificado vacío;
  - rechazados por la estructura del bloque: sin bloque, solo relleno, ID desconocido, ID repetido, datos sobrantes y truncado;
  - rechazados por la estructura del zip: EOCD con comentario, hueco entre el directorio central y el EOCD, localizador ZIP64, `0xffffffff`, tamaños del bloque distintos o fuera de rango, par fuera del bloque, y `META-INF/CERT.SF` o `cert.rsa` en el zip;
  - además, `FuzzSigners`.
- **`VerifyInstalled`**, con el adb falso del paquete:
  - casos buenos: el de release y el de desarrollo;
  - rechazados: firmante ajeno, `pm path` con dos líneas, con ruta rara o vacío, tamaño de más, `base64` ilegible, versión menor que la mínima y adb que falla. Todos cerrados.
- **`Handshake` y concesiones:** con un `Verified` cuya ruta ya no da `pm path`, no envían nada.
- **`Download`:** firmante ajeno, sin certificado (cero peticiones), y que el certificado de desarrollo no valga para una descarga.
- **`Install`:** `--no-incremental` y `ErrOtherSigner`, por stdout y por stderr.
- **`cli`:**
  - `setupApp`: los mensajes nuevos, que no se pida consentimiento ni se conceda nada a una app no verificada, y `--apk` que confía en el certificado de desarrollo con su aviso;
  - `startPhoneLink`: no entrega el token a una app no verificada, avisa una vez y repite la comprobación al reconectar;
  - `ADBUDDY_RELEASE_URL` con un esquema o host no permitidos.
- **Clasificador:** las líneas de §7 en `habitual.jsonl`.
- **Scripts y pipeline:** §9.

## 14. Coordinación

| Fichero | Dueño | M6a toca | Ok del supervisor |
|---|---|---|---|
| `host/internal/phoneapp/**`, `host/cmd/apkcert` | M6a | todo | sí |
| `host/internal/cli/appsetup.go` y su test | M6a | todo | sí |
| `host/internal/cli/setup.go` | — | la ayuda de `--apk` | sí |
| `host/internal/cli/phone.go` y `phone_test.go` | compartido | `connect()` (`VerifyInstalled` + `Handshake` con `Verified`) y el adb falso de los tests | **pedido** |
| `host/cmd/adbuddy/phone_e2e_test.go` | compartido | reglas `pm path`, `stat` y `base64` y el certificado de desarrollo en el directorio de datos | **pedido** |
| `host/internal/classify/special.go` y `testdata/habitual.jsonl` | — | §7 | sí (ruling) |
| `android/app/build.gradle.kts` | tern (dependencias) | las 2 líneas de versión, en su propio commit | sí |
| `pendientes-m3-m5.md` | cada uno su sección | solo M6 | sí |

M5 (nautilus) se beneficia de `Verified` sin cambios: si alguna vez la app necesita algo más del host por adb, la función que lo dé pide un `Verified`.
