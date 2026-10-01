# ADBuddy · M6a · Pipeline de releases y app de confianza · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario. Revisión de seguridad 1 (nautilus-19 con `adbuddy-reviewer-opus`): 2 críticos, 4 importantes y 6 menores. Re-revisión 2: 0 críticos, 4 importantes y 8 menores. Re-revisión 3: 1 crítico, 2 importantes y 8 menores. Todo incorporado (§12). Las decisiones están en §11.
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

**Grafo de `needs:`** (revisión 2, I-4):

- `test` necesita `version`;
- `host` necesita `version` y `test`;
- `build-apk` necesita `version` y `test`;
- `sign` necesita `build-apk`;
- `publish-apk` necesita `host` y `sign`.

Una etiqueta que no pase `version` (fuera de `main` o mal formada) no crea ni el borrador.

**Sin cachés en las builds de etiqueta** (revisión 2, M-7): `setup-go` con `cache: false` y `setup-gradle` con `cache-disabled: true` en `test`, `host`, `build-apk` y `publish-apk`. Una caché envenenada desde una rama no entra en lo que se publica. El contenido del APK lo decide `build-apk`, que ejecuta Gradle y sus dependencias. Es un riesgo aceptado (§11): la clave sigue a salvo, y la verificación de dependencias de Gradle queda para M6c.

**El job `sign` (spec §12, I2):**

- **Sin código de terceros:** ni Gradle, ni Go, ni cachés (`actions/cache`, ni `setup-gradle` o `setup-go` con caché), ni acciones que no sean de `actions/*`. Solo:
  - `actions/download-artifact` y `actions/upload-artifact`;
  - el `sdkmanager` de Google, para instalar `build-tools;37.0.0`;
  - `zipalign` y `apksigner` de esas build-tools, con rutas absolutas y el Java que trae el runner.

  El artefacto se descarga en `$RUNNER_TEMP/in` y ha de ser un único fichero, `app-release-unsigned.apk`.
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
- `release.extra_files: scripts/install.sh`: el instalador viaja con cada release y se sirve desde `releases/latest/download/install.sh`, no desde `main` (revisión 2, I-4);
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
8. Hay al menos uno de los tres bloques de firma; cada bloque tiene al menos un firmante y cada firmante, al menos un certificado no vacío. El certificado de un firmante es el **primero** de su lista, como en Android (`certs[0]`). Ningún dato sobra al final de un bloque.
9. En v3 y v3.1 ningún firmante lleva prueba de rotación de clave (atributo `0x3ba06f8c`): la rotación no está soportada.
10. **El APK no tiene firma v1:** ninguna entrada del directorio central empieza por `META-INF/` y acaba en `.SF`, `.RSA`, `.DSA` o `.EC`, a cualquier profundidad y sin distinguir mayúsculas. Nuestros APK se firman sin v1 (`minSdk` 30), así que una firma v1 solo puede venir de otro.
11. **El directorio central se recorre a mano**, sin `archive/zip`, que acepta recuentos módulo 65536:
    - exactamente tantas entradas como dice el EOCD, cada cabecera con su firma `0x02014b50`;
    - el recorrido acaba justo en `cdOffset+cdSize`;
    - ningún nombre lleva NUL ni UTF-8 inválido (como exige libziparchive).

Cualquier fallo es un error con la causa; nunca hay pánico ni lecturas fuera del búfer, con ninguna entrada (*fuzzing*).

**`phoneapp.CheckSigner(apk, trusted...)`** acepta el APK solo si `Signers` no da error y el certificado de **cada** firmante está en `trusted`. Si no, devuelve un error que envuelve `ErrSigner`.

**Herramienta `host/cmd/apkcert`:** imprime los certificados distintos de un APK (con todas las comprobaciones de arriba) o, con `-pinned`, el certificado fijado. Lo usa la CI y no se distribuye: goreleaser solo compila `./cmd/adbuddy`.

