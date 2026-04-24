package docs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHugoBuild runs `hugo --minify` in a temp directory and asserts
// the full set of expected pages are emitted. Guards against:
//   - Theme regressions (build fails silently)
//   - Broken front-matter (page doesn't render)
//   - Missing content files (section landing without children)
func TestHugoBuild(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hugo build failed: %v\noutput:\n%s", err, string(out))
	}

	wantPages := []string{
		"index.html",
		"using-gormes/index.html",
		"using-gormes/quickstart/index.html",
		"using-gormes/install/index.html",
		"using-gormes/tui-mode/index.html",
		"using-gormes/telegram-adapter/index.html",
		"using-gormes/configuration/index.html",
		"using-gormes/wire-doctor/index.html",
		"using-gormes/faq/index.html",
		"building-gormes/index.html",
		"building-gormes/core-systems/index.html",
		"building-gormes/core-systems/learning-loop/index.html",
		"building-gormes/core-systems/memory/index.html",
		"building-gormes/core-systems/tool-execution/index.html",
		"building-gormes/core-systems/gateway/index.html",
		"building-gormes/contract-readiness/index.html",
		"building-gormes/autoloop-handoff/index.html",
		"building-gormes/agent-queue/index.html",
		"building-gormes/next-slices/index.html",
		"building-gormes/blocked-slices/index.html",
		"building-gormes/umbrella-cleanup/index.html",
		"building-gormes/progress-schema/index.html",
		"building-gormes/upstream-lessons/index.html",
		"building-gormes/what-hermes-gets-wrong/index.html",
		"building-gormes/porting-a-subsystem/index.html",
		"building-gormes/testing/index.html",
		"building-gormes/architecture_plan/index.html",
		"building-gormes/gateway-donor-map/index.html",
		"building-gormes/gateway-donor-map/shared-adapter-patterns/index.html",
		"building-gormes/gateway-donor-map/telegram/index.html",
		"building-gormes/gateway-donor-map/discord/index.html",
		"building-gormes/gateway-donor-map/slack/index.html",
		"building-gormes/gateway-donor-map/whatsapp/index.html",
		"building-gormes/gateway-donor-map/matrix/index.html",
		"building-gormes/gateway-donor-map/irc/index.html",
		"building-gormes/gateway-donor-map/line/index.html",
		"building-gormes/gateway-donor-map/onebot/index.html",
		"building-gormes/gateway-donor-map/qq/index.html",
		"building-gormes/gateway-donor-map/wecom/index.html",
		"building-gormes/gateway-donor-map/weixin/index.html",
		"building-gormes/gateway-donor-map/feishu/index.html",
		"building-gormes/gateway-donor-map/dingtalk/index.html",
		"building-gormes/gateway-donor-map/vk/index.html",
		"building-gormes/gateway-donor-map/webhook/index.html",
		"building-gormes/architecture_plan/phase-1-dashboard/index.html",
		"building-gormes/architecture_plan/phase-2-gateway/index.html",
		"building-gormes/architecture_plan/phase-3-memory/index.html",
		"building-gormes/architecture_plan/phase-4-brain-transplant/index.html",
		"building-gormes/architecture_plan/phase-5-final-purge/index.html",
		"building-gormes/architecture_plan/phase-6-learning-loop/index.html",
		"building-gormes/architecture_plan/subsystem-inventory/index.html",
		"building-gormes/architecture_plan/mirror-strategy/index.html",
		"building-gormes/architecture_plan/technology-radar/index.html",
		"building-gormes/architecture_plan/boundaries/index.html",
		"building-gormes/architecture_plan/why-go/index.html",
		"upstream-hermes/index.html",
	}

	for _, p := range wantPages {
		full := filepath.Join(tmp, p)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected built page missing: %s", p)
		}
	}
}

// TestHugoBuild_IndexHasSidebarSections asserts the rendered home
// page contains all three colored sidebar group labels and the
// expected root section links.
func TestHugoBuild_IndexHasSidebarSections(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo build failed: %v\n%s", err, string(out))
	}
	body, err := os.ReadFile(filepath.Join(tmp, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"USING GORMES",
		"BUILDING GORMES",
		"UPSTREAM HERMES",
		"docs-nav-group-label-shipped",
		"docs-nav-group-label-progress",
		"docs-nav-group-label-next",
		`href=/using-gormes/`,
		`href=/building-gormes/`,
		`href=/upstream-hermes/`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("built index.html missing %q", want)
		}
	}
}

func TestHugoBuild_IndexQuickstartUsesCurrentInstallCommand(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo build failed: %v\n%s", err, string(out))
	}

	body, err := os.ReadFile(filepath.Join(tmp, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	if !strings.Contains(text, "curl -fsSL https://gormes.ai/install.sh | sh") {
		t.Fatalf("built index.html missing current install command")
	}
	if strings.Contains(text, "brew install trebuchet/gormes") {
		t.Fatalf("built index.html still contains stale Homebrew install command")
	}
}

func TestDocsDeployWorkflowUsesCloudflarePages(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "deploy-gormes-docs.yml"))
	if err != nil {
		t.Fatalf("read deploy workflow: %v", err)
	}
	text := string(raw)

	wants := []string{
		"name: Deploy docs.gormes.ai",
		"paths:",
		"- 'docs/**'",
		"workflow_dispatch:",
		"Verify homepage content",
		`grep -F "curl -fsSL https://gormes.ai/install.sh | sh" public/index.html >/dev/null`,
		`! grep -F "brew install trebuchet/gormes" public/index.html >/dev/null`,
		"cloudflare/wrangler-action@v3",
		"command: pages project create gormes-docs --production-branch=main",
		"command: pages deploy docs/public --project-name=gormes-docs --branch=main --commit-dirty=true",
		"domain=docs.gormes.ai",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("deploy workflow missing %q", want)
		}
	}
}
