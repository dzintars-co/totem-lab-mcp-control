# Arduino AVR core 1.8.8

## English

The included firmware HEX links the Arduino AVR core, licensed under **LGPL-2.1-or-later**. The unmodified core source and Uno `standard` variant are in [1.8.8](1.8.8/); the original build recipe is [platform.txt](1.8.8/platform.txt). Individual files retain their upstream copyright and licence notices, including ISC-licensed USB files. The full LGPL text is [LICENSE](1.8.8/LICENSE). Upstream source: [ArduinoCore-avr 1.8.8](https://github.com/arduino/ArduinoCore-avr/tree/1.8.8).

The project's own sketch is [totem_uno.ino](../../Projects/totem-mcp/firmware/totem_uno/totem_uno.ino), with [types.h](../../Projects/totem-mcp/firmware/totem_uno/types.h). Together with the core source, these files allow the firmware to be rebuilt and relinked with a modified core. Modification for personal use and reverse engineering to debug such modifications are permitted. Keep the corresponding source and licence notices available with the firmware when redistributing it.

From the project root, with an installed AVR GCC toolchain:

```bash
bash tools/build-firmware.sh
```

The script targets ATmega328P, 16 MHz and Uno, using the bundled core. It uses Arduino's installed AVR GCC `7.3.0-atmel3.6.1-arduino7` if available, otherwise tools on `PATH`. Set `TOTEM_AVR_TOOLCHAIN` to another installed toolchain directory if necessary. Build outputs go to a new `.cache/firmware-build.*` directory. The script prints the resulting HEX path and does not install tools, replace the supplied HEX or upload to the board. Flash use must fit the Uno bootloader's 32,256-byte limit; SRAM must fit 2,048 bytes with room for the stack.

The toolchain supplies avr-libc 2.0.0 (Modified BSD) and GCC runtime code covered by the GCC Runtime Library Exception 3.1. Their notices are in [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md). The compiler and complete toolchain are external requirements.

## Latviski

Komplektā esošais firmware HEX ir saistīts ar Arduino AVR kodolu, kura licence ir **LGPL-2.1-or-later**. Nemainīts kodola pirmkods un Uno `standard` variants atrodas [1.8.8](1.8.8/) mapē; sākotnējā kompilēšanas recepte ir [platform.txt](1.8.8/platform.txt). Katrā failā saglabātas sākotnējās autoru un licenču norādes, arī ISC licencētajos USB failos. Pilns LGPL teksts ir [LICENSE](1.8.8/LICENSE). Sākotnējais avots: [ArduinoCore-avr 1.8.8](https://github.com/arduino/ArduinoCore-avr/tree/1.8.8).

Projekta Arduino programma ir [totem_uno.ino](../../Projects/totem-mcp/firmware/totem_uno/totem_uno.ino) ar [types.h](../../Projects/totem-mcp/firmware/totem_uno/types.h). Šie faili kopā ar kodola pirmkodu ļauj pārkompilēt firmware un sasaistīt to ar pārveidotu kodolu. Atļauta pārveidošana personiskai lietošanai un reversā inženierija šo izmaiņu atkļūdošanai. Izplatot firmware, nodrošini arī atbilstošā pirmkoda un licenču norāžu pieejamību.

No projekta saknes, ja AVR GCC kompilēšanas rīki jau ir uzstādīti:

```bash
bash tools/build-firmware.sh
```

Skripts izmanto komplektā esošo kodolu un veido programmu ATmega328P, 16 MHz un Uno konfigurācijai. Tas izmanto Arduino uzstādīto AVR GCC `7.3.0-atmel3.6.1-arduino7`, ja tas ir pieejams; citādi meklē rīkus `PATH`. Vajadzības gadījumā norādi citas uzstādītas rīku kopas mapi ar `TOTEM_AVR_TOOLCHAIN`. Rezultāti nonāk jaunā `.cache/firmware-build.*` mapē. Skripts parāda izveidotā HEX ceļu, neko neinstalē, neaizstāj komplektā esošo HEX un neiešuj plati. Flash apjomam jāiekļaujas Uno bootloadera 32 256 baitu robežā; SRAM — 2048 baitos, atstājot vietu stekam.

Rīku kopā ietilpst avr-libc 2.0.0 (Modified BSD) un GCC izpildlaika kods ar GCC Runtime Library Exception 3.1 nosacījumiem. Norādes ir [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md). Pats kompilators un pilnā rīku kopa ir ārējas prasības.
