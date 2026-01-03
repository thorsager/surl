binary=bin/surl
version=$(shell git describe --tags --always --dirty)
image=ghcr.io/thorsager/surl
image_tag=$(version)


.PHONY: all
all: test build

build:
	go build -v -a -tags netgo --ldflags='-X main.version=$(version)' -o $(binary) ./...

.PHONY: test
test:
	go test -v ./...

.PHONY: clean
clean:
	rm -rf bin

.PHONY: image
image:
	docker build -t $(image):$(image_tag) .

.PHONY: get
get:
	go get -v -t ./...


.PHONY: snakeoil
snakeoil:
	openssl req  -new  -newkey rsa:2048  -nodes  -keyout localhost.key  -out localhost.csr
	openssl  x509  -req  -days 365  -in localhost.csr  -signkey localhost.key  -out localhost.crt
