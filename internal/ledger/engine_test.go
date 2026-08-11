package ledger_test

import (
	"context"
	"testing"

	"ledger-service/internal/ledger"
)

func TestEngine_Validation(t *testing.T) {
	// Basic validation test logic
	req := ledger.TransactionRequest{
		Description: "Unbalanced transaction",
		Postings: []ledger.Entry{
			{AccountID: "acc-1", Direction: ledger.Debit, Amount: 100},
			{AccountID: "acc-2", Direction: ledger.Credit, Amount: 50},
		},
	}

	engine := ledger.NewEngine(nil)
	err := engine.PostTransaction(context.Background(), req)
	if err == nil {
		t.Errorf("expected error for unbalanced transaction, got nil")
	}
}
