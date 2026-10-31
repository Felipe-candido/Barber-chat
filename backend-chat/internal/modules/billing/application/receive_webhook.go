package application

import (
	"context"
	"errors"
	"time"
)

type ReceiveWebhook struct {
	store    WebhookStore
	provider WebhookProvider
}

func NewReceiveWebhook(store WebhookStore, provider WebhookProvider) *ReceiveWebhook {
	return &ReceiveWebhook{store: store, provider: provider}
}

func (uc *ReceiveWebhook) Execute(ctx context.Context, event VerifiedWebhook) error {
	if uc.provider == nil {
		return ErrWebhookNotConfigured
	}
	if event.ID == "" || event.Type == "" || event.CreatedAt.IsZero() {
		return ErrInvalidWebhook
	}
	terminal, err := uc.store.AcceptWebhook(ctx, event)
	if err != nil {
		return ErrWebhookRetry
	}
	if terminal {
		return nil
	}
	ref, err := uc.provider.ResolveBillingReference(ctx, event)
	if err == nil {
		var target *ReconciliationTarget
		target, err = uc.store.ClaimReconciliation(ctx, event, ref)
		if err == nil && target == nil {
			return nil
		}
		if err == nil {
			var snapshot SubscriptionSnapshot
			snapshot, err = uc.provider.ReadSubscriptionSnapshot(ctx, *target)
			if err == nil {
				err = uc.store.ApplyReconciliation(ctx, *target, snapshot)
			}
		}
	}
	if err == nil {
		return nil
	}
	// Persist retry diagnostics even if the request deadline elapsed. The pending
	// receipt is already durable; no success/2xx for unprocessed supported events.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = uc.store.NoteWebhookFailure(cleanup, event.ID)
	if errors.Is(err, ErrInvalidWebhook) {
		return ErrInvalidWebhook
	}
	return ErrWebhookRetry
}
