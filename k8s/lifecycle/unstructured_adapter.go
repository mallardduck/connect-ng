package lifecycle

import (
	"context"
	"fmt"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	connectngv1 "github.com/SUSE/connect-ng/k8s/api/v1"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// UnstructuredRegistrationObject wraps an unstructured.Unstructured object
// and implements types.ProductRegistrationObject.
//
// This allows the library to work with any product's CRD without knowing
// the concrete type at compile time.
type UnstructuredRegistrationObject struct {
	obj    *unstructured.Unstructured
	spec   *UnstructuredSpec
	status *UnstructuredStatus
}

// NewUnstructuredRegistrationObject creates an adapter from an unstructured object.
func NewUnstructuredRegistrationObject(obj *unstructured.Unstructured) (*UnstructuredRegistrationObject, error) {
	// Extract spec
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("failed to extract spec: %w", err)
	}
	if !found {
		specMap = make(map[string]interface{})
	}

	// Extract status
	statusMap, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil {
		return nil, fmt.Errorf("failed to extract status: %w", err)
	}
	if !found {
		statusMap = make(map[string]interface{})
	}

	return &UnstructuredRegistrationObject{
		obj:    obj,
		spec:   &UnstructuredSpec{data: specMap},
		status: &UnstructuredStatus{data: statusMap, obj: obj},
	}, nil
}

// GetSpec implements types.ProductRegistrationObject
func (u *UnstructuredRegistrationObject) GetSpec() contract.ProductRegistrationSpec {
	return u.spec
}

// GetStatus implements types.ProductRegistrationObject
func (u *UnstructuredRegistrationObject) GetStatus() contract.ProductRegistrationStatus {
	return u.status
}

// metav1.Object methods (delegate to underlying unstructured)

func (u *UnstructuredRegistrationObject) GetNamespace() string {
	return u.obj.GetNamespace()
}

func (u *UnstructuredRegistrationObject) SetNamespace(namespace string) {
	u.obj.SetNamespace(namespace)
}

func (u *UnstructuredRegistrationObject) GetName() string {
	return u.obj.GetName()
}

func (u *UnstructuredRegistrationObject) SetName(name string) {
	u.obj.SetName(name)
}

func (u *UnstructuredRegistrationObject) GetGenerateName() string {
	return u.obj.GetGenerateName()
}

func (u *UnstructuredRegistrationObject) SetGenerateName(name string) {
	u.obj.SetGenerateName(name)
}

func (u *UnstructuredRegistrationObject) GetUID() k8stypes.UID {
	return u.obj.GetUID()
}

func (u *UnstructuredRegistrationObject) SetUID(uid k8stypes.UID) {
	u.obj.SetUID(uid)
}

func (u *UnstructuredRegistrationObject) GetResourceVersion() string {
	return u.obj.GetResourceVersion()
}

func (u *UnstructuredRegistrationObject) SetResourceVersion(version string) {
	u.obj.SetResourceVersion(version)
}

func (u *UnstructuredRegistrationObject) GetGeneration() int64 {
	return u.obj.GetGeneration()
}

func (u *UnstructuredRegistrationObject) SetGeneration(generation int64) {
	u.obj.SetGeneration(generation)
}

func (u *UnstructuredRegistrationObject) GetSelfLink() string {
	return u.obj.GetSelfLink()
}

func (u *UnstructuredRegistrationObject) SetSelfLink(selfLink string) {
	u.obj.SetSelfLink(selfLink)
}

func (u *UnstructuredRegistrationObject) GetCreationTimestamp() metav1.Time {
	return u.obj.GetCreationTimestamp()
}

func (u *UnstructuredRegistrationObject) SetCreationTimestamp(timestamp metav1.Time) {
	u.obj.SetCreationTimestamp(timestamp)
}

func (u *UnstructuredRegistrationObject) GetDeletionTimestamp() *metav1.Time {
	return u.obj.GetDeletionTimestamp()
}

func (u *UnstructuredRegistrationObject) SetDeletionTimestamp(timestamp *metav1.Time) {
	u.obj.SetDeletionTimestamp(timestamp)
}