## 6. App de confianza

### 6.1 Certificados y APK de confianza

- **El certificado de release:** `phoneapp.SigningCertSHA256`, una constante. Vale `""` hasta que el usuario genere la clave (§10). Mientras esté vacía, no se acepta ningún APK descargado (`ErrUnpinned`, sin tocar la red) y `publish-apk` falla.
- **Un APK de desarrollo concreto, solo si el usuario lo elige (revisión 2, I-1).** `adbuddy setup --apk <fichero>` es la vía para instalar un build propio:
  1. lee el fichero **una vez** y comprueba con `Signers` que tiene un bloque de firma válido; si no, no lo instala;
  2. si no lo firma el certificado de release, guarda el **SHA-256 del APK entero** (no el de su certificado) en `<data>/app/trusted-dev-apk`, con un aviso: `⚠ Trusting this development build of the app (SHA-256 <sha> of <file>): adbuddy serve will hand the phone app's token to exactly this APK. Delete <path> to stop.`;
  3. lo copia a `<data>/app/chosen.apk` (0600, escrito en un temporal y renombrado) e instala **esa copia**, para que nadie lo cambie entre la lectura y la instalación. Lo lee como mucho `MaxAPK`: el fichero podría ser un FIFO;
  4. escribe `trusted-dev-apk` **solo cuando la instalación termina bien**, y lo borra junto con `chosen.apk`.

  ¿Por qué el digest y no el certificado? La clave de debug de Android está en el PC (`~/.android/debug.keystore`, con la contraseña pública `android`). Con ella, un agente con acceso al PC podría firmar otra app y **actualizar** la de desarrollo con `adb install -r` (🟡). Android le dejaría sus datos, token incluido, y sus permisos. Con el digest solo vale ese APK exacto: cada build nuevo pide repetir `setup --apk`.
- **`LoadTrust`** lee `trusted-dev-apk` solo si es un fichero normal (no un enlace) que contiene un SHA-256 y, en Unix, con permisos 0600. Si no, no confía en nada de desarrollo. La carpeta de datos está fuera del entorno del agente, así que `adb pull` no puede escribir ahí: `daemon/pcwrite.go` solo deja escribir dentro del entorno.
- **El aviso no se calla:** `serve` avisa al arrancar siempre que exista `trusted-dev-apk`, lleve o no la app instalada ese APK. Un `setup` sin `--apk` que verifica una app firmada con el certificado de release borra `trusted-dev-apk` y lo dice.

### 6.2 Comprobar la app instalada

`(*phoneapp.Verifier).Verify(ctx, a)` devuelve un `phoneapp.Verified` de campos no exportados. Fuera del paquete solo se puede escribir su valor cero, `phoneapp.Verified{}`. **Toda función que recibe un `Verified` rechaza el valor cero de forma explícita** (revisión 2, I-2), con un test que lo comprueba.

`Verify` da estos pasos:

1. **Ruta.** `pm path tech.hybriq.adbuddy` ha de dar exactamente una línea `package:<ruta>`, y la ruta ha de casar con `^/data/app/[A-Za-z0-9._~=+/-]+/base\.apk$` y no tener `..`. Si no, falla cerrado con `ErrNoTrustedApp`. Pasa si no se puede leer, si la app no es la nuestra (la nuestra no tiene *splits*) o si está en almacenamiento adoptado, fuera de `/data/app`.
2. **Tamaño.** `stat -c %s <ruta>` no puede pasar de `maxAPK` (200 MiB).
3. **Lectura.** `base64 <ruta>` (toybox, Android 7+; la app pide API 30) devuelve el APK, que se decodifica en el PC:
   - con su propio plazo, `pullTimeout` (2 min);
   - con la salida **limitada** a lo que ocupa en base64 el tamaño de `stat`, más los saltos de línea;
   - el tamaño decodificado ha de ser exactamente el de `stat`.

   Así no hace falta ningún fichero temporal, y el adb falso de los tests lo sirve como texto.
