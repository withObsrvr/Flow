.PHONY: all clean

all: plugin

plugin:
	go build -buildmode=plugin -o ../plugins/flow-processor-latest-ledger.so main.go

clean:
	rm -f ../plugins/flow-processor-latest-ledger.so 