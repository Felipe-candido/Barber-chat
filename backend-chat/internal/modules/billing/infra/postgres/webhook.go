package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookStore struct{ pool *pgxpool.Pool }

var _ application.WebhookStore = (*WebhookStore)(nil)

func NewWebhookStore(pool *pgxpool.Pool) *WebhookStore { return &WebhookStore{pool: pool} }

// Every callback is local SQL only; provider requests live in application.
func (s *WebhookStore) within(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *WebhookStore) AcceptWebhook(ctx context.Context, event application.VerifiedWebhook) (bool, error) {
	terminal := false
	err := s.within(ctx, func(q *db.Queries) error {
		// Retain references, not the raw Stripe body containing billing PII.
		payload, _ := json.Marshal(struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}{event.ID, event.Type})
		state := "pending"
		var processed pgtype.Timestamptz
		if event.Kind == "ignored" {
			state = "ignored"
			processed = timestamp(time.Now())
		}
		err := q.InsertBillingReceipt(ctx, db.InsertBillingReceiptParams{StripeEventID: event.ID, EventType: event.Type, Column3: payload, State: state,
			ProcessedAt: processed, ResourceKind: pgtype.Text{String: event.Kind, Valid: true}, ResourceID: pgtype.Text{String: event.ResourceID, Valid: true}, EventCreatedAt: timestamp(event.CreatedAt), ApiVersion: checkoutText(event.APIVersion)})
		if err != nil {
			return err
		}
		receipt, err := q.LockBillingReceipt(ctx, event.ID)
		if err != nil {
			return err
		}
		if !sameReceipt(receipt, event) {
			return application.ErrInvalidWebhook
		}
		terminal = receipt.State == "processed" || receipt.State == "ignored"
		return nil
	})
	return terminal, err
}

func sameReceipt(row db.LockBillingReceiptRow, event application.VerifiedWebhook) bool {
	return row.ReceiptProtocol == 1 && row.EventType == event.Type && row.ResourceKind == event.Kind && row.ResourceID == event.ResourceID && row.ApiVersion == event.APIVersion && row.EventCreatedAt.Valid && row.EventCreatedAt.Time.Equal(event.CreatedAt)
}

