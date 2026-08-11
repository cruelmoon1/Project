package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Account struct for API responses
type Account struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"`
	CreatedAt time.Time   `json:"created_at"`
}

// TransactionHistoryItem represents entry record for an account
type TransactionHistoryItem struct {
	TransactionID string    `json:"transaction_id"`
	Description   string    `json:"description"`
	PostedAt      time.Time `json:"posted_at"`
	Direction     string    `json:"direction"`
	Amount        int64     `json:"amount"`
}

// Posting describes a single side of a journal entry.
type Posting struct {
	AccountID string `json:"account_id"`
	Direction string `json:"direction"`
	Amount    int64  `json:"amount"`
}

// TransactionRequest is the payload for a write-style ledger operation.
type TransactionRequest struct {
	IdempotencyKey *string   `json:"idempotency_key,omitempty"`
	Description    string    `json:"description"`
	Postings       []Posting `json:"postings"`
}

// Engine is the concrete service dependency.
type Engine struct {
	db *pgxpool.Pool
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{db: pool}
}

// CreateAccount registers a new account in the system
func (e *Engine) CreateAccount(ctx context.Context, id, name string, accType AccountType, currency string) (*Account, error) {
	validTypes := map[AccountType]bool{
		AccountAsset:     true,
		AccountLiability: true,
		AccountEquity:    true,
		AccountRevenue:   true,
		AccountExpense:   true,
	}

	if !validTypes[accType] {
		return nil, fmt.Errorf("invalid account type: %s", accType)
	}

	query := `
		INSERT INTO accounts (id, name, type, currency, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, name, type, currency, created_at
	`
	acc := &Account{}
	err := e.db.QueryRow(ctx, query, id, name, accType, currency).Scan(
		&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return acc, nil
}

// GetAccountTransactions retrieves the full transaction history for a specific account
func (e *Engine) GetAccountTransactions(ctx context.Context, accountID string) ([]TransactionHistoryItem, error) {
	query := `
		SELECT t.id, COALESCE(t.description, ''), t.posted_at, e.direction, e.amount
		FROM entries e
		JOIN transactions t ON e.transaction_id = t.id
		WHERE e.account_id = $1
		ORDER BY t.posted_at DESC
	`
	rows, err := e.db.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions: %w", err)
	}
	defer rows.Close()

	var history []TransactionHistoryItem
	for rows.Next() {
		var item TransactionHistoryItem
		if err := rows.Scan(&item.TransactionID, &item.Description, &item.PostedAt, &item.Direction, &item.Amount); err != nil {
			return nil, err
		}
		history = append(history, item)
	}

	return history, nil
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
