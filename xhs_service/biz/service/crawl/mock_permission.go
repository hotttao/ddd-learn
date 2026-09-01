package crawl

import "context"

// MockPermissionChecker keeps this module focused on JWT authentication.
// Resource authorization is introduced in the following Keto module.
type MockPermissionChecker struct{}

func (MockPermissionChecker) Check(context.Context, string, string, string, string) (bool, error) {
	return true, nil
}
