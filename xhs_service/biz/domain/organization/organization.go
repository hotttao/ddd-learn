package organization

import "context"

type Membership struct {
	OrganizationID string
	Role           string
}

type MembershipReader interface {
	ListMemberships(ctx context.Context, subject string) ([]Membership, error)
}
