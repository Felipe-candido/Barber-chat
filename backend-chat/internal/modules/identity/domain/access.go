package domain

// Roles are fixed product rules, not user-configurable permission sets.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleBarber Role = "barber"
)

func (r Role) Valid() bool { return r == RoleOwner || r == RoleBarber }
