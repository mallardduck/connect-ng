package controller

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	connectngv1 "github.com/SUSE/connect-ng/k8s/api/v1"
)

// EntrypointReconciler watches the entrypoint secret and creates/updates ProductRegistration CRs
type EntrypointReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// ProductGroup is the API group for the product (e.g., "rancher.registration.suse.com")
	ProductGroup string

	// EntrypointSecretName is the name of the secret to watch
	EntrypointSecretName string

	// EntrypointNamespace is the namespace of the entrypoint secret
	EntrypointNamespace string

	// ProductName is used for naming child resources
	ProductName string
}

// Reconcile handles entrypoint secret changes and creates/updates ProductRegistration CRs
func (r *EntrypointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Only reconcile the specific entrypoint secret
	if req.Name != r.EntrypointSecretName || req.Namespace != r.EntrypointNamespace {
		return ctrl.Result{}, nil
	}

	var secret corev1.Secret
	if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret deleted - clean up ProductRegistration CRs
			log.Info("entrypoint secret deleted, cleaning up")
			return r.cleanupProductRegistrations(ctx)
		}
		return ctrl.Result{}, err
	}

	// Ensure salt exists
	if err := r.ensureSalt(ctx, &secret); err != nil {
		log.Error(err, "failed to ensure salt on secret")
		return ctrl.Result{}, err
	}

	// Extract parameters from secret
	params, err := r.extractParams(&secret)
	if err != nil {
		log.Error(err, "failed to extract params from entrypoint secret")
		return ctrl.Result{}, err
	}

	// Get existing hashes from secret labels
	existingNameHash := secret.Labels["scc.suse.com/name-suffix"]
	existingContentHash := secret.Labels["scc.suse.com/scc-hash"]

	// Handle hash changes - cleanup if needed
	if existingNameHash != "" && existingNameHash != params.nameHash {
		log.Info("name hash changed, cleaning up old resources", "old", existingNameHash, "new", params.nameHash)
		if err := r.cleanupByHash(ctx, existingNameHash); err != nil {
			log.Error(err, "failed to cleanup resources by name hash")
			return ctrl.Result{}, err
		}
	}

	if existingContentHash != "" && existingContentHash != params.contentHash {
		log.Info("content hash changed, cleaning up old child secrets", "old", existingContentHash, "new", params.contentHash)
		if err := r.cleanupSecretsByHash(ctx, existingContentHash); err != nil {
			log.Error(err, "failed to cleanup child secrets by content hash")
			return ctrl.Result{}, err
		}
	}

	// Update secret labels with new hashes
	if existingNameHash != params.nameHash || existingContentHash != params.contentHash {
		secretCopy := secret.DeepCopy()
		if secretCopy.Labels == nil {
			secretCopy.Labels = make(map[string]string)
		}
		secretCopy.Labels["scc.suse.com/name-suffix"] = params.nameHash
		secretCopy.Labels["scc.suse.com/scc-hash"] = params.contentHash
		if err := r.Update(ctx, secretCopy); err != nil {
			log.Error(err, "failed to update secret labels")
			return ctrl.Result{}, err
		}
	}

	// Create/update child secrets
	if err := r.reconcileChildSecrets(ctx, &secret, params); err != nil {
		log.Error(err, "failed to reconcile child secrets")
		return ctrl.Result{}, err
	}

	// Create/update ProductRegistration CR
	if err := r.reconcileProductRegistration(ctx, params); err != nil {
		log.Error(err, "failed to reconcile ProductRegistration")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled entrypoint secret")
	return ctrl.Result{}, nil
}

// entrypointParams holds extracted data from the entrypoint secret
type entrypointParams struct {
	nameHash         string
	contentHash      string
	mode             string
	registrationCode []byte
	sccURL           string
	sccURLCert       []byte
	offlineCert      []byte
	namespace        string
	ownerRef         metav1.OwnerReference
}

