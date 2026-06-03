GOCORSET_VERSION:=$(shell git describe --always --tags)
GOCORSET_VERSION_PATH:="github.com/LFDT-Lineth/zkc/pkg/cmd"
GOLANGCI_VERSION:=2.4.0
PROJECT_NAME:=go-corset
GOPATH_BIN:=$(shell go env GOPATH)/bin
ZKC_LINTABLE_FILES=$(shell find testdata/zkc -name "*.zkc" -not -path "*/invalid/*")
# Define set of unit tests

install:
        # Install golangci-lint for go code linting.
	curl -sSfL \
		"https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh" | \
		sh -s -- -b ${GOPATH_BIN} v${GOLANGCI_VERSION}
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

asm-racer:
	@echo ">>> Running Assembly Racer Tests..."
	go test -race --timeout 0 -run "Test_AsmUtil_FillBytes" ./...

asm-bench:
	@echo ">>> Running Assembly Benchmark Tests..."
	go test -p 1 --timeout 0 -run "Test_AsmBench" ./...

asm-util:
	@echo ">>> Running Assembly Util Tests..."
	go test -p 1 --timeout 0 -run "Test_AsmUtil" ./...

asm-unit:
	@echo ">>> Running Assembly Unit Tests..."
	go test --timeout 0 -run "Test_AsmInvalid|Test_AsmUnit" ./...

corset-test:
	@echo ">>> Running Corset Tests..."
	go test --timeout 0 -run "Test_Agnostic|Test_Valid|Test_Invalid" ./...

corset-racer:
	@echo ">>> Running Corset Racer Tests..."
	go test -race --timeout 0 -run "Test_Bench_Bin|Test_Bench_Euc|Test_Bench_Mul" ./...

corset-bench:
	@echo ">>> Running Corset Benchmark Tests..."
	go test -p 1 --timeout 0 -run "Test_Bench" ./...

unit-test:
	@echo ">>> Running Unit Tests..."
	go test --timeout 0 -skip "Test_Asm|Test_Agnostic|Test_Bench|Test_Valid|Test_Invalid|Test_Zkc" ./...

build-zkc:
	@echo ">>> Building zkc... ${GOCORSET_VERSION}"
	go build -ldflags="-X 'github.com/LFDT-Lineth/zkc/pkg/cmd/zkc.Version=${GOCORSET_VERSION}'" -o bin/zkc cmd/zkc/main.go

zkc-lint: build-zkc
	@echo ">>> Linting ZkC source files..."
	./bin/zkc format --check $(ZKC_LINTABLE_FILES)

zkc-lint-apply:
	@echo ">>> Applying zkc code linting fixes..."
	go run ./cmd/zkc format $(ZKC_LINTABLE_FILES)

zkc-test: zkc-lint
	@echo ">>> Running ZkC Tests..."
	go test --timeout 0 -run "Test_ZkcBench|Test_ZkcUnit|Test_ZkcMixed|Test_ZkcInvalid" ./...

build:
	@echo ">>> Building ${PROJECT_NAME}... ${GOCORSET_VERSION}"
	go build -ldflags="-X '${GOCORSET_VERSION_PATH}.Version=${GOCORSET_VERSION}'" -o bin/${PROJECT_NAME} cmd/${PROJECT_NAME}/main.go

clean:
	@echo ">>> Removing old binaries and env files..."
	@rm -rf bin/*
	@rm -rf .env

lint-apply: go-lint-apply zkc-lint-apply