func (u *UnstructuredRegistrationObject) GetDeletionGracePeriodSeconds() *int64 {
	return u.obj.GetDeletionGracePeriodSeconds()
}

func (u *UnstructuredRegistrationObject) SetDeletionGracePeriodSeconds(i *int64) {
	u.obj.SetDeletionGracePeriodSeconds(i)
}

func (u *UnstructuredRegistrationObject) GetLabels() map[string]string {
	return u.obj.GetLabels()
}

func (u *UnstructuredRegistrationObject) SetLabels(labels map[string]string) {
	u.obj.SetLabels(labels)
}

func (u *UnstructuredRegistrationObject) GetAnnotations() map[string]string {
	return u.obj.GetAnnotations()
}

func (u *UnstructuredRegistrationObject) SetAnnotations(annotations map[string]string) {
	u.obj.SetAnnotations(annotations)
}

func (u *UnstructuredRegistrationObject) GetFinalizers() []string {
	return u.obj.GetFinalizers()
}

func (u *UnstructuredRegistrationObject) SetFinalizers(finalizers []string) {
	u.obj.SetFinalizers(finalizers)
}

func (u *UnstructuredRegistrationObject) GetOwnerReferences() []metav1.OwnerReference {
	return u.obj.GetOwnerReferences()
}

func (u *UnstructuredRegistrationObject) SetOwnerReferences(references []metav1.OwnerReference) {
	u.obj.SetOwnerReferences(references)
}

func (u *UnstructuredRegistrationObject) GetManagedFields() []metav1.ManagedFieldsEntry {
	return u.obj.GetManagedFields()
}

func (u *UnstructuredRegistrationObject) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	u.obj.SetManagedFields(managedFields)
}

// runtime.Object methods

func (u *UnstructuredRegistrationObject) GetObjectKind() schema.ObjectKind {
	return u.obj.GetObjectKind()
}

func (u *UnstructuredRegistrationObject) DeepCopyObject() runtime.Object {
	return &UnstructuredRegistrationObject{
		obj:    u.obj.DeepCopy(),
		spec:   &UnstructuredSpec{data: runtime.DeepCopyJSON(u.spec.data)},
		status: &UnstructuredStatus{data: runtime.DeepCopyJSON(u.status.data), obj: u.obj.DeepCopy()},
	}
}

// UnstructuredSpec implements types.ProductRegistrationSpec
type UnstructuredSpec struct {
	data map[string]interface{}
}

func (s *UnstructuredSpec) GetMode() primitives.RegistrationMode {
	mode, _, _ := unstructured.NestedString(s.data, "mode")
	if mode == "" {
		return primitives.RegistrationModeOnline
	}
	return primitives.RegistrationMode(mode)
}

func (s *UnstructuredSpec) GetRegistrationCodeRef() *corev1.SecretReference {
	regReq, found, _ := unstructured.NestedMap(s.data, "registrationRequest")
	if !found {
		return nil
	}

	refMap, found, _ := unstructured.NestedMap(regReq, "registrationCodeSecretRef")
	if !found {
		return nil
	}

	ref := &corev1.SecretReference{}
	name, _, _ := unstructured.NestedString(refMap, "name")
	namespace, _, _ := unstructured.NestedString(refMap, "namespace")
	ref.Name = name
	ref.Namespace = namespace

	if ref.Name == "" {
		return nil
	}

	return ref
}

func (s *UnstructuredSpec) GetRegistrationURL() *string {
	regReq, found, _ := unstructured.NestedMap(s.data, "registrationRequest")
	if !found {
		return nil
	}

	url, found, _ := unstructured.NestedString(regReq, "registrationAPIUrl")
	if !found {
		return nil
	}

	return &url
}

func (s *UnstructuredSpec) GetRegistrationURLCertRef() *corev1.SecretReference {
	regReq, found, _ := unstructured.NestedMap(s.data, "registrationRequest")
	if !found {
		return nil
	}

	refMap, found, _ := unstructured.NestedMap(regReq, "registrationAPICertificateSecretRef")
	if !found {
		return nil
	}

	ref := &corev1.SecretReference{}
	name, _, _ := unstructured.NestedString(refMap, "name")
	namespace, _, _ := unstructured.NestedString(refMap, "namespace")
	ref.Name = name
	ref.Namespace = namespace

	if ref.Name == "" {
		return nil
	}

	return ref
}