// extractParams extracts registration parameters from the entrypoint secret
func (r *EntrypointReconciler) extractParams(secret *corev1.Secret) (*entrypointParams, error) {
	// Get registration mode (default to online)
	mode := "online"
	if modeBytes, ok := secret.Data["registrationType"]; ok && len(modeBytes) > 0 {
		mode = string(modeBytes)
		if mode != "online" && mode != "offline" {
			return nil, fmt.Errorf("invalid registrationType: %s (must be 'online' or 'offline')", mode)
		}
	}

	// Get registration code (required for online mode)
	regCode, hasRegCode := secret.Data["registrationCode"]
	if mode == "online" && (!hasRegCode || len(regCode) == 0) {
		return nil, fmt.Errorf("registrationCode is required for online mode")
	}

	// Get optional fields
	sccURL := string(secret.Data["registrationURL"])
	sccURLCert := secret.Data["registrationURLCert"]
	offlineCert := secret.Data["offlineCertificate"]

	// Get salt (or generate if missing)
	salt := secret.Labels["scc.suse.com/object-salt"]
	if salt == "" {
		// TODO: Generate and apply salt label
		salt = "default-salt"
	}

	// Compute hashes
	hasher := md5.New()

	// Name hash: salt + mode + regCode + sccURL
	nameData := []byte(salt)
	nameData = append(nameData, []byte(mode)...)
	nameData = append(nameData, regCode...)
	nameData = append(nameData, []byte(sccURL)...)
	hasher.Write(nameData)
	nameHash := hex.EncodeToString(hasher.Sum(nil))

	// Content hash: nameData + offlineCert
	hasher.Reset()
	contentData := append(nameData, offlineCert...)
	hasher.Write(contentData)
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	// Create owner reference
	ownerRef := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       secret.Name,
		UID:        secret.UID,
	}

	return &entrypointParams{
		nameHash:         nameHash,
		contentHash:      contentHash,
		mode:             mode,
		registrationCode: regCode,
		sccURL:           sccURL,
		sccURLCert:       sccURLCert,
		offlineCert:      offlineCert,
		namespace:        secret.Namespace,
		ownerRef:         ownerRef,
	}, nil
}

// reconcileChildSecrets creates/updates child secrets based on the entrypoint secret
func (r *EntrypointReconciler) reconcileChildSecrets(ctx context.Context, entrypoint *corev1.Secret, params *entrypointParams) error {
	labels := map[string]string{
		"scc.suse.com/name-suffix":     params.nameHash,
		"scc.suse.com/scc-hash":        params.contentHash,
		"app.kubernetes.io/managed-by": r.ProductName + "-registration",
	}

	// Create registration code secret (online mode)
	if params.mode == "online" && len(params.registrationCode) > 0 {
		regCodeSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("registration-code-%s", params.nameHash),
				Namespace:       params.namespace,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{params.ownerRef},
			},
			Data: map[string][]byte{
				"registrationCode": params.registrationCode,
			},
		}
		if err := r.createOrUpdateSecret(ctx, regCodeSecret); err != nil {
			return fmt.Errorf("failed to create/update registration code secret: %w", err)
		}
	}

	// Create SCC URL cert secret (if provided)
	if len(params.sccURLCert) > 0 {
		urlCertSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("registration-url-cert-%s", params.nameHash),
				Namespace:       params.namespace,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{params.ownerRef},
			},
			Data: map[string][]byte{
				"registrationURLCert": params.sccURLCert,
			},
		}
		if err := r.createOrUpdateSecret(ctx, urlCertSecret); err != nil {
			return fmt.Errorf("failed to create/update URL cert secret: %w", err)
		}
	}

	// Create offline certificate secret (offline mode)
	if params.mode == "offline" && len(params.offlineCert) > 0 {
		offlineCertSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("offline-certificate-%s", params.nameHash),
				Namespace:       params.namespace,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{params.ownerRef},
			},
			Data: map[string][]byte{
				"certificate": params.offlineCert,
			},
		}
		if err := r.createOrUpdateSecret(ctx, offlineCertSecret); err != nil {
			return fmt.Errorf("failed to create/update offline cert secret: %w", err)
		}
	}

	return nil
}

