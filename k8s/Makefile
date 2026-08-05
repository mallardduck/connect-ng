.PHONY: generate
generate: ## Generate DeepCopy methods for API types
	@echo "Generating DeepCopy methods..."
	cd api/v1 && go generate
	@echo "Done. Don't forget to commit changes to zz_generated.deepcopy.go"

.PHONY: verify-generate
verify-generate: ## Verify that generated files are up to date
	@echo "Verifying generated files are up to date..."
	@cd api/v1 && go generate
	@if ! git diff --exit-code api/v1/zz_generated.deepcopy.go; then \
		echo "ERROR: Generated files are out of date. Run 'make generate' and commit the changes."; \
		exit 1; \
	fi
	@echo "Generated files are up to date."

.PHONY: test
test: ## Run tests
	go test ./...

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
