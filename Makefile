.PHONY: build run site test vet browser-test

build: ## Build the playground server into ./bin
	go build -o ./bin/cxplayground .

site: ## Build the in-browser playground into ./_site
	rm -rf ./_site
	cd web && go run ./cmd/site .. ../_site
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/cx.wasm github.com/skycoin/skycoin-cx/t11-cx-command-cx
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/cxcorefile.wasm ./cmd/cxcorefile
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/cx-core.wasm github.com/skycoin/skycoin-cx/t6-cx-command-core
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/cx-core-image.wasm github.com/skycoin/skycoin-cx/t4-cx-command-image
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/cxdesk.wasm ./cmd/cxdesk
	cd web && GOOS=js GOARCH=wasm go build -o ../_site/netscrape.wasm github.com/0magnet/netscrape/cmd/browser
	cp "$$(cd web && go env GOROOT)/lib/wasm/wasm_exec.js" ./_site/
	cd ./_site && for f in cx.wasm cxcorefile.wasm cx-core.wasm cx-core-image.wasm; do echo "$$f $$(wc -c < $$f)"; done > wasm-sizes.txt

run: build site ## Serve ./_site and the /eval API on :5336 from this directory
	./bin/cxplayground

test:
	go test ./...

vet:
	go vet ./...

FORTH_DIR = $$(cd browsertest && go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-forth)
CXTESTS_DIR = $$(cd web && go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx)/tests
MICROAGENTS_DIR = $$(cd browsertest && go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-playground-android/microagents)

browser-test: site ## Run the tests in browsertest/tests in headless Brave, flags in BROWSERTEST_FLAGS
	mkdir -p ./_site/forth
	cd browsertest && GOOS=js GOARCH=wasm go build -o ../_site/forth/cxforth.wasm github.com/skycoin/skycoin-cx-forth/cmd/cxforth-wasm
	cp "$(FORTH_DIR)/web/index.html" ./_site/forth/
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ./_site/forth/
	cd web && go build -o ../bin/cx github.com/skycoin/skycoin-cx/t11-cx-command-cx && go build -o ../bin/cx-core github.com/skycoin/skycoin-cx/t6-cx-command-core
	microagents="$(MICROAGENTS_DIR)" && cxtests="$(CXTESTS_DIR)" && cd browsertest && go run . -site ../_site -cx ../bin/cx -cxtests "$$cxtests" -microagents "$$microagents" $(BROWSERTEST_FLAGS)
