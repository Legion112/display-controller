# Display Brightness

Control all external monitors at once from the Ubuntu Quick Settings panel, using [ddcutil](https://www.ddcutil.com/) over DDC/CI (DisplayPort/HDMI).

## Motivation

With multiple monitors, matching brightness manually on each panel is slow. ddcutil can set brightness programmatically:

```bash
for d in 1 2 3; do
    ddcutil --display "$d" setvcp 10 50
done
```

This project adds a Go daemon plus a GNOME Shell extension so you get a slider next to volume in Quick Settings.

## Requirements

- Ubuntu 24.04+ with GNOME Shell 46
- `ddcutil` installed and working (`ddcutil detect`)
- User in the `i2c` group (or equivalent udev rules) for DDC/CI access
- Go 1.26.x to build (minimum 1.26.0; any patch version works; runtime is a single binary)

## Install

First install or after any change (daemon, extension, systemd unit):

```bash
make deploy
```

This builds `~/.local/bin/display-brightnessd`, installs the GNOME extension and systemd user unit, and restarts the service.

Equivalent to `make install` followed by `make restart`. The underlying script is [`scripts/install.sh`](scripts/install.sh).

## Development

| Command | What it does |
|---------|--------------|
| `make deploy` | Build, install binary + extension + systemd unit, restart service |
| `make build` | Compile to `bin/display-brightnessd` only |
| `make test` | Run Go tests |
| `make restart` | Restart user service only |
| `make clean-cache` | Clear Go build cache (use if toolchain version mismatch) |

`GOTOOLCHAIN=go1.26.0+auto` is set by the Makefile so any Go 1.26.x patch can be used automatically.

Daemon changes take effect immediately after `make deploy`. Extension JS changes require a GNOME Shell restart (`Alt+F2`, `r`) to appear in Quick Settings.

## Manual test

```bash
# Service status
systemctl --user status display-brightness

# Set all monitors to 50%
busctl --user call org.display.Brightness /org/display/Brightness \
  org.display.Brightness SetBrightness y 50

# Read current average brightness
busctl --user call org.display.Brightness /org/display/Brightness \
  org.display.Brightness GetBrightness
```

## Build only

```bash
go build -o display-brightnessd ./cmd/display-brightnessd
```

## Troubleshooting

- **No displays detected** — run `ddcutil detect`; check i2c permissions
- **Service not running** — `journalctl --user -u display-brightness -f`
- **Slider disabled** — ensure the D-Bus service is active before enabling the extension
- **Extension missing after install** — restart GNOME Shell (`Alt+F2`, `r`) or log out/in; new extensions are picked up on shell restart
- **Go version mismatch** (`compile: version "go1.26.x" does not match go tool version`) — run `make clean-cache && make deploy`

## Architecture

- `display-brightnessd` — Go 1.26 session D-Bus service (`org.display.Brightness`)
- `extension/` — GNOME Quick Settings slider (JavaScript)
- Auto-detects all DDC/CI displays via `ddcutil detect --brief`
- Sets the same brightness percentage on every display (normalized per monitor max)
