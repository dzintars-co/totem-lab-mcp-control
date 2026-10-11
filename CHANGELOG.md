# Totem Lab MCP Control — changelog / izmaiņu vēsture

## 1.0.0 — 2026-10-11

- Detect unresponsive firmware after an idle serial period without replaying output commands or inventing input events.
- Update the server's `golang.org/x/sys` dependency to `v0.47.0`.
- Align server, bridge and citation versions; rebuild both Linux x86_64 executables from this release's source.
- Include the latest English/Latvian quick start, hardware/dashboard previews, CI status and Arduino compilation check.
- Add this release history; retain the existing firmware, wiring, MCP tools/resources and project layout.

LV: Serveris atpazīst firmware, kas pārstāj atbildēt. Atjaunināta `x/sys` atkarība, saskaņotas versijas un abi Linux izpildfaili. Iekļautas jaunākās instrukcijas, attēli un Go/Arduino CI; firmware un pieslēgumi saglabāti.

## 0.1.1 — 2026-10-11

- Show interrupted MCP subscriptions as disconnected, mark retained readings stale and reconnect automatically.
- Fit and scroll the dashboard in smaller terminal windows; add regression tests and bilingual instructions.

LV: Labota MCP savienojuma statusa rādīšana, atkārtota pieslēgšanās un termināļa paneļa ritināšana.

## 0.1.0 — 2026-10-10

- Initial release: Arduino input readings, Go MCP subscriptions, terminal dashboard, optional Codex CLI event reports, RGB LED and relay control.

LV: Sākotnējais laidiens ar Arduino ieejām, MCP abonēšanu, termināļa paneli, Codex atbildēm un RGB/releja vadību.
