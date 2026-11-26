module github.com/skycoin/skycoin-cx-playground/browsertest

go 1.27.3

require github.com/coder/websocket v1.8.15

require (
	github.com/skycoin/skycoin-cx-forth v0.0.0-20261122010033-ec543b8edd76 // indirect
	github.com/skycoin/skycoin-cx-playground-android/microagents v0.0.0-20261125054539-a1192efdfe56 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

tool (
	github.com/skycoin/skycoin-cx-forth/cmd/cxforth
	github.com/skycoin/skycoin-cx-forth/cmd/cxforth-wasm
	github.com/skycoin/skycoin-cx-playground-android/microagents/backendstore
)