4. **Firmante.** Una de dos:
   - con el certificado de release: `CheckSigner(apk, release)`;
   - con un APK de desarrollo de confianza: el SHA-256 del APK leído es el de `trusted-dev-apk`, y `Signers` no da error.

   No verificar la criptografía es seguro aquí: Android verificó ese APK al instalarlo, y las condiciones de §5 garantizan que el host lee el mismo bloque.
5. **Versión.** El `versionCode` se lee del bloque `Package [tech.hybriq.adbuddy]` de `dumpsys package` (no de *Hidden system packages*). Ha de ser ≥ `phoneapp.MinAppVersionCode`, hoy 0; si no, `ErrAppTooOld`.
   - Es la defensa contra volver a una release vieja con fallos firmada por hybriqtech (revisión 1, I4).
   - Se sube cuando una release arregla un fallo de seguridad de la app. Lo dice `docs/releasing.md`.
6. **Ruta otra vez.** Vuelve a llamar a `pm path` y exige la misma ruta: la app no se reinstaló durante la lectura.

Falla **cerrado**: cualquier error (un adb que no responde, una salida rara) envuelve `ErrNoTrustedApp` o `ErrAppTooOld`, y no hay `Verified`.

`Verifier` recuerda el último resultado definitivo (bien, `ErrSigner`, `ErrAppTooOld` o `ErrUnpinned`), pero no los fallos de adb (re-revisión 3, I-a e I-b):

- **La clave de la caché** es la ruta, el `stat -c '%s %i %Y'` de `base.apk` (tamaño, inodo y mtime) y la confianza vigente. La seguridad no depende de que una actualización cambie de carpeta: basta con que cambie el `stat`, y eso se comprueba en el emulador (validación 3).
- **La confianza se relee en cada llamada:** `NewVerifier` recibe una función que devuelve la `Trust` actual. Borrar `trusted-dev-apk` surte efecto enseguida, también en un `serve` que ya está en marcha.

Así, las reconexiones de `serve` no vuelven a leer el APK hasta que algo cambia.

**Límites del modelo:**
- Con root o `adb root` (que es 🔴 especial), la shell del móvil puede mentir, y no hay garantía.
- No protege de un agente que ejecute herramientas en el PC fuera de `adbuddy`: podría escribir los ficheros del host o ejecutar el adb real. Hoy los separan los permisos del arnés: el entorno solo preaprueba `Bash(adbuddy:*)` (`workspace.go`), y `setup` se niega a ejecutarse con `ADBUDDY_SOURCE=agent`.
- `trusted-dev-apk` es uno solo por PC. Con dos móviles, el `setup` de uno con la app de release deja de confiar en el build de desarrollo del otro. Falla cerrado, así que solo molesta.
- En un móvil sin root, `pm`, `stat`, `base64` y `sh` están en `/system`, y el usuario `shell` no puede escribir en `/data/app`.

### 6.3 Dónde se exige

Las funciones de `phoneapp` que dan algo a la app **piden un `Verified` como argumento**:

- `Handshake(ctx, a, v Verified, token, port, host)`, la entrega del token;
- `AskConsent(ctx, a, v, …)`, `Grant(ctx, a, v)`, `AllowBackground(ctx, a, v)` y `EnableAccessibility(ctx, a, v)`.

Un llamador nuevo (M5 incluido) solo puede saltarse la comprobación pasándoles a propósito un valor cero, y eso falla en ejecución.

Antes de actuar, cada una vuelve a llamar a `pm path` y falla si la ruta cambió. Quedan dos cosas que conviene saber:

