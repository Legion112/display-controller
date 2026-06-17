.PHONY: build test deploy install restart

ifneq ($(wildcard $(HOME)/.local/go1.26/bin/go),)
GO := $(HOME)/.local/go1.26/bin/go
else
GO ?= go
endif

export GOTOOLCHAIN ?= auto

build:
	$(GO) build -o bin/display-brightnessd ./cmd/display-brightnessd

test:
	$(GO) test ./...

install:
	./scripts/install.sh

restart:
	systemctl --user restart display-brightness.service

deploy: install restart
	@echo "Deploy complete. Restart GNOME Shell (Alt+F2, r) if you changed the extension."
