package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type CreateShop struct{ transactions ShopCreationTransactor }

func NewCreateShop(transactions ShopCreationTransactor) *CreateShop {
	return &CreateShop{transactions: transactions}
}

func (uc *CreateShop) Execute(ctx context.Context, input CreateShopInput) (CreatedShop, error) {
	key, err := hex.DecodeString(input.IdempotencyKey)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if err != nil || len(key) != 32 || input.IdempotencyKey != strings.ToLower(input.IdempotencyKey) || input.OwnerID == uuid.Nil ||
		!utf8.ValidString(input.DisplayName) || utf8.RuneCountInString(input.DisplayName) < 1 || utf8.RuneCountInString(input.DisplayName) > 100 ||
		strings.IndexFunc(input.DisplayName, unicode.IsControl) >= 0 {
		return CreatedShop{}, ErrInvalidRegistration
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return CreatedShop{}, fmt.Errorf("generate shop identity: %w", err)
	}
	shop, err := domain.NewShop(id, input.Name, input.Slug, input.Timezone)
	if err != nil {
		return CreatedShop{}, err
	}
	shop.Address, err = domain.NewAddress(input.Address)
	if err != nil {
		return CreatedShop{}, err
	}
	canonical, err := json.Marshal(struct {
		DisplayName, Name, Slug, Timezone string
		Address                           domain.Address
	}{input.DisplayName, shop.Name, shop.Slug, shop.Timezone, shop.Address})
	if err != nil {
		return CreatedShop{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(canonical)
	var fingerprint [32]byte
	copy(fingerprint[:], mac.Sum(nil))
	keyHash := sha256.Sum256(key)
	var output CreatedShop
	err = uc.transactions.WithinShopCreationTx(ctx, func(ctx context.Context, tx ShopCreationTx) error {
		// This row lock serializes concurrent retries by the same account, even
		// when its user was just inserted by the competing registration request.
		if err := tx.Owners.EnsureActiveUser(ctx, input.OwnerID, input.DisplayName); err != nil {
			return err
		}
		previous, found, err := tx.Shops.FindCreation(ctx, input.OwnerID, keyHash)
		if err != nil {
			return err
		}
		if found {
			if !hmac.Equal(previous.Fingerprint[:], fingerprint[:]) {
				return ErrCreationConflict
			}
			if previous.Shop.ID == uuid.Nil {
				return ErrCreationMismatch
			}
			if !previous.Shop.Active {
				return ErrOwnershipDenied
			}
			if err := tx.Owners.RequireActiveOwner(ctx, input.OwnerID, previous.Shop.ID); err != nil {
				return err
			}
			output = CreatedShop{Shop: previous.Shop, Replayed: true}
			return nil
		}
		if err := tx.Shops.Create(ctx, shop); err != nil {
			return err
		}
		if err := tx.Owners.CreateOwnerMembership(ctx, input.OwnerID, shop.ID); err != nil {
			return err
		}
		if err := tx.Shops.RecordCreation(ctx, input.OwnerID, keyHash, fingerprint, shop.ID); err != nil {
			return err
		}
		output = CreatedShop{Shop: shop}
		return nil
	})
	if err != nil {
		return CreatedShop{}, fmt.Errorf("create shop with owner: %w", err)
	}
	return output, nil
}
