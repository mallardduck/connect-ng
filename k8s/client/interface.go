// Package client provides a minimal, framework-agnostic Kubernetes client interface.
//
// This package decouples the connect-ng library from sigs.k8s.io/controller-runtime,
// allowing consuming products to use any controller-runtime version without MVS conflicts.
//
// The library's core packages (scc, lifecycle) depend only on this interface and stable
// k8s.io/api types. Products provide thin adapters to bridge their controller-runtime
// version to this interface.
//
// See examples/rancher/pkg/adapter for a reference controller-runtime adapter implementation.
package client

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Object represents any Kubernetes resource object.
//
// This interface combines runtime.Object (for type system integration) with
// metav1.Object (for accessing metadata). All standard Kubernetes resources
// (corev1.Secret, etc.) and controller-runtime's unstructured.Unstructured
// implement this interface.
//
// This matches the contract of controller-runtime's client.Object, ensuring
// compatibility across different controller-runtime versions.
type Object interface {
	runtime.Object
	metav1.Object
}

// ObjectList represents a list of Kubernetes resources.
//
// This is used for List operations. List objects (like corev1.SecretList,
// unstructured.UnstructuredList) implement this interface.
type ObjectList interface {
	runtime.Object
}

// Client defines the minimal Kubernetes client operations needed by connect-ng.
//
// Products implement this by wrapping their controller-runtime client (any version)
// with a thin adapter. The interface is intentionally minimal to reduce coupling.
//
// Thread safety: Implementations must be safe for concurrent use.
type Client interface {
	// Get retrieves a resource from Kubernetes.
	//
	// Parameters:
	//   ctx: Request context
	//   namespace: Resource namespace (empty string for cluster-scoped resources)
	//   name: Resource name
	//   obj: Pointer to the object to populate (e.g., &corev1.Secret{})
	//
	// Returns an error if the resource doesn't exist (check with apierrors.IsNotFound).
	Get(ctx context.Context, namespace, name string, obj Object) error

	// Create creates a new resource in Kubernetes.
	//
	// The obj must have its metadata.namespace and metadata.name set.
	// Returns an error if the resource already exists.
	Create(ctx context.Context, obj Object) error

	// Update updates an existing resource in Kubernetes.
	//
	// This updates the resource spec/data. For status updates, use UpdateStatus.
	// The obj should be a recently-fetched resource to avoid conflicts.
	Update(ctx context.Context, obj Object) error

	// UpdateStatus updates only the status subresource of a resource.
	//
	// This is used to update resource status without triggering spec validation
	// or interfering with spec updates. The obj should be a recently-fetched
	// resource with the desired status changes applied.
	//
	// Note: Not all resources have a status subresource. This is primarily
	// used with CRDs that define status as a subresource.
	UpdateStatus(ctx context.Context, obj Object) error

	// Delete removes a resource from Kubernetes.
	//
	// The obj must have its metadata.namespace and metadata.name set to identify
	// which resource to delete. Returns an error if the resource doesn't exist.
	Delete(ctx context.Context, obj Object) error

	// List retrieves multiple resources matching the given criteria.
	//
	// Parameters:
	//   ctx: Request context
	//   list: Pointer to a list object (e.g., &corev1.SecretList{})
	//   opts: Optional filtering/selection criteria
	//
	// The list object will be populated with matching resources.
	List(ctx context.Context, list ObjectList, opts ...ListOption) error
}

// ListOption defines filtering/selection criteria for List operations.
//
// Products using controller-runtime can wrap controller-runtime's ListOption
// interface directly - they're compatible by design.
type ListOption interface {
	// ApplyToList applies this option to a list operation.
	// This is intentionally compatible with controller-runtime's ListOption interface.
	ApplyToList(opts *ListOptions)
}

// ListOptions contains the consolidated options for a List call.
//
// This mirrors a subset of controller-runtime's ListOptions, containing only
// the fields actually used by connect-ng's core packages.
type ListOptions struct {
	// LabelSelector filters resources by labels (e.g., "app=myapp,env=prod")
	LabelSelector string

	// FieldSelector filters resources by fields (e.g., "metadata.name=mysecret")
	FieldSelector string

	// Namespace restricts listing to a specific namespace.
	// Empty string lists across all namespaces (if permitted).
	Namespace string

	// Limit restricts the number of results returned.
	Limit int64

	// Continue is a token for pagination.
	Continue string

	// Raw stores any additional options that don't map to the above fields.
	// controller-runtime adapters can stash implementation-specific options here.
	Raw interface{}
}

// ApplyOptions applies a list of options to ListOptions.
// This is a helper for adapter implementations.
func ApplyOptions(opts *ListOptions, listOpts []ListOption) {
	for _, opt := range listOpts {
		opt.ApplyToList(opts)
	}
}

// InNamespace returns a ListOption that restricts listing to a namespace.
func InNamespace(namespace string) ListOption {
	return namespaceOption{namespace: namespace}
}

type namespaceOption struct {
	namespace string
}

func (n namespaceOption) ApplyToList(opts *ListOptions) {
	opts.Namespace = n.namespace
}

// MatchingLabels returns a ListOption that filters by labels.
// labels is a map of key-value pairs (e.g., map[string]string{"app": "myapp"})
func MatchingLabels(labels map[string]string) ListOption {
	return matchingLabelsOption{labels: labels}
}

type matchingLabelsOption struct {
	labels map[string]string
}

func (m matchingLabelsOption) ApplyToList(opts *ListOptions) {
	// Convert map to label selector string
	// Simple implementation - products can enhance via Raw field if needed
	selector := ""
	first := true
	for k, v := range m.labels {
		if !first {
			selector += ","
		}
		selector += k + "=" + v
		first = false
	}
	opts.LabelSelector = selector
}
