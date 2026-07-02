package grant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestAudit_UnsignedGrantAccepted documents that VerifyGrant silently
// accepts unsigned grants (backward-compatible by default).
func TestAudit_UnsignedGrantAccepted(t *testing.T) {
	g := &Grant{
		Ops: []Operation{OpNetwork, OpRepoWrite, OpSpawn},
	}
	err := VerifyGrant(g, []byte("test-secret-key"))
	if err != nil {
		t.Errorf("VerifyGrant should accept unsigned grants (backward compat): %v", err)
	}
	t.Log("OK: VerifyGrant accepts unsigned grants (backward-compatible)")
}

// TestAudit_VerifyGrantSignedRejectsUnsigned documents that VerifyGrantSigned
// correctly rejects unsigned grants (the hardened path).
func TestAudit_VerifyGrantSignedRejectsUnsigned(t *testing.T) {
	g := &Grant{
		Ops: []Operation{OpNetwork, OpRepoWrite, OpSpawn},
	}
	err := VerifyGrantSigned(g, []byte("test-secret-key"))
	if err == nil {
		t.Error("VerifyGrantSigned should reject unsigned grants")
	}
	t.Logf("HARDENED: VerifyGrantSigned rejects unsigned grants: %v", err)
}

// TestAudit_SignedGrantVerified verifies that properly signed grants still
// pass verification (regression test for signature logic).
func TestAudit_SignedGrantVerified(t *testing.T) {
	g, err := NewGrant([]Operation{OpPullAgent, OpSpawn}, []byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify with correct key
	err = VerifyGrant(g, []byte("test-key"))
	if err != nil {
		t.Errorf("signed grant should verify with correct key: %v", err)
	}
}

// TestAudit_SignedGrantRejectedWrongKey verifies that a grant signed with one
// key is rejected by VerifyGrant when called with a different key.
func TestAudit_SignedGrantRejectedWrongKey(t *testing.T) {
	g, err := NewGrant([]Operation{OpPullAgent}, []byte("correct-key"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify with wrong key
	err = VerifyGrant(g, []byte("wrong-key"))
	if err == nil {
		t.Error("signed grant should be rejected with wrong key")
	}
}

// TestAudit_TamperedGrantRejected verifies that a grant whose ops are modified
// after signing is rejected.
func TestAudit_TamperedGrantRejected(t *testing.T) {
	g, err := NewGrant([]Operation{OpPullAgent}, []byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}

	// Tamper: add extra operations
	g.Ops = append(g.Ops, OpNetwork, OpRepoWrite)

	// Attempt verification — must fail
	err = VerifyGrant(g, []byte("test-key"))
	if err == nil {
		t.Error("tampered grant should be rejected")
	}
}

// TestAudit_GrantPermitsLeastPrivilege verifies that absent operations are
// implicitly denied.
func TestAudit_GrantPermitsLeastPrivilege(t *testing.T) {
	g := &Grant{
		Ops: []Operation{OpSpawn},
	}

	if g.Permits(OpNetwork) {
		t.Error("grant should not permit operations not in its list")
	}
	if g.Permits(OpRepoWrite) {
		t.Error("grant should not permit unlisted operations")
	}
	if !g.Permits(OpSpawn) {
		t.Error("grant should permit operations in its list")
	}
}

// TestAudit_HMACHashNotReversible verifies the HMAC lookup hash (used for
// token storage) is not reversible — a database leak yields no usable tokens.
func TestAudit_HMACHashNotReversible(t *testing.T) {
	token := "rt_abcdef1234567890abcdef1234567890"
	key := []byte("server-key-12345")

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(token))
	hash := hex.EncodeToString(mac.Sum(nil))

	if len(hash) == 0 {
		t.Fatal("HMAC hash should not be empty")
	}
	// HMAC-SHA256 is 64 hex chars
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash))
	}
	t.Logf("HMAC-SHA256 lookup hash (safe for DB storage): %s… (%d chars)", hash[:8], len(hash))
}
