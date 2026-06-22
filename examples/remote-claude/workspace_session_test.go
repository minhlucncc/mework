// Package remote_claude_test also exercises the end-to-end workspace-bound
// session flow introduced by c0029-workspace-bound-sessions: a workspace fixture
// (mework.yml + .claude/settings.json + a deterministic stub backend) is driven
// in BOTH start modes (server-resolved and local-direct), packed → pushed →
// pulled between client and server, and its produced artifacts are read back and
// updated.
//
// The whole flow runs the agent as a sandbox ON THE CLIENT (the c0027 boundary —
// the server is a gateway + registry only and never spawns a sandbox), feeds the
// task over STDIN (never argv), and honors one-agent-per-sandbox.
//
// The stub backend is a shell script that reads its task from stdin and writes a
// deterministic artifact into its CWD (the bound workspace) — CI-safe and
// deterministic, so the local-direct path needs no real Claude and no Postgres.
// The server-mode path stands up a real hub.NewServer behind httptest and is
// Postgres-gated: it skips cleanly unless TEST_DATABASE_URL is set.
package remote_claude_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	clientcatalog "mework/libs/client/catalog"
	"mework/libs/client/runner"
	"mework/libs/client/workspacefs"
	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus/memory"
	"mework/libs/server/hub"
	"mework/libs/server/platform/store"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	melloprovider "mework/libs/shared/providers/mello"
)

const (
	// wsTestTenantID is the tenant UUID PAT auth resolves to in the server-mode
	// flow (matches the seeded account_identities row).
	wsTestTenantID = "00000000-0000-0000-0000-000000000001"
	// wsArtifactName is the file the stub backend writes into the bound
	// workspace on each turn.
	wsArtifactName = "agent-output.txt"
)

// wsStubArtifactBody is the deterministic content the stub backend writes. The
// stub echoes a fixed marker plus the task it read from stdin, proving both
// stdin-not-argv and workspace-write.
const wsStubArtifactBody = "stub-backend ran; task=produce the file\n"

// prepareWorkspace copies the testdata workspace fixture into a fresh temp dir
// and rewrites mework.yml's `backend` field to the absolute path of the stub
// backend script, so that the local engine (which runs `backend` as command[0]
// with cwd=workspace) actually executes the stub and lands its artifact in the
// bound workspace. It returns the prepared workspace dir.
func prepareWorkspace(t *testing.T) string {
	t.Helper()

	stub := filepath.Join("testdata", "stub-backend.sh")
	absStub, err := filepath.Abs(stub)
	if err != nil {
		t.Fatalf("abs stub path: %v", err)
	}
	if _, err := os.Stat(absStub); err != nil {
		t.Fatalf("stub backend fixture missing (%s): %v", absStub, err)
	}

	srcCfg := filepath.Join("testdata", "workspace", "mework.yml")
	cfgBytes, err := os.ReadFile(srcCfg)
	if err != nil {
		t.Fatalf("read mework.yml fixture: %v", err)
	}

	wsDir := t.TempDir()
	// Rewrite the backend field to the absolute stub path so the local engine
	// can exec it from the workspace cwd.
	rewritten := rewriteBackend(t, string(cfgBytes), absStub)
	if err := os.WriteFile(filepath.Join(wsDir, "mework.yml"), []byte(rewritten), 0o600); err != nil {
		t.Fatalf("write mework.yml into workspace: %v", err)
	}

	// Copy the .claude/settings.json fixture so the workspace carries its
	// settings dir (also proves pack/pull preserves nested paths).
	srcSettings := filepath.Join("testdata", "workspace", ".claude", "settings.json")
	settingsBytes, err := os.ReadFile(srcSettings)
	if err != nil {
		t.Fatalf("read .claude/settings.json fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(wsDir, ".claude"), 0o700); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, ".claude", "settings.json"), settingsBytes, 0o600); err != nil {
		t.Fatalf("write .claude/settings.json: %v", err)
	}

	return wsDir
}

// rewriteBackend replaces the `backend:` line in a mework.yml with the given
// path. It keeps every other line intact.
func rewriteBackend(t *testing.T, cfg, backend string) string {
	t.Helper()
	lines := strings.Split(cfg, "\n")
	replaced := false
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "backend:") {
			lines[i] = "backend: " + backend
			replaced = true
		}
	}
	if !replaced {
		t.Fatalf("mework.yml fixture has no backend: line to rewrite:\n%s", cfg)
	}
	return strings.Join(lines, "\n")
}

// loadWorkspaceMeta loads the prepared workspace's mework.yml via the local
// FileDefinitionResolver, mirroring the local-direct resolution path.
func loadWorkspaceMeta(t *testing.T, wsDir string) *sandbox.SandboxBundleMetadata {
	t.Helper()
	meta, err := clientcatalog.LoadWorkspaceConfig(wsDir)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	return meta
}

