GOLANGCI_LINT_VERSION ?= v2.5.0
COVERAGE_FILE ?= cover.out

GOLANGCI_LINT = $(shell which golangci-lint 2>/dev/null)

.PHONY: all
all: fmt vet lint test

##@ Development

.PHONY: fmt
fmt: ## Run gofmt and golangci-lint formatter.
	gofmt -w .
	@if [ -n "$(GOLANGCI_LINT)" ]; then golangci-lint fmt; fi

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint with --fix.
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test: ## Run tests with race detector and coverage.
	go test -race -coverprofile=$(COVERAGE_FILE) ./...

.PHONY: tidy
tidy: ## Run go mod tidy.
	go mod tidy

.PHONY: verify-tidy
verify-tidy: ## Verify go.mod and go.sum are tidy.
	go mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: verify-fmt
verify-fmt: ## Verify code is formatted.
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)

##@ Tools

.PHONY: golangci-lint
golangci-lint: ## Install golangci-lint if not present.
ifeq ($(GOLANGCI_LINT),)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(eval GOLANGCI_LINT = $(shell go env GOPATH)/bin/golangci-lint)
endif

##@ Help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
