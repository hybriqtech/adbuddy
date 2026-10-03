# Medidas de RAM del emulador (android-34, Windows, qemu-system-x86_64-headless)

Encargo: menos RAM con emuladores funcionales. AVD propio `adbuddy-puffin-*`, consola 5620, adb 5080, `ADBUDDY_EMU_STABLE_SECS=120`, `-no-snapshot`, partition 2047. Medido con Get-Process del qemu de la consola 5620 (WS = working set, Priv = memoria privada). Carga: `TestInstrumentedSuite` (corta, 8-18 s: sin pico apreciable).

| Variante | Pantalla/dpi | RAM pedida | RAM efectiva (hardware-qemu.ini) | Núcleos | GPU | Arranque | WS estable | Priv estable | WS tras carga |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| base | 1080x2400/420 | 2G | 2560 | 4 | swiftshader_indirect | 157 s | 3642 MB | 4227 MB | 3647 MB |
| small | 720x1600/320 | 2G | 2560 | 4 | swiftshader_indirect | 206 s | 3438 MB | 4093 MB | 3444 MB |
| small15 | 720x1600/320 | 1536M | 2560 (el emulador ignoró "1536M") | 2 | swiftshader_indirect | 185 s | 3655 MB | 4165 MB | 3655 MB |
| small15guest | 720x1600/320 | 1536M | 2560 | 2 | guest | 177 s | 3455 MB | 3959 MB | 3454 MB |
| small15host | — | — | — | — | host | no medida (pausa pedida por el supervisor) | | | |

Hallazgos:
- **Resolución:** 720x1600/320 ahorra ~200 MB de WS (3642 -> 3438) frente a 1080x2400.
- **Núcleos (4 -> 2) y GPU guest:** efecto pequeño o nulo (±200 MB, dentro del ruido).
- **hw.ramSize:** `2G` se convierte en 2560 MB efectivos y `1536M` es ignorado (también 2560). Para bajar la RAM del guest hay que escribir MB en número plano ("1536") y comprobar `hardware-qemu.ini`. Es la palanca grande: WS ~ RAM del guest + ~1 GB.
- **Crecimiento:** un emulador de tern con sesión larga llegó a 5,4-5,7 GB de WS; la carga corta no lo reproduce. Falta medir con el `emutest` completo (7-10 min).
- **Tests y resolución:** `confirm_test` usaba 540,1800 absolutos para `unlock`: a 720x1600 puede fallar (skimmer lo haría relativo).

Pendiente (segunda pasada, `scratchpad/runall2.sh` equivalente): c2560 (1080x2400, 2560, 2 núcleos), a1536 (1080x2400, 1536, 2 núcleos), b1536small (720x1600, 1536), con `emutest` completo (`-run .`) como carga y RAM en MB planos; luego la GPU `host` en 34.