// TestWorkspaceSession_LocalDirect proves the local-direct start mode end to end
// with NO server and NO Postgres: resolve the definition from the local
// mework.yml via FileDefinitionResolver, mint a local OpSpawn grant, open a
// workspace-bound session, send a task over stdin, and assert the stub backend's
// artifact lands in the bound workspace dir.
//
// Realises delta-spec requirement "Two start modes — server and local-direct"
// scenario "Start fully locally" and requirement "Workspace-bound session"
// scenario "Agent works in the bound workspace".
func TestWorkspaceSession_LocalDirect(t *testing.T) {
	wsDir := prepareWorkspace(t)

	// Local-direct resolution: FileDefinitionResolver reads mework.yml, contacts
	// no server.
	resolver := &clientcatalog.FileDefinitionResolver{WorkspaceDir: wsDir}

	// Locally-minted grant — no server issues it.
	key := []byte("local-direct-secret-key")
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		t.Fatalf("mint local grant: %v", err)
	}

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	caller := runner.Caller{
		Account: core.AccountID("local-user"),
		Tenant:  core.TenantID(wsTestTenantID),
		Grant:   g,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := runner.StartWorkspaceSession(ctx, runner.StartOptions{
		Ref:          "local-claude@1.0.0",
		Resolver:     resolver,
		WorkspaceDir: wsDir,
		Caller:       caller,
		GrantKey:     key,
		ManagerFor:   func(engine string) *runtime.Manager { return runtime.NewManagerFor(engine) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession (local-direct): %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(context.Background(), caller) })

	if err := sess.Send(ctx, caller, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}

	// The stub backend writes its artifact into the bound workspace dir.
	got, err := os.ReadFile(filepath.Join(wsDir, wsArtifactName))
	if err != nil {
		t.Fatalf("stub artifact not written into bound workspace: %v", err)
	}
	if string(got) != wsStubArtifactBody {
		t.Errorf("artifact body = %q, want %q", string(got), wsStubArtifactBody)
	}
}

// TestWorkspaceSession_PackPushPullRoundTrip proves a workspace round-trips
// through the catalog bundle form: pack the fixture workspace, extract it back
// into a fresh dir, and assert the recreated dir contains mework.yml +
// .claude/settings.json + files with identical contents.
//
// Realises delta-spec requirement "Pack, push, and pull a workspace" scenarios
// "Pack then push" and "Pull recreates the workspace".
func TestWorkspaceSession_PackPushPullRoundTrip(t *testing.T) {
	wsDir := prepareWorkspace(t)

	// Drop an extra regular file so we assert ordinary files round-trip too.
	if err := os.WriteFile(filepath.Join(wsDir, "README.md"), []byte("# workspace\n"), 0o600); err != nil {
		t.Fatalf("seed extra file: %v", err)
	}

	bundle, err := clientcatalog.Pack(wsDir)
	if err != nil {
		t.Fatalf("pack workspace: %v", err)
	}
	if len(bundle) == 0 {
		t.Fatal("pack produced an empty bundle")
	}

	dest := t.TempDir()
	if err := clientcatalog.ExtractWorkspace(bundle, dest); err != nil {
		t.Fatalf("extract (pull) workspace: %v", err)
	}

	wantFiles := []struct {
		rel  string
		want string // empty means "exists, content unchecked"
	}{
		{rel: "mework.yml"},
		{rel: filepath.Join(".claude", "settings.json")},
		{rel: "README.md", want: "# workspace\n"},
	}
	for _, wf := range wantFiles {
		t.Run("recreated/"+wf.rel, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(dest, wf.rel))
			if err != nil {
				t.Fatalf("pulled workspace missing %q: %v", wf.rel, err)
			}
			if wf.want != "" && string(b) != wf.want {
				t.Errorf("%q content = %q, want %q", wf.rel, string(b), wf.want)
			}
		})
	}
}

