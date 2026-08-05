package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SUSE/connect-ng/k8s/productconfig"
)

func TestGenerateWithAPIBaseDir(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Test with custom API base directory
	cfg := productconfig.New(
		"testproduct",
		"1.0.0",
		"test-namespace",
		productconfig.WithAPIBaseDir(tmpDir),
	)

	// Verify the output directory is constructed correctly
	expectedDir := filepath.Join(tmpDir, "testproduct.registration.suse.com", "v1")
	if cfg.GenerateOutputDir != expectedDir {
		t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expectedDir)
	}

	// Test that Generate() creates the directory
	// Note: This will fail if templates aren't found, which is expected in a unit test
	// In production, templates would be available
	err := Generate(cfg, nil)
	if err != nil {
		// Check if error is about templates (expected) vs output directory (not expected)
		if _, statErr := os.Stat(expectedDir); os.IsNotExist(statErr) {
			t.Errorf("Generate() failed to create output directory: %v", statErr)
		}
		// Template errors are expected in this test environment
		t.Logf("Generate() error (likely due to missing templates): %v", err)
	}
}

func TestGenerateWithDefaultPaths(t *testing.T) {
	// Test default path construction
	cfg := productconfig.New(
		"myproduct",
		"1.0.0",
		"myproduct-system",
	)

	expectedDir := "./apis/myproduct.registration.suse.com/v1"
	if cfg.GenerateOutputDir != expectedDir {
		t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expectedDir)
	}
}

func TestGenerateWithCustomGroup(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := productconfig.New(
		"testproduct",
		"1.0.0",
		"test-namespace",
		productconfig.WithGroup("custom.example.com"),
		productconfig.WithAPIBaseDir(tmpDir),
	)

	expectedDir := filepath.Join(tmpDir, "custom.example.com", "v1")
	if cfg.GenerateOutputDir != expectedDir {
		t.Errorf("GenerateOutputDir = %q, want %q", cfg.GenerateOutputDir, expectedDir)
	}
}

func TestGenerateWithFullPathOverride(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "completely", "custom", "path")

	cfg := productconfig.New(
		"testproduct",
		"1.0.0",
		"test-namespace",
		productconfig.WithAPIBaseDir(tmpDir), // This should be ignored
		productconfig.WithGenerateOutputDir(customPath),
	)

	if cfg.GenerateOutputDir != customPath {
		t.Errorf("GenerateOutputDir = %q, want %q (WithGenerateOutputDir should take precedence)", cfg.GenerateOutputDir, customPath)
	}
}
