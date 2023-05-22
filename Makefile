binary=bin/surl
version=$(shell git describe --tags --always --dirty)

.PHONY: surl
surl:
	go build -v --ldflags='-X main.version=$(version)' -o $(binary) ./...

.PHONY: url
clean:
	rm -rf bin
