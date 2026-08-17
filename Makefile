GOCORSET_VERSION:=$(shell git describe --always --tags)
GOCORSET_VERSION_PATH:="github.com/LFDT-Lineth/zkc/pkg/cmd"
GOLANGCI_VERSION:=2.12.2
PROJECT_NAME:=go-corset
GOPATH_BIN:=$(shell go env GOPATH)/bin
ZKC_LINTABLE_FILES=$(shell find testdata/zkc -name "*.zkc" -not -path "*/invalid/*")
# Define set of unit tests

install:
        # Install golangci-lint for go code linting.
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ${GOPATH_BIN} v${GOLANGCI_VERSION}
        # Install cobra-cli command generator.
	go install github.com/spf13/cobra-cli@latest

all: clean lint test build

lint:
	@echo ">>> Performing golang code linting.."
	golangci-lint run --config=.golangci.yml

go-lint-apply:
	@echo ">>> Applying golang code linting fixes..."
	golangci-lint run --config=.golangci.yml --fix

test:
	@echo ">>> Running All Tests..."
	go test --timeout 0 ./...

corset-test:
	@echo ">>> Running Corset Tests..."
	go test --timeout 0 -run "Test_Valid|Test_Invalid" ./...

corset-racer:
	@echo ">>> Running Corset Racer Tests..."
	go test -race --timeout 0 -run "Test_Bench_Bin|Test_Bench_Euc|Test_Bench_Mul" ./...

corset-bench:
	@echo ">>> Running Corset Benchmark Tests..."
	go test -p 1 --timeout 0 -run "Test_Bench" ./...

unit-test:
	@echo ">>> Running Unit Tests..."
	go test --timeout 0 -skip "Test_Bench|Test_Valid|Test_Invalid|Test_Zkc" ./...

build-zkc:
	@echo ">>> Building zkc... ${GOCORSET_VERSION}"
	go build -ldflags="-X 'github.com/LFDT-Lineth/zkc/pkg/cmd/zkc.Version=${GOCORSET_VERSION}'" -o bin/zkc cmd/zkc/main.go

zkc-lint: build-zkc
	@echo ">>> Linting ZkC source files..."
	./bin/zkc format --check $(ZKC_LINTABLE_FILES)

zkc-lint-apply:
	@echo ">>> Applying zkc code linting fixes..."
	go run ./cmd/zkc format $(ZKC_LINTABLE_FILES)

zkc-unit-test: zkc-lint
	@echo ">>> Running ZkC (Unit) Tests..."
	go test --timeout 0 -run "Test_ZkcUnit|Test_ZkcMixed|Test_ZkcInvalid" ./...

zkc-util-test: zkc-lint
	@echo ">>> Running ZkC (Util) Tests..."
	go test --timeout 0 -run "Test_ZkcUtil" ./...

zkc-bench-test: zkc-lint
	@echo ">>> Running ZkC (Bench) Tests..."
	go test --timeout 0 -run "Test_ZkcBench" ./...

zkc-racer-test:
	@echo ">>> Running ZkC (Racer) Tests..."
	go test -v -race --timeout 0 -run "Test_ZkcBench_FastPow|Test_ZkcBench_Sort" pkg/test/zkc_bench_test.go

build:
	@echo ">>> Building ${PROJECT_NAME}... ${GOCORSET_VERSION}"
	go build -ldflags="-X '${GOCORSET_VERSION_PATH}.Version=${GOCORSET_VERSION}'" -o bin/${PROJECT_NAME} cmd/${PROJECT_NAME}/main.go

clean:
	@echo ">>> Removing old binaries and env files..."
	@rm -rf bin/*
	@rm -rf .env

lint-apply: go-lint-apply zkc-lint-apply
