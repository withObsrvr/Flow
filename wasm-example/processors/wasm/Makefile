GOARCH=wasm
GOOS=js
WASM_EXEC_PATH=$(shell go env GOROOT)/misc/wasm/wasm_exec.js

.PHONY: all clean

all: example.wasm copy-wasm-exec

example.wasm: example.go
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o example.wasm example.go

copy-wasm-exec:
	cp $(WASM_EXEC_PATH) .

clean:
	rm -f *.wasm wasm_exec.js 