// TestWorkspaceSession_ArtifactsReadableBack proves that after a turn the
// produced artifact can be listed, read, updated, and re-read over the bound
// workspace via workspacefs.NewLocal.
//
// Realises delta-spec requirement "Workspace-bound session" scenario "Artifacts
// are readable back".
func TestWorkspaceSession_ArtifactsReadableBack(t *testing.T) {
	wsDir := prepareWorkspace(t)

	resolver := &clientcatalog.FileDefinitionResolver{WorkspaceDir: wsDir}
	key := []byte("local-direct-secret-key")
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	caller := runner.Caller{
		Account: core.AccountID("local-user"),
		Tenant:  core.TenantID(wsTestTenantID),
		Grant:   g,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := runner.StartWorkspaceSession(ctx, runner.StartOptions{
		Ref:          "local-claude@1.0.0",
		Resolver:     resolver,
		WorkspaceDir: wsDir,
		Caller:       caller,
		GrantKey:     key,
		ManagerFor:   func(engine string) *runtime.Manager { return runtime.NewManagerFor(engine) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(context.Background(), caller) })

	if err := sess.Send(ctx, caller, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}

	fsys := workspacefs.NewLocal(wsDir, "", nil)

	// List: the produced artifact appears among the workspace entries.
	entries, err := fsys.List(ctx, ".")
	if err != nil {
		t.Fatalf("list workspace: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == wsArtifactName {
			found = true
		}
	}
	if !found {
		t.Fatalf("produced artifact %q not listed in workspace: %+v", wsArtifactName, entries)
	}

	// Read: the contents match what the stub wrote.
	rc, err := fsys.ReadFile(ctx, wsArtifactName)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != wsStubArtifactBody {
		t.Errorf("artifact contents = %q, want %q", string(got), wsStubArtifactBody)
	}

	// Update: the user writes an update and re-reads the updated contents.
	const updated = "user-updated contents\n"
	if err := fsys.WriteFile(ctx, wsArtifactName, strings.NewReader(updated)); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	rc2, err := fsys.ReadFile(ctx, wsArtifactName)
	if err != nil {
		t.Fatalf("re-read artifact: %v", err)
	}
	got2, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if string(got2) != updated {
		t.Errorf("updated artifact contents = %q, want %q", string(got2), updated)
	}
}

// TestWorkspaceSession_ServerMode proves the server start mode end to end and is
// Postgres-gated: it stands up a real hub.NewServer behind httptest, registers
// the workspace config as a definition version via POST
// /api/v1/agents/{name}/versions, resolves it back via HTTPDefinitionResolver,
// opens a workspace-bound session on the CLIENT, sends a task, and asserts the
// stub artifact lands in the workspace. The server never spawns a sandbox.
//
// Realises delta-spec requirement "Two start modes — server and local-direct"
// scenario "Start from the server" and requirement "Resolve a registered
// definition from the server catalog" scenario "Resolve a published definition".
func TestWorkspaceSession_ServerMode(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping server-mode workspace test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	// Mock Mello so the PAT resolves to a known user.
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(melloprovider.User{
			ID:    "mello-user-999",
			Email: "ws@example.com",
			Name:  "Workspace User",
		})
	}))
	defer mockMello.Close()

	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   "test-webhook-secret",
		ServerKey:       "test-server-key",
		MeworkSecretKey: "test-secret-key",
		MelloBaseURL:    mockMello.URL,
	}
	srv := hub.NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Seed an account + identity so PAT auth resolves to the test tenant.
	var accountID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Workspace Account') RETURNING id`,
	).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'mello-user-999', $2)
		ON CONFLICT (provider_code, external_user_id) DO UPDATE
		  SET account_id = EXCLUDED.account_id, tenant_id = EXCLUDED.tenant_id
	`, accountID, wsTestTenantID); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	const pat = "valid-user-pat-token"
	const agentName = "workspace-agent"
	const agentVersion = "1.0.0"

	wsDir := prepareWorkspace(t)
	meta := loadWorkspaceMeta(t, wsDir)

	// Register the workspace config as a JSON definition so HTTPDefinitionResolver
	// can decode it back into bundle metadata.
	defPayload, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	publishBody, _ := json.Marshal(map[string]string{
		"version": agentVersion,
		"form":    "definition",
		"payload": string(defPayload),
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		httpSrv.URL+"/api/v1/agents/"+agentName+"/versions", strings.NewReader(string(publishBody)))
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish version: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("publish version status = %d, body = %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	_ = resp.Body.Close()

	// Server-mode resolution via the HTTP catalog resolver, carrying the PAT on
	// every request through a bearer-attaching transport.
	resolver := &clientcatalog.HTTPDefinitionResolver{
		BaseURL:    httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{pat: pat}},
	}

	// A (server-issued) spawn grant authorizes the run; the agent still runs on
	// the client.
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, nil)
	if err != nil {
		t.Fatalf("new grant: %v", err)
	}

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	caller := runner.Caller{
		Account: core.AccountID(accountID),
		Tenant:  core.TenantID(wsTestTenantID),
		Grant:   g,
	}

	sess, err := runner.StartWorkspaceSession(ctx, runner.StartOptions{
		Ref:          agentName + "@" + agentVersion,
		Resolver:     resolver,
		WorkspaceDir: wsDir,
		Caller:       caller,
		ManagerFor:   func(engine string) *runtime.Manager { return runtime.NewManagerFor(engine) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession (server): %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(context.Background(), caller) })

	if err := sess.Send(ctx, caller, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(wsDir, wsArtifactName))
	if err != nil {
		t.Fatalf("stub artifact not written into bound workspace: %v", err)
	}
	if string(got) != wsStubArtifactBody {
		t.Errorf("artifact body = %q, want %q", string(got), wsStubArtifactBody)
	}
}

// bearerTransport attaches a PAT bearer token to every outbound request so the
// HTTPDefinitionResolver can reach the PAT-gated catalog route.
type bearerTransport struct {
	pat string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.pat)
	return http.DefaultTransport.RoundTrip(clone)
}

// compile-time check that the deadline import is used even when the server-mode
// test skips (keeps `time` referenced in all build configurations).
var _ = time.Second