- **La ventana entre ese `pm path` y la orden** es de milisegundos. Con el certificado de release, cambiar la app en ella exige desinstalarla, y eso es 🔴 (§7). Con un APK de desarrollo, una actualización con la misma clave de debug cambia la ruta, y con ella el digest.
- **La autoridad del provider.** `Handshake` y `Call` van a `content://tech.hybriq.adbuddy.bridge`, que se resuelve por autoridad y no por paquete. Es seguro porque Android no deja instalar dos providers con la misma autoridad (`INSTALL_FAILED_CONFLICTING_PROVIDER`): mientras la app verificada siga instalada, la autoridad es suya.

**En `setup` (`appsetup.go`):** tras instalar (o si la app ya estaba), `Verify`; después, con su `Verified`, el consentimiento y las concesiones. Si `Verify` falla:

- con otra clave: `⚠ The ADBuddy app on the phone is not signed by hybriqtech (a development build?): ADBuddy will not trust it. Uninstall it on the phone and run adbuddy setup again, or pass --apk <that APK> to trust a development build.`;
- demasiado vieja: `⚠ The ADBuddy app on the phone is too old for this host: run adbuddy setup to update it.`;
- en los dos casos setup sigue sin la app, y las 🔴 van a la terminal.

**En `serve` (`phone.go`, `connect`):** `Verify` antes de cada `Handshake`, también al reconectar. El token rota **antes** de verificar: si `Verify` falla, la app que tenía el anterior ya no puede entrar. Si falla, no se entrega token nuevo, se avisa una vez (como hoy con los fallos de conexión) y las 🔴 siguen en la terminal.

**Cada conexión, atada a la instalación verificada (re-revisión 3, C-1).** Una actualización firmada con la misma clave (`adb install -r`, 🟡) conserva los datos de la app, el token incluido, y `adb reverse` sobrevive. La app nueva podría reconectarse con el token heredado sin que hubiera un handshake nuevo. Por eso el canal tiene un gancho `bridge.Options.Admit`:

- se llama en la goroutine de cada conexión, tras validar el token y antes de `OnConnect` y `OnAttach`;
- falla cerrado: `false`, un `ctx` cancelado o 20 s sin respuesta rechazan la conexión como si no tuviera el token;
- `serve` lo usa con `Verifier.Same`: la app que llega ha de tener la ruta y el `stat` del último `Verify` bueno, y su confianza tiene que seguir vigente;
- si no, la rechaza y pide una comprobación nueva: `connect()` rota el token y vuelve a verificar antes de dar ninguno.

Con el certificado de release ya no se podía actualizar sin la clave de hybriqtech. Esto cierra también el caso de desarrollo.

**`setup --apk` e `INSTALL_FAILED_UPDATE_INCOMPATIBLE`:** Android no actualiza una app firmada con otra clave. `Install` reconoce ese error (`ErrOtherSigner`) y setup dice qué hacer: `Uninstall it on the phone (it forgets its pairing with this PC) and run adbuddy setup again.`

- **ADBuddy no desinstala nada por su cuenta.**
- El Pixel del usuario tiene hoy un build de debug, así que la primera release lo pedirá.

**`Install`** pasa `--no-incremental`. Con la instalación incremental, adb leería el APK del PC después de comprobarlo, y el APK instalado no sería idéntico byte a byte.

### 6.4 Descarga

`phoneapp.Download(ctx, client, base, dir, cert)`:

- sin `cert`, devuelve `ErrUnpinned` antes de tocar la red;
- tras el `.sha256`, `CheckSigner(data, cert)` **solo** con el certificado de release: la confianza de desarrollo nunca vale para un APK descargado;
- si falla, `ErrSigner`, y setup lo dice: `⚠ The downloaded ADBuddy app is not signed by hybriqtech, so nothing was installed. Please report it at https://github.com/hybriqtech/adbuddy/issues, or pass --apk <file>.`

Si el APK cambia en disco entre `Download` e `Install`, lo cubre §6.2: lo que se instale y no sea nuestro no recibe nada.

**`ADBUDDY_RELEASE_URL`** se queda para los tests e2e, con dos límites:

