package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestSkillManagementAPIKeepsFilesystemIngestionHostLocal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	runtime, err := bootstrap.NewRuntime(
		ctx,
		bootstrap.RuntimeOptions{
			DatabasePath: filepath.Join(temp, "mender.db"),
			SkillRoot:    filepath.Join(temp, "managed-skills"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	listed := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/skills",
		nil,
		http.StatusOK,
	)
	installations, ok := listed["installations"].([]any)
	if !ok || len(installations) != 1 {
		t.Fatalf("initial Skills = %#v", listed)
	}
	builtIn, ok := installations[0].(map[string]any)
	if !ok ||
		builtIn["name"] != "mender-tools" ||
		builtIn["sourceKind"] != "builtin" ||
		builtIn["builtIn"] != true ||
		builtIn["trusted"] != true ||
		builtIn["enabled"] != true {
		t.Fatalf("built-in Skill = %#v", builtIn)
	}
	builtInID, _ := builtIn["installationId"].(string)
	if builtInID == "" {
		t.Fatalf("built-in installationId = %#v", builtIn["installationId"])
	}

	inspection := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/skills/"+builtInID,
		nil,
		http.StatusOK,
	)
	if metadata, ok := inspection["metadata"].(map[string]any); !ok ||
		metadata["name"] != "mender-tools" {
		t.Fatalf("built-in inspection = %#v", inspection)
	}

	requestStatus(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills",
		[]byte(`{"sourcePath":"/tmp/untrusted-skill"}`),
		http.StatusMethodNotAllowed,
	)
	requestStatus(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skill-deployments",
		[]byte(`{"root":"/tmp/project"}`),
		http.StatusMethodNotAllowed,
	)
	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/not-an-id/enable",
		nil,
		http.StatusBadRequest,
	)

	source := filepath.Join(temp, "source", "api-skill")
	writeHTTPSkill(t, source)
	managed, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatalf("InstallLocal() error = %v", err)
	}
	managedID := managed.ID().String()

	trusted := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/"+managedID+"/trust",
		nil,
		http.StatusOK,
	)
	trustedInstallation, _ := trusted["installation"].(map[string]any)
	if trustedInstallation["trusted"] != true ||
		trustedInstallation["enabled"] != false {
		t.Fatalf("trusted installation = %#v", trusted)
	}

	enabled := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/"+managedID+"/enable",
		nil,
		http.StatusOK,
	)
	enabledInstallation, _ := enabled["installation"].(map[string]any)
	if enabledInstallation["trusted"] != true ||
		enabledInstallation["enabled"] != true {
		t.Fatalf("enabled installation = %#v", enabled)
	}

	projectRoot := filepath.Join(temp, "project")
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	deployment, err := runtime.Skills.Deploy(
		ctx,
		managed.ID(),
		skilldomain.DeploymentAgentsProject,
		projectRoot,
	)
	if err != nil {
		t.Fatalf("Skills.Deploy() error = %v", err)
	}

	deploymentList := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/skill-deployments",
		nil,
		http.StatusOK,
	)
	deployments, ok := deploymentList["deployments"].([]any)
	if !ok || len(deployments) != 1 {
		t.Fatalf("deployments = %#v", deploymentList)
	}
	deploymentValue, _ := deployments[0].(map[string]any)
	if deploymentValue["deploymentId"] != deployment.ID().String() ||
		deploymentValue["installationId"] != managedID ||
		deploymentValue["drift"] != "current" {
		t.Fatalf("deployment = %#v", deploymentValue)
	}

	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/"+managedID+"/disable",
		nil,
		http.StatusConflict,
	)

	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/skill-deployments/"+deployment.ID().String(),
		http.StatusNoContent,
	)

	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/"+managedID+"/disable",
		nil,
		http.StatusOK,
	)
	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/skills/"+managedID+"/untrust",
		nil,
		http.StatusOK,
	)
	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/skills/"+managedID,
		http.StatusNoContent,
	)

	final := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/skills",
		nil,
		http.StatusOK,
	)
	finalItems, ok := final["installations"].([]any)
	if !ok || len(finalItems) != 1 {
		t.Fatalf("final Skills = %#v", final)
	}
}

func requestStatus(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	body []byte,
	want int,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		context.Background(),
		method,
		url,
		bytesReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf(
			"%s %s status = %d, want %d body=%q",
			method,
			url,
			response.StatusCode,
			want,
			raw,
		)
	}
}

func bytesReader(raw []byte) io.Reader {
	if len(raw) == 0 {
		return nil
	}
	return &staticBytesReader{raw: raw}
}

type staticBytesReader struct {
	raw    []byte
	offset int
}

func (r *staticBytesReader) Read(target []byte) (int, error) {
	if r.offset >= len(r.raw) {
		return 0, io.EOF
	}
	count := copy(target, r.raw[r.offset:])
	r.offset += count
	return count, nil
}

func writeHTTPSkill(t *testing.T, rootPath string) {
	t.Helper()
	if err := os.MkdirAll(rootPath, 0o750); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(
		"SKILL.md",
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(
		"---\n" +
			"name: api-skill\n" +
			"description: HTTP management Skill test.\n" +
			"---\n" +
			"# API Skill\n",
	)
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
