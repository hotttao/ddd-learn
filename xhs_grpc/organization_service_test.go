package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	organization "media_agent/xhs_grpc/kitex_gen/xhs_service/organization"
)

type fakeMembershipReader struct {
	subject string
	items   []membership
}

func (f *fakeMembershipReader) listMemberships(_ context.Context, subject string) ([]membership, error) {
	f.subject = subject
	return f.items, nil
}

func TestListMyOrganizationsGroupsAndSortsKetoMemberships(t *testing.T) {
	reader := &fakeMembershipReader{items: []membership{
		{organizationID: "G", role: "members"},
		{organizationID: "G", role: "admins"},
		{organizationID: "W", role: "members"},
	}}
	service := &organizationService{memberships: reader}
	ctx := serverjwt.WithPrincipal(context.Background(), serverjwt.Principal{Subject: "alice-id"})

	response, err := service.ListMyOrganizations(ctx, &organization.ListMyOrganizationsRequest{})

	require.NoError(t, err)
	require.Equal(t, "User:alice-id", reader.subject)
	require.Len(t, response.Organizations, 2)
	require.Equal(t, "G", response.Organizations[0].Id)
	require.Equal(t, []string{"admins", "members"}, response.Organizations[0].Roles)
	require.Equal(t, "W", response.Organizations[1].Id)
}

func TestListMyOrganizationsRequiresPrincipal(t *testing.T) {
	service := &organizationService{memberships: &fakeMembershipReader{}}

	_, err := service.ListMyOrganizations(context.Background(), &organization.ListMyOrganizationsRequest{})

	require.ErrorContains(t, err, "unauthenticated")
}
