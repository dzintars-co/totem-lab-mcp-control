# Totem Lab MCP Control

**Author: XeS. AI coding assistant: ExE (Codex).** Powered by [Omarchy Linux](https://omarchy.org).

Totem Lab MCP Control connects a physical Totem MiniLab panel to a computer through Arduino, a Go MCP server and Codex CLI. It reads switches, buttons, potentiometers and the encoder, displays changes in a terminal dashboard, and controls the panel’s RGB LED and relay through MCP tools.

Assembled Totem MiniLab IO Side Panel and TotemDuino board:

![Assembled Totem MiniLab panel and TotemDuino board](Pictures/plate.jpg)

[![CI](https://github.com/dzintars-co/totem-lab-mcp-control/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/dzintars-co/totem-lab-mcp-control/actions/workflows/go-ci.yml)

The project includes Arduino firmware, the Go MCP server, the Go/Codex terminal dashboard, source code, tests and ready-to-run outputs. Latvian documentation: [README.lv.md](README.lv.md). References: [SOURCES.md](SOURCES.md).

Version **1.0.0**. Release history: [CHANGELOG.md](CHANGELOG.md). The v1 interface comprises the documented MCP tools/resources, command-line options and serial protocol v1; incompatible changes to these interfaces require a new major version.

The sections below describe the architecture, wiring and startup commands. All startup paths are relative to the project root, **totem-lab-mcp-control**.

## Quick start

On compatible Linux x86-64 with this firmware already on the board, connect USB. Open two terminals in the project root and run one block in each; replace `/dev/ttyUSB0` with the actual port.

```bash
# Terminal 1 — MCP server
unset TOTEM_MCP_TOKEN
./Projects/totem-mcp/bin/totem-mcp -port /dev/ttyUSB0 -baud 115200 -listen 127.0.0.1:8080
```

```bash
# Terminal 2 — live dashboard without AI
./Projects/totem-mcp/codex-bridge/bin/totem-codex-bridge -ui -dry-run
```

The dashboard shows input events without a Codex login. Codex reporting and manual chat are described in section 5 below; requirements and firmware setup are in sections 3 and 7.

Terminal dashboard with panel values and Codex event reports:

![Terminal dashboard and Codex event reports](Pictures/izvade.png)

Live input values, RGB and relay commands, and MCP subscription status:

![Live panel values, output commands and MCP subscription](Pictures/dashboard-live-events.png)

## 1. Contents and code map

```text
totem-lab-mcp-control/
├── README.md / README.lv.md             Project documentation EN/LV
├── SOURCES.md / SOURCES.lv.md           Sources and dependency versions EN/LV
├── LICENSE / THIRD_PARTY_NOTICES.md     MIT and third-party licence notices
├── CITATION.cff                        Software citation metadata
├── SHA256SUMS                          Integrity checks
├── .gitignore                          Local/private/generated file exclusions
├── .gitattributes                      Preserve file bytes across platforms
├── tools/build-firmware.sh             Rebuild firmware with the bundled core
├── third_party/                        Arduino core source and required licences
├── presentation/
│   └── uno-r3-demo-cods.ino             Annotated Latvian presentation sketch
├── Pictures/
│   ├── panelis.jpg                     IO Side Panel
│   ├── uno r3.jpg                      TotemDuino board
│   ├── plate.jpg                       Assembled hardware
│   ├── izvade.png                      Terminal dashboard example
│   └── dashboard-live-events.png       Live values, output commands and subscription
└── Projects/totem-mcp/
    ├── go.mod / go.sum                  MCP server module and dependency checksums
    ├── cmd/totem-mcp/main.go            Server entry point
    ├── internal/                       MCP, panel state, serial protocol/controller, tests
    ├── bin/totem-mcp                    Ready Linux server
    ├── firmware/totem_uno/
    │   ├── totem_uno.ino                Arduino C++ sketch
    │   ├── types.h                     Required sketch types
    │   └── totem_uno.ino.hex            Ready sketch HEX for Uno upload
    └── codex-bridge/
        ├── go.mod / go.sum              Independent bridge module
        ├── main.go / mcp.go / events.go Bridge options, subscriptions and event processing
        ├── codex.go / dashboard.go      Codex execution and terminal UI
        ├── *_test.go                    Tests for these components
        └── bin/totem-codex-bridge        Ready Linux dashboard/bridge
```

| Start reading here | Purpose |
|---|---|
| [Presentation sketch](presentation/uno-r3-demo-cods.ino) | Extensive Latvian explanations of the same firmware, with types included; for reading/presentation, not upload |
| [Arduino sketch](Projects/totem-mcp/firmware/totem_uno/totem_uno.ino) and [types.h](Projects/totem-mcp/firmware/totem_uno/types.h) | Pin definitions, input debouncing, ADC/encoder readings, output commands and serial frames |
| [Server main.go](Projects/totem-mcp/cmd/totem-mcp/main.go) | Options, HTTP listener, serial-controller lifetime |
| [internal/protocol](Projects/totem-mcp/internal/protocol) | Firmware command/frame encoding and parsing |
| [internal/serial](Projects/totem-mcp/internal/serial) | USB connection/reconnection and acknowledgements |
| [internal/panel](Projects/totem-mcp/internal/panel) | Valid state and bounded event history |
| [internal/mcp](Projects/totem-mcp/internal/mcp) | MCP tools, resources and HTTP transport |
| [Bridge main.go](Projects/totem-mcp/codex-bridge/main.go), [mcp.go](Projects/totem-mcp/codex-bridge/mcp.go), [events.go](Projects/totem-mcp/codex-bridge/events.go) | Subscriptions, cursors, thresholds, batching and event queue |
| [codex.go](Projects/totem-mcp/codex-bridge/codex.go) | CLI subprocess, reporting instruction, conversation ID and answers |
| [dashboard.go](Projects/totem-mcp/codex-bridge/dashboard.go) | Bubble Tea/Lip Gloss dashboard |

The presentation sketch explains the firmware in Latvian and includes the `types.h` structures in one file. For upload, use the sketch or HEX file in `Projects/totem-mcp/firmware/totem_uno/`.

Each Go module records its dependencies in `go.mod` and `go.sum`. Tests support checking changes when rebuilding the programs.

## 2. Hardware and direction of communication

Hardware: **Totem MiniLab IO Side Panel v1.7** and **TotemDuino v1.9**, an Arduino Uno R3-compatible ATmega328P board, connected to the computer by USB.

**Panel → user:** a switch/button/potentiometer/encoder changes → firmware sends a serial state → Go server records it → MCP subscription notifies the bridge → dashboard refreshes. AI mode additionally asks Codex CLI to describe the event in the dashboard.

**User → panel:** an interactive Codex chat calls `set_rgb` or `set_relay` → Go server sends the command over USB → firmware applies and acknowledges it. `get_panel_state` returns the latest firmware-confirmed state. RGB/relay values describe applied controls, not independent measurements of light or relay contacts.

The dashboard is its own program. Its Codex answers appear there, not automatically in an already open desktop/interactive chat. The default event worker reports in Latvian and does not control outputs. A separate interactive chat provides manual controls.

The bridge uses MCP `2026-07-28` subscriptions through the official MCP Go SDK. In `mcp.go`, a subscription notification triggers reading the new events from `panel://events`; `events.go` preserves their order and filters changes for reporting. In AI mode, `codex.go` passes those events to Codex CLI. The subscription does not require a Codex account and does not periodically poll the panel. AI replies additionally take batching, subprocess and model time.

The server keeps a bounded event history in memory. A new bridge process starts from the latest state. After a connection interruption, the same process recovers retained events; if history is missing or the server has restarted, it reports resynchronization rather than inventing button presses. This technical subscription is separate from a paid ChatGPT subscription.

The bridge monitors the actual HTTP/SSE subscription stream. If it ends or a connection attempt fails, the dashboard shows `DISCONNECTED` and the MCP error. Previously received values remain visible as **last reading — not current**. The bridge retries after two seconds. `CONNECTED` requires an active subscription, `valid=true` and hardware `status=connected`; a cached reading alone cannot restore it.

The server checks firmware responsiveness after five seconds without a fresh serial reading, using a read-only `GET` command with its own acknowledgement ID. A missing reply closes the serial connection, marks retained state invalid and triggers reconnection. The default acknowledgement timeout is two seconds (`-command-timeout`). These checks do not change outputs or create input events when values are unchanged; physical input changes still arrive as firmware events through the MCP subscription.

## 3. Requirements

- The included Go executables require **Linux x86-64, glibc 2.34 or newer**, loader `/lib64/ld-linux-x86-64.so.2`, `libc` and `libresolv`. They do not directly run on Windows/macOS/ARM/musl; build natively if required.
- USB serial-device access and the matching firmware on the board. Close other applications using that port.
- A terminal; about 110 columns allows the dashboard's two columns to appear side by side. Narrower windows stack them vertically. The view fits the window height and supports scrolling.
- For AI only: an installed/authenticated **Codex CLI**, internet/account availability and support for `exec`, `exec resume`, `--json`, `--ignore-user-config`. `codex` must be on `PATH` or supplied using `-codex`.
- For rebuilding/uploading only: a suitable Go toolchain/C compiler and Arduino CLI/IDE with AVR tools. The included Go programs are built with **Go 1.27.2**; module declarations are Go 1.25.0 for the server and 1.26.0 for the bridge. Firmware uses **Arduino AVR core 1.8.8**, target `arduino:avr:uno`.

The development environment is Omarchy Linux. Runtime compatibility follows the Linux requirements above. Running compatible binaries does not require Go; the dashboard’s `-dry-run` mode does not require Codex. Install the external tools required for your chosen mode separately. All shell examples use Linux Bash.

## 4. Wiring

[Panel photograph](Pictures/panelis.jpg), [board photograph](Pictures/uno%20r3.jpg) and [assembled hardware](Pictures/plate.jpg). Orient the panel with its component side toward you, **POWER IN at the top and the 3.5 mm jack at the bottom**; A/B/C run left to right. The panel photograph itself is sideways. Use board labels D/A/3V3/5V/GND rather than numbered positions of its large connector.

Disconnect power before wiring. Select 5 V logic on the board and panel `OBS! Select VCC` jumper at `5v`. Keep both VCC/GND jumpers on each potentiometer. Connect board `3V3` to panel `+3.3v`, `5V` to `+5v`, and common GND. Use a connected five-hole breadboard group for board GND, panel POWER IN GND and the three switch ground wires; the group across the central gap is separate.

| Element | Panel contact in the orientation above | Board pin |
|---|---|---|
| Switch A / B / C | Middle socket above each indicator | D7 / D8 / D10 |
| Switch grounds | Left socket of each switch header | Common GND; right sockets unused |
| Pot A / B / C | Middle socket above each knob | A0 / A1 / A2 |
| Button A / B / C | Left / middle / right socket of shared header above button B | D11 / D12 / A3 |
| Encoder button | Socket marked with button symbol | D4 |
| Encoder A / B | Sockets marked ROTATE-A / ROTATE-B | D2 / D3 |
| RGB R / G / B | Match the panel's R/G/B labels | D5 / D6 / D9 |
| Relay control | Signal socket beside RELAY ON +3–5v=ON | A4 |
| Board activity LED | On-board LED; no extra wire | D13 |

A/B/C correspond to data fields 1/2/3. Buttons/switches are active-low; firmware maps pressed to 1. Encoder inputs have panel pull-ups. RGB has a common anode, so firmware inverts PWM internally; send normal `#RRGGBB` values. Relay is active-high; both control-header contacts carry the same signal, so the second is not GND. D0/D1 are reserved for serial communication. Leave relay load terminals unused for the initial control test. The described wiring uses 23 wires: five GND, two power and sixteen signals.

## 5. Start after switching on the computer

The board does not need reflashing after a computer restart if the correct sketch is already installed. Connect USB and close Arduino Serial Monitor. Find the current device:

```bash
ls -l /dev/serial/by-id/
ls -l /dev/ttyUSB0
```

Replace `/dev/ttyUSB0` in subsequent commands with the actual device or stable by-id path. Resolve permissions according to your operating system’s policy.

### Terminal 1 — server

Open in **totem-lab-mcp-control**:

```bash
cd Projects/totem-mcp
unset TOTEM_MCP_TOKEN
./bin/totem-mcp -port /dev/ttyUSB0 -baud 115200 -listen 127.0.0.1:8080
```

Keep it running. The startup command binds to the local computer without a bearer token; the bridge has no bearer configuration. The address is **http://127.0.0.1:8080/mcp**. Confirm `valid=true` and `status=connected` in MCP state data, or `CONNECTED` in the dashboard header. When disconnected, any retained values describe the last reading. Do not start a second process against the same serial/TCP port.

### Terminal 2 — dashboard

Open another terminal in **totem-lab-mcp-control**. First verify without AI:

```bash
cd Projects/totem-mcp/codex-bridge
./bin/totem-codex-bridge -ui -dry-run
```

Change physical inputs and check the display. Exit with `q`, `Esc` or `Ctrl+C`; the server remains running. To enable Codex reporting, in the same directory:

```bash
codex login status
```

If not authenticated, run `codex login`, then:

```bash
./bin/totem-codex-bridge -ui
```

New events produce Codex answers in the dashboard. Manual chat is separate. If needed, add `-codex /actual/absolute/path/to/codex`.

Scroll with `↑`/`↓` or `k`/`j`, `PgUp`/`PgDn`, `Home`/`End`, or the mouse wheel. The connection header and navigation hints remain visible while scrolling. Resizing the terminal adjusts the layout and available scroll range.

#### Codex authentication and conversation

`codex.go` launches the installed CLI and inherits the terminal's environment. AI mode uses that CLI's local authentication: a ChatGPT login or an API-key login configured through Codex. The project does not select a particular account. Codex manages its credentials in its own storage, normally under `~/.codex` or in the operating system's credential store; `CODEX_HOME` selects the Codex home directory.

Each user authenticates their own local Codex CLI. On a computer where Codex is already signed in, AI mode uses that computer's current login. The MCP server and the dashboard's `-dry-run` mode require no OpenAI account or paid subscription.

The reporting worker uses `--ignore-user-config` to skip `$CODEX_HOME/config.toml`. **This flag does not disable authentication.** `codex login status` shows the login status/mode. MCP event subscriptions and `-dry-run` work without a Codex login. See the official [authentication documentation](https://learn.chatgpt.com/docs/auth) and [CLI reference](https://learn.chatgpt.com/docs/cli/reference).

On the first reported event, the bridge starts a new conversation and saves its ID in `thread-id.txt` in the launch directory. Later events and restarts resume that conversation. This file is a conversation identifier, not an access credential; it is generated locally and excluded by `.gitignore`. Use separate ID files for simultaneous AI processes or after changing CLI authentication or `CODEX_HOME`. To start a new conversation, choose a filename that does not already exist, for example:

```bash
./bin/totem-codex-bridge -ui -thread-file thread-id-new.txt
```

### Terminal 3 — optional manual chat

Open in **totem-lab-mcp-control**:

```bash
cd Projects/totem-mcp
codex mcp list
```

If there is no `totem` entry, register the local server:

```bash
codex mcp add totem --url http://127.0.0.1:8080/mcp
```

Check an existing entry rather than creating a duplicate. Registration updates the local Codex configuration. Start a new interactive chat:

```bash
codex -C "$PWD"
```

Example requests:

```text
Use the totem MCP tool to set RGB to #FF0000.
Use the totem MCP tool to turn the relay on.
Use the totem MCP tools to set RGB to #000000 and turn the relay off.
```

Keep the server running. The dashboard is optional for control, but shows applied output changes if open. The MCP URL is a protocol endpoint, not a browser dashboard; a browser GET/session error is not the right way to inspect it.

## 6. Event and connection checks

In another terminal opened in **totem-lab-mcp-control**:

```bash
cd Projects/totem-mcp/codex-bridge
timeout 5s ./bin/totem-codex-bridge -dry-run
```

Expect `MCP abonēts: stream=... event=...`. This opens a new subscription rather than inspecting another process's subscription. Timeout normally exits with code 124. From the same bridge directory, inspect all new events without the bridge's extra potentiometer threshold:

```bash
./bin/totem-codex-bridge -dry-run -pot-step 0
```

Firmware sampling/debounce thresholds still apply; stop with `Ctrl+C`. No AI calls or output commands are made. Additional checks, when the utilities are installed:

```bash
pgrep -af 'totem-mcp|totem-codex-bridge'
fuser --verbose /dev/ttyUSB0
ss --tcp --listening --numeric --process 'sport = :8080'
arduino-cli board list
codex mcp list
./bin/totem-codex-bridge -h
```

## 7. Upload only if needed

Stop bridge/server and close Serial Monitor first. The C++ sketch uses the standard Arduino AVR core with no extra Arduino libraries. If the required core is missing, install it:

```bash
arduino-cli core update-index
arduino-cli core install arduino:avr@1.8.8
```

From a terminal in **totem-lab-mcp-control**, set the actual port and upload the included firmware HEX:

```bash
cd Projects/totem-mcp
export TOTEM_PROJECT_DIR="$PWD"
export TOTEM_SERIAL_PORT='/dev/ttyUSB0'
arduino-cli upload --fqbn arduino:avr:uno --port "$TOTEM_SERIAL_PORT" \
  --input-file "$TOTEM_PROJECT_DIR/firmware/totem_uno/totem_uno.ino.hex" \
  --verify "$TOTEM_PROJECT_DIR/firmware/totem_uno"
```

This still needs installed CLI/AVR upload tools and the board's existing Uno bootloader. To rebuild first, from the same directory/environment:

```bash
arduino-cli compile --fqbn arduino:avr:uno \
  --build-path "$TOTEM_PROJECT_DIR/.cache/arduino-build" \
  "$TOTEM_PROJECT_DIR/firmware/totem_uno"
arduino-cli upload --fqbn arduino:avr:uno --port "$TOTEM_SERIAL_PORT" \
  --input-dir "$TOTEM_PROJECT_DIR/.cache/arduino-build" \
  --verify "$TOTEM_PROJECT_DIR/firmware/totem_uno"
```

Require successful compilation and adequate Flash/SRAM headroom; keep `types.h` beside the sketch. After upload restart server and dashboard.

The firmware links the **Arduino AVR core under LGPL-2.1-or-later**. Its corresponding source is included in `third_party/arduino-avr-core/1.8.8`. To rebuild and relink with that core, including your modifications, run from the project root with an installed AVR GCC toolchain:

```bash
bash tools/build-firmware.sh
```

This writes a new HEX under `.cache/firmware-build.*` and prints its path; it does not upload or replace the included firmware. Use the printed path as `--input-file` in the upload command. Toolchain selection, memory limits and licence details: [bundled core instructions](third_party/arduino-avr-core/README.md).

## 8. Rebuild Go programs if needed

From a terminal in **totem-lab-mcp-control**. These commands download the pinned dependencies and require network access:

```bash
cd Projects/totem-mcp
export TOTEM_PROJECT_DIR="$PWD"
mkdir -p .cache/go-build .cache/go-mod .cache/go-path .cache/tmp bin
export GOTOOLCHAIN=local
export GOCACHE="$TOTEM_PROJECT_DIR/.cache/go-build"
export GOMODCACHE="$TOTEM_PROJECT_DIR/.cache/go-mod"
export GOPATH="$TOTEM_PROJECT_DIR/.cache/go-path"
export GOTMPDIR="$TOTEM_PROJECT_DIR/.cache/tmp"
export TMPDIR="$TOTEM_PROJECT_DIR/.cache/tmp"
go mod download
go test -race ./...
go vet ./...
go build -trimpath -mod=readonly -buildvcs=false -o bin/totem-mcp ./cmd/totem-mcp
```

Then build the separate bridge module in the same terminal:

```bash
cd "$TOTEM_PROJECT_DIR/codex-bridge"
mkdir -p bin
go mod download
go test -race ./...
go vet ./...
go build -trimpath -mod=readonly -buildvcs=false -o bin/totem-codex-bridge .
```

Race tests require a supported platform and C compiler; use `go test ./...` if race support is unavailable. Tests use simulated serial devices, a local test MCP server and a simulated Codex subprocess; they do not call a real AI service or operate the board. Native Windows builds need `.exe` filenames, an actual COM port and equivalent shell commands. Restart a running process after replacing its executable.

## 9. Troubleshooting and shutdown

- Missing serial port: find the current device and check USB connection and serial-device permissions.
- Busy serial port: close Serial Monitor and any other process using the device. Run only one Totem MCP server against it.
- No panel updates: confirm `status=connected` and `valid=true`, then use `-dry-run` to view new events.
- No Codex answers: first confirm events in `-dry-run`, then check `codex login status`, CLI availability and account access.

If outputs must be off, send RGB `#000000` and relay off commands while the server is running. Process shutdown does not itself turn outputs off. Exit the dashboard and chat, stop the server with `Ctrl+C`, then disconnect USB if desired.

### File checksums

From the project root:

```bash
sha256sum -c SHA256SUMS
```

This compares the project files with `SHA256SUMS`. Editing or rebuilding a file changes its checksum.

## 10. Licence and citation

The project's original code and documentation use the [MIT licence](LICENSE), copyright XeS. ExE (Codex) is credited as the AI coding assistant. Dependencies keep their own licences; their copyright notices and full licence texts are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and `third_party/`. The firmware includes LGPL-licensed Arduino code; keep its corresponding source and licence notices available with redistributed firmware. Modification for personal use and reverse engineering to debug such modifications are permitted.

For scientific use, cite the software using [CITATION.cff](CITATION.cff). Hardware names, software names and trademarks identify the components used; they do not imply endorsement by their owners.
