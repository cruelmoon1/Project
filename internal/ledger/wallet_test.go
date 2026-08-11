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

func TestWalletService_DepositAndWithdraw(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://tergel:tergelgod@localhost:5432/mydatabase?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	engine := ledger.NewEngine(pool)
	walletSvc := ledger.NewWalletService(engine)

	nano := time.Now().UnixNano()
	cashAccID := fmt.Sprintf("test-cash-%d", nano)
	userAccID := fmt.Sprintf("test-user-%d", nano)
	equityAccID := fmt.Sprintf("test-equity-%d", nano)

	// Setup Accounts
	_, _ = engine.CreateAccount(ctx, cashAccID, "Test Cash", ledger.AccountAsset, "MNT")
	_, _ = engine.CreateAccount(ctx, userAccID, "Test User", ledger.AccountAsset, "MNT")
	_, _ = engine.CreateAccount(ctx, equityAccID, "Initial Capital", ledger.AccountEquity, "MNT")

	// CashAccount дээр анхны үлдэгдэл нэмэх (Equity -> Cash)
	initKey := fmt.Sprintf("init-%d", nano)
	_ = engine.PostTransaction(ctx, ledger.TransactionRequest{
		IdempotencyKey: &initKey,
		Description:    "Fund Cash Account",
		Postings: []ledger.Posting{
			{AccountID: cashAccID, Direction: ledger.Debit, Amount: 100000},
			{AccountID: equityAccID, Direction: ledger.Credit, Amount: 100000},
		},
	})

	// 1. Test Deposit (10,000 MNT)
	depositKey := fmt.Sprintf("dep-key-%d", nano)
	err = walletSvc.Deposit(ctx, cashAccID, userAccID, 10000, depositKey, "Initial Deposit")
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}

	bal, err := walletSvc.GetAccountBalance(ctx, userAccID, nil)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}
	if bal != 10000 {
		t.Errorf("Expected balance 10000, got %d", bal)
	}

	// 2. Test Withdraw (3,000 MNT)
	withdrawKey := fmt.Sprintf("with-key-%d", nano)
	err = walletSvc.Withdraw(ctx, userAccID, cashAccID, 3000, withdrawKey, "ATM Withdraw")
	if err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}

	bal, err = walletSvc.GetAccountBalance(ctx, userAccID, nil)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}
	if bal != 7000 {
		t.Errorf("Expected balance 7000 after withdraw, got %d", bal)
	}
}
