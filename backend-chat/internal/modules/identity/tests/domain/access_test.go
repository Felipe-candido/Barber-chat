package domain_test

import (
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
)

func TestSimpleAccess(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleOwner, domain.RoleBarber} {
		if !role.Valid() {
			t.Fatalf("valid role rejected: %s", role)
		}
	}
	for _, role := range []domain.Role{"", "admin", "manager", "OWNER"} {
		if role.Valid() {
			t.Fatalf("unknown role accepted: %s", role)
		}
	}
}
