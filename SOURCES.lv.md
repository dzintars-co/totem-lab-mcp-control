# Totem Lab MCP Control — izmantotie avoti un atkarību versijas

Totem Lab MCP Control avoti un versijas. Projekta dokumentācija: [README.lv.md](README.lv.md). Avoti angliski: [SOURCES.md](SOURCES.md).

## Projekta sastāvdaļas

Projekta kods nosaka izvadu sadalījumu, seriālo protokolu, stāvokļu apstrādi un MCP rīkus. Oficiālie avoti skaidro aparatūras un programmatūras sastāvdaļas.

| Sastāvdaļa | Uzdevums |
|---|---|
| [Prezentācijas programma](presentation/uno-r3-demo-cods.ino) | Latviski komentēts firmware skaidrojums ar vienā failā ietvertām `types.h` struktūrām |
| [Arduino programma](Projects/totem-mcp/firmware/totem_uno/totem_uno.ino), [types.h](Projects/totem-mcp/firmware/totem_uno/types.h) | Izvadi, aktīvas LOW ieejas, kopējā anoda RGB invertētā PWM vadība, relejs, seriālās ziņas un datu LED |
| [Servera ieejas punkts](Projects/totem-mcp/cmd/totem-mcp/main.go), [MCP realizācija](Projects/totem-mcp/internal/mcp), [seriālais kontrolieris](Projects/totem-mcp/internal/serial), [protokols](Projects/totem-mcp/internal/protocol), [stāvokļi/notikumi](Projects/totem-mcp/internal/panel) | Servera parametri, rīki/resursi, USB atjaunošana, komandu apstiprinājumi un ierobežota vēsture |
| [Bridge parametri](Projects/totem-mcp/codex-bridge/main.go), [abonēšanas klients](Projects/totem-mcp/codex-bridge/mcp.go), [notikumu apstrāde](Projects/totem-mcp/codex-bridge/events.go) | Abonēšana/kursori, grupēšana, potenciometru slieksnis un dry-run režīms |
| [Codex palaišana](Projects/totem-mcp/codex-bridge/codex.go), [termināļa panelis](Projects/totem-mcp/codex-bridge/dashboard.go) | CLI palaišana/sarunas turpināšana, ziņošanas instrukcija, sarunas ID un attēlošana |
| [Servera modulis](Projects/totem-mcp/go.mod), [servera kontrolsummas](Projects/totem-mcp/go.sum), [bridge modulis](Projects/totem-mcp/codex-bridge/go.mod), [bridge kontrolsummas](Projects/totem-mcp/codex-bridge/go.sum) | Pārbūvei vajadzīgo tiešo/netiešo atkarību versijas un moduļu kontrolsummas |
| [Paneļa foto](Pictures/panelis.jpg), [plates foto](Pictures/uno%20r3.jpg), [saslēgtā aparatūra](Pictures/plate.jpg), [paneļa izvades piemērs](Pictures/izvade.png) | Aparatūras foto un termināļa paneļa piemērs: IO Side Panel v1.7 un TotemDuino v1.9 |
| [SHA256SUMS](SHA256SUMS) | Projekta failu integritāte |
| [MIT licence](LICENSE), [trešo pušu norādes](THIRD_PARTY_NOTICES.md), [Arduino kodola pirmkods un pārbūves instrukcija](third_party/arduino-avr-core/README.md) | Oriģinālā koda licence, atkarību licences un firmware atbilstošais pirmkods |
| [CITATION.cff](CITATION.cff) | Programmatūras citēšanas dati zinātniskam darbam |

### Būves un atkarību versijas

- MCP Go SDK: `github.com/modelcontextprotocol/go-sdk v1.8.0` abos moduļos; serveris/bridge izmanto MCP `2026-07-28` abonēšanas API.
- Seriālā saziņa: `go.bug.st/serial v1.8.0` serverī.
- Termināļa panelis: `charm.land/bubbletea/v2 v2.0.10`, `charm.land/lipgloss/v2 v2.0.6` un `github.com/charmbracelet/x/ansi v0.11.8` bridge modulī.
- Go versiju deklarācijas moduļos: serverim `1.25.0`, bridge `1.26.0`. Gatavie izpildfaili: Go `1.27.2`, Linux/amd64, ieslēgts CGO un būves parametrs `-trimpath`. Platformas prasības aprakstītas README.
- Firmware: Arduino AVR core `1.8.8`, mērķis `arduino:avr:uno`. Vajadzīgs Arduino core, bez papildu Arduino bibliotēkām.
- Firmware rīku kopa: AVR GCC `7.3.0-atmel3.6.1-arduino7`, avr-libc `2.0.0`; [build-firmware.sh](tools/build-firmware.sh) ļauj pārkompilēt ar komplektā esošā kodola pirmkodu.

