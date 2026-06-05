PREFIX ?= $(HOME)/.local
BIN    ?= $(PREFIX)/bin

.PHONY: build install vet test clean

build:
	go build -o devcon .

install: build
	install -d $(BIN)
	install -m 0755 devcon $(BIN)/devcon
	@echo "installed devcon -> $(BIN)/devcon"

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -f devcon
