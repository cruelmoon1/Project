package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountType string

const (
	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountEquity    AccountType = "equity"
	AccountRevenue   AccountType = "revenue"
	AccountExpense   AccountType = "expense"
)

type Account struct {
	ID        int         `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"`
	CreatedAt time.Time   `json:"created_at"`
}

type Transaction struct {
	ID             int       `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Description    string    `json:"description"`
	PostedAt       time.Time `json:"posted_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type Entry struct {
	ID            int    `json:"id"`
	TransactionID int    `json:"transaction_id"`
	AccountID     int    `json:"account_id"`
	Direction     string `json:"direction"` // "debit" or "credit"
	Amount        int64  `json:"amount"`    // Minor units (cents)
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewPool() (*pgxpool.Pool, error) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		user := os.Getenv("PGUSER")
		if user == "" {
			user = "tergel"
		}
		password := os.Getenv("PGPASSWORD")
		if password == "" {
			password = "tergelgod"
		}
		host := os.Getenv("PGHOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("PGPORT")
		if port == "" {
			port = "5432"
		}
		dbName := os.Getenv("PGDATABASE")
		if dbName == "" {
			dbName = "mydatabase"
		}

		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			user, password, host, port, dbName)
	}

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return nil, fmt.Errorf("database pool connection failed: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	if err := ensureSchema(context.Background(), pool); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func ensureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id SERIAL PRIMARY KEY,
		idempotency_key TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL,
		posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS entries (
		id SERIAL PRIMARY KEY,
		transaction_id INTEGER NOT NULL REFERENCES transactions(id),
		account_id INTEGER NOT NULL REFERENCES accounts(id),
		direction TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
		amount BIGINT NOT NULL CHECK (amount > 0),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	`
	_, err := pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Default System Accounts
	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count)
	if count == 0 {
		seed := `
		INSERT INTO accounts (id, name, type, currency) VALUES
			(1, 'World Cash Clearing', 'asset', 'USD'),
			(2, 'System Fee Revenue', 'revenue', 'USD'),
			(3, 'User Alice Wallet', 'liability', 'USD'),
			(4, 'User Bob Wallet', 'liability', 'USD');
		SELECT setval('accounts_id_seq', (SELECT MAX(id) FROM accounts));
		`
		_, _ = pool.Exec(ctx, seed)
	}

	return nil
}

// CreateAccount adds a new general ledger account
func (s *Service) CreateAccount(ctx context.Context, name string, accType AccountType, currency string) (*Account, error) {
	acc := &Account{Name: name, Type: accType, Currency: currency}
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO accounts (name, type, currency) VALUES ($1, $2, $3) RETURNING id, created_at`,
		name, accType, currency,
	).Scan(&acc.ID, &acc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create account failed: %w", err)
	}
	return acc, nil
}

