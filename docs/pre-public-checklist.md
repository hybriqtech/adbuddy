# Antes de hacer público el repo

Lista de comprobación. Revisar cada punto antes de cambiar la visibilidad de `hybriqtech/adbuddy` a pública.

- [ ] **IPs de Tailscale** del móvil y del PC en `spikes/env.sh`, `docs/spikes/s1-modo-remoto.md` y el plan de M0. Mitigación: cambiar las IPs de los nodos en la consola de Tailscale, o redactarlas.
- [ ] **Rutas con el nombre de usuario de Windows** en `spikes/env.sh`, `spikes/s2-a11y/build.sh`, `spikes/s3-pty/main.go` y el plan de M0.
- [ ] **Nombres de equipos** (`rtx5080`, `pixel-9-pro-xl`) en los mismos ficheros.
- [ ] **Número de serie del móvil fuera del historial de `main`.** Buscar el serial del Pixel (y su identificador mDNS) en todo el historial: `git log -p main | grep -E "<serie>|<id-mdns>"` debe salir vacío.
- [x] **Fichero `LICENSE`** (Apache-2.0, §12 de la spec): ya existe.
- [x] **README** en inglés con traducción al español (§12 de la spec): hecho en M6d (`README.md` y `README.es.md`).
- [ ] **Decidir si `spikes/` se publica o se elimina.**
- [ ] **GitHub Pages:** publicar `docs/` (Settings → Pages → rama `main`, carpeta `/docs`) y comprobar que `https://hybriqtech.github.io/adbuddy/setup/` carga: `adbuddy setup` enseña un QR a esa página. En un repo privado Pages necesita un plan de pago.
- [ ] **Clave de firma del APK** de release, fuera del repositorio (secreto de GitHub Actions) y con copia de seguridad: perderla impide actualizar la app instalada.
- [ ] **Release del APK** con `adbuddy.apk` y `adbuddy.apk.sha256` (lo que descarga `adbuddy setup`).
- [ ] **Capturas reales** en `docs/setup/img/` en lugar de `img/pending.svg`: hay 4 marcadores en `docs/setup/index.md` (inglés) y 4 en `docs/setup/es/index.md` (español), 8 en total. Sustituir cada marcador `![Pending screenshot: …](img/pending.svg)` (en español, `![Captura pendiente: …](../img/pending.svg)`) por la captura, con estos nombres de fichero:
  - [ ] `01-build-number.png` y `01-build-number-es.png`: Ajustes › Acerca del teléfono › Número de compilación (§1 de la guía).
  - [ ] `02-wireless-debugging.png` y `02-wireless-debugging-es.png`: el interruptor de Depuración inalámbrica (§2).
  - [ ] `03-pair-qr.png` y `03-pair-qr-es.png`: Emparejar dispositivo con código QR (§3).
  - [ ] `04-pair-code.png` y `04-pair-code-es.png`: Emparejar dispositivo con código de sincronización (§3, «If the QR code does not work»).

  Revisar además que los textos de los menús de las guías coinciden con Android en inglés y en español, y quitar `img/pending.svg` si ya no lo usa nadie.
- [ ] **Donaciones: crear las cuentas** (perfil de GitHub Sponsors y página de Ko-fi) y poner los identificadores reales: descomentar `.github/FUNDING.yml` (`github:` y `ko_fi:`) y el comentario HTML "Support the project" de `README.md` y de `README.es.md` (sustituyendo `<handle>`); enlazarlos desde "Acerca de" de la app (M5c). Hasta entonces no hay ningún enlace a Sponsors ni a Ko-fi.
- [ ] **Política de seguridad:** habilitar la *private vulnerability reporting* de GitHub (Settings › Code security) o crear un `SECURITY.md`. Cuando exista, enlazarlo desde la sección "Reporting a security problem" de `docs/safety.md`, que hoy dice que no existe.
- [ ] **Repasar la documentación contra la primera release publicada:** `docs/install.md`, el Quick start del README y `docs/setup/` (nombres de los archivos de la release, que la URL de `install.sh` responda, que `adbuddy setup` descargue la app y que el aviso de SmartScreen siga siendo cierto) y quitar lo de «cuando haya release» / «no public release yet» / «once there is a release» (hallazgos de `grep -rn -i "until there is\|when there is a public release\|cuando haya\|todavía no hay\|once there is\|no public release" README.md README.es.md docs/*.md docs/setup`: `README.md` líneas 34, 46 y 150; `README.es.md` líneas 34, 46 y 152; `docs/install.md` líneas 9, 11, 70 (sección "When there is a public release"), 174 y 175; `docs/faq.md` línea 53; `docs/safety.md` línea 96; `docs/troubleshooting.md` línea 284; `docs/setup/index.md` línea 81; `docs/usage.md` no tiene ninguno).
- [ ] **Self-hosted CI runners gone.** Before making the repository public, remove the runners in Settings > Actions > Runners, delete their folders, run `gh variable delete` for the five `CI_*` variables (`CI_HOST_MATRIX`, `CI_RUNNER_ANDROID`, `CI_RUNNER_EMULATOR`, `CI_RUNNER_RELEASE`, `CI_RUNNER_SIGN`), and check that "Approval for running fork pull request workflows" requires approval for all outside collaborators. Dependabot (`.github/dependabot.yml`): its pull requests already run on GitHub-hosted runners only (docs/ci-self-hosted.md section 6); once the repository is public, review that rule and the Dependabot settings (the self-hosted runners are gone by then).
