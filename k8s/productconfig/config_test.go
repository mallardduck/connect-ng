package productconfig

import "testing"

func TestNew(t *testing.T) {
	cfg := New("testproduct", "1.0.0", "test-namespace",
		WithMetricsSecret("test-metrics"),
	)

	// Verify required fields are set
	if cfg.Product != "testproduct" {
		t.Errorf("Product = %q, want %q", cfg.Product, "testproduct")
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", cfg.Version, "1.0.0")
	}
	if cfg.Namespace != "test-namespace" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "test-namespace")
	}

	// Verify option was applied
	if cfg.MetricsSecretName != "test-metrics" {
		t.Errorf("MetricsSecretName = %q, want %q", cfg.MetricsSecretName, "test-metrics")
	}

	// Verify defaults were applied
	expectedGroup := "testproduct.registration.suse.com"
	if cfg.Group != expectedGroup {
		t.Errorf("Group = %q, want %q (should be auto-defaulted)", cfg.Group, expectedGroup)
	}

	// Verify MetricsSecretNamespace defaults to Namespace
	if cfg.MetricsSecretNamespace != cfg.Namespace {
		t.Errorf("MetricsSecretNamespace = %q, want %q (should default to Namespace)", cfg.MetricsSecretNamespace, cfg.Namespace)
	}

	expectedOutputDir := "./apis/testproduct.registration.suse.com/v1"
	if cfg.GenerateOutputDir != expectedOutputDir {
		t.Errorf("GenerateOutputDir = %q, want %q (should be auto-defaulted)", cfg.GenerateOutputDir, expectedOutputDir)
	}
}

func TestNewWithAllOptions(t *testing.T) {
	cfg := New("testproduct", "1.0.0", "test-namespace",
		WithGroup("custom.example.com"),
		WithMetricsSecret("custom-metrics"),
		WithMetricsSecretNamespace("custom-metrics-ns"),
		WithGenerateOutputDir("./custom/output"),
	)

	// Verify all options were applied
	if cfg.Group != "custom.example.com" {
		t.Errorf("Group = %q, want %q", cfg.Group, "custom.example.com")
	}
	if cfg.MetricsSecretName != "custom-metrics" {
		t.Errorf("MetricsSecretName = %q, want %q", cfg.MetricsSecretName, "custom-metrics")
	}
	if cfg.MetricsSecretNamespace != "custom-metrics-ns" {
		t.Errorf("MetricsSecretNamespace = %q, want %q", cfg.MetricsSecretNamespace, "custom-metrics-ns")
	}
	if cfg.GenerateOutputDir != "./custom/output" {
		t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, "./custom/output")
	}
}

func TestApplyDefaultsIdempotent(t *testing.T) {
	cfg := New("testproduct", "1.0.0", "test-namespace")

	group1 := cfg.Group
	outputDir1 := cfg.GenerateOutputDir

	// Apply defaults again (simulates what Setup() does)
	cfg.ApplyDefaults()

	// Values should be unchanged
	if cfg.Group != group1 {
		t.Errorf("ApplyDefaults is not idempotent: Group changed from %q to %q", group1, cfg.Group)
	}
	if cfg.GenerateOutputDir != outputDir1 {
		t.Errorf("ApplyDefaults is not idempotent: GenerateOutputDir changed from %q to %q", outputDir1, cfg.GenerateOutputDir)
	}
}

