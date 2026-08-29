package authservice

import (
	"testing"
	"time"

	"github.com/hotttao/ddd-learn/internal/security"
)

func TestOrganizationPermissions(t *testing.T) {
	signer, err := security.NewEphemeralSigner("https://identity.test", "internal-api", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := New(signer, time.Hour)

	tests := []struct {
		username string
		password string
		action   string
		allowed  bool
	}{
		{username: "alice", password: "alice-pass", action: "crawl:start", allowed: true},
		{username: "alice", password: "alice-pass", action: "content:read", allowed: true},
		{username: "alice", password: "alice-pass", action: "keyword:update", allowed: true},
		{username: "bob", password: "bob-pass", action: "crawl:start", allowed: true},
		{username: "bob", password: "bob-pass", action: "content:read", allowed: true},
		{username: "bob", password: "bob-pass", action: "keyword:update", allowed: false},
	}
	for _, test := range tests {
		t.Run(test.username+"_"+test.action, func(t *testing.T) {
			session, err := service.Login(test.username, test.password)
			if err != nil {
				t.Fatal(err)
			}
			raw, _, err := service.IssueInternalToken(session)
			if err != nil {
				t.Fatal(err)
			}
			claims, err := service.VerifyInternalToken(raw)
			if err != nil {
				t.Fatal(err)
			}
			decision := service.Authorize(claims, test.action, DefaultTenantID)
			if decision.Allowed != test.allowed {
				t.Fatalf("allowed=%v, want %v; reason=%s", decision.Allowed, test.allowed, decision.Reason)
			}
		})
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	signer, err := security.NewEphemeralSigner("https://identity.test", "internal-api", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := New(signer, time.Hour)
	if _, err := service.Login("alice", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}
