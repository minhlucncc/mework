package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/server/registry"
	"mework/internal/store"
)

// harness.go wires the e2e World to the real subsystems so the tenancy scenarios
// execute as live acceptance tests (not Skip). It is the Green counterpart to the
// design stubs in api_test.go: where those panic, the handles here are backed by the
// test Postgres and the real internal/server/registry.Service.
//
// Only the surface the tenancy + auth scenarios drive is wired (World.Registry and
// EnrollInto). The remaining World handles stay nil/stubbed until their own changes
// build them.

const e2eServerKey = "e2e-test-server-key"

// enrollSeq makes every enrolled runner's code unique within the shared account, so
// the runtimes (account_id, code) unique constraint never collides across scenarios.
var enrollSeq atomic.Uint64

// NewWorld builds a live World backed by the test Postgres, or skips when
// TEST_DATABASE_URL is unset (the repo convention for every DB-backed test).
//
// It runs migrations, truncates the tenant-scoped tables for isolation between
// scenarios, seeds one account to own enrolled runners, and wires World.Registry to a
// real registry.Service through a thin adapter that bridges the e2e tenancy types.
func NewWorld(t *testing.T) *World {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed e2e scenario")
	}

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)

	// Truncate tenant-scoped tables in FK-safe order so each scenario starts clean.
	// registration_tokens and tenants are scenario-created, so they are cleared too.
	_, err = pool.Exec(context.Background(),
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM registration_tokens;
		 DELETE FROM accounts;
		 DELETE FROM tenants WHERE id <> '`+registry.DefaultTenantID+`';`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	svc := registry.NewService(pool, e2eServerKey)

	// Seed one account to own every enrolled runner (runtimes.account_id is NOT NULL).
	var accountID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO accounts (name) VALUES ('e2e') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	reg := &registryAdapter{svc: svc, accountID: accountID, tokenTenant: make(map[string]TenantID)}
	return &World{Registry: reg}
}

// EnrollInto registers a runner under the given tenant via a tenant-scoped
// registration token and returns its RunnerID. Because the runner inherits the token's
// tenant by construction, the resulting identity is tenant-bound — the seam the
// isolation scenarios assert against.
func (w *World) EnrollInto(t *testing.T, tenant TenantID, code string) RunnerID {
	t.Helper()

	tok, err := w.Registry.IssueRegistrationToken(ctx(), tenant)
	if err != nil {
		t.Fatalf("IssueRegistrationToken(%s): %v", tenant, err)
	}
	reg := w.Registry.(*registryAdapter)
	id, err := reg.enroll(ctx(), tok, code)
	if err != nil {
		t.Fatalf("EnrollInto(%s, %s): %v", tenant, code, err)
	}
	return id.Runner
}

// registryAdapter satisfies the e2e Registry interface against the real
// registry.Service, bridging the e2e tenancy types (TenantID/RunnerID/Tenant/
// RunnerIdentity) to the service's (*registry.Tenant/*registry.Runtime). All enrolled
// runners share the seeded account; each gets a unique code so the per-account code
// uniqueness constraint holds.
type registryAdapter struct {
	svc       *registry.Service
	accountID string

	mu          sync.Mutex
	tokenTenant map[string]TenantID // raw registration token → its owning tenant
}

func (r *registryAdapter) RegisterTenant(ctx context.Context, name string) (Tenant, error) {
	tn, err := r.svc.RegisterTenant(ctx, name)
	if err != nil {
		return Tenant{}, err
	}
	return Tenant{ID: TenantID(tn.ID), Name: tn.Name}, nil
}

func (r *registryAdapter) IssueRegistrationToken(ctx context.Context, tenant TenantID) (string, error) {
	tok, err := r.svc.IssueRegistrationToken(ctx, registry.Tenant{ID: string(tenant)})
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.tokenTenant[tok] = tenant
	r.mu.Unlock()
	return tok, nil
}

// EnrollRunner enrolls a runner with an auto-generated code; used by scenarios that
// drive the interface directly (TENANT-03) without choosing a code.
func (r *registryAdapter) EnrollRunner(ctx context.Context, regToken string) (RunnerIdentity, error) {
	code := fmt.Sprintf("runner-%d", enrollSeq.Add(1))
	return r.enroll(ctx, regToken, code)
}

// enroll runs the real enrollment, binding the runner to the token's tenant. The
// returned identity's Tenant comes from the persisted runtime, so a token issued for
// tenant A can only ever yield an A-bound runner.
func (r *registryAdapter) enroll(ctx context.Context, regToken, code string) (RunnerIdentity, error) {
	rt, err := r.svc.EnrollRunner(ctx, regToken, r.accountID, code, code)
	if err != nil {
		return RunnerIdentity{}, err
	}
	return RunnerIdentity{
		Runner: RunnerID(rt.ID),
		Tenant: TenantID(rt.TenantID),
		Secret: regToken,
	}, nil
}

func (r *registryAdapter) ListRunners(ctx context.Context, tenant TenantID) ([]RunnerID, error) {
	runtimes, err := r.svc.ListRunners(ctx, registry.Tenant{ID: string(tenant)}, r.accountID)
	if err != nil {
		return nil, err
	}
	ids := make([]RunnerID, 0, len(runtimes))
	for _, rt := range runtimes {
		ids = append(ids, RunnerID(rt.ID))
	}
	return ids, nil
}

func (r *registryAdapter) Presence(ctx context.Context, runner RunnerID) (bool, error) {
	return false, fmt.Errorf("presence not wired in e2e harness")
}
