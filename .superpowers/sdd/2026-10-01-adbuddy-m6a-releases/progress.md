# M6a · ledger

Plan: `docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md` · Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` · Rama `m6a` (rebasada sobre main f36e74d).

## Estado (2026-10-01)

- **Spec cerrada tras cuatro revisiones de seguridad.** Revisión 1 de nautilus-19 y tres re-revisiones de opus; la tabla de resoluciones está en §12.
- **Revisión opus del plan: NO lista para ejecutar.** Tiene 1 crítico y 9 importantes, que bloquean la compilación o los tests de las tareas. No hay ningún fallo de seguridad.
- **En PAUSA por orden del supervisor (adbuddy-e5)**, por el límite de tokens. Se reanuda cuando él avise. Siguiente paso: corregir el plan con los hallazgos de abajo y montar la réplica acumulada.

## Rulings

- **Ruling:** la confianza de desarrollo es el SHA-256 de un APK concreto, no su certificado. **Por qué:** la clave de debug vive en el PC con una contraseña pública. **Qué cuesta si me equivoco:** repetir `setup --apk` con cada build.
- **Ruling:** la prueba en emulador (API 30, 33 y la última) se hace antes de la primera release pública, no antes de integrar. **Por qué:** la imagen del emulador está rota hasta M2.5b. Lo aceptó el supervisor. **Qué cuesta si me equivoco:** un APK de ataque que el host acepta; no llega a nadie, porque aún no hay release pública.
- **Ruling:** el gancho `bridge.Options.Admit` lo pongo yo, en un commit propio sobre m3-bridge. **Por qué:** cierra C-1 de la re-revisión 3. El supervisor y tern dieron el ok, con dos condiciones: falla cerrado y, si rechaza, no se llama a OnConnect ni a OnAttach. **Qué cuesta si me equivoco:** un rebase de tern.

## Revisión del plan (adbuddy-reviewer-opus): hallazgos que hay que corregir

### Crítico

- **C1 · Task 5, Step 4.** Los tests e2e de `phone_e2e_test.go` fallarán: `startWith` (de `e2e_test.go`, de tern) arranca `serve` antes de que `trustE2EApp` escriba la confianza. El primer `Verify` da `ErrUnpinned` y el siguiente tick llega a los 15 s, más allá del plazo de `waitFor`, que es de 10 s.
  - **Arreglo:** `startWithData(t, cfg, prepare func(data string), rules...)` en `e2e_test.go`.
  - Pide el ok del supervisor y de tern, y añadir esa ruta a las Global Constraints y a la §14 de la spec.

### Importantes

- **I1 · Task 2.**
  - `TestReleaseURLEndsWithASlash` (`appsetup_test.go:290`) usa `releaseURL` con un solo resultado: reescribirlo con casos buenos y malos (`example.com`, `file://`, `ftp://127.0.0.1`, `127.0.0.1.evil.com`, `127.0.0.1@evil.com`, `https://`).
  - El import `encoding/base64` va en la Task 3, no en la 2.
- **I2 · Task 3.** El helper `verified(f)` también tiene que responder a `stat` y devolver `key`: `Verified{path: testPath, key: "100 1 1", cert: "test"}`, con la regla `stat -c '%s %i %Y'`. Añadir `shq` a los imports. Ojo: cada función hace dos llamadas más (`pm path` y `stat`).
- **I3 · Task 3.** En los casos con `otherCert`, `stat` sigue dando el tamaño del APK de `testCert`. Hay que añadir un campo `apk` a la tabla y llamar a `appRulesFor(consent, apk, …)`, sin sustituir `base64` en `over`.
- **I4 · Task 3.** El caso "apk that is not signed" espera `want{consent: "skipped"}` (`Installed: false`). Borrar el párrafo que lo justifica.
- **I5 · Task 3.** Añadir `func init() { releaseCert = apktest.SHA256(testCert) }` en `appsetup_test.go`. Sin él se rompen `TestSetupSetsUpTheApp` (`:107`) y el caso "consent" de Ctrl+C (`setup_test.go:405`).
- **I6 · Task 5.** `TestServeInterruptedDuringTheHandshake` (`phone_test.go:373`) necesita las reglas de lectura. Sacarlas a un helper `appReadRules(apk)` y usarlo ahí.
- **I7 · Task 5.** No importar `apktest` en `phone_test.go`: no se usa.
- **I8 · Faltan tests:**
  - (a) en `cli`: Admit → Same → nuevo `connect()` tras una actualización en el sitio, comprobando que el token viejo se rechaza y que no se entrega otro a la app actualizada (campo `statKey` en `appPhone`);
  - (b) el aviso de `trusted-dev-apk` al arrancar `serve`, una sola vez;
  - (c) la caché con un `stat` distinto en la misma ruta: se vuelve a leer la app y da `ErrSigner`;
  - (d) borrar la confianza de desarrollo con `Release` puesto da `ErrSigner`.
- **I9 · Task 10.** Quitar `release.github` de `.goreleaser.yaml`: tiene que funcionar también en un fork.

### Menores

1. Ayuda de `--apk`: "exactly this APK becomes the trusted development build".
2. Al aviso de desarrollo le falta "of <file>": pasar la ruta original.
3. Comprobar que no hay `requestSetup` ni `am start` con una app sin verificar.
4. Un test del valor cero para cada función, también `Handshake`.
5. `resetsEveryApp`:
   - saltar el valor de `--user`;
   - casos de corpus nuevos: `adbudd*`, `adbu""ddy`, `sh -c '…'`, `deviceidle whitelist -pkg`, `cmd appops reset --user 0` y `cmd activity force-stop`;
   - quitar el duplicado de `habitual.jsonl:145`.
6. `shellcheck`:
   - SC2086 en `$sum` (`install.sh`): usar una función `sum()`;
   - SC2086 en `set -- $c` y SC2016 en los tests: `disable` con su motivo.
7. Fijar una versión exacta de goreleaser en las acciones.
8. Task 5, Step 6: comprobar con `go build ./...` justo después del primer commit, sin stash.
9. Global Constraints y §14: `phone.go` cambia más (campos, `Admit` y el `select` del ticker). Avisar a tern.
10. `Same` no puede esperar a que `Verify` suelte el mutex: leer `ok` con un `atomic.Pointer`.
11. Erratas:
    - "gancho de la Task 5" → Task 4;
    - "sustitúyelo en la 10" → 11;
    - `appRulesFor`, con la lista completa de reglas.
12. Opcional: en el EOCD, exigir que el número de entradas del disco sea igual al total.
