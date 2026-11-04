package domain

import (
	"errors"
	"github.com/google/uuid"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidContact = errors.New("invalid contact")

type Contact struct {
	ID, ShopID                 uuid.UUID
	FirstName, LastName, Phone string
	CreatedAt                  time.Time
}

func ValidName(value string, optional bool) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n <= 100 && (optional || n > 0) && !strings.ContainsFunc(value, unicode.IsControl)
}

func (c Contact) Name() string { return strings.TrimSpace(c.FirstName + " " + c.LastName) }

// Contact values are private even when accidentally formatted in diagnostics.
func (Contact) String() string       { return "[private contact]" }
func (Contact) GoString() string     { return "[private contact]" }
func (Contact) LogValue() slog.Value { return slog.StringValue("[private contact]") }
