.PHONY: all clean clean-state test

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

tless: *.go cmd/*.go pkg/backup/*.go pkg/cryptography/*.go pkg/database/*.go pkg/fstraverse/*.go pkg/objstore/*.go pkg/util/*.go daemon/*.go pkg/snapshots/*.go ${RPC_PB}
	go build -ldflags "-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTimestamp=$(BUILD_TIMESTAMP)"

clean:
	@rm tless
	@rm rpc/rpc.pb.go
	@rm rpc/rpc_grpc.pb.go

clean-state:
	@rm $(HOME)/.tless/state.db

test:
	@integration-tests/test.sh
