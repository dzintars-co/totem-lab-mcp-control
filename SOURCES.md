# Totem Lab MCP Control — sources and dependency versions

Sources and versions for Totem Lab MCP Control. Project documentation: [README.md](README.md). Latvian sources: [SOURCES.lv.md](SOURCES.lv.md).

## Project components

The project code defines its pin allocation, serial protocol, state processing and MCP tools. The official references explain the hardware and software components.

| Component | Purpose |
|---|---|
| [Presentation sketch](presentation/uno-r3-demo-cods.ino) | Annotated Latvian explanation of the firmware with the `types.h` structures included |
| [Arduino sketch](Projects/totem-mcp/firmware/totem_uno/totem_uno.ino), [types.h](Projects/totem-mcp/firmware/totem_uno/types.h) | Actual pins, active-low inputs, inverted common-anode RGB PWM, relay control, serial frames and activity LED |
| [Server entry point](Projects/totem-mcp/cmd/totem-mcp/main.go), [MCP implementation](Projects/totem-mcp/internal/mcp), [serial controller](Projects/totem-mcp/internal/serial), [protocol](Projects/totem-mcp/internal/protocol), [state/events](Projects/totem-mcp/internal/panel) | Server options, tools/resources, USB reconnection, command acknowledgements and bounded event history |
| [Bridge options](Projects/totem-mcp/codex-bridge/main.go), [subscription client](Projects/totem-mcp/codex-bridge/mcp.go), [event processing](Projects/totem-mcp/codex-bridge/events.go) | Subscription/cursor handling, batching, potentiometer threshold and dry-run mode |
| [Codex runner](Projects/totem-mcp/codex-bridge/codex.go), [dashboard](Projects/totem-mcp/codex-bridge/dashboard.go) | CLI execution/resume, reporting instruction, conversation ID and terminal display |
| [Server module](Projects/totem-mcp/go.mod), [server checksums](Projects/totem-mcp/go.sum), [bridge module](Projects/totem-mcp/codex-bridge/go.mod), [bridge checksums](Projects/totem-mcp/codex-bridge/go.sum) | Pinned direct/transitive dependency versions and module checksums required for rebuilding |
| [Panel photo](Pictures/panelis.jpg), [board photo](Pictures/uno%20r3.jpg), [assembled hardware](Pictures/plate.jpg), [dashboard example](Pictures/izvade.png) | Hardware photographs and terminal dashboard example: IO Side Panel v1.7 and TotemDuino v1.9 |
| [SHA256SUMS](SHA256SUMS) | Integrity of the project files |
| [MIT licence](LICENSE), [third-party notices](THIRD_PARTY_NOTICES.md), [Arduino core source and rebuild instructions](third_party/arduino-avr-core/README.md) | Original-code licence, dependency licences and corresponding source for the firmware |
| [CITATION.cff](CITATION.cff) | Scientific software citation metadata |

### Build and dependency versions

- MCP Go SDK: `github.com/modelcontextprotocol/go-sdk v1.8.0` in both modules; the server and bridge use MCP `2026-07-28` subscription APIs.
- Serial: `go.bug.st/serial v1.8.0` in the server.
- Terminal UI: `charm.land/bubbletea/v2 v2.0.10`, `charm.land/lipgloss/v2 v2.0.6`, and `github.com/charmbracelet/x/ansi v0.11.8` in the bridge.
- Module Go declarations: server `1.25.0`; bridge `1.26.0`. Included executables: Go `1.27.2`, Linux/amd64, CGO enabled, built with `-trimpath`. Platform requirements are listed in the README.
- Firmware: Arduino AVR core `1.8.8`, target `arduino:avr:uno`. The sketch needs the Arduino core, with no additional Arduino libraries.
- Firmware toolchain: AVR GCC `7.3.0-atmel3.6.1-arduino7`, avr-libc `2.0.0`; [build-firmware.sh](tools/build-firmware.sh) can rebuild against the included core source.

The Go module files specify the dependency versions. Test files accompany the server and bridge source code.

## Official references

The following official documentation and repositories explain the project’s components. Their online content may change; the local code and module files above identify this implementation.

| Component | Official source | Use in these instructions |
|---|---|---|
| Totem IO Side Panel | [TotemMaker IO panel documentation](https://docs.totemmaker.net/side-panels/io-panel/) | Power and logic selection, switch contacts, potentiometers, encoder, RGB and relay properties |
| TotemDuino | [TotemMaker TotemDuino documentation](https://docs.totemmaker.net/totemduino/) | Uno-compatible board, logic selection and board connections |
| Arduino upload | [Arduino CLI upload reference](https://docs.arduino.cc/arduino-cli/commands-reference/arduino-cli_upload) | FQBN, port, HEX input and upload verification options |
| Arduino compile | [Arduino CLI compile reference](https://docs.arduino.cc/arduino-cli/commands-reference/arduino-cli_compile) | Sketch compilation and explicit build directory |
| Arduino AVR core | [Official core 1.8.8 source](https://github.com/arduino/ArduinoCore-avr/tree/1.8.8) | Corresponding core/Uno variant source and compilation recipe |
| avr-libc | [Official 2.0.0 licence](https://github.com/avrdudes/avr-libc/blob/avr-libc-2_0_0-release/LICENSE) | Firmware C-library licence and copyright notices |
| LGPL | [LGPL-2.1-or-later text](https://spdx.org/licenses/LGPL-2.1-or-later.html) | Arduino library source, modification and relinking rights |
| GCC runtime | [GCC Runtime Library Exception 3.1](https://spdx.org/licenses/GCC-exception-3.1.html) | Runtime code in compiled firmware |
| MCP | [MCP specification 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28) | Protocol reference for the subscription implementation |
| MCP Go SDK | [Official Go SDK repository](https://github.com/modelcontextprotocol/go-sdk) | Go client/server and transport APIs; exact project version is in the included module files |
| Go serial | [go-serial v1.8.0 source](https://github.com/bugst/go-serial/tree/v1.8.0) | USB serial access used by the server |
| Bubble Tea | [Charm Bubble Tea repository](https://github.com/charmbracelet/bubbletea) | Terminal application/event model; version comes from the bridge module |
| Lip Gloss | [Charm Lip Gloss v2.0.6 source](https://github.com/charmbracelet/lipgloss/tree/v2.0.6) | Terminal styling |
| Omarchy Linux | [Official Omarchy website](https://omarchy.org) | Development environment |
| Codex CLI commands | [Official CLI reference](https://learn.chatgpt.com/docs/cli/reference) | CLI login, MCP registration, interactive chat and `--ignore-user-config` |
| Codex authentication | [Official authentication documentation](https://learn.chatgpt.com/docs/auth) | Local CLI credentials, `CODEX_HOME` and login modes |
| Codex non-interactive execution | [Official non-interactive mode documentation](https://learn.chatgpt.com/docs/non-interactive-mode) | `exec`, conversation resume and JSON event output used by the bridge |

The README wiring table follows the project firmware. The pin allocation is specific to this project; manufacturer documentation describes the hardware properties.
