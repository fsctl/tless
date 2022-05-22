.PHONY: all clean clean-state test

VERSION=0.1.0
BUILD_TIMESTAMP=$(shell date +'%Y-%m-%dT%T')
COMMIT=$(shell git rev-parse HEAD | cut -c 1-6)

all: trustlessbak

trustlessbak: *.go cmd/*.go pkg/backup/*.go pkg/cryptography/*.go pkg/database/*.go pkg/fstraverse/*.go pkg/objstore/*.go pkg/util/*.go
	go build -ldflags "-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTimestamp=$(BUILD_TIMESTAMP)"

clean:
	@rm trustlessbak

clean-state:
	@rm trustlessbak
	@rm trustlessbak-state.db

test:
	@integration-tests/test.sh
