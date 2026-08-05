package reconciler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"maps"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	connectngv1 "github.com/SUSE/connect-ng/k8s/api/v1"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/lifecycle"
	"github.com/SUSE/connect-ng/k8s/reconciler/salt"
)

// EntrypointReconciler watches the entrypoint secret and creates/updates ProductRegistration CRs.
// Based on SCC Operator's entrypoint reconciliation pattern in resources.go.
type EntrypointReconciler struct {
	client k8sclient.Client
	Scheme *runtime.Scheme

	// ProductGroup is the API group for the product (e.g., "rancher.registration.suse.com")
	ProductGroup string

	// EntrypointSecretName is the name of the secret to watch
	EntrypointSecretName string

	// EntrypointNamespace is the namespace of the entrypoint secret
	EntrypointNamespace string

	// ProductName is used for naming child resources and managed-by labels
	ProductName string

	// ProductIdentifier is the SCC product identifier for labeling (e.g., "rancher", "harvester")
	ProductIdentifier string
}

// Reconcile handles entrypoint secret changes and creates/updates ProductRegistration CRs.
// Follows SCC Operator's pattern:
// 1. Extract params from entrypoint secret (with salt-based hashing)
// 2. Create/update child secrets (regCode, offlineCert, urlCert) with finalizers
// 3. Create/update ProductRegistration CR with finalizer
// 4. Handle hash changes (cleanup old resources)
func (r *EntrypointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Only reconcile the specific entrypoint secret
	if req.Name != r.EntrypointSecretName || req.Namespace != r.EntrypointNamespace {
		return ctrl.Result{}, nil
	}

	var secret corev1.Secret
	if err := r.client.Get(ctx, req.Namespace, req.Name, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret deleted - nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !secret.DeletionTimestamp.IsZero() {
		if containsString(secret.Finalizers, primitives.FinalizerRegistration) {
			// Run cleanup - delete all managed resources
			log.Info("entrypoint secret being deleted, cleaning up managed resources")
			if err := r.cleanupManagedResources(ctx); err != nil {
				log.Error(err, "failed to cleanup managed resources")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			secret.Finalizers = removeString(secret.Finalizers, primitives.FinalizerRegistration)
			if err := r.client.Update(ctx, &secret); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsString(secret.Finalizers, primitives.FinalizerRegistration) {
		secret.Finalizers = append(secret.Finalizers, primitives.FinalizerRegistration)
		if err := r.client.Update(ctx, &secret); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure salt exists on entrypoint secret
	preparedSecret, err := r.ensureSalt(ctx, &secret)
	if err != nil {
		log.Error(err, "failed to ensure salt on entrypoint secret")
		return ctrl.Result{}, err
	}
	secret = *preparedSecret

	// Extract parameters from secret (with proper hashing)
	params, err := r.extractRegistrationParams(&secret)
	if err != nil {
		log.Error(err, "failed to extract registration params from entrypoint secret")
		return ctrl.Result{}, err
	}

	// Get existing hashes from secret labels
	existingNameHash := secret.Labels[primitives.LabelNameSuffix]
	existingContentHash := secret.Labels[primitives.LabelSccHash]

	// Handle name hash changes - cleanup old ProductRegistration
	if existingNameHash != "" && existingNameHash != params.nameHash {
		log.Info("name hash changed, cleaning up old ProductRegistration", "old", existingNameHash, "new", params.nameHash)
		if err := r.cleanupProductRegistrationByHash(ctx, existingNameHash); err != nil {
			log.Error(err, "failed to cleanup ProductRegistration by name hash")
			return ctrl.Result{}, err
		}
	}

	// Handle content hash changes - cleanup old child secrets
	if existingContentHash != "" && existingContentHash != params.contentHash {
		log.Info("content hash changed, cleaning up old child secrets", "old", existingContentHash, "new", params.contentHash)
		if err := r.cleanupSecretsByHash(ctx, existingContentHash); err != nil {
			log.Error(err, "failed to cleanup child secrets by content hash")
			return ctrl.Result{}, err
		}
	}

	// Update entrypoint secret labels with new hashes
	if existingNameHash != params.nameHash || existingContentHash != params.contentHash {
		secretCopy := secret.DeepCopy()
		if secretCopy.Labels == nil {
			secretCopy.Labels = make(map[string]string)
		}
		secretCopy.Labels[primitives.LabelNameSuffix] = params.nameHash
		secretCopy.Labels[primitives.LabelSccHash] = params.contentHash
		if err := r.client.Update(ctx, secretCopy); err != nil {
			log.Error(err, "failed to update entrypoint secret labels")
			return ctrl.Result{}, err
		}
		secret = *secretCopy
	}

	// Create/update child secrets with finalizers (following SCC operator pattern)
	if err := r.reconcileChildSecrets(ctx, params); err != nil {
		log.Error(err, "failed to reconcile child secrets")
		return ctrl.Result{}, err
	}

	// Create/update ProductRegistration CR with finalizer
	if err := r.reconcileProductRegistration(ctx, params); err != nil {
		log.Error(err, "failed to reconcile ProductRegistration")
		return ctrl.Result{}, err
	}

	// Update last-processed annotation on entrypoint secret
	secretCopy := secret.DeepCopy()
	if secretCopy.Annotations == nil {
		secretCopy.Annotations = make(map[string]string)
	}
	secretCopy.Annotations[primitives.AnnotationLastProcessed] = time.Now().Format(time.RFC3339)
	if err := r.client.Update(ctx, secretCopy); err != nil {
		log.Error(err, "failed to update last-processed annotation")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled entrypoint secret")
	return ctrl.Result{}, nil
}

// registrationParams holds extracted data from the entrypoint secret.
// Based on SCC Operator's RegistrationParams.
type registrationParams struct {
	managedByName        string
	productIdentifier    string
	nameHash             string
	contentHash          string
	mode                 primitives.RegistrationMode
	regCode              []byte
	regCodeSecretRef     *corev1.SecretReference
	regURL               string
	regURLCertSet        bool
	hasRegURLCertData    bool
	regURLCertData       *[]byte
	regURLCertSecretRef  *corev1.SecretReference
	hasOfflineCertData   bool
	offlineCertData      *[]byte
	offlineCertSecretRef *corev1.SecretReference
}

// Labels produces the labels to apply to related resources.
// Matches SCC Operator's RegistrationParams.Labels() pattern.
func (p registrationParams) Labels() map[string]string {
	return map[string]string{
		primitives.LabelNameSuffix:       p.nameHash,
		primitives.LabelSccHash:          p.contentHash,
		primitives.LabelSccManagedBy:     p.managedByName + "_" + primitives.ManagedByValueSecretBroker,
		primitives.LabelK8sManagedBy:     p.managedByName,
		primitives.LabelRegistrationType: primitives.LabelRegistrationTypeValue, // "suse.com/type": "registration"
		primitives.LabelCredentialsType:  p.productIdentifier,                   // "suse.com/credentials": "rancher" (or product name)
	}
}

// ensureSalt ensures the entrypoint secret has a salt label for hash generation.
// Uses proper random salt generation from salt package (matches SCC operator).
func (r *EntrypointReconciler) ensureSalt(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	log := logr.FromContextOrDiscard(ctx)

	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}

	// If salt already exists, return
	if _, hasSalt := secret.Labels[primitives.LabelObjectSalt]; hasSalt {
		return secret, nil
	}

	log.Info("generating salt for entrypoint secret", "secret", secret.Name)

	// Generate random 8-character salt (matches SCC operator)
	preparedSecret := secret.DeepCopy()
	generatedSalt := salt.NewSaltGen(nil, nil).GenerateSalt()

	existingLabels := make(map[string]string)
	if objLabels := secret.GetLabels(); objLabels != nil {
		existingLabels = objLabels
	}
	existingLabels[primitives.LabelObjectSalt] = generatedSalt
	preparedSecret.SetLabels(existingLabels)

	if err := r.client.Update(ctx, preparedSecret); err != nil {
		log.Error(err, "failed to apply salt to entrypoint secret")
		return nil, err
	}

	log.Info("added salt to entrypoint secret", "secret", secret.Name, "salt", generatedSalt)
	return preparedSecret, nil
}

// extractRegistrationParams extracts registration parameters from the entrypoint secret.
// Based on SCC Operator's extractRegistrationParamsFromSecret.
func (r *EntrypointReconciler) extractRegistrationParams(secret *corev1.Secret) (*registrationParams, error) {
	log := logr.FromContextOrDiscard(context.Background())

	salt := secret.Labels[primitives.LabelObjectSalt]
	if salt == "" {
		return nil, fmt.Errorf("entrypoint secret missing salt label")
	}

	// Get registration mode (default to online)
	regMode := primitives.RegistrationModeOnline
	regType, ok := secret.Data[primitives.SecretKeyRegistrationType]
	if !ok || len(regType) == 0 {
		log.V(1).Info("secret missing registrationType field, defaulting to online")
	} else {
		regMode = primitives.RegistrationMode(regType)
		if regMode != primitives.RegistrationModeOnline && regMode != primitives.RegistrationModeOffline {
			return nil, fmt.Errorf("invalid registration mode %s", string(regMode))
		}
	}

	// Get registration code (required for online mode)
	regCode, ok := secret.Data[primitives.SecretKeyRegistrationCode]
	if !ok || len(regCode) == 0 {
		if regMode == primitives.RegistrationModeOnline {
			return nil, fmt.Errorf("secret does not have data %s; this is required in online mode", primitives.SecretKeyRegistrationCode)
		}
	}

	// Get offline certificate data
	offlineRegCertData, certOk := secret.Data[primitives.SecretKeyOfflineCertificate]
	hasOfflineCert := certOk && len(offlineRegCertData) > 0

	// Get registration URL and cert for online mode
	hasRegCertField := false
	var regURLBytes, regCertBytes []byte
	regURLString := ""
	if regMode == primitives.RegistrationModeOnline {
		// Get registration URL from secret or env var
		regURLBytes, _ = secret.Data[primitives.SecretKeyRegistrationURL]
		regURLString = string(regURLBytes)
		// Get registration URL cert
		regCertBytes, hasRegCertField = secret.Data[primitives.SecretKeyRegistrationURLCert]
	}

	// Compute hashes (matches SCC operator logic)
	hasher := md5.New()

	// Name hash: salt + regType + regCode + regURL
	nameData := []byte(salt)
	nameData = append(nameData, regType...)
	nameData = append(nameData, regCode...)
	nameData = append(nameData, regURLBytes...)

	if _, err := hasher.Write(nameData); err != nil {
		return nil, fmt.Errorf("failed to hash name data: %v", err)
	}
	nameHash := hex.EncodeToString(hasher.Sum(nil))

	// Content hash: nameData + offlineCertData
	hasher.Reset()
	contentData := append(nameData, offlineRegCertData...)
	if _, err := hasher.Write(contentData); err != nil {
		return nil, fmt.Errorf("failed to hash content data: %v", err)
	}
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	log.V(1).Info("extracted registration params", "nameHash", nameHash, "contentHash", contentHash, "mode", regMode)

	return &registrationParams{
		managedByName:     r.ProductName,
		productIdentifier: r.ProductIdentifier,
		nameHash:          nameHash,
		contentHash:       contentHash,
		mode:              regMode,
		regCode:           regCode,
		regCodeSecretRef: &corev1.SecretReference{
			Name:      primitives.RegistrationCodeSecretName(nameHash),
			Namespace: secret.Namespace,
		},
		hasOfflineCertData: hasOfflineCert,
		offlineCertData:    &offlineRegCertData,
		offlineCertSecretRef: &corev1.SecretReference{
			Name:      primitives.OfflineCertificateSecretName(nameHash),
			Namespace: secret.Namespace,
		},
		regURL:            regURLString,
		regURLCertSet:     hasRegCertField,
		hasRegURLCertData: hasRegCertField && len(regCertBytes) > 0,
		regURLCertData:    &regCertBytes,
		regURLCertSecretRef: &corev1.SecretReference{
			Name:      primitives.RegistrationURLCertificateSecretName(nameHash),
			Namespace: secret.Namespace,
		},
	}, nil
}

// reconcileChildSecrets creates/updates child secrets with proper labels and finalizers.
// Follows SCC Operator's pattern: regCodeFromSecretEntrypoint, offlineCertFromSecretEntrypoint, etc.
func (r *EntrypointReconciler) reconcileChildSecrets(ctx context.Context, params *registrationParams) error {
	// Create/update registration code secret (online mode)
	if params.mode == primitives.RegistrationModeOnline && len(params.regCode) > 0 {
		regCodeSecret, err := r.regCodeSecretFromParams(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to prepare registration code secret: %w", err)
		}
		if err := r.createOrUpdateSecret(ctx, regCodeSecret); err != nil {
			return fmt.Errorf("failed to create/update registration code secret: %w", err)
		}
	}

	// Create/update registration URL cert secret (if provided)
	if params.hasRegURLCertData {
		regURLCertSecret, err := r.regURLCertSecretFromParams(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to prepare registration URL cert secret: %w", err)
		}
		if err := r.createOrUpdateSecret(ctx, regURLCertSecret); err != nil {
			return fmt.Errorf("failed to create/update registration URL cert secret: %w", err)
		}
	}

	// Create/update offline certificate secret (offline mode)
	if params.hasOfflineCertData {
		offlineCertSecret, err := r.offlineCertSecretFromParams(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to prepare offline cert secret: %w", err)
		}
		if err := r.createOrUpdateSecret(ctx, offlineCertSecret); err != nil {
			return fmt.Errorf("failed to create/update offline cert secret: %w", err)
		}
	}

	return nil
}

// regCodeSecretFromParams creates the registration code secret with proper labels and finalizer.
// Based on SCC Operator's regCodeFromSecretEntrypoint.
func (r *EntrypointReconciler) regCodeSecretFromParams(ctx context.Context, params *registrationParams) (*corev1.Secret, error) {
	secretName := params.regCodeSecretRef.Name

	// Try to get existing secret
	regCodeSecret := &corev1.Secret{}
	err := r.client.Get(ctx, params.regCodeSecretRef.Namespace, secretName, regCodeSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// Create new secret
		regCodeSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: params.regCodeSecretRef.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				primitives.SecretKeyRegistrationCode: params.regCode,
			},
		}
	}

	// Apply labels (use maps.Copy pattern from SCC operator)
	if regCodeSecret.Labels == nil {
		regCodeSecret.Labels = map[string]string{}
	}
	defaultLabels := params.Labels()
	defaultLabels[primitives.LabelSecretRole] = string(primitives.SecretRoleRegistrationCode)
	maps.Copy(regCodeSecret.Labels, defaultLabels)

	// Add finalizer
	if !containsString(regCodeSecret.Finalizers, primitives.FinalizerRegistrationCode) {
		regCodeSecret.Finalizers = append(regCodeSecret.Finalizers, primitives.FinalizerRegistrationCode)
	}

	return regCodeSecret, nil
}

// regURLCertSecretFromParams creates the registration URL cert secret with proper labels and finalizer.
// Based on SCC Operator's regURLCertFromSecretEntrypoint.
func (r *EntrypointReconciler) regURLCertSecretFromParams(ctx context.Context, params *registrationParams) (*corev1.Secret, error) {
	secretName := params.regURLCertSecretRef.Name

	// Try to get existing secret
	regURLCertSecret := &corev1.Secret{}
	err := r.client.Get(ctx, params.regURLCertSecretRef.Namespace, secretName, regURLCertSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// Create new secret
		regURLCertSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: params.regURLCertSecretRef.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				primitives.SecretKeyRegistrationURLCert: *params.regURLCertData,
			},
		}
	}

	// Apply labels (use maps.Copy pattern from SCC operator)
	if regURLCertSecret.Labels == nil {
		regURLCertSecret.Labels = map[string]string{}
	}
	defaultLabels := params.Labels()
	defaultLabels[primitives.LabelSecretRole] = string(primitives.SecretRoleRegistrationURLCert)
	maps.Copy(regURLCertSecret.Labels, defaultLabels)

	// Add finalizer
	if !containsString(regURLCertSecret.Finalizers, primitives.FinalizerRegistrationURLCert) {
		regURLCertSecret.Finalizers = append(regURLCertSecret.Finalizers, primitives.FinalizerRegistrationURLCert)
	}

	return regURLCertSecret, nil
}

// offlineCertSecretFromParams creates the offline certificate secret with proper labels and finalizer.
// Based on SCC Operator's offlineCertFromSecretEntrypoint.
func (r *EntrypointReconciler) offlineCertSecretFromParams(ctx context.Context, params *registrationParams) (*corev1.Secret, error) {
	secretName := params.offlineCertSecretRef.Name

	// Try to get existing secret
	offlineCertSecret := &corev1.Secret{}
	err := r.client.Get(ctx, params.offlineCertSecretRef.Namespace, secretName, offlineCertSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// Create new secret
		offlineCertSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: params.offlineCertSecretRef.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				primitives.SecretKeyOfflineCertificate: *params.offlineCertData,
			},
		}
	}

	// Apply labels (use maps.Copy pattern from SCC operator)
	if offlineCertSecret.Labels == nil {
		offlineCertSecret.Labels = map[string]string{}
	}
	defaultLabels := params.Labels()
	defaultLabels[primitives.LabelSecretRole] = string(primitives.SecretRoleOfflineCert)
	maps.Copy(offlineCertSecret.Labels, defaultLabels)

	// Add finalizer
	if !containsString(offlineCertSecret.Finalizers, primitives.FinalizerOfflineCertificate) {
		offlineCertSecret.Finalizers = append(offlineCertSecret.Finalizers, primitives.FinalizerOfflineCertificate)
	}

	return offlineCertSecret, nil
}