func (s *WebhookStore) ClaimReconciliation(ctx context.Context, event application.VerifiedWebhook, ref application.BillingReference) (*application.ReconciliationTarget, error) {
	var result *application.ReconciliationTarget
	err := s.within(ctx, func(q *db.Queries) error {
		account, err := q.LockReconciliationCustomer(ctx, checkoutText(ref.CustomerID))
		if errors.Is(err, pgx.ErrNoRows) {
			return q.FinishBillingReceipt(ctx, db.FinishBillingReceiptParams{StripeEventID: event.ID, State: "ignored"})
		}
		if err != nil {
			return err
		}
		receipt, err := q.LockBillingReceipt(ctx, event.ID)
		if err != nil {
			return err
		}
		if !sameReceipt(receipt, event) {
			return application.ErrInvalidWebhook
		}
		if receipt.State == "processed" || receipt.State == "ignored" {
			return nil
		}
		if account.StripeMode != "test" || !account.StripeCustomerID.Valid || account.StripeCustomerID.String != ref.CustomerID ||
			(ref.ShopID != uuid.Nil && ref.ShopID != account.ShopID) {
			return application.ErrReconciliationConflict
		}
		if event.Kind == "invoice" && ref.SubscriptionID == "" {
			return q.FinishBillingReceipt(ctx, db.FinishBillingReceiptParams{StripeEventID: event.ID, State: "ignored"})
		}
		attemptID := ref.AttemptID
		if ref.SubscriptionID != "" {
			binding, err := q.FindReconciliationBinding(ctx, ref.SubscriptionID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				if binding.ShopID != account.ShopID || binding.StripeCustomerID != ref.CustomerID || binding.ReconciliationProtocol != 1 || binding.AttemptID == uuid.Nil ||
					(attemptID != uuid.Nil && attemptID != binding.AttemptID) {
					return application.ErrReconciliationConflict
				}
				attemptID = binding.AttemptID
			}
		}
		if attemptID == uuid.Nil {
			return application.ErrReconciliationConflict
		}
		attempt, err := q.LockReconciliationAttempt(ctx, db.LockReconciliationAttemptParams{ID: attemptID, ShopID: account.ShopID})
		if err != nil {
			return err
		}
		if attempt.ProtocolVersion != 1 || attempt.PriceID == "" {
			return application.ErrReconciliationConflict
		}
		revision, err := q.AdvanceReconciliationRevision(ctx, account.ShopID)
		if err != nil {
			return err
		}
		if err := q.StartBillingReceiptAttempt(ctx, db.StartBillingReceiptAttemptParams{StripeEventID: event.ID, ShopID: pgtype.UUID{Bytes: account.ShopID, Valid: true}}); err != nil {
			return err
		}
		result = &application.ReconciliationTarget{Event: event, Account: application.CheckoutAccount{ShopID: account.ShopID, CustomerID: ref.CustomerID, Mode: account.StripeMode, CreatedAt: account.CreatedAt.Time},
			Attempt:        application.CheckoutAttempt{ID: attempt.ID, ShopID: attempt.ShopID, Intent: domain.CheckoutIntent(attempt.Intent), PlanKey: attempt.PlanKey, PriceID: attempt.PriceID, ReturnURL: attempt.ReturnUrl, State: attempt.State, SessionID: attempt.SessionID, Protocol: attempt.ProtocolVersion, CreatedAt: attempt.CreatedAt.Time},
			SubscriptionID: ref.SubscriptionID, Revision: revision}
		return nil
	})
	return result, err
}

func (s *WebhookStore) ApplyReconciliation(ctx context.Context, target application.ReconciliationTarget, snapshot application.SubscriptionSnapshot) error {
	if snapshot.CustomerID != target.Account.CustomerID || snapshot.ID != target.SubscriptionID || snapshot.Session.CustomerID != target.Account.CustomerID ||
		snapshot.Session.AttemptID != target.Attempt.ID || snapshot.Session.ShopID != target.Account.ShopID || snapshot.Session.SubscriptionID != snapshot.ID ||
		snapshot.Session.ID == "" || snapshot.Session.ExpiresAt.IsZero() || (target.Attempt.SessionID != "" && snapshot.Session.ID != target.Attempt.SessionID) {
		return application.ErrReconciliationConflict
	}
	return s.within(ctx, func(q *db.Queries) error {
		account, err := q.LockReconciliationShop(ctx, target.Account.ShopID)
		if err != nil {
			return err
		}
		receipt, err := q.LockBillingReceipt(ctx, target.Event.ID)
		if err != nil {
			return err
		}
		if !sameReceipt(receipt, target.Event) {
			return application.ErrInvalidWebhook
		}
		if receipt.State == "processed" || receipt.State == "ignored" {
			return nil
		}
		if account.StripeMode != "test" || account.StripeCustomerID.String != target.Account.CustomerID || account.ReconciliationRevision != target.Revision {
			return application.ErrReconciliationConflict
		}
		state := "completed"
		if snapshot.Session.Status == "expired" {
			state = "expired"
		} else if snapshot.Session.Status != "complete" {
			return application.ErrReconciliationConflict
		}
		if snapshot.ID == "" {
			if state != "expired" {
				return application.ErrReconciliationConflict
			}
		} else {
			if !snapshot.Status.Valid() || snapshot.PriceID != target.Attempt.PriceID {
				return application.ErrReconciliationConflict
			}
			if state == "expired" && snapshot.Status != domain.SubscriptionCanceled {
				return application.ErrReconciliationConflict
			}
			if err := applySubscription(ctx, q, target, snapshot); err != nil {
				return err
			}
		}
		count, err := q.LinkReconciledCheckout(ctx, db.LinkReconciledCheckoutParams{ShopID: target.Account.ShopID, ID: target.Attempt.ID, StripeSessionID: checkoutText(snapshot.Session.ID), ExpiresAt: timestamp(snapshot.Session.ExpiresAt), NextState: state})
		if err != nil {
			return err
		}
		if count != 1 {
			return application.ErrReconciliationConflict
		}
		return q.FinishBillingReceipt(ctx, db.FinishBillingReceiptParams{StripeEventID: target.Event.ID, State: "processed"})
	})
}

