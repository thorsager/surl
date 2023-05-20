version=v0.0.1
binary=bin/surl

.PHONY: surl
surl:
	go build --ldflags='-X main.version=$(version)' -o $(binary) .

.PHONY: url
clean:
	rm -rf bin
