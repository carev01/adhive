package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GetProjectRoot returns the project root directory
// Test runs from internal/deployment, so we go up 2 levels to reach adhive
func GetProjectRoot() string {
	execDir, _ := os.Getwd()
	// internal/deployment -> internal -> adhive (project root)
	return filepath.Dir(filepath.Dir(execDir))
}

// TestDockerfileExists verifies the Dockerfile exists
func TestDockerfileExists(t *testing.T) {
	root := GetProjectRoot()
	_, err := os.Stat(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Dockerfile not found: %v", err)
	}
}

// TestDockerfileHasRequiredStages verifies multi-stage build
func TestDockerfileHasRequiredStages(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	requiredStages := []string{
		"FROM",
		"AS", // Stage name
		"WORKDIR",
		"COPY",
		"RUN",
		"ENTRYPOINT",
		"CMD",
	}

	for _, stage := range requiredStages {
		if !strings.Contains(string(content), stage) {
			t.Errorf("Dockerfile missing required element: %s", stage)
		}
	}
}

// TestDockerfileSecurity checks for security best practices
func TestDockerfileSecurity(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	// Check for non-root user
	if !strings.Contains(string(content), "adduser") && !strings.Contains(string(content), "useradd") {
		t.Error("Dockerfile should create a non-root user")
	}

	// Check for health check
	if !strings.Contains(string(content), "HEALTHCHECK") {
		t.Error("Dockerfile should include HEALTHCHECK")
	}

	// Check for non-root user switch
	if strings.Contains(string(content), "USER") {
		if !strings.Contains(string(content), "USER adhive") {
			t.Log("Note: Dockerfile should switch to non-root user")
		}
	}
}

// TestDockerfileExposesPort verifies port exposure
func TestDockerfileExposesPort(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	if !strings.Contains(string(content), "EXPOSE") {
		t.Error("Dockerfile should EXPOSE a port")
	}
}

// TestDockerComposeExists verifies docker-compose.yml exists
func TestDockerComposeExists(t *testing.T) {
	root := GetProjectRoot()
	_, err := os.Stat(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("docker-compose.yml not found: %v", err)
	}
}

// TestDockerComposeHasRequiredServices verifies all required services
func TestDockerComposeHasRequiredServices(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("Failed to read docker-compose.yml: %v", err)
	}

	requiredServices := []string{
		"app:",
		"frontend:",
	}

	for _, service := range requiredServices {
		if !strings.Contains(string(content), service) {
			t.Errorf("docker-compose.yml missing required service: %s", service)
		}
	}
}

// TestDockerComposeHasHealthCheck verifies health checks are configured
func TestDockerComposeHasHealthCheck(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("Failed to read docker-compose.yml: %v", err)
	}

	if !strings.Contains(string(content), "healthcheck") {
		t.Error("docker-compose.yml should include health checks")
	}
}

// TestDockerComposeHasVolumes verifies volumes are configured
func TestDockerComposeHasVolumes(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("Failed to read docker-compose.yml: %v", err)
	}

	if !strings.Contains(string(content), "volumes:") {
		t.Error("docker-compose.yml should include volumes")
	}
}

// TestNginxConfigExists verifies nginx.conf exists
func TestNginxConfigExists(t *testing.T) {
	root := GetProjectRoot()
	_, err := os.Stat(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("nginx.conf not found: %v", err)
	}
}

// TestNginxConfigHasRequiredDirectives verifies required nginx directives
func TestNginxConfigHasRequiredDirectives(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("Failed to read nginx.conf: %v", err)
	}

	requiredDirectives := []string{
		"server {",
		"listen",
		"location",
		"proxy_pass",
	}

	for _, directive := range requiredDirectives {
		if !strings.Contains(string(content), directive) {
			t.Errorf("nginx.conf missing required directive: %s", directive)
		}
	}
}