// reconcileProductRegistration creates/updates the ProductRegistration CR
func (r *EntrypointReconciler) reconcileProductRegistration(ctx context.Context, params *entrypointParams) error {
	// Create spec
	spec := connectngv1.ProductRegistrationSpec{
		Mode: connectngv1.RegistrationMode(params.mode),
	}

	// Set registration request for online mode
	if params.mode == "online" {
		spec.RegistrationRequest = &connectngv1.RegistrationRequest{
			RegistrationCodeSecretRef: &corev1.SecretReference{
				Name:      fmt.Sprintf("registration-code-%s", params.nameHash),
				Namespace: params.namespace,
			},
		}
		if params.sccURL != "" {
			spec.RegistrationRequest.RegistrationAPIUrl = &params.sccURL
		}
		if len(params.sccURLCert) > 0 {
			spec.RegistrationRequest.RegistrationAPICertificateSecretRef = &corev1.SecretReference{
				Name:      fmt.Sprintf("registration-url-cert-%s", params.nameHash),
				Namespace: params.namespace,
			}
		}
	}

	// Set offline certificate for offline mode
	if params.mode == "offline" && len(params.offlineCert) > 0 {
		spec.OfflineRegistrationCertificateSecretRef = &corev1.SecretReference{
			Name:      fmt.Sprintf("offline-certificate-%s", params.nameHash),
			Namespace: params.namespace,
		}
	}

	// Create ProductRegistration using unstructured adapter
	// We use the unstructured adapter because we don't import the product's typed CRD
	prName := fmt.Sprintf("%s-scc-registration-%s", r.ProductName, params.nameHash)

	adapter := NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	// Try to get existing PR
	existingPR, err := adapter.Get(ctx, r.Client, prName)

	if apierrors.IsNotFound(err) {
		// Create new ProductRegistration
		pr := adapter.New()
		pr.SetName(prName)
		pr.SetLabels(map[string]string{
			"scc.suse.com/name-suffix":     params.nameHash,
			"scc.suse.com/scc-hash":        params.contentHash,
			"app.kubernetes.io/managed-by": r.ProductName + "-registration",
		})
		pr.SetOwnerReferences([]metav1.OwnerReference{params.ownerRef})

		if err := adapter.SetSpec(pr, &spec); err != nil {
			return fmt.Errorf("failed to set spec: %w", err)
		}

		if err := r.Create(ctx, pr); err != nil {
			return fmt.Errorf("failed to create ProductRegistration: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get ProductRegistration: %w", err)
	} else {
		// Update existing
		existingPR.SetLabels(map[string]string{
			"scc.suse.com/name-suffix":     params.nameHash,
			"scc.suse.com/scc-hash":        params.contentHash,
			"app.kubernetes.io/managed-by": r.ProductName + "-registration",
		})

		if err := adapter.SetSpec(existingPR, &spec); err != nil {
			return fmt.Errorf("failed to set spec: %w", err)
		}

		if err := r.Update(ctx, existingPR); err != nil {
			return fmt.Errorf("failed to update ProductRegistration: %w", err)
		}
	}

	return nil
}

// createOrUpdateSecret creates or updates a secret
func (r *EntrypointReconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: secret.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		return r.Create(ctx, secret)
	} else if err != nil {
		return err
	}

	// Update existing
	existing.Data = secret.Data
	existing.Labels = secret.Labels
	return r.Update(ctx, existing)
}

// ensureSalt ensures the entrypoint secret has a salt label for hash generation
func (r *EntrypointReconciler) ensureSalt(ctx context.Context, secret *corev1.Secret) error {
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}

	if _, hasSalt := secret.Labels["scc.suse.com/object-salt"]; hasSalt {
		return nil
	}

	// Generate random salt (8 bytes hex-encoded)
	hasher := md5.New()
	hasher.Write([]byte(secret.Name))
	hasher.Write([]byte(secret.Namespace))
	hasher.Write([]byte(secret.UID))
	salt := hex.EncodeToString(hasher.Sum(nil))[:16]

	secretCopy := secret.DeepCopy()
	secretCopy.Labels["scc.suse.com/object-salt"] = salt
	return r.Update(ctx, secretCopy)
}

// cleanupByHash removes ProductRegistration CR with the given name hash
func (r *EntrypointReconciler) cleanupByHash(ctx context.Context, nameHash string) error {
	// Delete the ProductRegistration with this name hash
	prName := fmt.Sprintf("%s-scc-registration-%s", r.ProductName, nameHash)

	adapter := NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	pr := adapter.New()
	pr.SetName(prName)

	err := r.Delete(ctx, pr)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ProductRegistration %s: %w", prName, err)
	}

	return nil
}

// cleanupSecretsByHash removes child secrets with the given content hash
func (r *EntrypointReconciler) cleanupSecretsByHash(ctx context.Context, contentHash string) error {
	// List all secrets with this content hash
	secretList := &corev1.SecretList{}
	listOpts := []client.ListOption{
		client.InNamespace(r.EntrypointNamespace),
		client.MatchingLabels{
			"scc.suse.com/scc-hash": contentHash,
		},
	}

	if err := r.List(ctx, secretList, listOpts...); err != nil {
		return fmt.Errorf("failed to list secrets by content hash: %w", err)
	}

	// Delete all child secrets (but not the entrypoint itself)
	for _, secret := range secretList.Items {
		if secret.Name == r.EntrypointSecretName {
			continue // Don't delete the entrypoint secret itself
		}

		if err := r.Delete(ctx, &secret); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %w", secret.Name, err)
		}
	}

	return nil
}

// cleanupProductRegistrations removes all ProductRegistration CRs managed by this entrypoint
func (r *EntrypointReconciler) cleanupProductRegistrations(ctx context.Context) (ctrl.Result, error) {
	adapter := NewUnstructuredAdapter(
		r.ProductGroup,
		"v1",
		"ProductRegistration",
	)

	// List all ProductRegistrations managed by this entrypoint
	prList, err := adapter.List(ctx, r.Client, client.MatchingLabels{
		"app.kubernetes.io/managed-by": r.ProductName + "-registration",
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list ProductRegistrations: %w", err)
	}

	// Delete all managed ProductRegistrations
	for _, pr := range prList {
		if err := r.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete ProductRegistration %s: %w", pr.GetName(), err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *EntrypointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(r)
}
