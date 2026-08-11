package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Debit  = "DEBIT"
	Credit = "CREDIT"
)

// Account types matching required research
type AccountType string

const (
	AccountAsset     AccountType = "ASSET"
	AccountLiability AccountType = "LIABILITY"
	AccountEquity    AccountType = "EQUITY"
	AccountRevenue   AccountType = "REVENUE"
	AccountExpense   AccountType = "EXPENSE"
)

// Posting describes a single side of a journal entry.
type Posting struct {
	AccountID string
	Direction string
	Amount    int64
}

// TransactionRequest is the payload for a write-style ledger operation.
type TransactionRequest struct {
	IdempotencyKey *string
	Description    string
	Postings       []Posting
}

// Engine is the concrete service dependency.
type Engine struct {
	db *pgxpool.Pool
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{db: pool}
}

// PostTransaction opens a DB transaction and executes the postings.
func (e *Engine) PostTransaction(ctx context.Context, req TransactionRequest) error {
	tx, err := e.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := e.PostTransactionTx(ctx, tx, req); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// PostTransactionTx executes ledger posting within an existing database transaction.
func (e *Engine) PostTransactionTx(ctx context.Context, tx pgx.Tx, req TransactionRequest) error {
	if len(req.Postings) < 2 {
		return errors.New("a double-entry transaction requires at least 2 postings")
	}

	// 1. Invariant Check: Debits MUST Equal Credits
	var totalDebit, totalCredit int64
	for _, p := range req.Postings {
		if p.Amount <= 0 {
			return errors.New("posting amount must be positive")
		}
		switch p.Direction {
		case Debit:
			totalDebit += p.Amount
		case Credit:
			totalCredit += p.Amount
		default:
			return fmt.Errorf("invalid direction: %s", p.Direction)
		}
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("transaction unbalance: total debits (%d) != total credits (%d)", totalDebit, totalCredit)
	}

	// 2. Idempotency Check
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		var existingID string
		err := tx.QueryRow(ctx, `SELECT id FROM transactions WHERE idempotency_key = $1`, *req.IdempotencyKey).Scan(&existingID)
		if err == nil {
			// Already processed successfully, return idempotently
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("idempotency check error: %w", err)
		}
	}

	// 3. Insert Transaction Header
	var txID string
	err := tx.QueryRow(ctx,
		`INSERT INTO transactions (idempotency_key, description) VALUES ($1, $2) RETURNING id`,
		req.IdempotencyKey, req.Description,
	).Scan(&txID)
	if err != nil {
		return fmt.Errorf("failed to insert transaction header: %w", err)
	}

	// 4. Lock Accounts and Insert Entries (Concurrency Safety)
	for _, p := range req.Postings {
		var accType AccountType
		// Row lock to prevent race conditions during concurrent transactions
		err := tx.QueryRow(ctx, `SELECT type FROM accounts WHERE id = $1 FOR UPDATE`, p.AccountID).Scan(&accType)
		if err != nil {
			return fmt.Errorf("account %s not found: %w", p.AccountID, err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO entries (transaction_id, account_id, direction, amount) VALUES ($1, $2, $3, $4)`,
			txID, p.AccountID, p.Direction, p.Amount,
		)
		if err != nil {
			return fmt.Errorf("failed to insert entry: %w", err)
		}
	}

	// 5. Invariant Check: No Overdraft / Negative Balance
	for _, p := range req.Postings {
		bal, err := calculateTxAccountBalance(ctx, tx, p.AccountID)
		if err != nil {
			return err
		}
		if bal < 0 {
			return fmt.Errorf("transaction rejected: insufficient balance on account %s (current balance: %d)", p.AccountID, bal)
		}
	}

	return nil
}

// Internal helper function to compute real-time balance inside an active SQL transaction
func calculateTxAccountBalance(ctx context.Context, tx pgx.Tx, accountID string) (int64, error) {
	var accType AccountType
	err := tx.QueryRow(ctx, `SELECT type FROM accounts WHERE id = $1`, accountID).Scan(&accType)
	if err != nil {
		return 0, err
	}

	var totalDebit, totalCredit int64
	err = tx.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0)
		FROM entries WHERE account_id = $1
	`, accountID).Scan(&totalDebit, &totalCredit)
	if err != nil {
		return 0, err
	}

	// Accounting Balance Rules
	switch accType {
	case AccountAsset, AccountExpense:
		return totalDebit - totalCredit, nil
	case AccountLiability, AccountEquity, AccountRevenue:
		return totalCredit - totalDebit, nil
	default:
		return totalCredit - totalDebit, nil
	}
}
