package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChartYAMLExists verifies Chart.yaml exists
func TestChartYAMLExists(t *testing.T) {
	root := GetProjectRoot()
	chartPath := filepath.Join(root, "deploy", "helm", "adhive", "Chart.yaml")
	_, err := os.Stat(chartPath)
	if err != nil {
		t.Fatalf("Chart.yaml not found: %v", err)
	}
}

// TestChartYAMLValid verifies Chart.yaml is valid
func TestChartYAMLValid(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "Chart.yaml"))
	if err != nil {
		t.Fatalf("Failed to read Chart.yaml: %v", err)
	}

	// Check for required fields
	requiredFields := []string{
		"apiVersion: v2",
		"name: adhive",
		"version:",
		"appVersion:",
	}

	for _, field := range requiredFields {
		if !strings.Contains(string(content), field) {
			t.Errorf("Chart.yaml missing required field: %s", field)
		}
	}
}

// TestValuesYAMLExists verifies values.yaml exists
func TestValuesYAMLExists(t *testing.T) {
	root := GetProjectRoot()
	valuesPath := filepath.Join(root, "deploy", "helm", "adhive", "values.yaml")
	_, err := os.Stat(valuesPath)
	if err != nil {
		t.Fatalf("values.yaml not found: %v", err)
	}
}

// TestDeploymentTemplateExists verifies deployment.yaml exists
func TestDeploymentTemplateExists(t *testing.T) {
	root := GetProjectRoot()
	deployPath := filepath.Join(root, "deploy", "helm", "adhive", "templates", "deployment.yaml")
	_, err := os.Stat(deployPath)
	if err != nil {
		t.Fatalf("deployment.yaml not found: %v", err)
	}
}

// TestDeploymentTemplateValid verifies deployment.yaml is valid
func TestDeploymentTemplateValid(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "templates", "deployment.yaml"))
	if err != nil {
		t.Fatalf("Failed to read deployment.yaml: %v", err)
	}

	// Check for required fields
	requiredFields := []string{
		"apiVersion: apps/v1",
		"kind: Deployment",
		"name: {{ include",
		"containers:",
		"image:",
		"ports:",
	}

	for _, field := range requiredFields {
		if !strings.Contains(string(content), field) {
			t.Errorf("deployment.yaml missing required field: %s", field)
		}
	}
}

// TestServiceTemplateExists verifies service.yaml exists
func TestServiceTemplateExists(t *testing.T) {
	root := GetProjectRoot()
	servicePath := filepath.Join(root, "deploy", "helm", "adhive", "templates", "service.yaml")
	_, err := os.Stat(servicePath)
	if err != nil {
		t.Fatalf("service.yaml not found: %v", err)
	}
}

// TestIngressTemplateExists verifies ingress.yaml exists
func TestIngressTemplateExists(t *testing.T) {
	root := GetProjectRoot()
	ingressPath := filepath.Join(root, "deploy", "helm", "adhive", "templates", "ingress.yaml")
	_, err := os.Stat(ingressPath)
	if err != nil {
		t.Fatalf("ingress.yaml not found: %v", err)
	}
}

// TestPVCsExist verifies PVC templates exist
func TestPVCsExist(t *testing.T) {
	root := GetProjectRoot()
	templatesDir := filepath.Join(root, "deploy", "helm", "adhive", "templates")

	requiredPVCs := []string{
		"pvc-database.yaml",
		"pvc-archives.yaml",
		"pvc-thumbnails.yaml",
	}

	for _, pvc := range requiredPVCs {
		pvcPath := filepath.Join(templatesDir, pvc)
		_, err := os.Stat(pvcPath)
		if err != nil {
			t.Errorf("%s not found: %v", pvc, err)
		}
	}
}

// TestConfigMapAndSecretExist verifies configmap and secret exist
func TestConfigMapAndSecretExist(t *testing.T) {
	root := GetProjectRoot()
	templatesDir := filepath.Join(root, "deploy", "helm", "adhive", "templates")

	required := []string{
		"configmap.yaml",
		"secret.yaml",
	}

	for _, file := range required {
		filePath := filepath.Join(templatesDir, file)
		_, err := os.Stat(filePath)
		if err != nil {
			t.Errorf("%s not found: %v", file, err)
		}
	}
}

// TestHelmHelpersExist verifies _helpers.tpl exists
func TestHelmHelpersExist(t *testing.T) {
	root := GetProjectRoot()
	helpersPath := filepath.Join(root, "deploy", "helm", "adhive", "templates", "_helpers.tpl")
	_, err := os.Stat(helpersPath)
	if err != nil {
		t.Fatalf("_helpers.tpl not found: %v", err)
	}
}

// TestVolumeMountsInDeployment verifies deployment has volume mounts
func TestVolumeMountsInDeployment(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "templates", "deployment.yaml"))
	if err != nil {
		t.Fatalf("Failed to read deployment.yaml: %v", err)
	}

	// Check for volume mounts
	mounts := []string{
		"volumeMounts:",
		"volumes:",
		"persistentVolumeClaim:",
	}

	for _, mount := range mounts {
		if !strings.Contains(string(content), mount) {
			t.Errorf("deployment.yaml missing: %s", mount)
		}
	}
}

// TestSecurityContext verifies deployment has security context
func TestSecurityContext(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "templates", "deployment.yaml"))
	if err != nil {
		t.Fatalf("Failed to read deployment.yaml: %v", err)
	}

	// Check for security context
	if !strings.Contains(string(content), "securityContext:") {
		t.Error("deployment.yaml missing securityContext")
	}
}

// TestResourcesDefined verifies resources are defined in values
func TestResourcesDefined(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "values.yaml"))
	if err != nil {
		t.Fatalf("Failed to read values.yaml: %v", err)
	}

	// Check for resource limits
	if !strings.Contains(string(content), "resources:") {
		t.Error("values.yaml missing resources section")
	}
}

// TestPersistenceEnabled verifies persistence is enabled by default
func TestPersistenceEnabled(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "adhive", "values.yaml"))
	if err != nil {
		t.Fatalf("Failed to read values.yaml: %v", err)
	}

	if !strings.Contains(string(content), "persistence:") {
		t.Error("values.yaml missing persistence section")
	}
}

// TestGetProjectRoot returns project root for tests
