# Fragmento: capturas reales de la guía de preparación del móvil

Rama `docs/capturas-setup`. Sustituye los 8 marcadores `img/pending.svg` de `docs/setup/index.md` y `docs/setup/es/index.md` por capturas reales y borra `docs/setup/img/pending.svg`.

## Qué se hizo

- Capturas hechas en el emulador (Android 14, API 34, imagen `google_apis`), nunca en el móvil real, primero con el sistema en inglés y luego en español: `01-build-number`, `02-wireless-debugging`, `03-pair-qr` y `04-pair-code`, cada una con su `-es`. Recortadas a lo que explica cada paso y reducidas a 480 px de ancho con paleta de 64 colores (11-19 KB cada una).
- Recortes sin datos personales: sin barra de estado, sin el título «About emulated device» y sin la lista «Paired devices» (que enseña el nombre de usuario y del equipo del PC). El cuadro del código de emparejamiento y la pantalla de depuración inalámbrica enseñan solo la IP interna del emulador (`10.0.2.x`) y un código de un solo uso.
- Textos de los menús comprobados contra las pantallas completas del emulador en los dos idiomas (incluidos el aviso de red, «Permitir siempre en esta red» y los dos botones de emparejar, que las capturas recortadas no enseñan). Corregidos en español: «Dispositivos vinculados» pasa a «Dispositivos emparejados» y «código de vinculación Wi-Fi» a «código de vinculación de Wi-Fi». El resto coincide.

## Pendiente

- El emulador titula la pantalla «About emulated device» / «Información sobre el dispositivo emulado» (por `Build.IS_EMULATOR`), no «About phone» / «Información del teléfono» como en un móvil real. Las capturas se recortaron para no enseñarlo y los textos de la guía son los de un móvil real. «About phone» solo se pudo comprobar por el texto de otros recursos de Ajustes.
- El aviso al tocar siete veces «Número de compilación»: el texto de Ajustes (`show_dev_on`, sacado del APK de Settings del AVD con `aapt2 dump resources`) es «¡Ahora están activadas las opciones para desarrolladores!» en es y «¡Ya eres desarrollador!» solo en es-US; la guía usa el primero y avisa de que cambia según móvil y región. En inglés es exacto («You are now a developer!»). «Información del teléfono» (es) está en AOSP, en el texto de la garantía de Ajustes.
- Opcional: sustituir las capturas por otras de un móvil real (Pixel) cuando haya ocasión; los nombres de fichero no cambian.
