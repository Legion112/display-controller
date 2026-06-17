#!/usr/bin/env bash
# Called by `make deploy`; service restart is handled by the Makefile, not here.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${HOME}/.config/systemd/user"
EXT_SRC="${ROOT}/extension/display-brightness@legion"
EXT_DST="${HOME}/.local/share/gnome-shell/extensions/display-brightness@legion"

echo "==> Building display-brightnessd"
mkdir -p "${BIN_DIR}"

GO_BIN="${GO_BIN:-go}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.0+auto}"
(cd "${ROOT}" && "${GO_BIN}" build -o "${BIN_DIR}/display-brightnessd" ./cmd/display-brightnessd)

echo "==> Installing systemd user service"
mkdir -p "${SYSTEMD_DIR}"
install -m 0644 "${ROOT}/systemd/display-brightness.service" "${SYSTEMD_DIR}/display-brightness.service"

systemctl --user daemon-reload
systemctl --user enable --now display-brightness.service

echo "==> Installing GNOME Shell extension"
mkdir -p "${EXT_DST}"
install -m 0644 "${EXT_SRC}/extension.js" "${EXT_DST}/extension.js"
install -m 0644 "${EXT_SRC}/metadata.json" "${EXT_DST}/metadata.json"

if command -v gnome-extensions >/dev/null 2>&1; then
    gnome-extensions enable display-brightness@legion 2>/dev/null || true
fi

if command -v gsettings >/dev/null 2>&1; then
    CURRENT="$(gsettings get org.gnome.shell enabled-extensions)"
    if [[ "${CURRENT}" != *"display-brightness@legion"* ]]; then
        NEW="$(echo "${CURRENT}" | sed "s/]$/, 'display-brightness@legion']/")"
        gsettings set org.gnome.shell enabled-extensions "${NEW}"
    fi
fi

echo
echo "Installed."
echo "  binary:   ${BIN_DIR}/display-brightnessd"
echo "  service:  systemctl --user status display-brightness"
echo "  test:     busctl --user call org.display.Brightness /org/display/Brightness org.display.Brightness SetBrightness y 50"
echo
echo "If the Quick Settings slider does not appear, restart GNOME Shell (Alt+F2, r, Enter) or log out and back in."
