package organization

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/xhs_service/biz/domain/organization"
)

func TestListMineGroupsAndSortsMemberships(t *testing.T) {
	reader := &fakeMembershipReader{memberships: []domain.Membership{
		{OrganizationID: "W", Role: "members"},
		{OrganizationID: "G", Role: "members"},
		{OrganizationID: "G", Role: "admins"},
		{OrganizationID: "G", Role: "admins"},
	}}
	organizations, err := New(reader).ListMine(context.Background(), "identity-id")
	if err != nil {
		t.Fatalf("ListMine() error = %v", err)
	}
	if reader.subject != "User:identity-id" {
		t.Fatalf("subject = %q, want User:identity-id", reader.subject)
	}
	if len(organizations) != 2 || organizations[0].ID != "G" || organizations[1].ID != "W" {
		t.Fatalf("organizations = %#v", organizations)
	}
	if got := organizations[0].Roles; len(got) != 2 || got[0] != "admins" || got[1] != "members" {
		t.Fatalf("G roles = %#v", got)
	}
}

func TestListMineFailsClosed(t *testing.T) {
	reader := &fakeMembershipReader{err: errors.New("Keto down")}
	if _, err := New(reader).ListMine(context.Background(), "identity-id"); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("ListMine() error = %v, want ErrDependencyUnavailable", err)
	}
}

type fakeMembershipReader struct {
	memberships []domain.Membership
	err         error
	subject     string
}

func (f *fakeMembershipReader) ListMemberships(_ context.Context, subject string) ([]domain.Membership, error) {
	f.subject = subject
	return f.memberships, f.err
}
