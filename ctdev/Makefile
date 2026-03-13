VERSION := $(shell cat ../VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test

build:
	go build $(LDFLAGS) -o ctdev .

clean:
	rm -f ctdev

test:
	go test ./...
