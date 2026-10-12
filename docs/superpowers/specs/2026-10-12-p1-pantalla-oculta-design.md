# Pantalla oculta y overlay en la captura: diseño (2026-10-12)

> Compañero `seguridad`. Hallazgos de la prueba en un Pixel 9 Pro XL real (Android 17, 2026-10-12). Complementa v1 §5.7 y M5 §3.1, §6 y §12; no reabre el ruling del 2026-10-11 21:47 (el overlay avisa también al leer).

## 1. Problemas

**P1.1. `adbuddy screen` devuelve «unknown app» y ningún elemento** en Opciones para desarrolladores (`com.android.settings/.SubSettings`), también con `--all`, mientras `adbuddy screenshot` sí la ve.

**P1.2. La píldora y el marco del overlay salen dentro de `adbuddy screenshot`** y tapan contenido, porque el aviso se muestra antes de `screencap` y sigue puesto durante la captura.

## 2. Diagnóstico de P1.1

Datos del Pixel (`dumpsys window`, `dumpsys accessibility`, solo lectura, 2026-10-12; en el repositorio solo se citan sin seriales):

- La ventana `SubSettings` existe, es la activa y la enfocada, tiene título, ocupa toda la pantalla y **no lleva `FLAG_SECURE`** (por eso `screencap` la ve). Sus flags son los mismos que en el emulador API 36, donde se lee bien.
- El servicio de ADBuddy está vinculado con `capabilities=33` (leer contenido + gestos) y `TYPE_WINDOWS_CHANGED`: tiene acceso a ventanas. El gestor de accesibilidad solo le enseña **dos ventanas**: la barra de estado y la de la app. No lista barra de navegación ni teclado, que sí existen.
- `Dumper` salta en silencio toda ventana cuyo `root` es `null`, y `foreground()` devuelve `"" to ""` si la ventana de app activa no tiene `root`. Eso encaja con «unknown app» y cero elementos, y con la barra de estado ausente: el servicio **no recibe contenido de ninguna ventana**.
- Android 17 no corre en emulador (surfaceflinger aborta; ya documentado), y en API 36 no se reproduce. **La causa última (qué mecanismo del sistema oculta el contenido) no se puede confirmar sin el móvil**; hipótesis sin verificar: datos sensibles para accesibilidad en servicios que no declaran `isAccessibilityTool`, o una protección propia de Ajustes en Android 17.

Decisión: **no** declarar `isAccessibilityTool` ni buscar rodeos. Sería saltarse una protección del sistema contra malware que abusa de la accesibilidad, y no cumple la política de uso de la API. Tampoco se usa `uiautomator` por adb (desconecta el servicio, M5 §5). Lo que sí es nuestro, y se arregla:

- El agente no sabe **por qué** la pantalla sale vacía. Se añade al dump el campo `unreadable` y una nota fija que explica, sin afirmar más de lo que se sabe, qué pasa y qué puede hacer (§4).
- Para diagnosticar la causa en el Pixel sin revelar contenido, el servicio deja una línea de log con los tipos de ventana y si tienen contenido (§5).

## 3. Diseño de P1.2: retirar el overlay durante la captura

Medido en emulador API 36 (`adb exec-out screencap -p` incluye marco y píldora; sustituir la captura por `AccessibilityService.takeScreenshot` no los quita del todo: excluye la píldora pero incluye el marco, comportamiento no documentado, y añade una capacidad más al servicio, así que se descartó):

| Método | Capturas limpias | Latencia |
|---|---|---|
| Quitar las vistas del overlay (`removeViewImmediate`), esperar 3 fotogramas | 6/6 (18/18 con los tres métodos probados) | ack en 40-60 ms, restaurar 5-40 ms, `screencap` 190-420 ms (a veces 600-900) |
| `alpha = 0` / `INVISIBLE` | 6/6 | igual, pero la píldora seguiría tocable e invisible |

**Flujo de `adbuddy screenshot`** (cambia solo este helper):

1. Igual que hoy: guarda de primer plano, pausa, `notice` (el overlay sale y la persona lo ve).
2. Nuevo: `hideNotice {ms: 2000}`. La app espera a que el aviso lleve **al menos 400 ms** en pantalla (`MIN_SHOWN_MS`: sin ese mínimo la persona apenas llegaría a verlo antes de la captura), quita marco y píldora, espera 3 fotogramas (y 50 ms como mínimo, por si la pantalla va a 120 Hz) y responde. Programa su **restauración automática** a los `ms` (tope `MaxHideNoticeMs = 5000`); si la pausa llega, no se vuelve a mostrar. Cuesta de 0 a 0,4 s más por captura.
3. Pausa otra vez y, si retirar el aviso tardó más de `baselineSlack` (150 ms), la guarda de primer plano otra vez (una app protegida pudo llegar delante); `screencap`.
4. Nuevo: `notice` otra vez (restaura el overlay y rearma sus 4 s). Si el host muriera aquí, el temporizador de la app lo restaura igualmente.

