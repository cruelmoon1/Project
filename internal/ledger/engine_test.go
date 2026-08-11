package ledger_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledger-service/internal/ledger"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://tergel:tergelgod@localhost:5432/mydatabase?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	return pool
}

func TestCreateAccount(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	engine := ledger.NewEngine(pool)
	ctx := context.Background()

	// Удавшрахаас сэргийлж давтагдахгүй ID үүсгэнэ
	uniqueID := fmt.Sprintf("test-acc-%d", time.Now().UnixNano())

	// 1. Valid Account Creation
	acc, err := engine.CreateAccount(ctx, uniqueID, "Test Wallet", ledger.AccountAsset, "MNT")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if acc.ID != uniqueID {
		t.Errorf("Expected account ID %s, got %s", uniqueID, acc.ID)
	}

	// 2. Invalid Account Type Test
	_, err = engine.CreateAccount(ctx, uniqueID+"-inv", "Invalid Wallet", "INVALID_TYPE", "MNT")
	if err == nil {
		t.Error("Expected error for invalid account type, got nil")
	}
}

func TestPostTransaction_DoubleEntryBalance(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	engine := ledger.NewEngine(pool)
	ctx := context.Background()

	nano := time.Now().UnixNano()
	cashAccID := fmt.Sprintf("cash-%d", nano)
	userAccID := fmt.Sprintf("user-%d", nano)

	// Setup Accounts
	_, _ = engine.CreateAccount(ctx, cashAccID, "Cash Account", ledger.AccountAsset, "MNT")
	_, _ = engine.CreateAccount(ctx, userAccID, "User Wallet", ledger.AccountAsset, "MNT")

	// Unbalanced Transaction Test (Debits != Credits)
	unbalancedReq := ledger.TransactionRequest{
		Description: "Unbalanced Transaction",
		Postings: []ledger.Posting{
			{AccountID: cashAccID, Direction: ledger.Credit, Amount: 1000},
			{AccountID: userAccID, Direction: ledger.Debit, Amount: 500},
		},
	}

	err := engine.PostTransaction(ctx, unbalancedReq)
	if err == nil {
		t.Error("Expected error for unbalanced transaction, got nil")
	}
}
