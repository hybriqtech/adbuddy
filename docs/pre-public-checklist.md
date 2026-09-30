# Antes de hacer público el repo

Lista de comprobación. Revisar cada punto antes de cambiar la visibilidad de `hybriqtech/adbuddy` a pública.

- [ ] **IPs de Tailscale** del móvil y del PC en `spikes/env.sh`, `docs/spikes/s1-modo-remoto.md` y el plan de M0. Mitigación: cambiar las IPs de los nodos en la consola de Tailscale, o redactarlas.
- [ ] **Rutas con el nombre de usuario de Windows** en `spikes/env.sh`, `spikes/s2-a11y/build.sh`, `spikes/s3-pty/main.go` y el plan de M0.
- [ ] **Nombres de equipos** (`rtx5080`, `pixel-9-pro-xl`) en los mismos ficheros.
- [ ] **Número de serie del móvil fuera del historial de `main`.** Buscar el serial del Pixel (y su identificador mDNS) en todo el historial: `git log -p main | grep -E "<serie>|<id-mdns>"` debe salir vacío.
- [ ] **Fichero `LICENSE`** (Apache-2.0, §12 de la spec), que aún no existe.
- [ ] **README** en inglés con traducción al español (§12 de la spec).
- [ ] **Decidir si `spikes/` se publica o se elimina.**
- [ ] **GitHub Pages:** publicar `docs/` (Settings → Pages → rama `main`, carpeta `/docs`) y comprobar que `https://hybriqtech.github.io/adbuddy/setup/` carga: `adbuddy setup` enseña un QR a esa página. En un repo privado Pages necesita un plan de pago.
- [ ] **Capturas reales** en `docs/setup/` en lugar de `img/pending.svg`, y revisar que los textos de los menús coinciden con Android en inglés y en español.
