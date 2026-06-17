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
- Go 1.26+ to build (runtime is a single binary)

## Install

```bash
./scripts/install.sh
```

This builds `~/.local/bin/display-brightnessd`, enables the user systemd service, and installs the GNOME extension.

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

## Architecture

- `display-brightnessd` — Go 1.26 session D-Bus service (`org.display.Brightness`)
- `extension/` — GNOME Quick Settings slider (JavaScript)
- Auto-detects all DDC/CI displays via `ddcutil detect --brief`
- Sets the same brightness percentage on every display (normalized per monitor max)
