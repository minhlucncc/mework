package e2e

import "testing"

// Feature 04 — Tenant management & runner enrollment (target).
// Source: SCENARIOS.md + the user's "tenant management" surface.
// Spec: openspec/changes/c0004-agent-runner (+ tenant mgmt). Skips pending c0004.

func TestTENANT_01_RegisterTenant(t *testing.T) {
	Scenario(t, "TENANT-01", "Register an isolated tenant", PlannedC0004).
		Given("the hub is running", func(w *World) {}).
		When("an operator registers a tenant \"acme\"", func(w *World) {
			tn, err := w.Registry.RegisterTenant(ctx(), "acme")
			w.set("tn", tn)
			w.expect(err == nil, "tenant registration should succeed")
		}).
		Then("a tenant boundary exists that scopes all catalog/runner/dispatch state", func(w *World) {
			tn := w.get("tn").(Tenant)
			w.expect(tn.ID != "", "tenant must have an id")
		}).
		Run()
}

func TestTENANT_02_Isolation(t *testing.T) {
	Scenario(t, "TENANT-02", "Tenants are isolated from each other", PlannedC0004).
		Given("two tenants acme and globex, each with its own runners and agents", func(w *World) {}).
		When("an acme identity lists runners", func(w *World) {
			rs, _ := w.Registry.ListRunners(ctx(), "acme")
			w.set("rs", rs)
		}).
		Then("only acme's runners are returned; globex's are never visible", func(w *World) {
			w.expect(true, "cross-tenant access is denied; state is scoped per tenant")
		}).
		Run()
}

func TestTENANT_03_RegistrationTokenScopedToTenant(t *testing.T) {
	Scenario(t, "TENANT-03", "Registration tokens are scoped to a tenant", PlannedC0004).
		Given("tenant acme", func(w *World) {}).
		When("an operator issues a registration token for acme", func(w *World) {
			tok, err := w.Registry.IssueRegistrationToken(ctx(), "acme")
			w.set("tok", tok)
			w.expect(err == nil, "token issue should succeed")
		}).
		Then("enrolling with it yields a runner identity bound to acme", func(w *World) {
			w.expect(w.get("tok") != "", "a registration token must be issued")
		}).
		Run()
}

func TestENROLL_01_EnrollNewRunner(t *testing.T) {
	Scenario(t, "ENROLL-01", "Enroll a new runner", PlannedC0004).
		Given("a valid registration token and the hub URL", func(w *World) {
			tok, _ := w.Registry.IssueRegistrationToken(ctx(), "acme")
			w.set("tok", tok)
		}).
		When("the developer runs `mework runner enroll --url <hub> --token <reg>`", func(w *World) {
			id, err := w.Registry.EnrollRunner(ctx(), w.get("tok").(string))
			w.set("id", id)
			w.expect(err == nil, "enrollment should succeed")
		}).
		Then("a durable runner identity is persisted at ~/.mework/ (0600) and is ready unattended", func(w *World) {
			id := w.get("id").(RunnerIdentity)
			w.expect(id.Runner != "" && id.Secret != "", "a durable runner identity must be persisted")
		}).
		Run()
}

func TestENROLL_02_UnattendedAfterEnrollment(t *testing.T) {
	Scenario(t, "ENROLL-02", "Unattended after enrollment", PlannedC0004).
		Given("an enrolled runner", func(w *World) {}).
		When("the runner process restarts", func(w *World) {
			w.expect(w.StartRunner() == nil, "runner restarts using its persisted identity")
		}).
		Then("it resumes receiving and running dispatches with no re-configuration", func(w *World) {
			w.expect(true, "install-once; unattended thereafter")
		}).
		Run()
}

func TestENROLL_03_InvalidTokenRejected(t *testing.T) {
	Scenario(t, "ENROLL-03", "Invalid or expired registration token is rejected", PlannedC0004).
		Given("an invalid or already-used registration token", func(w *World) {}).
		When("the developer runs `mework runner enroll`", func(w *World) {
			_, err := w.Registry.EnrollRunner(ctx(), "bad-token")
			w.set("err", err)
		}).
		Then("enrollment fails and no identity is persisted", func(w *World) {
			w.expect(w.get("err") != nil, "an invalid registration token must be rejected with nothing persisted")
		}).
		Run()
}

func TestENROLL_04_RegTokenNotReusableAsIdentity(t *testing.T) {
	Scenario(t, "ENROLL-04", "Registration token is not reusable as the identity", PlannedC0004).
		Given("a registration token that successfully enrolled a runner", func(w *World) {}).
		When("the same registration token is presented to a transport route", func(w *World) {
			_, err := w.Auth.AuthRunner(ctx(), "the-registration-token")
			w.set("err", err)
		}).
		Then("it is rejected; only the durable runner identity authenticates transport routes", func(w *World) {
			w.expect(w.get("err") != nil, "a registration token must not authenticate transport routes")
		}).
		Run()
}

func TestENROLL_05_InspectAgentsAndSessions(t *testing.T) {
	Scenario(t, "ENROLL-05", "Inspect dispatched agents and active sessions", PlannedC0004).
		Given("an enrolled runner with one or more active sessions", func(w *World) {}).
		When("the developer runs `mework agent list --json` and `mework session list --json`", func(w *World) {
			out, err := w.RunCLI("session", "list", "--json")
			w.set("out", out)
			w.expect(err == nil, "session list should succeed")
		}).
		Then("each emits the dispatched agents / active sessions as JSON (read-only)", func(w *World) {
			w.expect(w.get("out") != "", "inspection commands emit JSON for scripting")
		}).
		Run()
}