// reconcileProductRegistration creates/updates the ProductRegistration CR with finalizer.
// Based on SCC Operator's registrationFromSecretEntrypoint.
func (r *EntrypointReconciler) reconcileProductRegistration(ctx context.Context, params *registrationParams) error {
	// Build spec from params
	spec := connectngv1.ProductRegistrationSpec{
		Mode: params.mode,
	}

	// Set registration request for online mode
	if params.mode == primitives.RegistrationModeOnline {
		spec.RegistrationRequest = &connectngv1.RegistrationRequest{
			RegistrationCodeSecretRef: params.regCodeSecretRef,
		}
		if params.regURL != "" {
			spec.RegistrationRequest.RegistrationAPIUrl = &params.regURL
		}
		if params.hasRegURLCertData {
			spec.RegistrationRequest.RegistrationAPICertificateSecretRef = params.regURLCertSecretRef
		}
	}

	// Set offline certificate for offline mode
	if params.mode == primitives.RegistrationModeOffline && params.hasOfflineCertData {
		spec.OfflineRegistrationCertificateSecretRef = params.offlineCertSecretRef
	}

	// Create ProductRegistration name (matches SCC operator)
	prName := primitives.RegistrationName(params.nameHash)

	adapter := lifecycle.NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	// Try to get existing PR
	existingPR, err := adapter.Get(ctx, r.client, prName)

	if apierrors.IsNotFound(err) {
		// Create new ProductRegistration
		pr := adapter.New()
		pr.SetName(prName)

		// Apply labels
		labels := params.Labels()
		pr.SetLabels(labels)

		if err := adapter.SetSpec(pr, &spec); err != nil {
			return fmt.Errorf("failed to set spec: %w", err)
		}

		// Add finalizer
		pr.SetFinalizers(append(pr.GetFinalizers(), primitives.FinalizerRegistration))

		if err := r.client.Create(ctx, pr); err != nil {
			return fmt.Errorf("failed to create ProductRegistration: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get ProductRegistration: %w", err)
	} else {
		// Update existing
		labels := params.Labels()
		existingPR.SetLabels(labels)

		if err := adapter.SetSpec(existingPR, &spec); err != nil {
			return fmt.Errorf("failed to set spec: %w", err)
		}

		// Ensure finalizer exists
		if !containsString(existingPR.GetFinalizers(), primitives.FinalizerRegistration) {
			existingPR.SetFinalizers(append(existingPR.GetFinalizers(), primitives.FinalizerRegistration))
		}

		if err := r.client.Update(ctx, existingPR); err != nil {
			return fmt.Errorf("failed to update ProductRegistration: %w", err)
		}
	}

	return nil
}

// createOrUpdateSecret creates or updates a secret.
func (r *EntrypointReconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	existing := &corev1.Secret{}
	err := r.client.Get(ctx, secret.Namespace, secret.Name, existing)

	if apierrors.IsNotFound(err) {
		return r.client.Create(ctx, secret)
	} else if err != nil {
		return err
	}

	// Update existing
	existing.Data = secret.Data
	existing.Labels = secret.Labels
	existing.Finalizers = secret.Finalizers
	return r.client.Update(ctx, existing)
}

// cleanupProductRegistrationByHash removes ProductRegistration CR with the given name hash.
func (r *EntrypointReconciler) cleanupProductRegistrationByHash(ctx context.Context, nameHash string) error {
	prName := primitives.RegistrationName(nameHash)

	adapter := lifecycle.NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	pr := adapter.New()
	pr.SetName(prName)

	err := r.client.Delete(ctx, pr)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ProductRegistration %s: %w", prName, err)
	}

	return nil
}

