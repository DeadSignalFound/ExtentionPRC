# firefox-discord-rpc

Show what you're doing in Firefox as a Discord Rich Presence — automatic favicons for every site, Spotify-style "now playing" for YouTube & YouTube Music, zero bot tokens, zero external services.

> Your browser can't speak Discord IPC directly, so a tiny local Go bridge does the translation. Nothing leaves your machine.

---

## Features

- Per-tab presence that follows the active Firefox window
- Automatic favicon lookup — the site's own icon becomes the large image
- YouTube / YT Music "listening to…" mode with live progress, album art, title, and artist
- Cleared automatically on idle / lost focus / closed tab
- Reliable through long idle periods via MV3 `alarms` heartbeat
- Windows, Linux, and macOS support (Discord desktop client required)
- No bot token, no OAuth, no assets to pre-upload

---

## Install (quick start)

1. **Download the bridge binary** for your OS from the [latest release](../../releases/latest) — or build it yourself (see below).
2. Run it. It listens on `127.0.0.1:6473`.
   ```
   ./discord-rpc-bridge
   ```
3. **Install the extension** in Firefox:
   - Easiest: open `about:debugging#/runtime/this-firefox`, click **Load Temporary Add-on…**, pick `extension/manifest.json`.
   - Or drop the `firefox-discord-rpc.xpi` (from the release) into Firefox if you're on Dev Edition / Nightly with `xpinstall.signatures.required = false`.
4. Start Discord desktop. That's it — browse a tab and your profile updates within a second.

### Using your own Discord application

The repo ships with a default Discord Application ID baked into `extension/background.js`. If you want your own app name to show in the header or your own asset uploads, create one at <https://discord.com/developers/applications>, copy the **Application ID**, and paste it into the extension popup (or edit `DEFAULT_CLIENT_ID` in `background.js`).

---

## Build from source

### Bridge

Requires Go 1.22+.

```bash
cd bridge
go build -trimpath -ldflags="-s -w" -o ../dist/discord-rpc-bridge .
```

Cross-compile:

```bash
GOOS=linux   GOARCH=amd64 go build -o ../dist/discord-rpc-bridge-linux   .
GOOS=darwin  GOARCH=amd64 go build -o ../dist/discord-rpc-bridge-darwin  .
GOOS=windows GOARCH=amd64 go build -o ../dist/discord-rpc-bridge.exe     .
```

### Extension `.xpi`

```bash
npx web-ext build --source-dir=extension --artifacts-dir=dist --overwrite-dest
```

Or, if you prefer `make`:

```bash
make          # builds both bridge + xpi into dist/
make bridge   # just the Go binary
make xpi      # just the extension
make run      # build & launch the bridge
```

---

## How it works

```
Firefox tab change ──► background.js ──WebSocket──► bridge ──named pipe──► Discord
```

- The extension watches `tabs.onActivated`, `tabs.onUpdated`, window focus, and idle state.
- On each change it pushes a JSON activity frame over a local WebSocket.
- The bridge holds a Discord IPC connection (`\\.\pipe\discord-ipc-N` on Windows, `$XDG_RUNTIME_DIR/discord-ipc-N` on Linux/macOS), performs the RPC handshake, and forwards `SET_ACTIVITY` commands.
- Favicon URLs ≤ 256 chars are sent directly to Discord; SVGs and unknown favicons fall through to DuckDuckGo's icon proxy so they always rasterize to PNG.
- For YouTube / YouTube Music a content script scrapes title, artist, album art, and playback position from the page DOM, and the background builds a `type: 2 (Listening)` activity with live timestamps.

### WebSocket protocol (extension ↔ bridge)

```jsonc
// extension → bridge
{ "type": "connect", "client_id": "1493453061987893318" }

{ "type": "activity", "activity": {
    "name": "YouTube",
    "type": 2,
    "status_display_type": 1,
    "details": "Some Video Title",
    "state":   "Channel Name",
    "timestamps": { "start": 1734550000, "end": 1734550600 },
    "assets":     { "large_image": "https://…/thumb.jpg", "large_text": "YouTube" }
}}

{ "type": "clear" }
{ "type": "ping" }

// bridge → extension
{ "status": "ok",    "msg": "connected" }
{ "status": "error", "msg": "..." }
```

---

## Repo layout

```
.
├── bridge/                    Go IPC bridge (WebSocket ↔ Discord IPC)
│   ├── main.go                HTTP server and per-connection plumbing
│   ├── discord.go             low-level Discord IPC client
│   ├── assets.go              asset URL normalisation
│   ├── go.mod
│   └── wstest/                dev-only WebSocket ping tester
├── extension/                 Firefox WebExtension (Manifest V3)
│   ├── manifest.json
│   ├── background.js
│   ├── content-media.js       DOM scraper for YouTube / YT Music
│   ├── popup.html · popup.js
│   └── icons/
├── scripts/
│   └── make_icons.py          regenerates extension icons from a source PNG
├── .github/workflows/build.yml  CI: cross-compile bridge + package .xpi
├── Makefile
├── LICENSE
└── README.md
```

---

## Privacy

- Only the local machine is contacted (`127.0.0.1`) — no telemetry, no analytics.
- The only outbound HTTP request is the favicon image Discord itself pulls; the extension never makes an outbound call on its own.
- Tab title and hostname of the **active** tab are the only values forwarded, one-way, to the local bridge. Background/inactive tabs are never touched.
- Disable or uninstall the extension to stop everything.

---

## Troubleshooting

| Symptom | Cause / Fix |
|---|---|
| `discord ipc pipe not found` | Desktop Discord isn't running. Web Discord doesn't expose the IPC socket. |
| Popup says *disconnected* | Bridge isn't running — start `discord-rpc-bridge`. |
| Connected but no activity on profile | Check Discord → **Settings → Activity Privacy** → enable *Display current activity as a status message*. |
| Header shows the app name instead of site / song title | Your Discord client is ignoring `status_display_type`. Rename your Discord application at the [developer portal](https://discord.com/developers/applications) or upgrade Discord. |
| `?` placeholder where the favicon should be | Site ships an SVG favicon; the extension already falls back to DuckDuckGo's PNG icon proxy — reload the extension. |

---

## Contributing

PRs welcome. Keep the code style consistent with the existing files and run `make lint` (or `go vet ./...` + `web-ext lint`) before opening a PR. CI builds the bridge for Windows/Linux/macOS and packages the extension automatically; tagging `v*.*.*` also cuts a GitHub release with all artifacts attached.

---

## License

MIT — see [`LICENSE`](./LICENSE).
