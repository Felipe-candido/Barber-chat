package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type planningFixture struct {
	policy     domain.Policy
	steps      []string
	fail       string
	permission app.BookingPermission
	prepared   app.PreparedBookingNotifications
	shop       app.MessageShop
}

func (f *planningFixture) Policy(context.Context, uuid.UUID) (domain.Policy, error) {
	f.steps = append(f.steps, "policy")
	if f.fail == "policy" {
		return domain.Policy{}, errors.New("policy failed")
	}
	return f.policy, nil
}
func (f *planningFixture) FindMessageShop(context.Context, uuid.UUID) (app.MessageShop, bool, error) {
	f.steps = append(f.steps, "shop")
	return f.shop, true, nil
}
func (f *planningFixture) RecordBookingPermission(_ context.Context, p app.BookingPermission) error {
	f.steps = append(f.steps, "permission")
	f.permission = p
	if f.fail == "permission" {
		return errors.New("permission failed")
	}
	return nil
}
func (f *planningFixture) Save(_ context.Context, _ app.PrepareBookingNotificationsInput, p app.PreparedBookingNotifications) error {
	f.steps = append(f.steps, "save")
	f.prepared = p
	return nil
}

func TestDurablePlanningUsesCurrentPolicyAndPermissionBeforeSave(t *testing.T) {
	i := preparation()
	f := &planningFixture{policy: i.Policy, shop: app.MessageShop{ShopID: i.ShopID, Name: i.Snapshot.ShopName, Slug: i.Snapshot.ShopSlug, Timezone: i.Snapshot.ShopTimezone, Address: i.Snapshot.ShopAddress}}
	sealer := &recordingSealer{}
	actor, customer := uuid.New(), uuid.New()
	uc := app.NewPlanBookingNotifications(true, app.NewPrepareBookingNotifications(sealer), f, f, f)
	i.ScheduleGeneration = 1
	err := uc.Execute(context.Background(), app.PlanBookingInput{Preparation: i, CustomerID: customer, ActorID: actor})
	if err != nil || !reflect.DeepEqual(f.steps, []string{"policy", "shop", "permission", "save"}) || !f.prepared.Schedule.Reminder.Valid() ||
		f.permission.ActorID != actor || f.permission.CustomerID != customer || !f.permission.Allowed {
		t.Fatal("invalid planning sequence", err, f.steps)
	}
	f.steps = nil
	f.fail = "permission"
	if err := uc.Execute(context.Background(), app.PlanBookingInput{Preparation: i, CustomerID: customer}); err == nil || !reflect.DeepEqual(f.steps, []string{"policy", "shop", "permission"}) {
		t.Fatal("failed permission allowed plan persistence")
	}
}

func TestDisabledDeploymentDoesNotTouchStores(t *testing.T) {
	if err := app.NewPlanBookingNotifications(false, nil, nil, nil, nil).Execute(context.Background(), app.PlanBookingInput{}); err != nil {
		t.Fatal(err)
	}
}
