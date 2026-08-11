package ledger_test

import (
	"context"
	"testing"

	"ledger-service/internal/ledger"
)

func TestWalletService_Validation(t *testing.T) {
	svc := ledger.NewWalletService(nil)

	err := svc.Deposit(context.Background(), "cash-1", "wallet-1", -100, "key-1", "Invalid deposit")
	if err == nil {
		t.Errorf("expected error for negative amount, got nil")
	}
}
