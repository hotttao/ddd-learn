package authservice

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/hotttao/ddd-learn/internal/security"
)

const DefaultTenantID = "org-g"

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidSession     = errors.New("invalid or expired session")
)

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	password string
}

type Session struct {
	ID                          string    `json:"id"`
	Active                      bool      `json:"active"`
	ExpiresAt                   time.Time `json:"expires_at"`
	AuthenticatedAt             time.Time `json:"authenticated_at"`
	AuthenticatorAssuranceLevel string    `json:"authenticator_assurance_level"`
	Identity                    User      `json:"identity"`
	Token                       string    `json:"-"`
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type Service struct {
	signer *security.Signer
	ttl    time.Duration
	now    func() time.Time

	mu        sync.RWMutex
	users     map[string]User
	usersByID map[string]User
	sessions  map[string]Session
}

func New(signer *security.Signer, sessionTTL time.Duration) *Service {
	users := map[string]User{
		"alice": {
			ID: "9f425a8d-7efc-4768-8f23-7647a74fdf13", Username: "alice", TenantID: DefaultTenantID,
			Roles: []string{"member", "admin"}, password: "alice-pass",
		},
		"bob": {
			ID: "71b021ef-1d1f-4631-9095-4f621318ad62", Username: "bob", TenantID: DefaultTenantID,
			Roles: []string{"member"}, password: "bob-pass",
		},
	}
	usersByID := make(map[string]User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}
	return &Service{
		signer: signer, ttl: sessionTTL, now: time.Now,
		users: users, usersByID: usersByID, sessions: map[string]Session{},
	}
}

func (s *Service) Login(username, password string) (Session, error) {
	user, ok := s.users[strings.ToLower(strings.TrimSpace(username))]
	if !ok || user.password != password {
		return Session{}, ErrInvalidCredentials
	}
	now := s.now().UTC()
	session := Session{
		ID: randomToken("session"), Active: true, ExpiresAt: now.Add(s.ttl), AuthenticatedAt: now,
		AuthenticatorAssuranceLevel: "aal1", Identity: publicUser(user), Token: randomToken("ory_st"),
	}
	s.mu.Lock()
	s.sessions[session.Token] = session
	s.mu.Unlock()
	return session, nil
}

func (s *Service) Session(token string) (Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok || !session.Active || !session.ExpiresAt.After(s.now()) {
		return Session{}, ErrInvalidSession
	}
	return session, nil
}

func (s *Service) IssueInternalToken(session Session) (string, time.Time, error) {
	return s.signer.Issue(security.Subject{
		ID: session.Identity.ID, PrincipalType: "user", TenantID: session.Identity.TenantID,
		ClientID: "web-app", SessionID: session.ID, AAL: session.AuthenticatorAssuranceLevel,
	})
}

func (s *Service) VerifyInternalToken(raw string) (*security.Claims, error) {
	return s.signer.Verify(raw)
}

func (s *Service) JWKS() security.JWKS {
	return s.signer.PublicJWKS()
}

func (s *Service) Authorize(claims *security.Claims, action, tenantID string) Decision {
	if tenantID == "" || claims.TenantID != tenantID {
		return Decision{Allowed: false, Reason: "tenant mismatch"}
	}
	if claims.PrincipalType == "service" {
		if hasScope(claims.Scope, action) {
			return Decision{Allowed: true, Reason: "service scope permits action"}
		}
		return Decision{Allowed: false, Reason: "service scope does not permit action"}
	}
	user, ok := s.usersByID[claims.Subject]
	if !ok || user.TenantID != tenantID {
		return Decision{Allowed: false, Reason: "unknown user or organization"}
	}
	switch action {
	case "crawl:start", "content:read":
		return Decision{Allowed: hasRole(user, "member"), Reason: "organization member permission"}
	case "keyword:update":
		return Decision{Allowed: hasRole(user, "admin"), Reason: "organization admin permission"}
	default:
		return Decision{Allowed: false, Reason: "unknown action"}
	}
}

func publicUser(user User) User {
	user.password = ""
	return user
}

func hasRole(user User, wanted string) bool {
	for _, role := range user.Roles {
		if role == wanted {
			return true
		}
	}
	return false
}

func hasScope(scope, wanted string) bool {
	for _, value := range strings.Fields(scope) {
		if value == wanted {
			return true
		}
	}
	return false
}

func randomToken(prefix string) string {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(value)
}
