# Handoff · M6b (skills de contexto) tras M6a

Sesión nueva y limpia. Supervisor: adbuddy-e5 (avísale al empezar; mensajes de 1-3 líneas; push, PR e integración solo con su ok; Sonnet para implementar y re-revisar, opus solo en seguridad y la revisión final; un subagente a la vez).

## Dónde queda M6a
- Rama `m6a` (rebasada sobre main a11cbf2), plan `docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md`, spec `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md`, ledger `.superpowers/sdd/2026-10-01-adbuddy-m6a-releases/progress.md`.
- Hecho: parser del APK Signing Block, `phoneapp.Verifier`/`Verified`, `bridge.Options.Admit`, clasificador (app de ADBuddy 🔴 especial), apkcert, versión desde la etiqueta, install.sh, goreleaser y workflows, LICENSE y docs/releasing.md.
- Lo que solo comprueba la CI: `-race`, shellcheck, actionlint, `assembleRelease`/R8, Gradle, apksigner frente a apkcert, `gh release upload` a un borrador.
- Quedan fuera de M6a: pin real de `SigningCertSHA256` (paso 6 de `docs/releasing.md`; al hacerlo, corregir la frase del README sobre builds desde el código), la clave de release (la genera el usuario), la prueba en emulador (API 30, 33 y la última) antes de la primera release pública, y los aplazados de la sección `## M6 · Publicación` de `docs/superpowers/plans/pendientes-m3-m5.md`.

## M6b
- Alcance: las skills de contexto (spec de M6a, línea 44: van después de M6a, junto con la i18n y el README completo). Empieza por la spec (`superpowers:brainstorming`), luego plan, réplica acumulada, revisión del plan y `superpowers:subagent-driven-development`.
- Reglas del proyecto en `CLAUDE.md`; trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; gh con `GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech"` y `-R hybriqtech/adbuddy`; `git grep` de seriales e IPs antes de cada push; nunca `--force`.

## Reglas nuevas (supervisor, 2026-10-01)
- Hooks de git locales en el repo (commit-msg: identidad hybriqtech y trailer exacto, también en merges; pre-push: nada de no-ff, IPs privadas ni borrar main): si bloquean, se arregla la causa, nunca `--no-verify`. Merges con el trailer en `-m`.
- `gh` siempre con `GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech"` y `-R hybriqtech/adbuddy`; nunca `--force` (si una rama ya pushed necesita reescribirse: rama nueva).
- Una revisión por tarea con lógica (no por cada fichero mecánico); tras una revisión final limpia, push y PR sin pedir ok; el merge lo hace el supervisor. `main` está en 244e56f.
