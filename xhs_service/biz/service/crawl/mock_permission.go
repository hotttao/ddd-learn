package crawl

import "context"

// MockPermissionChecker keeps this module focused on JWT authentication.
// Resource authorization is introduced in the following Keto module.
type MockPermissionChecker struct{}

// Check reserves organization "forbidden" as a deterministic denial case so
// the HTTP authorization boundary can be verified before Keto is introduced.
func (MockPermissionChecker) Check(_ context.Context, _, _, object, _ string) (bool, error) {
	return object != "forbidden", nil
}
