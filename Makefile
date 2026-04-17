.PHONY: all bridge xpi clean lint icons run

DIST := dist

all: bridge xpi

bridge:
	@mkdir -p $(DIST)
	cd bridge && go build -trimpath -ldflags="-s -w" -o ../$(DIST)/discord-rpc-bridge .

xpi:
	@mkdir -p $(DIST)
	npx --yes web-ext@8 build --source-dir=extension --artifacts-dir=$(DIST) --overwrite-dest
	@mv $(DIST)/*.zip $(DIST)/firefox-discord-rpc.xpi 2>/dev/null || true

lint:
	cd bridge && go vet ./...
	npx --yes web-ext@8 lint --source-dir=extension --warnings-as-errors=false

icons:
	python scripts/make_icons.py

run: bridge
	$(DIST)/discord-rpc-bridge

clean:
	rm -rf $(DIST)
