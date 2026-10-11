# Fragmento UX P6b: CLI (errores de uso, `doctor`, QR de `setup`, ejemplos)

El integrador copia lo que haga falta a `pendientes-m3-m5.md` y a `2026-10-03-validaciones-manuales.md`. Este fragmento no edita ninguno de los dos.

## Qué cambió

- **H2 (8a8bb61c, a3dba090):** los errores de uso de cobra (flag desconocido, argumentos de más o de menos, comando desconocido, valor de flag inválido) salen en el idioma de la persona, con la mejor sugerencia (por prefijo y, si no, por distancia <= 2). Con `--json`, el `message` del error `USAGE` pasa del texto crudo de cobra a nuestro inglés; `code`, `fix` y exit 2 no cambian. `bad flag syntax` pasa de INTERNAL/exit 1 a USAGE/exit 2.
- **H5 (ad195b45, 7899d37f):** `doctor` imprime cada comprobación según termina. Solo en modo humano, sin `--fix` y fuera de la sesión del agente; con `--json`, `--fix` o la sesión del agente se escribe todo al final, como antes. Ctrl+C no imprime líneas de cancelación; el tope de 60 s sí imprime sus ✗ como hoy. La salida final es byte a byte la de antes.
- **H13 (d10fc3fb):** `qrStyle` sustituye a `colorOK`: el QR lleva escapes (negro sobre blanco) solo si stderr es una terminal, `TERM != dumb` y, en Windows, se consiguió el modo de terminal virtual (`setup` lo activa al empezar y lo restaura al salir). `NO_COLOR` ya no lo desactiva (el QR es funcional). Con la salida redirigida no hay ningún `\x1b` y solo sale `HelpURL`; el payload del emparejamiento nunca se imprime en texto.
- **H6 (5915abcd):** `Example` en `attach`, `new`, `stop`, `send`, `env` y `version`.

## Goldens regrabados

Seis, solo con el bloque `Examples:` añadido: `host/internal/cli/testdata/help/en/{attach,env,new,send,stop,version}.txt`.

## Rulings

- `message` de USAGE en `--json` en nuestro inglés; el QR ignora `NO_COLOR`; iconos de `doctor` y QR en ASCII aplazados; sin línea de progreso en `doctor` (el streaming ya da el latido).
- `enableVTOK` devuelve ok si el fichero no es una consola de Windows (mintty, pipe): `isTerminal` ya filtra y mintty procesa ANSI.

## Aplazado

- `Long` a <= 80 columnas, prefijos `⚠ `/`⏳ ` de `serve.go`, H14 (`platformtools`, fuera de nuestro árbol).
- Menores de revisión: sleeps de la suite de `doctor_stream_test.go` (~15 s de más); comparación byte a byte solo con el móvil sano; sin test del tope de 60 s en streaming; `TestHelpExamples` valida solo el primer token; sin test del QR de emparejamiento (sin `--code`) redirigido; sin test unitario de `enableVTOK` en Windows; `e == nil` sin comprobar en un test de doble escape.

## Notas honestas

- La traducción de los errores de argumentos y de comando desconocido depende del texto inglés de cobra 1.10.2: si cambia, `translateArgsError` devuelve `nil` y el error queda como antes (hay un test del caso no reconocido). Los errores de flags usan los tipos de pflag.

## Validaciones manuales (ninguna se pide al usuario ahora)

- QR de `setup` en una consola de Windows antigua, en un terminal oscuro con `NO_COLOR=1` y redirigido a un fichero.
- `doctor` con el móvil apagado: las líneas van saliendo una a una.
