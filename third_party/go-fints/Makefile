.PHONY: all test test-go test-c cli shared clean

GOCACHE ?= /tmp/fints-go-cache

all: test cli shared

test: test-go test-c

test-go:
	GOCACHE=$(GOCACHE) go test ./...

test-c: shared
	cc -I dist cshared/testdata/typed_api_test.c dist/libfints.so -o dist/fints-c-api-test
	DYLD_LIBRARY_PATH=dist LD_LIBRARY_PATH=dist ./dist/fints-c-api-test

cli:
	mkdir -p dist
	GOCACHE=$(GOCACHE) go build -o dist/fints ./cmd/fints

shared:
	mkdir -p dist
	GOCACHE=$(GOCACHE) CGO_ENABLED=1 go build -buildmode=c-shared -o dist/libfints.so ./cshared

clean:
	rm -f dist/fints dist/libfints.so dist/libfints.h dist/fints-c-api-test
