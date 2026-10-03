# Fragmento para pendientes-m3-m5.md (sección "CI y runners (A5, revisión de seguridad 2026-10-03)")

Sustituye las viñetas A5-1 y A5-3 de "Documentado, sin cierre técnico" por:

- **A5-1 y A5-3: riesgo aceptado por el usuario el 2026-10-03; no resueltos.** El usuario decidió que todo se queda en su cuenta de Windows y que no se creará la cuenta dedicada (receta opcional: bloque 3 de `docs/superpowers/plans/2026-10-03-runners-pasos-usuario.md`). El runner Windows usa su `ANDROID_HOME` y su `adb`; una rama que llegue a un runner propio puede alcanzar su adbkey, su servidor adb y su SDK. Condiciones de la aceptación (`docs/ci-self-hosted.md` §6, "Accepted risk"):
  - repositorio privado, nunca público con runners registrados;
  - solo los PRs del propietario usan runners propios (lista blanca por autor de #54);
  - sin auto-merge y sin "Dependabot en self-hosted";
  - mitigación opcional: revocar en el Pixel las autorizaciones de depuración inalámbrica cuando no se prueba.
  Si alguna condición cambia (repositorio público, colaboradores, auto-merge), el riesgo deja de estar aceptado y hay que hacer la cuenta dedicada antes.
- **Runners (2026-10-03):** etiquetas por capacidad y opt-in: Windows 1 (SDK y AVDs) `adbuddy-emulator` y `adbuddy-host-windows`; Windows 2 (solo host, sin SDK ni JDK) `adbuddy-host-windows`; Linux 1 y 2 (WSL) `adbuddy-linux`. Ningún workflow lleva etiquetas: las leen de `CI_RUNNER_*` y `CI_HOST_MATRIX`. `adbuddy-local` y `adbuddy-host` son legado prohibido; `python scripts/check-ci-runner-rules.py [--live [--strict]]` lo hace cumplir. Los jobs en cola con una etiqueta antigua la conservan tras un cambio de variable (el 2026-10-03 un job de emulador antiguo cayó en el runner 2): retirar una etiqueta solo con la cola vacía de ella.