- solo admite `https://`, o `http://` hacia `127.0.0.1`, `::1` o `localhost`; cualquier otra cosa es un error de setup;
- si sirve una release vieja firmada por hybriqtech, la frena `MinAppVersionCode`.

Con esto se cierra el pendiente de M6.

**`--apk <fichero>`** no se compara con el certificado de release, porque el fichero lo elige el usuario. Sigue §6.1, y la ayuda de la opción lo dice.

## 7. Defensa en el clasificador

Ruling del supervisor. Un caso de `host/internal/classify`, con revisión opus. Estas órdenes sobre `tech.hybriq.adbuddy` han de ser 🔴 especiales:

- `pm uninstall` (con o sin `-k` y `--user`);
- `cmd package uninstall`;
- `pm clear`;
- `pm disable` y `pm disable-user`;
- `pm suspend`;
- `am force-stop` y `am kill`;
- `adb uninstall` (también con `-k`).

Además (revisión 2, M-6):

- `cmd` entra en la regla de argumentos dinámicos de `special.go`, como ya lo están `pm`, `appops` y `am`;
- las órdenes que neutralizan la app sin nombrarla pasan a 🔴 especiales, también en sus otras formas:
  - `pm reset-permissions` y `cmd package reset-permissions` sin `-p`, que quitarían WRITE_SECURE_SETTINGS, y con ello "Cortar acceso";
  - `dumpsys deviceidle whitelist reset` y `cmd deviceidle whitelist reset`;
  - `cmd appops reset` y `appops reset` sin paquete.

Si alguna de estas órdenes llega a quitar la app, la real deja de estar y entra en juego §6. Hoy `mentionsADBuddy` ya debería cubrirlas, por nombre de paquete en las palabras (en `shell` solo cuando el nivel no es 🟢). M6a lo deja escrito en `testdata/habitual.jsonl`, una línea por orden con `"level":"red","special":true`, y añade a `special.go` lo que no salga.

`adb install <apk>` sigue siendo 🟡: el host no sabe qué paquete lleva el APK, y §6 es lo que impide que una app falsa reciba nada.

## 8. Script de instalación (`scripts/install.sh`)

Para Linux y macOS: `curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh`. El instalador viaja con la release (§4); un cambio en `main` no llega a los usuarios hasta la siguiente release.

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

`host.yml` ya ejecuta los tests unitarios del parser y de `Verify`, con un corpus de *fuzzing* corto.

## 10. Acciones del usuario

Están en `docs/releasing.md`, en "User actions", y en "Validaciones manuales pendientes" del plan. **No bloquean el resto de M6a.** El script de la clave lo ejecuta el usuario en su terminal, **nunca el agente**: así el keystore no pasa por el transcript.

1. **Clave:** `sh scripts/release-key.sh`. Necesita `keytool`, de un JDK o del `jbr` de Android Studio. Genera un keystore PKCS12 RSA 4096, válido 30 años:
   - lo deja en `~/.adbuddy-release/` (carpeta 0700, fichero 0600, `umask 077`), y se niega si esa ruta está dentro del repo o si el keystore ya existe;
   - la contraseña la pide `keytool`; nunca pasa por la línea de órdenes;
   - imprime el SHA-256 del certificado y las órdenes para los secretos, **no** el base64.
2. **Copia de seguridad** del keystore y su contraseña. Si se pierden, ninguna release podrá actualizar la app instalada.
3. **Environment `release`** en GitHub:
   - solo para etiquetas `v*`;
   - con el propio usuario como revisor obligatorio;
   - con "Allow administrators to bypass" desactivado.

   Dentro, sus dos secretos, con las órdenes que imprime el script. Se ejecutan en Git Bash (el `|` y `<` de PowerShell no sirven). `gh` lee el valor de stdin o lo pide sin mostrarlo.