func (s *UnstructuredSpec) GetOfflineCertificateRef() *corev1.SecretReference {
	refMap, found, _ := unstructured.NestedMap(s.data, "offlineRegistrationCertificateSecretRef")
	if !found {
		return nil
	}

	ref := &corev1.SecretReference{}
	name, _, _ := unstructured.NestedString(refMap, "name")
	namespace, _, _ := unstructured.NestedString(refMap, "namespace")
	ref.Name = name
	ref.Namespace = namespace

	if ref.Name == "" {
		return nil
	}

	return ref
}

func (s *UnstructuredSpec) GetSyncNow() bool {
	syncNow, found, _ := unstructured.NestedBool(s.data, "syncNow")
	return found && syncNow
}

func (s *UnstructuredSpec) SetSyncNow(syncNow bool) {
	_ = unstructured.SetNestedField(s.data, syncNow, "syncNow")
}

// UnstructuredStatus implements types.ProductRegistrationStatus
type UnstructuredStatus struct {
	data map[string]interface{}
	obj  *unstructured.Unstructured // Need reference to update the parent object
}

// Helper to update the status in the parent object
func (s *UnstructuredStatus) sync() {
	_ = unstructured.SetNestedMap(s.obj.Object, s.data, "status")
}

func (s *UnstructuredStatus) GetConditions() []metav1.Condition {
	conditionsRaw, found, _ := unstructured.NestedSlice(s.data, "conditions")
	if !found {
		return nil
	}

	var conditions []metav1.Condition
	for _, condRaw := range conditionsRaw {
		condMap, ok := condRaw.(map[string]interface{})
		if !ok {
			continue
		}

		cond := metav1.Condition{}
		cond.Type, _, _ = unstructured.NestedString(condMap, "type")
		statusStr, _, _ := unstructured.NestedString(condMap, "status")
		cond.Status = metav1.ConditionStatus(statusStr)
		cond.Reason, _, _ = unstructured.NestedString(condMap, "reason")
		cond.Message, _, _ = unstructured.NestedString(condMap, "message")

		// Parse timestamp
		timestampStr, _, _ := unstructured.NestedString(condMap, "lastTransitionTime")
		if timestampStr != "" {
			cond.LastTransitionTime.UnmarshalQueryParameter(timestampStr)
		}

		conditions = append(conditions, cond)
	}

	return conditions
}

func (s *UnstructuredStatus) SetCondition(condType string, status metav1.ConditionStatus, reason, message string) {
	conditions := s.GetConditions()

	now := metav1.Now()
	newCond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Update existing or append new
	found := false
	for i := range conditions {
		if conditions[i].Type == condType {
			conditions[i] = newCond
			found = true
			break
		}
	}
	if !found {
		conditions = append(conditions, newCond)
	}

	// Convert back to unstructured
	var conditionsRaw []interface{}
	for _, cond := range conditions {
		condMap := map[string]interface{}{
			"type":               cond.Type,
			"status":             string(cond.Status),
			"reason":             cond.Reason,
			"message":            cond.Message,
			"lastTransitionTime": cond.LastTransitionTime.Format("2006-01-02T15:04:05Z"),
		}
		conditionsRaw = append(conditionsRaw, condMap)
	}

	_ = unstructured.SetNestedSlice(s.data, conditionsRaw, "conditions")
	s.sync()
}

func (s *UnstructuredStatus) HasCondition(condType string) bool {
	conditions := s.GetConditions()
	for _, cond := range conditions {
		if cond.Type == condType {
			return true
		}
	}
	return false
}

func (s *UnstructuredStatus) IsConditionTrue(condType string) bool {
	conditions := s.GetConditions()
	for _, cond := range conditions {
		if cond.Type == condType {
			return cond.Status == metav1.ConditionTrue
		}
	}
	return false
}

