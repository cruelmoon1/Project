package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountType string

const (
	Asset     AccountType = "ASSET"
	Liability AccountType = "LIABILITY"
	Equity    AccountType = "EQUITY"
	Revenue   AccountType = "REVENUE"
	Expense   AccountType = "EXPENSE"
)

type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

type Account struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"`
	CreatedAt time.Time   `json:"created_at"`
}

type Entry struct {
	AccountID string    `json:"account_id"`
	Direction Direction `json:"direction"`
	Amount    int64     `json:"amount"`
}

type TransactionRequest struct {
	ID             string  `json:"id,omitempty"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
	Description    string  `json:"description,omitempty"`
	Postings       []Entry `json:"postings"`
}

type TransactionHistory struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Direction   Direction `json:"direction"`
	Amount      int64     `json:"amount"`
	PostedAt    time.Time `json:"posted_at"`
}

type Engine struct {
	pool *pgxpool.Pool
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{pool: pool}
}

func (e *Engine) CreateAccount(ctx context.Context, id, name string, accType AccountType, currency string) (*Account, error) {
	query := `
		INSERT INTO accounts (id, name, type, currency)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, currency, created_at;
	`

	acc := &Account{}
	err := e.pool.QueryRow(ctx, query, id, name, accType, currency).Scan(
		&acc.ID, &acc.Name, &acc.Type, &acc.Currency, &acc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return acc, nil
}

func (e *Engine) PostTransaction(ctx context.Context, req TransactionRequest) error {
	if len(req.Postings) < 2 {
		return errors.New("transaction must have at least two postings")
	}

	var totalDebit, totalCredit int64
	for _, p := range req.Postings {
		if p.Amount <= 0 {
			return fmt.Errorf("posting amount must be positive, got: %d", p.Amount)
		}
		if p.Direction == Debit {
			totalDebit += p.Amount
		} else if p.Direction == Credit {
			totalCredit += p.Amount
		} else {
			return fmt.Errorf("invalid direction: %s", p.Direction)
		}
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("transaction is unbalanced: debits (%d) != credits (%d)", totalDebit, totalCredit)
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// CONCURRENCY SAFETY: Гүйлгээнд оролцох бүх дансыг FOR UPDATE хийж түгжинэ
	for _, p := range req.Postings {
		var accType AccountType
		accErr := tx.QueryRow(ctx, "SELECT type FROM accounts WHERE id = $1 FOR UPDATE", p.AccountID).Scan(&accType)
		if accErr != nil {
			if errors.Is(accErr, pgx.ErrNoRows) {
				return fmt.Errorf("account not found: %s", p.AccountID)
			}
			return fmt.Errorf("failed to fetch account type: %w", accErr)
		}

		if accType == Asset {
			var currentBalance int64
			balQuery := `
				SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE -amount END), 0)
				FROM entries
				WHERE account_id = $1;
			`
			if err := tx.QueryRow(ctx, balQuery, p.AccountID).Scan(&currentBalance); err != nil {
				return fmt.Errorf("failed to calculate current balance: %w", err)
			}

			var balanceChange int64
			if p.Direction == Debit {
				balanceChange = p.Amount
			} else {
				balanceChange = -p.Amount
			}

			if currentBalance+balanceChange < 0 {
				return fmt.Errorf("transaction rejected: insufficient balance on account %s (current balance: %d)", p.AccountID, currentBalance)
			}
		}
	}

	var txID string
	txQuery := `
		INSERT INTO transactions (idempotency_key, description)
		VALUES ($1, $2)
		RETURNING id;
	`
	var idempotencyKey *string
	if req.IdempotencyKey != "" {
		idempotencyKey = &req.IdempotencyKey
	}

	err = tx.QueryRow(ctx, txQuery, idempotencyKey, req.Description).Scan(&txID)
	if err != nil {
		return fmt.Errorf("failed to insert transaction: %w", err)
	}

	entryQuery := `
		INSERT INTO entries (transaction_id, account_id, direction, amount)
		VALUES ($1, $2, $3, $4);
	`

	for _, p := range req.Postings {
		_, err := tx.Exec(ctx, entryQuery, txID, p.AccountID, p.Direction, p.Amount)
		if err != nil {
			return fmt.Errorf("failed to insert entry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}

	return nil
}

func (e *Engine) GetAccountTransactions(ctx context.Context, accountID string) ([]TransactionHistory, error) {
	query := `
		SELECT t.id, COALESCE(t.description, ''), e.direction, e.amount, t.posted_at
		FROM entries e
		JOIN transactions t ON e.transaction_id = t.id
		WHERE e.account_id = $1
		ORDER BY t.posted_at DESC;
	`
	rows, err := e.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []TransactionHistory
	for rows.Next() {
		var h TransactionHistory
		if err := rows.Scan(&h.ID, &h.Description, &h.Direction, &h.Amount, &h.PostedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, nil
}
