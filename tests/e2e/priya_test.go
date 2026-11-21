package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// repoRoot finds the repository root by walking up from this source file to go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", filepath.Dir(file))
		}
		dir = parent
	}
}

func TestPriya_DockerBuild(t *testing.T) {
	// Skip cleanly when Docker is missing or its daemon is not running.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	infoCtx, cancelInfo := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelInfo()
	if err := exec.CommandContext(infoCtx, "docker", "info").Run(); err != nil {
		t.Skipf("docker daemon not available: %v", err)
	}

	// Build Docker image and check size
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelBuild()
	cmd := exec.CommandContext(buildCtx, "docker", "build", "-t", "mcp-proxy-e2e-test", ".")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		record(finding{Persona: "priya", Category: "fail", Summary: "Docker build failed", Detail: string(out)})
		t.Fatalf("Docker build failed: %s", string(out))
	}
	record(finding{Persona: "priya", Category: "pass", Summary: "Docker build succeeds"})

	// Check image size
	sizeCmd := exec.Command("docker", "image", "inspect", "mcp-proxy-e2e-test", "--format", "{{.Size}}")
	sizeOut, err := sizeCmd.Output()
	if err != nil {
		t.Logf("Could not inspect image size: %v", err)
	} else {
		t.Logf("Docker image size: %s bytes", strings.TrimSpace(string(sizeOut)))
		record(finding{Persona: "priya", Category: "pass", Summary: fmt.Sprintf("Docker image size: %s bytes", strings.TrimSpace(string(sizeOut)))})
	}

	// Cleanup
	exec.Command("docker", "rmi", "mcp-proxy-e2e-test").Run() //nolint:errcheck
}

func TestPriya_RBACAdminVsReadonly(t *testing.T) {
	mock := mockMCPServer(0)
	defer mock.Close()

	server, auditLog := newE2EPipeline(t, mock.URL)

	// Priya (admin) calls tools/call — should succeed
	resp := sendReq(t, server.URL, toolsCallBody("read_file"), tokenPriyaAdmin)
	body := readBody(t, resp)
	result := decodeJSON(t, body)
	if result["error"] != nil {
		record(finding{Persona: "priya", Category: "fail", Summary: "Admin tools/call denied", Detail: fmt.Sprintf("%v", result["error"])})
		t.Fatalf("Admin should have access: %v", result["error"])
	}
	record(finding{Persona: "priya", Category: "pass", Summary: "Admin (priya) can call tools/call"})

	// Intern (readonly) calls tools/call — should be denied
	resp2 := sendReq(t, server.URL, toolsCallBody("read_file"), tokenPriyaIntern)
	body2 := readBody(t, resp2)
	result2 := decodeJSON(t, body2)
	if result2["error"] == nil {
		record(finding{Persona: "priya", Category: "fail", Summary: "Readonly intern was NOT denied tools/call"})
		t.Fatalf("Readonly should be denied tools/call")
	}

	// Check denial message quality
	errObj, ok := result2["error"].(map[string]interface{})
	if ok {
		msg, _ := errObj["message"].(string)
		if strings.Contains(strings.ToLower(msg), "denied") || strings.Contains(strings.ToLower(msg), "forbidden") || strings.Contains(strings.ToLower(msg), "not allowed") {
			record(finding{Persona: "priya", Category: "pass", Summary: "RBAC denial message is clear", Detail: msg})
		} else {
			record(finding{Persona: "priya", Category: "warning", Summary: "RBAC denial message could be clearer", Detail: msg})
		}
	}
	t.Logf("Intern readonly denied: PASS")

	// Intern can still call tools/list
	resp3 := sendReq(t, server.URL, jsonRPCBody("tools/list", nil), tokenPriyaIntern)
	body3 := readBody(t, resp3)
	result3 := decodeJSON(t, body3)
	if result3["error"] != nil {
		record(finding{Persona: "priya", Category: "fail", Summary: "Readonly denied tools/list — should be allowed", Detail: fmt.Sprintf("%v", result3["error"])})
		t.Errorf("Readonly should be able to list tools")
	} else {
		record(finding{Persona: "priya", Category: "pass", Summary: "Readonly (intern) can call tools/list"})
	}

	// Verify audit shows both users correctly attributed
	entries := auditLog.snapshot()
	adminFound, internFound := false, false
	for _, e := range entries {
		if e.ClientID == "priya" {
			adminFound = true
		}
		if e.ClientID == "intern" {
			internFound = true
		}
	}
	if adminFound && internFound {
		record(finding{Persona: "priya", Category: "pass", Summary: "Audit log correctly attributes requests to different users"})
	} else {
		record(finding{Persona: "priya", Category: "fail", Summary: fmt.Sprintf("Audit attribution incomplete: admin=%v intern=%v", adminFound, internFound)})
	}
}

func TestPriya_LicenseKeyTierExpansion(t *testing.T) {
	// Generate a Pro license and verify it's a valid JWT
	jwt, _ := generateTestLicenseKey(t, "pro")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		record(finding{Persona: "priya", Category: "fail", Summary: "Generated license key is not valid JWT format"})
		t.Fatalf("Expected 3 JWT parts, got %d", len(parts))
	}
	record(finding{Persona: "priya", Category: "pass", Summary: "Pro license key generates as valid JWT"})
	t.Logf("Pro license key generated: %s...%s", jwt[:20], jwt[len(jwt)-10:])
}