4. **Rulesets:**
   - etiquetas `v*`: solo los administradores las crean, mueven o borran;
   - `main`: sin push directo ni force-push, cambios solo por PR, y sin ningún actor que pueda saltárselo, `github-actions` incluido (revisión 2, I-4).

   Los jobs con `contents: write` (`host` y `publish-apk`) no pueden entonces tocar `main`.
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
- **Ruling: la confianza de desarrollo es el SHA-256 de un APK concreto, con aviso en cada `serve` mientras exista.** Por qué: la clave de debug está en el PC con una contraseña pública. Confiar en su certificado dejaría al agente sustituir la app con una actualización 🟡 que hereda token y permisos. Si me equivoco: cada build de desarrollo pide repetir `setup --apk`.
- **Ruling: el comportamiento de Android que §5 da por hecho se demuestra en un emulador antes de la primera release pública, no antes de integrar.** Por qué: la imagen del emulador está rota hasta M2.5b. Las condiciones de §5 son las de AOSP o más estrictas, y que el host exija todos los firmantes de todos los bloques hace que no importe cuál elige Android. Si me equivoco: un APK de ataque que el host acepte y Android instale con otra firma. No llega a ningún usuario antes de esa prueba, porque no hay release pública hasta entonces (validación 3 del plan).
- **Ruling: el contenido del APK lo decide `build-apk` (Gradle y sus dependencias), un riesgo aceptado.** Por qué: firmar sin compilar lo deja todo en el job aislado; reproducir el build en otro sitio es trabajo de F-Droid y M6c. Si me equivoco: una dependencia comprometida entra firmada, igual que en cualquier app Android.
- **Ruling: compilar y firmar en jobs distintos, y el de firma sin código de terceros, con environment y revisor.** Por qué: Gradle ejecuta plugins y dependencias de terceros que podrían leer el keystore o la contraseña. Y unos secretos del repo los lee cualquier workflow de cualquier rama. Si me equivoco: un artefacto y un job más por release.
- **Ruling: release en borrador, que publica el usuario.** Por qué: publicar es irreversible hacia fuera (la URL `latest` la usan todos los hosts instalados). Si me equivoco: un clic más por release.
- **Ruling: `MinAppVersionCode` en el host, hoy 0.** Por qué: con el certificado fijado, toda release vieja firmada por hybriqtech sigue siendo "nuestra". Si me equivoco: olvidar subirla deja la vuelta atrás abierta hasta la siguiente; `docs/releasing.md` lo pone en la lista de cada release de seguridad.
- **Ruling: firma con `apksigner` en el workflow, sin `signingConfig` en Gradle.** Por qué: la clave nunca toca Gradle ni su caché, y `android/` cambia solo en dos líneas de versión. Si me equivoco: mover la firma a Gradle es un cambio del job.
- **Ruling: sin rotación de clave.** Por qué: no la necesitamos y su linaje complica el parser. Si me equivoco: rotar la clave pide primero una release del host que lea el linaje.
- **Ruling: `versionCode` derivado de la etiqueta.** Por qué: reproducible y sin estado. Si me equivoco: solo caben versiones mayores hasta la 20.
- **Ruling: brew, winget, IzzyOnDroid, SignPath y la procedencia firmada, a M6c.** Por qué: necesitan cuentas, repos o una release pública. Si me equivoco: goreleaser los añade con pocas líneas.
- **Ruling: `CGO_ENABLED=0`.** Por qué: compilación cruzada y binarios estáticos. M3 confirma que su PTY es Go puro (conpty, creack/pty y charmbracelet/x/vt). Si me equivoco: si algún destino necesita cgo, pasa a su runner nativo.

## 12. Revisiones de seguridad y cómo se resolvieron

### Revisión 1

| Hallazgo | Resuelto en |
|---|---|
| C1: el certificado solo se comprueba al descargar, no en la app que recibe el token y las concesiones | §6.2 y §6.3 (`Verify` y el tipo `Verified`) y §7 |
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

### Re-revisión 2

