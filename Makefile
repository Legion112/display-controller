.PHONY: build test deploy install restart clean-cache

GO ?= go
export GOTOOLCHAIN ?= go1.26.0+auto

build:
	$(GO) build -o bin/display-brightnessd ./cmd/display-brightnessd

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