// CalculateBalance derives balance strictly from immutable entries based on double-entry accounting rules
func (s *Service) CalculateBalance(ctx context.Context, accountID int, atTime *time.Time) (int64, error) {
	var accType AccountType
	err := s.Pool.QueryRow(ctx, `SELECT type FROM accounts WHERE id = $1`, accountID).Scan(&accType)
	if err != nil {
		return 0, fmt.Errorf("account not found: %w", err)
	}

	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount ELSE 0 END), 0)
		FROM entries e
		JOIN transactions t ON e.transaction_id = t.id
		WHERE e.account_id = $1
	`
	args := []interface{}{accountID}

	if atTime != nil {
		query += ` AND t.posted_at <= $2`
		args = append(args, *atTime)
	}

	var totalDebit, totalCredit int64
	err = s.Pool.QueryRow(ctx, query, args...).Scan(&totalDebit, &totalCredit)
	if err != nil {
		return 0, fmt.Errorf("calculate balance failed: %w", err)
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

// PostTransaction posts atomic double-entry postings with Idempotency & Concurrency safety
func (s *Service) PostTransaction(ctx context.Context, key, description string, entries []Entry) error {
	if len(entries) < 2 {
		return errors.New("a transaction requires at least two entries")
	}

	var totalDebit, totalCredit int64
	for _, e := range entries {
		if e.Amount <= 0 {
			return errors.New("entry amount must be positive")
		}
		if e.Direction == "debit" {
			totalDebit += e.Amount
		} else if e.Direction == "credit" {
			totalCredit += e.Amount
		} else {
			return errors.New("invalid entry direction: must be debit or credit")
		}
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("double-entry mismatch: total debits (%d) != total credits (%d)", totalDebit, totalCredit)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Idempotency Check
	var existingID int
	err = tx.QueryRow(ctx, `SELECT id FROM transactions WHERE idempotency_key = $1`, key).Scan(&existingID)
	if err == nil {
		// Already processed successfully
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("idempotency check error: %w", err)
	}

	// Insert Transaction Header
	var txID int
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions (idempotency_key, description) VALUES ($1, $2) RETURNING id`,
		key, description,
	).Scan(&txID)
	if err != nil {
		return fmt.Errorf("insert transaction failed: %w", err)
	}

	// Insert Entries and lock accounts for concurrency check
	for _, e := range entries {
		// Row-level lock to prevent concurrent overdrafts
		var accType AccountType
		err := tx.QueryRow(ctx, `SELECT type FROM accounts WHERE id = $1 FOR UPDATE`, e.AccountID).Scan(&accType)
		if err != nil {
			return fmt.Errorf("account %d not found: %w", e.AccountID, err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO entries (transaction_id, account_id, direction, amount) VALUES ($1, $2, $3, $4)`,
			txID, e.AccountID, e.Direction, e.Amount,
		)
		if err != nil {
			return fmt.Errorf("insert entry failed: %w", err)
		}
	}

	// Overdraft Prevention Check
	for _, e := range entries {
		bal, err := s.calculateBalanceTx(ctx, tx, e.AccountID)
		if err != nil {
			return err
		}
		// Wallets / Liabilities or Assets shouldn't drop below 0
		if bal < 0 {
			return fmt.Errorf("transaction rejected: insufficient balance on account %d (balance: %d)", e.AccountID, bal)
		}
	}

	return tx.Commit(ctx)
}

func (s *Service) calculateBalanceTx(ctx context.Context, tx pgx.Tx, accountID int) (int64, error) {
	var accType AccountType
	err := tx.QueryRow(ctx, `SELECT type FROM accounts WHERE id = $1`, accountID).Scan(&accType)
	if err != nil {
		return 0, err
	}

	var totalDebit, totalCredit int64
	err = tx.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount ELSE 0 END), 0)
		FROM entries WHERE account_id = $1
	`, accountID).Scan(&totalDebit, &totalCredit)
	if err != nil {
		return 0, err
	}

	switch accType {
	case AccountAsset, AccountExpense:
		return totalDebit - totalCredit, nil
	default:
		return totalCredit - totalDebit, nil
	}
}

// Wallet Operations
func (s *Service) Deposit(ctx context.Context, idempotencyKey string, walletID int, amount int64) error {
	// Cash Asset (1) Debit, Wallet Liability (walletID) Credit
	entries := []Entry{
		{AccountID: 1, Direction: "debit", Amount: amount},
		{AccountID: walletID, Direction: "credit", Amount: amount},
	}
	return s.PostTransaction(ctx, idempotencyKey, fmt.Sprintf("Deposit to wallet %d", walletID), entries)
}

func (s *Service) Withdraw(ctx context.Context, idempotencyKey string, walletID int, amount int64) error {
	// Wallet Liability (walletID) Debit, Cash Asset (1) Credit
	entries := []Entry{
		{AccountID: walletID, Direction: "debit", Amount: amount},
		{AccountID: 1, Direction: "credit", Amount: amount},
	}
	return s.PostTransaction(ctx, idempotencyKey, fmt.Sprintf("Withdraw from wallet %d", walletID), entries)
}

func (s *Service) Transfer(ctx context.Context, idempotencyKey string, fromWallet, toWallet int, amount int64) error {
	if fromWallet == toWallet {
		return errors.New("cannot transfer to the same wallet")
	}
	// From Wallet Liability Debit, To Wallet Liability Credit
	entries := []Entry{
		{AccountID: fromWallet, Direction: "debit", Amount: amount},
		{AccountID: toWallet, Direction: "credit", Amount: amount},
	}
	return s.PostTransaction(ctx, idempotencyKey, fmt.Sprintf("Transfer from %d to %d", fromWallet, toWallet), entries)
}
