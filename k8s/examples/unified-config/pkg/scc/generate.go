package scc

// This file triggers code generation for SCC registration CRDs.
//
// Run: go generate ./...
//
// The generator will:
//   1. Find Config in this package (config.go)
//   2. Generate product-specific CRD types
//   3. Output to ../../apis/<product>.registration.suse.com/v1/
//
// Then run controller-gen to generate DeepCopy and CRD manifests:
//   controller-gen object paths=./apis/...
//   controller-gen crd paths=./apis/... output:crd:dir=./config/crd

//go:generate go run github.com/SUSE/connect-ng/k8s/cmd/generate