| Hallazgo | Resuelto en |
|---|---|
| I-1: confiar en el certificado de debug deja actualizar la app con la misma clave y heredar el token | §6.1 (SHA-256 del APK, copia privada, aviso en cada `serve`, borrado al verificar una release) |
| I-2: `Verified{}` se puede construir fuera del paquete | §6.2 y §6.3 (cada función rechaza el valor cero, con test) |
| I-3: las afirmaciones sobre Android no tienen evidencia grabada | §11 (ruling) y validación 3 del plan: emulador en API 30, 33 y la última, antes de la primera release |
| I-4: la integridad de `main`, `install.sh` servido desde `main` y el grafo de `needs:` | §4 (grafo y `extra_files`), §8 y §10.4 (ruleset de `main`) |
| M-1: primer certificado, `META-INF/` a cualquier profundidad, recuento exacto, nombres con NUL o UTF-8 inválido | §5, condiciones 8, 10 y 11 |
| M-2: salida de `base64` sin límite, plazo propio, `pm path` al final | §6.2, pasos 3 y 6 |
| M-3: `versionCode` del bloque del paquete | §6.2, paso 5 |
| M-4: la autoridad del provider | §6.3 |
| M-5: TOCTOU de `--apk`; fichero de confianza normal y 0600 | §6.1 |
| M-6: `cmd` con argumentos dinámicos; `pm reset-permissions`, `deviceidle whitelist reset` y `cmd appops reset` | §7. El `dump()` de la app es cosa de M5 (aviso a nautilus-19) |
| M-7: cachés y contenido de lo que se firma | §4 (sin cachés, artefacto único, rutas absolutas) y §11 (riesgo aceptado) |
| M-8: PowerShell, bypass de administradores, almacenamiento adoptado y `doctor` | §10, §6.2 y M6c (`doctor`, en `pendientes-m3-m5.md`) |

### Re-revisión 3

| Hallazgo | Resuelto en |
|---|---|
| C-1: una actualización con la clave de debug hereda el token vigente sin handshake nuevo | §6.3 (`bridge.Options.Admit` con `Verifier.Same`, y el token rota antes de verificar) |
| I-a: borrar `trusted-dev-apk` no surte efecto hasta reiniciar `serve` | §6.2 (la confianza se relee en cada llamada y forma parte de la clave de la caché) |
| I-b: la caché por ruta da por hecho, sin evidencia, que la misma ruta es el mismo APK | §6.2 (clave = ruta + `stat` + confianza) y validación 3 |
| Me-1: el valor cero, en `still()` | §6.2 |
| Me-2: `--apk` limitado, copia atómica, confianza solo tras instalar | §6.1 |
| Me-3: otras formas de las órdenes de *reset* | §7 |
| Me-4: la autoridad con el provider desactivado | validación 3 |
| Me-5: el límite del modelo en el PC | §6.2, "Límites del modelo" |
| Me-6: comillas y CR LF | `shq.Join` cita la ruta; el límite de `base64` cuenta CR LF |
| Me-7: un `trusted-dev-apk` por PC | §6.2, "Límites del modelo" |
| Me-8: el plan desalineado | plan reescrito con esta spec |

## 13. Tests

- **Parser** (`phoneapp/apksig_test.go`), con APK sintéticos de `phoneapp/apktest`:
  - aceptados: v2, v3 y v3.1 buenos, v2 + v3 y los bloques permitidos que no son de firma;
  - rechazados por quién firma o por los firmantes: v2 nuestro + v3 ajeno, dos firmantes con uno ajeno, rotación, sin firmantes y certificado vacío;
  - rechazados por la estructura del bloque: sin bloque, solo relleno, ID desconocido, ID repetido, datos sobrantes y truncado;
  - rechazados por la estructura del zip: EOCD con comentario, hueco entre el directorio central y el EOCD, localizador ZIP64, `0xffffffff`, tamaños del bloque distintos o fuera de rango, par fuera del bloque, `META-INF/CERT.SF`, `cert.rsa` o `META-INF/a/b.SF`, un recuento de entradas que no cuadra con el directorio central, y nombres con NUL o UTF-8 inválido;
  - además, `FuzzSigners`.
