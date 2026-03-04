package plan

import "fmt"

// validateName checks that name matches the plan/phase naming convention:
// must start with a lowercase letter, end with a lowercase letter or digit,
// contain only lowercase letters, digits, and hyphens, and be at least 2 chars.
func validateName(name string) error {
	if len(name) < 2 || !planNameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q: must match ^[a-z][a-z0-9-]*[a-z0-9]$ (min 2 chars)", name)
	}
	return nil
}
