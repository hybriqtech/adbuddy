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

## Segunda pasada: crecimiento del lado host con el emutest completo (test/guard-samples 612d8f3)

AVD 1080x2400/420, hw.ramSize 2560 (efectivo), 4 núcleos, swiftshader_indirect, android-34, muestreo cada 30 s (`curvas/*.samples`: segundos, WS MB, Priv MB). Un solo emulador en el PC, ningún job de CI concurrente.

| Run | Tests | WS estable | WS pico | Priv pico | WS final | Duración tests |
| --- | --- | --- | --- | --- | --- | --- |
| leakfull | emutest completo (50 pass, 1 skip) | 3643 MB | 6817 MB | 7947 MB | 7261 MB | 574 s |
| leakNoGuard | sin `TestTheGuard*` (37 pass, 1 skip) | 3650 MB | 5018 MB | 5763 MB | 5455 MB | 487 s |

- El WS **solo crece y nunca devuelve memoria**: +3,2 GB en 10 min con las pruebas del guard (T5), +1,4 GB sin ellas. Las pruebas del guard (pantallas reales, capturas) cuestan ~2 GB más de crecimiento.
- El job de CI que llegó a 14-15 GB de WS (run 37105144718, 30+ min, con el oráculo 37 antes) es esta misma curva alargada: el emulador no libera.
- Con un qemu concurrente (job de CI) y 2 núcleos, 20 tests fallaron por contención: medir siempre con un emulador.
- Pendiente: GPU `guest` vs swiftshader, RAM del guest 1536 MB plana, reiniciar el emulador entre grupos de tests.
