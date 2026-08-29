package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignerAndJWKSVerifier(t *testing.T) {
	signer, err := NewEphemeralSigner("https://identity.test", "internal-api", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(signer.PublicJWKS()); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	raw, _, err := signer.Issue(Subject{
		ID: "alice-id", PrincipalType: "user", TenantID: "org-g", SessionID: "session-1", AAL: "aal1",
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewJWKSVerifier(server.URL, "https://identity.test", "internal-api", server.Client())
	claims, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "alice-id" || claims.TenantID != "org-g" || claims.PrincipalType != "user" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestVerifierRejectsWrongAudience(t *testing.T) {
	signer, err := NewEphemeralSigner("https://identity.test", "internal-api", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := signer.Issue(Subject{ID: "alice-id", PrincipalType: "user"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := parseToken(raw, "https://identity.test", "other-api", func(_ *jwt.Token) (any, error) {
		return &signer.privateKey.PublicKey, nil
	})
	if err == nil || claims != nil {
		t.Fatal("expected audience validation failure")
	}
}
