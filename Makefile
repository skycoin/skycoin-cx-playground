.PHONY: build run site test vet

build: ## Build the playground server into ./bin
	go build -o ./bin/cxplayground .

site: ## Build the in-browser playground into ./_site
	rm -rf ./_site
	go run ./cmd/site ./_site
	GOOS=js GOARCH=wasm go build -o ./_site/cx.wasm github.com/skycoin/skycoin-cx/t11-cx
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ./_site/

run: build site ## Serve ./_site and the /eval API on :5336 from this directory
	./bin/cxplayground

test:
	go test ./...

vet:
	go vet ./...
