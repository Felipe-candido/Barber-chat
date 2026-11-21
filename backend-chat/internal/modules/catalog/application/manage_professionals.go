package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

var (
	ErrProfessionalNotFound     = errors.New("professional not found in this shop")
	ErrProfessionalAccessDenied = errors.New("professional owner access required")
	ErrProfessionalChanged      = errors.New("professional settings changed")
	ErrInvalidProfessionalEdit  = errors.New("invalid professional edit")
)

type WorkingPeriod struct{ Weekday, OpensAtMinute, ClosesAtMinute int }
type ProfessionalSchedule struct {
	IntervalMinutes, BufferMinutes int
	WorkingHours                   []WorkingPeriod
}
type ProfessionalDetail struct {
	ProfessionalOutput
	Schedule ProfessionalSchedule
	Revision string
}
type ProfessionalOwnerAccess interface {
	RequireProfessionalOwner(context.Context, uuid.UUID, uuid.UUID) error
}
type ProfessionalManagementStore interface {
	LockProfessional(context.Context, uuid.UUID, uuid.UUID) error
	ReadProfessional(context.Context, uuid.UUID, uuid.UUID) (ProfessionalOutput, error)
	ReplaceProfessional(context.Context, uuid.UUID, ProfessionalOutput) error
}
type ProfessionalCalendarEditor interface {
	ReadProfessionalSchedule(context.Context, uuid.UUID, uuid.UUID) (ProfessionalSchedule, error)
	ReplaceProfessionalSchedule(context.Context, uuid.UUID, uuid.UUID, ProfessionalSchedule) error
}
type ProfessionalEditScope struct{ ActorID, ShopID uuid.UUID }
type ManageProfessionals struct{ transactions ProfessionalTransactor }

func NewManageProfessionals(tx ProfessionalTransactor) *ManageProfessionals {
	return &ManageProfessionals{tx}
}

func normalizeProfessional(p *ProfessionalDetail) error {
	name, err := domain.NormalizeProfessionalName(p.Name)
	if err != nil {
		return err
	}
	p.Name = name
	if p.ID == uuid.Nil || len(p.ServiceIDs) > 100 || p.ServiceIDs == nil {
		return ErrInvalidProfessionalEdit
	}
	p.ServiceIDs = append([]uuid.UUID{}, p.ServiceIDs...)
	sort.Slice(p.ServiceIDs, func(i, j int) bool { return p.ServiceIDs[i].String() < p.ServiceIDs[j].String() })
	for i, id := range p.ServiceIDs {
		if id == uuid.Nil || (i > 0 && id == p.ServiceIDs[i-1]) {
			return ErrInvalidProfessionalServices
		}
	}
	s := &p.Schedule
	if s.IntervalMinutes < 1 || s.IntervalMinutes > 1440 || s.BufferMinutes < 0 || s.BufferMinutes > 1440 || s.WorkingHours == nil || len(s.WorkingHours) > 28 {
		return ErrInvalidProfessionalSchedule
	}
	s.WorkingHours = append([]WorkingPeriod{}, s.WorkingHours...)
	sort.Slice(s.WorkingHours, func(i, j int) bool {
		a, b := s.WorkingHours[i], s.WorkingHours[j]
		if a.Weekday != b.Weekday {
			return a.Weekday < b.Weekday
		}
		return a.OpensAtMinute < b.OpensAtMinute
	})
	for i, w := range s.WorkingHours {
		if w.Weekday < 0 || w.Weekday > 6 || w.OpensAtMinute < 0 || w.ClosesAtMinute > 1440 || w.OpensAtMinute >= w.ClosesAtMinute {
			return ErrInvalidProfessionalSchedule
		}
		if i > 0 {
			previous := s.WorkingHours[i-1]
			if previous.Weekday == w.Weekday && previous.ClosesAtMinute > w.OpensAtMinute {
				return ErrInvalidProfessionalSchedule
			}
		}
	}
	return nil
}
func professionalRevision(p ProfessionalDetail) string {
	p.Revision = ""
	encoded, _ := json.Marshal(p)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
func readProfessionalDetail(ctx context.Context, tx ProfessionalTx, shop, id uuid.UUID) (ProfessionalDetail, error) {
	p, err := tx.Management.ReadProfessional(ctx, shop, id)
	if err != nil {
		return ProfessionalDetail{}, err
	}
	s, err := tx.Calendar.ReadProfessionalSchedule(ctx, shop, id)
	if err != nil {
		return ProfessionalDetail{}, err
	}
	out := ProfessionalDetail{ProfessionalOutput: p, Schedule: s}
	out.Revision = professionalRevision(out)
	return out, nil
}
func lockProfessionalEditor(ctx context.Context, tx ProfessionalTx, s ProfessionalEditScope, id uuid.UUID) error {
	// Same lock as booking, time off and account changes, before membership locks.
	if err := tx.Management.LockProfessional(ctx, s.ShopID, id); err != nil {
		return err
	}
	return tx.Owner.RequireProfessionalOwner(ctx, s.ActorID, s.ShopID)
}
func (uc *ManageProfessionals) Get(ctx context.Context, s ProfessionalEditScope, id uuid.UUID) (ProfessionalDetail, error) {
	var out ProfessionalDetail
	if s.ActorID == uuid.Nil || s.ShopID == uuid.Nil {
		return out, ErrProfessionalAccessDenied
	}
	if id == uuid.Nil {
		return out, ErrProfessionalNotFound
	}
	err := uc.transactions.WithinProfessionalTx(ctx, func(ctx context.Context, tx ProfessionalTx) error {
		if err := lockProfessionalEditor(ctx, tx, s, id); err != nil {
			return err
		}
		var err error
		out, err = readProfessionalDetail(ctx, tx, s.ShopID, id)
		return err
	})
	return out, err
}
func (uc *ManageProfessionals) Update(ctx context.Context, s ProfessionalEditScope, p ProfessionalDetail) (ProfessionalDetail, error) {
	var out ProfessionalDetail
	if s.ActorID == uuid.Nil || s.ShopID == uuid.Nil {
		return out, ErrProfessionalAccessDenied
	}
	if hash, err := hex.DecodeString(p.Revision); err != nil || len(hash) != 32 || hex.EncodeToString(hash) != p.Revision {
		return out, ErrInvalidProfessionalEdit
	}
	if err := normalizeProfessional(&p); err != nil {
		return out, err
	}
	err := uc.transactions.WithinProfessionalTx(ctx, func(ctx context.Context, tx ProfessionalTx) error {
		if err := lockProfessionalEditor(ctx, tx, s, p.ID); err != nil {
			return err
		}
		current, err := readProfessionalDetail(ctx, tx, s.ShopID, p.ID)
		if err != nil {
			return err
		}
		// An identical saved result safely recovers a lost response, including a stale revision.
		if professionalRevision(p) == current.Revision {
			out = current
			return nil
		}
		if current.Revision != p.Revision {
			return ErrProfessionalChanged
		}
		if err := tx.Management.ReplaceProfessional(ctx, s.ShopID, p.ProfessionalOutput); err != nil {
			return err
		}
		if err := tx.Calendar.ReplaceProfessionalSchedule(ctx, s.ShopID, p.ID, p.Schedule); err != nil {
			return err
		}
		out, err = readProfessionalDetail(ctx, tx, s.ShopID, p.ID)
		return err
	})
	return out, err
}