Go moduļu faili nosaka atkarību versijas. Testu faili atrodas kopā ar servera un bridge pirmkodu.

## Oficiālie avoti

Oficiālā dokumentācija un repozitoriji skaidro projekta sastāvdaļas. Tie var mainīties; šo realizāciju fiksē iepriekš norādītais vietējais kods un moduļu faili.

| Sastāvdaļa | Oficiālais avots | Kam izmantots |
|---|---|---|
| Totem IO Side Panel | [TotemMaker IO paneļa dokumentācija](https://docs.totemmaker.net/side-panels/io-panel/) | Barošana/loģikas izvēle, slēdžu kontakti, potenciometri, enkoders, RGB un releja īpašības |
| TotemDuino | [TotemMaker TotemDuino dokumentācija](https://docs.totemmaker.net/totemduino/) | Ar Uno saderīgā plate, loģikas izvēle un pieslēgumi |
| Arduino iešūšana | [Arduino CLI upload apraksts](https://docs.arduino.cc/arduino-cli/commands-reference/arduino-cli_upload) | FQBN, ports, HEX un iešūšanas pārbaudes parametri |
| Arduino kompilēšana | [Arduino CLI compile apraksts](https://docs.arduino.cc/arduino-cli/commands-reference/arduino-cli_compile) | Arduino skices kompilēšana un norādīta būves mape |
| Arduino AVR kodols | [Oficiālais kodola 1.8.8 pirmkods](https://github.com/arduino/ArduinoCore-avr/tree/1.8.8) | Atbilstošais kodola/Uno varianta pirmkods un kompilēšanas recepte |
| avr-libc | [Oficiālā 2.0.0 licence](https://github.com/avrdudes/avr-libc/blob/avr-libc-2_0_0-release/LICENSE) | Firmware C bibliotēkas licence un autoru norādes |
| LGPL | [LGPL-2.1-or-later teksts](https://spdx.org/licenses/LGPL-2.1-or-later.html) | Arduino bibliotēkas pirmkods, pārveidošanas un atkārtotas sasaistīšanas tiesības |
| GCC izpildlaiks | [GCC Runtime Library Exception 3.1](https://spdx.org/licenses/GCC-exception-3.1.html) | Kompilētajā firmware ietvertais izpildlaika kods |
| MCP | [MCP 2026-07-28 specifikācija](https://modelcontextprotocol.io/specification/2026-07-28) | Abonēšanas realizācijas protokola avots |
| MCP Go SDK | [Oficiālais Go SDK repozitorijs](https://github.com/modelcontextprotocol/go-sdk) | Go klienta/servera un transporta API; precīzā projekta versija ir moduļu failos |
| Go serial | [go-serial v1.8.0 pirmkods](https://github.com/bugst/go-serial/tree/v1.8.0) | Servera USB seriālā saziņa |
| Bubble Tea | [Charm Bubble Tea repozitorijs](https://github.com/charmbracelet/bubbletea) | Termināļa lietotnes/notikumu modelis; versija fiksēta bridge modulī |
| Lip Gloss | [Charm Lip Gloss v2.0.6 pirmkods](https://github.com/charmbracelet/lipgloss/tree/v2.0.6) | Termināļa noformējums |
| Omarchy Linux | [Oficiālā Omarchy vietne](https://omarchy.org) | Izstrādes vide |
| Codex CLI komandas | [Oficiālais CLI apraksts](https://learn.chatgpt.com/docs/cli/reference) | CLI autorizācija, MCP reģistrācija, interaktīvs čats un `--ignore-user-config` |
| Codex autentifikācija | [Oficiālā autentifikācijas dokumentācija](https://learn.chatgpt.com/docs/auth) | Lokālie CLI piekļuves dati, `CODEX_HOME` un pieteikšanās režīmi |
| Codex neinteraktīvā palaišana | [Oficiālais neinteraktīvā režīma apraksts](https://learn.chatgpt.com/docs/non-interactive-mode) | Bridge izmantotie `exec`, sarunas turpināšana un JSON notikumu izvade |

README pieslēgumu tabula atbilst plates programmai. Izvadu sadalījums ir šī projekta izvēle; ražotāja dokumentācija apraksta aparatūras īpašības.
