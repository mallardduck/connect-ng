/*
Copyright 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package v1 contains common API types for SUSE product registration.
//
// These types are imported and embedded by product-specific CRDs (Rancher, NeuVector, etc.)
// to ensure consistency across implementations.
//
// This package only contains the shared Spec/Status types - products generate their own
// wrapper CRD types that embed these.
//
// +kubebuilder:object:generate=true
package v1

//go:generate sh -c "cd ../../.. && controller-gen object:headerFile=hack/boilerplate.go.txt paths=./k8s/api/v1/..."