func TestSCCProductIdentifier(t *testing.T) {
	t.Run("defaults to Product", func(t *testing.T) {
		cfg := New("rancher", "2.10.0", "cattle-system")

		// SCCProductIdentifier should default to Product
		if cfg.SCCProductIdentifier != cfg.Product {
			t.Errorf("SCCProductIdentifier = %q, want %q (should default to Product)", cfg.SCCProductIdentifier, cfg.Product)
		}
		if cfg.SCCProductIdentifier != "rancher" {
			t.Errorf("SCCProductIdentifier = %q, want %q", cfg.SCCProductIdentifier, "rancher")
		}
	})

	t.Run("can be overridden for product variants", func(t *testing.T) {
		cfg := New("rancher", "2.10.0", "cattle-system",
			WithSCCProductIdentifier("rancher-prime"),
		)

		// Product should be used for K8s resources
		if cfg.Product != "rancher" {
			t.Errorf("Product = %q, want %q", cfg.Product, "rancher")
		}

		// SCCProductIdentifier should be the variant
		if cfg.SCCProductIdentifier != "rancher-prime" {
			t.Errorf("SCCProductIdentifier = %q, want %q", cfg.SCCProductIdentifier, "rancher-prime")
		}

		// Group should still be based on Product, not SCCProductIdentifier
		expectedGroup := "rancher.registration.suse.com"
		if cfg.Group != expectedGroup {
			t.Errorf("Group = %q, want %q (should use Product, not SCCProductIdentifier)", cfg.Group, expectedGroup)
		}
	})

	t.Run("struct literal with ApplyDefaults", func(t *testing.T) {
		// Backward compatibility: struct literals still work
		cfg := ProductConfig{
			Product:   "neuvector",
			Version:   "5.0.0",
			Namespace: "neuvector-system",
		}
		cfg.ApplyDefaults()

		// SCCProductIdentifier should default to Product
		if cfg.SCCProductIdentifier != "neuvector" {
			t.Errorf("SCCProductIdentifier = %q, want %q (should default after ApplyDefaults)", cfg.SCCProductIdentifier, "neuvector")
		}
	})
}

func TestAPIBaseDir(t *testing.T) {
	t.Run("default uses ./apis", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace")

		expected := "./apis/testproduct.registration.suse.com/v1"
		if cfg.GenerateOutputDir != expected {
			t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expected)
		}
	})

	t.Run("WithAPIBaseDir constructs full path", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace",
			WithAPIBaseDir("./pkg/apis"),
		)

		expected := "./pkg/apis/testproduct.registration.suse.com/v1"
		if cfg.GenerateOutputDir != expected {
			t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expected)
		}
	})

	t.Run("WithAPIBaseDir with custom group", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace",
			WithGroup("custom.example.com"),
			WithAPIBaseDir("./internal/apis"),
		)

		expected := "./internal/apis/custom.example.com/v1"
		if cfg.GenerateOutputDir != expected {
			t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expected)
		}
	})

	t.Run("WithGenerateOutputDir provides full control", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace",
			WithGenerateOutputDir("./completely/custom/path"),
		)

		expected := "./completely/custom/path"
		if cfg.GenerateOutputDir != expected {
			t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expected)
		}
	})

	t.Run("WithGenerateOutputDir takes precedence over WithAPIBaseDir", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace",
			WithAPIBaseDir("./pkg/apis"),
			WithGenerateOutputDir("./override/path"),
		)

		// GenerateOutputDir should win
		expected := "./override/path"
		if cfg.GenerateOutputDir != expected {
			t.Errorf("GenerateOutputDir = %q, want %q (should use GenerateOutputDir over APIBaseDir)", cfg.GenerateOutputDir, expected)
		}
	})

	t.Run("APIBaseDir is stored for introspection", func(t *testing.T) {
		cfg := New("testproduct", "1.0.0", "test-namespace",
			WithAPIBaseDir("./pkg/apis"),
		)

		if cfg.APIBaseDir != "./pkg/apis" {
			t.Errorf("APIBaseDir = %q, want %q", cfg.APIBaseDir, "./pkg/apis")
		}
	})
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ProductConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ProductConfig{
				Product:   "test",
				Version:   "1.0.0",
				Namespace: "test-ns",
			},
			wantErr: false,
		},
		{
			name: "missing product",
			cfg: ProductConfig{
				Version:   "1.0.0",
				Namespace: "test-ns",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			cfg: ProductConfig{
				Product:   "test",
				Namespace: "test-ns",
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			cfg: ProductConfig{
				Product: "test",
				Version: "1.0.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