func (s *UnstructuredStatus) RemoveCondition(condType string) {
	conditions := s.GetConditions()
	var filtered []metav1.Condition
	for _, cond := range conditions {
		if cond.Type != condType {
			filtered = append(filtered, cond)
		}
	}

	// Convert back to unstructured
	var conditionsRaw []interface{}
	for _, cond := range filtered {
		condMap := map[string]interface{}{
			"type":               cond.Type,
			"status":             string(cond.Status),
			"reason":             cond.Reason,
			"message":            cond.Message,
			"lastTransitionTime": cond.LastTransitionTime.Format("2006-01-02T15:04:05Z"),
		}
		conditionsRaw = append(conditionsRaw, condMap)
	}

	_ = unstructured.SetNestedSlice(s.data, conditionsRaw, "conditions")
	s.sync()
}

func (s *UnstructuredStatus) GetSCCSystemID() *int {
	id, found, _ := unstructured.NestedInt64(s.data, "sccSystemID")
	if !found {
		return nil
	}
	idInt := int(id)
	return &idInt
}

func (s *UnstructuredStatus) SetSCCSystemID(id *int) {
	if id == nil {
		unstructured.RemoveNestedField(s.data, "sccSystemID")
	} else {
		_ = unstructured.SetNestedField(s.data, int64(*id), "sccSystemID")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetRegisteredProduct() *string {
	product, found, _ := unstructured.NestedString(s.data, "registeredProduct")
	if !found {
		return nil
	}
	return &product
}

func (s *UnstructuredStatus) SetRegisteredProduct(product *string) {
	if product == nil {
		unstructured.RemoveNestedField(s.data, "registeredProduct")
	} else {
		_ = unstructured.SetNestedField(s.data, *product, "registeredProduct")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetRegistrationProcessedTS() *metav1.Time {
	ts, found, _ := unstructured.NestedString(s.data, "registrationProcessedTS")
	if !found {
		return nil
	}
	t := &metav1.Time{}
	_ = t.UnmarshalQueryParameter(ts)
	return t
}

func (s *UnstructuredStatus) SetRegistrationProcessedTS(ts *metav1.Time) {
	if ts == nil {
		unstructured.RemoveNestedField(s.data, "registrationProcessedTS")
	} else {
		_ = unstructured.SetNestedField(s.data, ts.Format("2006-01-02T15:04:05Z"), "registrationProcessedTS")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetRegistrationExpiresAt() *metav1.Time {
	ts, found, _ := unstructured.NestedString(s.data, "registrationExpiresAt")
	if !found {
		return nil
	}
	t := &metav1.Time{}
	_ = t.UnmarshalQueryParameter(ts)
	return t
}

func (s *UnstructuredStatus) SetRegistrationExpiresAt(ts *metav1.Time) {
	if ts == nil {
		unstructured.RemoveNestedField(s.data, "registrationExpiresAt")
	} else {
		_ = unstructured.SetNestedField(s.data, ts.Format("2006-01-02T15:04:05Z"), "registrationExpiresAt")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetActivated() bool {
	activated, _, _ := unstructured.NestedBool(s.data, "activationStatus", "activated")
	return activated
}

func (s *UnstructuredStatus) SetActivated(activated bool) {
	_ = unstructured.SetNestedField(s.data, activated, "activationStatus", "activated")
	s.sync()
}

func (s *UnstructuredStatus) GetLastValidatedTS() *metav1.Time {
	ts, found, _ := unstructured.NestedString(s.data, "activationStatus", "lastValidatedTS")
	if !found {
		return nil
	}
	t := &metav1.Time{}
	_ = t.UnmarshalQueryParameter(ts)
	return t
}

func (s *UnstructuredStatus) SetLastValidatedTS(ts *metav1.Time) {
	if ts == nil {
		unstructured.RemoveNestedField(s.data, "activationStatus", "lastValidatedTS")
	} else {
		_ = unstructured.SetNestedField(s.data, ts.Format("2006-01-02T15:04:05Z"), "activationStatus", "lastValidatedTS")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetSystemURL() *string {
	url, found, _ := unstructured.NestedString(s.data, "activationStatus", "systemURL")
	if !found {
		return nil
	}
	return &url
}

func (s *UnstructuredStatus) SetSystemURL(url *string) {
	if url == nil {
		unstructured.RemoveNestedField(s.data, "activationStatus", "systemURL")
	} else {
		_ = unstructured.SetNestedField(s.data, *url, "activationStatus", "systemURL")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetSystemCredentialsSecretRef() *corev1.SecretReference {
	refMap, found, _ := unstructured.NestedMap(s.data, "systemCredentialsSecretRef")
	if !found {
		return nil
	}

	ref := &corev1.SecretReference{}
	name, _, _ := unstructured.NestedString(refMap, "name")
	namespace, _, _ := unstructured.NestedString(refMap, "namespace")
	ref.Name = name
	ref.Namespace = namespace

	if ref.Name == "" {
		return nil
	}

	return ref
}

func (s *UnstructuredStatus) SetSystemCredentialsSecretRef(ref *corev1.SecretReference) {
	if ref == nil {
		unstructured.RemoveNestedField(s.data, "systemCredentialsSecretRef")
	} else {
		refMap := map[string]interface{}{
			"name":      ref.Name,
			"namespace": ref.Namespace,
		}
		_ = unstructured.SetNestedMap(s.data, refMap, "systemCredentialsSecretRef")
	}
	s.sync()
}

func (s *UnstructuredStatus) GetOfflineRegistrationRequestRef() *corev1.SecretReference {
	refMap, found, _ := unstructured.NestedMap(s.data, "offlineRegistrationRequest")
	if !found {
		return nil
	}

	ref := &corev1.SecretReference{}
	name, _, _ := unstructured.NestedString(refMap, "name")
	namespace, _, _ := unstructured.NestedString(refMap, "namespace")
	ref.Name = name
	ref.Namespace = namespace

	if ref.Name == "" {
		return nil
	}

	return ref
}

func (s *UnstructuredStatus) SetOfflineRegistrationRequestRef(ref *corev1.SecretReference) {
	if ref == nil {
		unstructured.RemoveNestedField(s.data, "offlineRegistrationRequest")
	} else {
		refMap := map[string]interface{}{
			"name":      ref.Name,
			"namespace": ref.Namespace,
		}
		_ = unstructured.SetNestedMap(s.data, refMap, "offlineRegistrationRequest")
	}
	s.sync()
}

// UnstructuredAdapter helps create and manipulate unstructured ProductRegistration CRs
type UnstructuredAdapter struct {
	group   string
	version string
	kind    string
}

// NewUnstructuredAdapter creates a new adapter for unstructured CRs
func NewUnstructuredAdapter(group, version, kind string) *UnstructuredAdapter {
	return &UnstructuredAdapter{
		group:   group,
		version: version,
		kind:    kind,
	}
}

// New creates a new unstructured object with the correct GVK
func (a *UnstructuredAdapter) New() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   a.group,
		Version: a.version,
		Kind:    a.kind,
	})
	return obj
}

// Get retrieves an unstructured object by name
func (a *UnstructuredAdapter) Get(ctx context.Context, c k8sclient.Client, name string) (*unstructured.Unstructured, error) {
	obj := a.New()
	if err := c.Get(ctx, "", name, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// List retrieves all unstructured objects matching the given options
func (a *UnstructuredAdapter) List(ctx context.Context, c k8sclient.Client, opts ...k8sclient.ListOption) ([]*unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   a.group,
		Version: a.version,
		Kind:    a.kind + "List",
	})

	if err := c.List(ctx, list, opts...); err != nil {
		return nil, err
	}

	var result []*unstructured.Unstructured
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

// SetSpec sets the spec on an unstructured object
func (a *UnstructuredAdapter) SetSpec(obj *unstructured.Unstructured, spec *connectngv1.ProductRegistrationSpec) error {
	// Convert spec to unstructured map
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(spec)
	if err != nil {
		return fmt.Errorf("failed to convert spec to unstructured: %w", err)
	}

	if err := unstructured.SetNestedMap(obj.Object, specMap, "spec"); err != nil {
		return fmt.Errorf("failed to set spec: %w", err)
	}

	return nil
}