func applySubscription(ctx context.Context, q *db.Queries, target application.ReconciliationTarget, snapshot application.SubscriptionSnapshot) error {
	provisional := domain.SubscriptionSuspended
	if snapshot.Status == domain.SubscriptionCanceled {
		provisional = domain.SubscriptionCanceled
	}
	count, err := q.EnsureReconciledSubscription(ctx, db.EnsureReconciledSubscriptionParams{ID: uuid.New(), ShopID: target.Account.ShopID,
		StripeCustomerID: target.Account.CustomerID, StripeSubscriptionID: snapshot.ID, PlanKey: target.Attempt.PlanKey, Status: string(provisional),
		CheckoutAttemptID: pgtype.UUID{Bytes: target.Attempt.ID, Valid: true}, StripePriceID: checkoutText(target.Attempt.PriceID)})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrReconciliationConflict
	}
	for _, invoice := range snapshot.Invoices {
		if invoice.ID == "" || invoice.PriceID != target.Attempt.PriceID {
			return application.ErrReconciliationConflict
		}
		if !invoice.Eligible {
			if err := q.InvalidateBillingPaidInvoice(ctx, db.InvalidateBillingPaidInvoiceParams{StripeInvoiceID: invoice.ID, ShopID: target.Account.ShopID, StripeSubscriptionID: snapshot.ID}); err != nil {
				return err
			}
			continue
		}
		if !invoice.Period.Valid() || invoice.AmountPaid <= 0 {
			return application.ErrReconciliationConflict
		}
		count, err := q.ConfirmBillingPaidInvoice(ctx, db.ConfirmBillingPaidInvoiceParams{StripeInvoiceID: invoice.ID, ShopID: target.Account.ShopID, StripeSubscriptionID: snapshot.ID,
			StripePriceID: invoice.PriceID, PaidFrom: timestamp(invoice.Period.From), PaidUntil: timestamp(invoice.Period.Until), AmountPaid: invoice.AmountPaid})
		if err != nil {
			return err
		}
		if count != 1 {
			return application.ErrReconciliationConflict
		}
	}
	period, err := q.LatestConfirmedBillingPeriod(ctx, db.LatestConfirmedBillingPeriodParams{ShopID: target.Account.ShopID, StripeSubscriptionID: snapshot.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	status, cancelAtEnd := snapshot.Status, snapshot.CancelAtPeriodEnd
	if status == domain.SubscriptionActive && !period.PaidFrom.Valid {
		status = domain.SubscriptionPending
	}
	if status == domain.SubscriptionPending {
		period = db.LatestConfirmedBillingPeriodRow{}
		cancelAtEnd = false
	}
	count, err = q.UpdateReconciledSubscription(ctx, db.UpdateReconciledSubscriptionParams{ShopID: target.Account.ShopID, StripeSubscriptionID: snapshot.ID, Status: string(status), PaidFrom: period.PaidFrom, PaidUntil: period.PaidUntil, CancelAtPeriodEnd: cancelAtEnd})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrReconciliationConflict
	}
	return nil
}

func (s *WebhookStore) NoteWebhookFailure(ctx context.Context, id string) error {
	return db.New(s.pool).NoteBillingReceiptFailure(ctx, id)
}
func timestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}
