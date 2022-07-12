.PHONY: all clean clean-state test

SRC_FILES=*.go cmd/*.go pkg/backup/*.go pkg/cryptography/*.go pkg/database/*.go pkg/fstraverse/*.go pkg/objstore/*.go pkg/util/*.go daemon/*.go pkg/snapshots/*.go
LDFLAGS=-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTimestamp=$(BUILD_TIMESTAMP)

VERSION=0.1.0
BUILD_TIMESTAMP=$(shell date +'%Y-%m-%dT%T')
COMMIT=$(shell git rev-parse HEAD | cut -c 1-6)

RPC_PROTO=rpc/rpc.proto
RPC_PB=$(RPC_PROTO:.proto=.pb.go)

%.pb.go: %.proto
	protoc $< \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative

all: tless

tless: ${SRC_FILES} ${RPC_PB}
	go build -ldflags "${LDFLAGS}"

darwin:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o tless.darwin.x86_64 -ldflags "${LDFLAGS}"
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o tless.darwin.arm64 -ldflags "${LDFLAGS}"
	lipo -create -output tless tless.darwin.x86_64 tless.darwin.arm64
	@rm tless.darwin.x86_64 tless.darwin.arm64

clean:
	@rm tless
	@rm rpc/rpc.pb.go
	@rm rpc/rpc_grpc.pb.go

clean-state:
	@rm $(HOME)/.tless/state.db

test:
	@integration-tests/test.sh