- **`Verify`**, con el adb falso del paquete:
  - casos buenos: el certificado de release y el APK de desarrollo exacto;
  - rechazados: firmante ajeno, otro APK con la misma clave que el de desarrollo, `pm path` con dos líneas, con ruta rara, vacío o que cambia durante la lectura, tamaño de más, salida de `base64` más larga de lo esperado o ilegible, `versionCode` solo fuera del bloque del paquete, versión menor que la mínima y adb que falla. Todos cerrados;
  - la caché: no vuelve a leer la misma ruta, sí una nueva, y no recuerda los fallos de adb.
- **`Handshake` y concesiones:** con `Verified{}`, o con uno cuya ruta ya no da `pm path`, no envían nada.
- **`Download`:** firmante ajeno, sin certificado (cero peticiones), y que la confianza de desarrollo no valga para una descarga.
- **`Install`:** `--no-incremental` y `ErrOtherSigner`, por stdout y por stderr.
- **`cli`:**
  - `setupApp`: los mensajes nuevos, que no se pida consentimiento ni se conceda nada a una app no verificada, `--apk` que confía en el APK de desarrollo con su aviso e instala la copia privada, y un setup con la app de release que borra `trusted-dev-apk`;
  - `startPhoneLink`: no entrega el token a una app no verificada, avisa una vez y no relee el APK en cada tick; avisa al arrancar si existe `trusted-dev-apk`;
  - `ADBUDDY_RELEASE_URL` con un esquema o host no permitidos.
- **Clasificador:** las líneas de §7 en `habitual.jsonl`, también las de argumentos dinámicos con `cmd` y las tres órdenes de *reset*.
- **Scripts y pipeline:** §9.

## 14. Coordinación

| Fichero | Dueño | M6a toca | Ok del supervisor |
|---|---|---|---|
| `host/internal/phoneapp/**`, `host/cmd/apkcert` | M6a | todo | sí |
| `host/internal/cli/appsetup.go` y su test | M6a | todo | sí |
| `host/internal/cli/setup.go` | — | la ayuda de `--apk` | sí |
| `host/internal/cli/phone.go` y `phone_test.go` | compartido | `connect()` (`Verify` + `Handshake` con `Verified`), el `Verifier` y el canal `recheck` de `phoneLink`, el gancho `Admit` (`Same`), el `select` del ticker, el aviso de arranque de `trusted-dev-apk` y el adb falso de los tests | sí, en un commit propio |
| `host/cmd/adbuddy/phone_e2e_test.go` | compartido | reglas `pm path`, `stat` y `base64`, y el APK de desarrollo de confianza en el directorio de datos | sí |
| `host/cmd/adbuddy/e2e_test.go` | tern (M3) | solo `startWithData(t, cfg, prepare, rules...)`: `startWith` delega en él con `prepare` nil; solo prepara el arnés (la confianza de desarrollo, antes de arrancar serve) y no cambia ninguna aserción | sí (ruling del supervisor), en un commit propio |
| `host/internal/bridge/server.go` | tern (M3) | `Options.Admit`, `admitTimeout` y su llamada en `handle`, en un commit propio tras m3-bridge | sí |
| `host/internal/classify/special.go` y `testdata/habitual.jsonl` | — | §7 | sí (ruling) |
| `android/app/build.gradle.kts` | tern (dependencias) | las 2 líneas de versión, en su propio commit | sí |
| `pendientes-m3-m5.md` | cada uno su sección | solo M6 | sí |

M5 (nautilus) se beneficia de `Verified` sin cambios: si alguna vez la app necesita algo más del host por adb, la función que lo dé pide un `Verified`.
