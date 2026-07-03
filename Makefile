.PHONY: build test deploy install restart clean-cache flash-sensor build-lux-read lux-read deploy-sensor

GO ?= go
export GOTOOLCHAIN ?= go1.26.0+auto

build:
	$(GO) build -o bin/display-brightnessd ./cmd/display-brightnessd

build-lux-read:
	$(GO) build -o bin/lux-read ./cmd/lux-read

lux-read: build-lux-read
	./bin/lux-read

flash-sensor:
	cd firmware/ambient-sensor && cargo run --release

deploy-sensor: flash-sensor lux-read
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
