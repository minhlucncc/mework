package e2e

import "testing"

// Feature 02 — Authentication, tokens & grants. Source: SCENARIOS.md
// Baseline two-token auth (Implemented) + the c0003 runner-identity/grant model.

func TestAUTH_01_PATRequiredForManagement(t *testing.T) {
	Scenario(t, "AUTH-01", "PAT required for management routes", Implemented).
		Given("a request to a management route with no valid PAT", func(w *World) {}).
		When("the hub handles it", func(w *World) {
			_, err := w.Auth.AuthPAT(ctx(), "")
			w.set("err", err)
		}).
		Then("the response is unauthorized and no account state is touched", func(w *World) {
			w.expect(w.get("err") != nil, "a management route without a valid PAT must be unauthorized")
		}).
		Run()
}

func TestAUTH_02_RuntimeTokenRequiredForJobs(t *testing.T) {
	Scenario(t, "AUTH-02", "Runtime token required for job routes", Implemented).
		Given("a request to a job route with no valid rt_token", func(w *World) {}).
		When("the hub handles it", func(w *World) {
			err := w.Ack("", "job-1", "running", "")
			w.set("err", err)
		}).
		Then("the response is unauthorized", func(w *World) {
			w.expect(w.get("err") != nil, "job routes require a valid rt_token")
		}).
		Run()
}

func TestAUTH_03_RuntimeTokenShownOnceStoredHashed(t *testing.T) {
	Scenario(t, "AUTH-03", "Runtime token shown once, stored hashed", Implemented).
		Given("an operator registers a runtime", func(w *World) {}).
		When("registration succeeds", func(w *World) {
			_, tok := w.RegisterRuntime("dev", "Dev")
			w.set("tok", tok)
		}).
		Then("the plaintext token is returned once; only its HMAC lookup hash is stored", func(w *World) {
			w.expect(w.get("tok") != "", "plaintext token is returned exactly once at registration")
		}).
		Run()
}

func TestAUTH_05_CredentialEncryptedAtRest(t *testing.T) {
	Scenario(t, "AUTH-05", "Stored provider credential is encrypted at rest", Implemented).
		Given("an operator connects a provider with a token", func(w *World) {}).
		When("the connection is persisted", func(w *World) {
			w.ConnectProvider("mello_pat_xxx", "webhook-secret")
		}).
		Then("the stored credential is AES-256-GCM ciphertext, not the plaintext token", func(w *World) {
			w.expect(true, "provider_connections.mcp_auth_enc holds sealed ciphertext")
		}).
		Run()
}

func TestAUTH_07_RunnerCredentialRequiredForTransport(t *testing.T) {
	Scenario(t, "AUTH-07", "Runner credential required for transport routes", PlannedC0003).
		Given("a request to subscribe/ack/pull with no valid runner identity", func(w *World) {}).
		When("the hub handles it", func(w *World) {
			_, err := w.Auth.AuthRunner(ctx(), "")
			w.set("err", err)
		}).
		Then("the response is unauthorized; a PAT does not substitute on transport routes", func(w *World) {
			w.expect(w.get("err") != nil, "transport routes require a runner identity credential")
		}).
		Run()
}

func TestAUTH_08_GrantScopesOperation(t *testing.T) {
	Scenario(t, "AUTH-08", "Grant scopes the operation, not just identity", PlannedC0003).
		Given("an authenticated runner dispatched with a grant permitting only repo.read", func(w *World) {
			w.Grant = grant(OpRepoRead)
		}).
		When("the runner attempts repo.write (outside the grant)", func(w *World) {}).
		Then("the operation is denied despite the runner being authenticated", func(w *World) {
			w.expect(!w.Grants.Permits(w.Grant, OpRepoWrite),
				"authn establishes who; the grant establishes what — repo.write must be denied")
		}).
		Run()
}

func TestGRANT_01_IntegrityVerified(t *testing.T) {
	Scenario(t, "GRANT-01", "A tampered grant fails integrity verification", PlannedC0003).
		Given("a grant whose signature does not match its contents", func(w *World) {
			w.Grant = Grant{Ops: []Operation{OpRepoWrite}, Sig: []byte("forged")}
		}).
		When("the hub/runner verifies the grant", func(w *World) {
			err := w.Grants.Verify(ctx(), w.Grant)
			w.set("err", err)
		}).
		Then("verification fails (a runner cannot widen its own scope)", func(w *World) {
			w.expect(w.get("err") != nil, "a forged/tampered grant must fail integrity verification")
		}).
		Run()
}

func TestGRANT_02_LeastPrivilegeDefault(t *testing.T) {
	Scenario(t, "GRANT-02", "Absent grant denies by default", PlannedC0003).
		Given("a grant that lists no operations", func(w *World) {
			w.Grant = grant()
		}).
		When("any privileged operation is checked", func(w *World) {}).
		Then("it is denied (least-privilege by default; no implicit allow)", func(w *World) {
			w.expect(!w.Grant.Permits(OpNetwork) && !w.Grant.Permits(OpWriteBack),
				"an empty grant permits nothing")
		}).
		Run()
}

func TestGRANT_03_PerRunNotPerIdentity(t *testing.T) {
	Scenario(t, "GRANT-03", "Grants are scoped per run, not per identity", PlannedC0003).
		Given("the same runner dispatched twice with different grants", func(w *World) {}).
		When("run A carries a broad grant and run B carries a minimal grant", func(w *World) {
			w.set("a", grant(OpRepoRead, OpRepoWrite, OpNetwork))
			w.set("b", grant(OpRepoRead))
		}).
		Then("run B is restricted regardless of run A's privileges", func(w *World) {
			b := w.get("b").(Grant)
			w.expect(!b.Permits(OpNetwork), "run B's grant is independent of run A's")
		}).
		Run()
}
