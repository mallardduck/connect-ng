# Example Makefile showing unified config pattern

# Generate all code (CRD types, DeepCopy, CRD manifests)
.PHONY: generate
generate:
	# Step 1: Generate SCC registration types using go:generate
	# This reads pkg/scc/config.go and generates types
	go generate ./pkg/scc/...

	# Step 2: Generate DeepCopy methods
	controller-gen object paths=./apis/...

	# Step 3: Generate CRD manifests
	controller-gen crd:crdVersions=v1 paths=./apis/... output:crd:dir=./config/crd

	@echo "✅ Code generation complete!"
	@echo "   - Types: ./apis/$(shell go run -tags=getconfig ./pkg/scc/... --print-group)/v1/"
	@echo "   - CRDs:  ./config/crd/"

# Alternative: Run just the SCC generator
.PHONY: generate-scc
generate-scc:
	go generate ./pkg/scc/...

# Alternative: Specify config file explicitly (if not using default location)
.PHONY: generate-with-config
generate-with-config:
	go run github.com/SUSE/connect-ng/k8s/cmd/generate --config=pkg/scc/config.go

# Clean generated files
.PHONY: clean
clean:
	rm -rf ./apis/
	rm -rf ./config/crd/
	@echo "✅ Cleaned generated files"
