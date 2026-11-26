.PHONY: build run site test vet browser-test

build: ## Build the playground server into ./bin
	go build -o ./bin/cxplayground .

site: ## Build the in-browser playground into ./_site
	rm -rf ./_site
	go run ./cmd/site ./_site
	GOOS=js GOARCH=wasm go build -o ./_site/cx.wasm github.com/skycoin/skycoin-cx/t11-cx
	GOOS=js GOARCH=wasm go build -o ./_site/cxcorefile.wasm ./cmd/cxcorefile
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ./_site/

run: build site ## Serve ./_site and the /eval API on :5336 from this directory
	./bin/cxplayground

test:
	go test ./...

vet:
	go vet ./...

FORTH_DIR = $$(cd browsertest && go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-forth)
MICROAGENTS_DIR = $$(cd browsertest && go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-playground-android/microagents)

browser-test: site ## Run the tests in browsertest/tests in headless Brave, flags in BROWSERTEST_FLAGS
	mkdir -p ./_site/forth
	cd browsertest && GOOS=js GOARCH=wasm go build -o ../_site/forth/cxforth.wasm github.com/skycoin/skycoin-cx-forth/cmd/cxforth-wasm
	cp "$(FORTH_DIR)/web/index.html" ./_site/forth/
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ./_site/forth/
	microagents="$(MICROAGENTS_DIR)" && cd browsertest && go run . -site ../_site -microagents "$$microagents" $(BROWSERTEST_FLAGS)
