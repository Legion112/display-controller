.PHONY: build test deploy install restart clean-cache flash-sensor build-lux-read lux-read deploy-sensor sensor-boot-log sensor-scan
GO ?= go
export GOTOOLCHAIN ?= go1.26.0+auto
export RAVEDUDE_PORT = /dev/ttyUSB0

build:
	$(GO) build -o bin/display-brightnessd ./cmd/display-brightnessd

build-lux-read:
	$(GO) build -o bin/lux-read ./cmd/lux-read

lux-read: build-lux-read
	./bin/lux-read

flash-sensor:
	cd firmware/ambient-sensor && cargo run --release

sensor-boot-log:
	python3 scripts/read-sensor-boot.py

sensor-scan: flash-sensor sensor-boot-log

deploy-sensor: flash-sensor sensor-boot-log lux-read
	@echo "Sensor deploy complete."

test:
	$(GO) test ./...

install:
	./scripts/install.sh

restart:
	systemctl --user restart display-brightness.service

clean-cache:
	$(GO) clean -cache

deploy: install restart
	@echo "Deploy complete. Restart GNOME Shell (Alt+F2, r) if you changed the extension."
