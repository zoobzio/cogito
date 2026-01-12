.DEFAULT_GOAL := help

.PHONY: test test-unit test-integration test-bench bench lint lint-fix coverage clean check ci install-tools install-hooks help

## Testing

test: ## Run all tests with race detector
	@go test -v -race -timeout=5m ./...

test-unit: ## Run unit tests only (short mode)
	@go test -v -race -short -timeout=2m ./...

test-integration: ## Run integration tests (requires database)
	@go test -v -race -tags=integration -timeout=10m ./testing/integration/...

test-bench: bench ## Run benchmarks (alias)

bench: ## Run benchmarks
	@go test -bench=. -benchmem -benchtime=100ms -timeout=15m ./...

## Code Quality

lint: ## Run linters
	@golangci-lint run --config=.golangci.yml --timeout=5m

lint-fix: ## Run linters with auto-fix
	@golangci-lint run --config=.golangci.yml --fix

## Coverage

coverage: ## Generate coverage report
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1
	@echo "Coverage report generated: coverage.html"

## Maintenance

clean: ## Remove generated files
	@rm -f coverage.out coverage.html
	@find . -name "*.test" -delete
	@find . -name "*.prof" -delete
	@find . -name "*.out" -delete

## Development Setup

install-tools: ## Install required development tools
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2

install-hooks: ## Install git hooks
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@echo '#!/bin/sh\nmake lint' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

## Composite Targets

check: test lint ## Quick validation (test + lint)
	@echo "All checks passed!"

ci: clean lint test coverage ## Full CI simulation
	@echo "Full CI simulation complete!"

## Help

help: ## Display available commands
	@echo "cogito Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