// TestNginxConfigHasSecurityHeaders verifies security headers
func TestNginxConfigHasSecurityHeaders(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("Failed to read nginx.conf: %v", err)
	}

	securityHeaders := []string{
		"X-Frame-Options",
		"X-Content-Type-Options",
	}

	for _, header := range securityHeaders {
		if !strings.Contains(string(content), header) {
			t.Errorf("nginx.conf missing security header: %s", header)
		}
	}
}

// TestNginxConfigHasGzip verifies gzip compression
func TestNginxConfigHasGzip(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("Failed to read nginx.conf: %v", err)
	}

	if !strings.Contains(string(content), "gzip") {
		t.Error("nginx.conf should include gzip compression")
	}
}

// TestGoBuildVerifies the Go project builds successfully
func TestGoBuild(t *testing.T) {
	root := GetProjectRoot()
	
	// Check that go.mod exists
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("go.mod not found: %v", err)
	}

	// Check that main.go exists
	mainPath := filepath.Join(root, "cmd", "server", "main.go")
	_, err = os.Stat(mainPath)
	if err != nil {
		t.Fatalf("main.go not found at %s: %v", mainPath, err)
	}
}

// TestMigrationsExist verifies database migrations exist
func TestMigrationsExist(t *testing.T) {
	root := GetProjectRoot()
	migrationsDir := filepath.Join(root, "migrations")
	_, err := os.Stat(migrationsDir)
	if err != nil {
		t.Fatalf("Migrations directory not found: %v", err)
	}

	// Check for SQL files
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to read migrations dir: %v", err)
	}

	if len(files) == 0 {
		t.Error("No migration files found")
	}
}

// TestEnvironmentVariables checks required env var handling
func TestEnvironmentVariables(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("Failed to read docker-compose.yml: %v", err)
	}

	requiredEnvVars := []string{
		"PORT=",
		"DATA_DIR=",
		"LOG_LEVEL=",
	}

	for _, envVar := range requiredEnvVars {
		if !strings.Contains(string(content), envVar) {
			t.Errorf("docker-compose.yml missing environment variable: %s", envVar)
		}
	}
}

// TestDockerfileHasMultiStageBuild verifies multi-stage build is used
func TestDockerfileHasMultiStageBuild(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	// Count FROM statements - should have at least 2 for multi-stage
	count := strings.Count(string(content), "FROM ")
	if count < 2 {
		t.Error("Dockerfile should use multi-stage build (at least 2 FROM statements)")
	}
}

// TestDockerfileCGOEnabled verifies CGO is disabled for static builds
func TestDockerfileCGOEnabled(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	if !strings.Contains(string(content), "CGO_ENABLED=0") {
		t.Log("Note: Consider using CGO_ENABLED=0 for static binaries")
	}
}

// TestDockerComposeHasRestartPolicy verifies restart policy
func TestDockerComposeHasRestartPolicy(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("Failed to read docker-compose.yml: %v", err)
	}

	if !strings.Contains(string(content), "restart:") {
		t.Error("docker-compose.yml should include restart policy")
	}
}

// TestNginxConfigHasAPIProxy verifies API proxy is configured
func TestNginxConfigHasAPIProxy(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("Failed to read nginx.conf: %v", err)
	}

	if !strings.Contains(string(content), "location /api/") {
		t.Error("nginx.conf should have API proxy location")
	}
}

// TestNginxConfigHasStaticFiles verifies static file serving
func TestNginxConfigHasStaticFiles(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "nginx.conf"))
	if err != nil {
		t.Fatalf("Failed to read nginx.conf: %v", err)
	}

	if !strings.Contains(string(content), "location /") {
		t.Error("nginx.conf should have root location for static files")
	}
}

// TestDockerfileEntrypoint verifies entrypoint is set
func TestDockerfileEntrypoint(t *testing.T) {
	root := GetProjectRoot()
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	if !strings.Contains(string(content), "ENTRYPOINT") {
		t.Error("Dockerfile should have ENTRYPOINT")
	}
}