**Lo que cambia respecto al ruling del 2026-10-11:** el aviso lleva en pantalla al menos 400 ms antes de la captura y vuelve justo después, pero está **ausente unos 0,3 s durante la propia captura** (tope de 2 s por la restauración automática) de `adbuddy screenshot`. La persona ve el aviso al empezar y al acabar, y marco y píldora siguen 4 s tras la captura. Durante ese instante no hay píldora que pulsar (cuesta: un toque de la persona en esa zona cae en la app de debajo; se descartó dejar la píldora transparente y tocable porque es una ventana más que medir y el mínimo de 400 ms más los 4 s posteriores la dejan accesible casi siempre). Las lecturas en bruto del agente (`screencap`, `screenrecord`, `uiautomator` por `adb shell`) **no** retiran el overlay: siguen mostrando el aviso, también dentro de la imagen (el agente que quiere una captura limpia usa `adbuddy screenshot`).

**Si no se puede retirar** (app antigua sin la op, error, tiempo agotado): la captura sale igualmente, con el aviso dentro, y el agente lo sabe: `ScreenshotResult.NoticeInImage` y una nota por stderr. Sin app o con la accesibilidad apagada no hay aviso que retirar y no hay nota (límite conocido: si la app desaparece justo entre `notice` y `hideNotice` con el servicio aún dibujando, la nota puede faltar). Si la app responde que la sesión está en pausa, la captura se bloquea como cualquier lectura.

**Concurrencia (límite conocido):** el ejecutor de la app serializa las ops, pero otro comando concurrente puede cruzarse con el velo: una lectura en bruto cuyo `notice` llega antes que `hideNotice` se hace sin aviso durante el velo (hasta 2 s) y el aviso vuelve después; un `tap`, `type` o segundo `screenshot` concurrentes vuelven a mostrar el aviso y la captura sale con él dentro aunque diga `noticeInImage: false`. Nada se lee o se hace sin que el aviso suba antes o inmediatamente después; no se bloquea.

`hideNotice` no es un helper del agente: solo lo llama el host dentro de `screenshot`. El agente no puede pedir que el aviso se quite para actuar: cualquier `tap`/`type` vuelve a mostrarlo (`acting`).

## 4. Diseño de P1.1: decir la verdad cuando no se puede leer

- **Protocolo (aditivo):** `UiDump.unreadable` (`bool`, `omitempty` en Go; `Boolean? = null` en Kotlin). La app lo pone a `true` cuando no hay ninguna ventana, o alguna ventana de app o diálogo no entrega su contenido.
- **CLI:** `printScreen` imprime, tras la cabecera, una nota fija en inglés (es para el agente): ADBuddy no puede leer esta pantalla porque Android o la app ocultan su contenido a los servicios de accesibilidad (o está cambiando: probar una vez más); `adbuddy screenshot` puede enseñarla (negra si la app bloquea las capturas); ADBuddy rechaza tocar lo que no puede leer: pedir a la persona esa parte. `--json` lleva `"unreadable": true`.
- Sin sugerir rodeos: no se menciona `uiautomator`, ni activar nada.

## 5. Diagnóstico para la causa

`AgentAccessibilityService.windows()` registra (`Log.i`, solo cuando alguna ventana no entrega contenido) una línea con tipo y capa de cada ventana y si tiene contenido. Nada de títulos, paquetes ni texto. Con ella, una prueba en el Pixel (`adb logcat -d -s ADBuddyA11y` tras un `adbuddy screen`) distingue «el sistema no entrega la ventana» de «la entrega sin contenido». Si el resultado indica un ocultamiento deliberado del sistema, solo se explica (§4); si indica un fallo nuestro, se abre otro arreglo.

## 6. Fuera de alcance

- FLAG_SECURE en ADBuddy frente a solo Inicio: decisión de la persona usuaria.
- Permiso de notificaciones del Pixel: de la persona usuaria.
- Hacer legible lo que el sistema o la app ocultan a propósito.
