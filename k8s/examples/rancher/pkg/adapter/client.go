// Package adapter provides a bridge from controller-runtime to the connect-ng k8s client interface.
//
// This adapter lives in the product's codebase (not the library) to avoid forcing
// controller-runtime version constraints on all consumers.
//
// Each product using connect-ng can copy and adapt this file to their own codebase,
// adjusting it as needed for their specific controller-runtime version.
package adapter

import (
	"context"

	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ControllerRuntimeAdapter wraps a controller-runtime client to implement k8sclient.Client.
//
// This adapter is compatible with controller-runtime v0.14.0+. If you're using a different
// version and encounter issues, copy this file into your codebase and adjust as needed.
//
// Thread-safe: The underlying controller-runtime client is thread-safe.
type ControllerRuntimeAdapter struct {
	client client.Client
}

// NewAdapter creates a k8sclient.Client from a controller-runtime client.
//
// Usage:
//
//	import (
//	    "github.com/SUSE/connect-ng/k8s/scc"
//	    "github.com/myproduct/pkg/adapter"
//	    "sigs.k8s.io/controller-runtime/pkg/manager"
//	)
//
//	func setup(mgr manager.Manager) {
//	    k8sClient := adapter.NewAdapter(mgr.GetClient())
//
//	    sccClient := &scc.DefaultClient{
//	        BaseURL:              "https://scc.suse.com",
//	        K8sClient:            k8sClient,
//	        CredentialsNamespace: "rancher-system",
//	        CredentialsSecretName: "rancher-scc-credentials",
//	    }
//	}
func NewAdapter(c client.Client) k8sclient.Client {
	return &ControllerRuntimeAdapter{client: c}
}

// Get implements k8sclient.Client.Get.
func (a *ControllerRuntimeAdapter) Get(ctx context.Context, namespace, name string, obj k8sclient.Object) error {
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	return a.client.Get(ctx, key, obj)
}

// Create implements k8sclient.Client.Create.
func (a *ControllerRuntimeAdapter) Create(ctx context.Context, obj k8sclient.Object) error {
	return a.client.Create(ctx, obj)
}

// Update implements k8sclient.Client.Update.
func (a *ControllerRuntimeAdapter) Update(ctx context.Context, obj k8sclient.Object) error {
	return a.client.Update(ctx, obj)
}

// UpdateStatus implements k8sclient.Client.UpdateStatus.
func (a *ControllerRuntimeAdapter) UpdateStatus(ctx context.Context, obj k8sclient.Object) error {
	return a.client.Status().Update(ctx, obj)
}

// Delete implements k8sclient.Client.Delete.
func (a *ControllerRuntimeAdapter) Delete(ctx context.Context, obj k8sclient.Object) error {
	return a.client.Delete(ctx, obj)
}

// List implements k8sclient.Client.List.
func (a *ControllerRuntimeAdapter) List(ctx context.Context, list k8sclient.ObjectList, opts ...k8sclient.ListOption) error {
	// Convert k8sclient.ListOptions to controller-runtime ListOptions
	var ctrlOpts []client.ListOption

	// Apply our options to extract the settings
	listOpts := &k8sclient.ListOptions{}
	k8sclient.ApplyOptions(listOpts, opts)

	// Convert to controller-runtime options
	if listOpts.Namespace != "" {
		ctrlOpts = append(ctrlOpts, client.InNamespace(listOpts.Namespace))
	}
	if listOpts.LabelSelector != "" {
		// Parse label selector string into MatchingLabels
		labels := parseLabels(listOpts.LabelSelector)
		if len(labels) > 0 {
			ctrlOpts = append(ctrlOpts, labels)
		}
	}
	if listOpts.FieldSelector != "" {
		// Parse field selector string into MatchingFields
		fields := parseFields(listOpts.FieldSelector)
		if len(fields) > 0 {
			ctrlOpts = append(ctrlOpts, fields)
		}
	}
	if listOpts.Limit > 0 {
		ctrlOpts = append(ctrlOpts, client.Limit(int(listOpts.Limit)))
	}
	if listOpts.Continue != "" {
		ctrlOpts = append(ctrlOpts, client.Continue(listOpts.Continue))
	}

	// If the caller stashed raw controller-runtime options, use them
	if raw, ok := listOpts.Raw.([]client.ListOption); ok {
		ctrlOpts = append(ctrlOpts, raw...)
	}

	// Type assert to ObjectList - all List objects implement this in controller-runtime
	objList, ok := list.(client.ObjectList)
	if !ok {
		// Fallback: try the List call anyway, controller-runtime might handle it
		// This maintains compatibility with different controller-runtime versions
		return a.client.List(ctx, list.(client.ObjectList), ctrlOpts...)
	}

	return a.client.List(ctx, objList, ctrlOpts...)
}

// parseLabels converts a label selector string to MatchingLabels.
// Format: "key1=value1,key2=value2"
func parseLabels(selector string) client.MatchingLabels {
	if selector == "" {
		return nil
	}

	labels := make(client.MatchingLabels)
	// Simple parser for "key=value,key=value" format
	// For complex selectors, products can use Raw field with controller-runtime options
	pairs := splitByComma(selector)
	for _, pair := range pairs {
		kv := splitByEquals(pair)
		if len(kv) == 2 {
			labels[kv[0]] = kv[1]
		}
	}
	return labels
}

// parseFields converts a field selector string to MatchingFields.
// Format: "key1=value1,key2=value2"
func parseFields(selector string) client.MatchingFields {
	if selector == "" {
		return nil
	}

	fields := make(client.MatchingFields)
	// Simple parser for "key=value,key=value" format
	pairs := splitByComma(selector)
	for _, pair := range pairs {
		kv := splitByEquals(pair)
		if len(kv) == 2 {
			fields[kv[0]] = kv[1]
		}
	}
	return fields
}

// splitByComma splits a string by commas.
func splitByComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// splitByEquals splits a string by the first equals sign.
func splitByEquals(s string) []string {
	for i, ch := range s {
		if ch == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// Ensure ControllerRuntimeAdapter implements k8sclient.Client
var _ k8sclient.Client = (*ControllerRuntimeAdapter)(nil)
