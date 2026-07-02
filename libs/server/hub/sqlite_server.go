package hub

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"mework/libs/server/platform/store"
	"mework/libs/server/platform/token"
)

// NewSQLiteServer creates a minimal HTTP server backed by SQLite.
// Supports only the endpoints needed for offline enrollment.
// Uses *sql.DB directly — no pgxpool dependency.
func NewSQLiteServer(s *store.Store, cfg *Config, serverKey string) *Server {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer, chimiddleware.RequestSize(maxRequestBytes))

	r.Get("/healthz", jsonOK)
	r.Get("/livez", jsonOK)
	r.Get("/readyz", jsonOK)

	reg := &sqliteReg{db: s.DB(), serverKey: serverKey}
	r.Post("/api/v1/runners/registration-tokens", reg.issueToken)
	r.Post("/api/v1/runners/enroll", reg.enroll)

	return &Server{Router: r, Config: cfg}
}

func jsonOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

type sqliteReg struct {
	db        *sql.DB
	serverKey string
}

func (r *sqliteReg) issueToken(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	tid := body.TenantID
	if tid == "" {
		tid = "00000000-0000-0000-0000-000000000001"
	}

	raw, _ := token.GenerateRandomToken()
	lookup := token.ComputeLookup(raw, r.serverKey)
	_, err := r.db.ExecContext(req.Context(),
		"INSERT INTO registration_tokens (token_lookup, tenant_id) VALUES ($1, $2)",
		lookup, tid)
	if err != nil {
		log.Printf("sqlite-server: insert token: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"token": raw})
}

func (r *sqliteReg) enroll(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	lookup := token.ComputeLookup(body.Token, r.serverKey)
	var tenantID string
	err := r.db.QueryRowContext(req.Context(),
		"SELECT tenant_id FROM registration_tokens WHERE token_lookup = $1",
		lookup).Scan(&tenantID)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	r.db.ExecContext(req.Context(),
		"DELETE FROM registration_tokens WHERE token_lookup = $1", lookup)

	// Create runtime
	rtRaw, _ := token.GenerateRandomToken()
	rtLookup := token.ComputeLookup(rtRaw, r.serverKey)
	acct := "00000000-0000-0000-0000-000000000001"
	r.db.ExecContext(req.Context(),
		"INSERT INTO accounts (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING",
		acct, "offline")

	var rid string
	r.db.QueryRowContext(req.Context(),
		`INSERT INTO runtimes (tenant_id, account_id, code, label, token_lookup)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		tenantID, acct, "offline", "Offline", rtLookup).Scan(&rid)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"runner_id": rid, "secret": rtRaw})
}
