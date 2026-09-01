package organization

import (
	"context"
	"errors"
	"fmt"
	"sort"

	domain "media_agent/xhs_service/biz/domain/organization"
)

var (
	ErrUnauthenticated       = errors.New("unauthenticated")
	ErrDependencyUnavailable = errors.New("authorization dependency unavailable")
)

type Organization struct {
	ID    string
	Roles []string
}

type Service struct {
	memberships domain.MembershipReader
}

func New(memberships domain.MembershipReader) *Service {
	return &Service{memberships: memberships}
}

func (s *Service) ListMine(ctx context.Context, subject string) ([]Organization, error) {
	if subject == "" {
		return nil, ErrUnauthenticated
	}
	memberships, err := s.memberships.ListMemberships(ctx, "User:"+subject)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDependencyUnavailable, err)
	}

	rolesByOrganization := make(map[string]map[string]struct{}, len(memberships))
	for _, membership := range memberships {
		roles := rolesByOrganization[membership.OrganizationID]
		if roles == nil {
			roles = make(map[string]struct{})
			rolesByOrganization[membership.OrganizationID] = roles
		}
		roles[membership.Role] = struct{}{}
	}

	organizations := make([]Organization, 0, len(rolesByOrganization))
	for organizationID, roleSet := range rolesByOrganization {
		roles := make([]string, 0, len(roleSet))
		for role := range roleSet {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		organizations = append(organizations, Organization{ID: organizationID, Roles: roles})
	}
	sort.Slice(organizations, func(i, j int) bool { return organizations[i].ID < organizations[j].ID })
	return organizations, nil
}