// cleanupSecretsByHash removes child secrets with the given content hash.
func (r *EntrypointReconciler) cleanupSecretsByHash(ctx context.Context, contentHash string) error {
	log := logr.FromContextOrDiscard(ctx)

	// List all secrets with this content hash
	secretList := &corev1.SecretList{}
	listOpts := []k8sclient.ListOption{
		k8sclient.InNamespace(r.EntrypointNamespace),
		k8sclient.MatchingLabels(map[string]string{
			primitives.LabelSccHash: contentHash,
		}),
	}

	if err := r.client.List(ctx, secretList, listOpts...); err != nil {
		return fmt.Errorf("failed to list secrets by content hash: %w", err)
	}

	// Delete all child secrets (but not the entrypoint itself)
	for _, secret := range secretList.Items {
		if secret.Name == r.EntrypointSecretName {
			continue // Don't delete the entrypoint secret itself
		}

		if err := r.client.Delete(ctx, &secret); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %w", secret.Name, err)
		}
		log.Info("deleted child secret due to content hash change", "name", secret.Name)
	}

	return nil
}

// cleanupManagedResources removes all ProductRegistration CRs and child secrets managed by this entrypoint.
func (r *EntrypointReconciler) cleanupManagedResources(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	adapter := lifecycle.NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	// List all ProductRegistrations managed by this entrypoint
	prList, err := adapter.List(ctx, r.client, k8sclient.MatchingLabels(map[string]string{
		primitives.LabelK8sManagedBy: r.ProductName,
	}))
	if err != nil {
		return fmt.Errorf("failed to list ProductRegistrations: %w", err)
	}

	// Delete all managed ProductRegistrations
	for _, pr := range prList {
		if err := r.client.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ProductRegistration %s: %w", pr.GetName(), err)
		}
		log.Info("deleted ProductRegistration during cleanup", "name", pr.GetName())
	}

	// List all child secrets managed by this entrypoint
	secretList := &corev1.SecretList{}
	listOpts := []k8sclient.ListOption{
		k8sclient.InNamespace(r.EntrypointNamespace),
		k8sclient.MatchingLabels(map[string]string{
			primitives.LabelK8sManagedBy: r.ProductName,
		}),
	}

	if err := r.client.List(ctx, secretList, listOpts...); err != nil{
		return fmt.Errorf("failed to list managed secrets: %w", err)
	}

	// Delete all child secrets (but not the entrypoint itself)
	for _, secret := range secretList.Items {
		if secret.Name == r.EntrypointSecretName {
			continue // Don't delete the entrypoint secret itself
		}

		if err := r.client.Delete(ctx, &secret); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %w", secret.Name, err)
		}
		log.Info("deleted child secret during cleanup", "name", secret.Name)
	}

	return nil
}

// Helper functions for finalizer management (reused from reconciler.go)
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *EntrypointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(r)
}
