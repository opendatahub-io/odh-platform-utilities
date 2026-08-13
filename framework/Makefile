GOLANGCI_LINT_VERSION ?= v2.12.2
CONTROLLER_GEN_VERSION ?= v0.20.1
COVERAGE_FILE ?= cover.out

GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
CONTROLLER_GEN = go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

# Pin the toolchain to the exact Go version declared in go.mod so that
# the race-instrumented stdlib is compiled with the same compiler version
# used for user code.
GO_MOD_VERSION := $(shell awk '/^go /{print $$2; exit}' go.mod)
export GOTOOLCHAIN := go$(GO_MOD_VERSION)

.PHONY: all
all: fmt vet lint test

##@ Code Generation

.PHONY: generate
generate: ## Regenerate DeepCopy methods for api/ types.
	$(CONTROLLER_GEN) object paths="./api/..."

##@ Development

.PHONY: fmt
fmt: ## Run gofmt and golangci-lint formatter.
	$(GOLANGCI_LINT) fmt

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with --fix.
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test: ## Run tests with race detector and coverage.
	go test -race -coverprofile="$(COVERAGE_FILE)" ./...

.PHONY: tidy
tidy: ## Run go mod tidy.
	go mod tidy

.PHONY: verify-tidy
verify-tidy: ## Verify go.mod and go.sum are tidy.
	go mod tidy
	@git diff --exit-code go.mod go.sum

.PHONY: verify-fmt
verify-fmt: ## Verify code is formatted.
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)

.PHONY: verify-generate
verify-generate: generate ## Verify generated files are up to date.
	@git diff --quiet --exit-code api/ || (echo "Generated files are out of date. Run 'make -C framework generate' and commit." && git diff --stat api/ && exit 1)

##@ Help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)