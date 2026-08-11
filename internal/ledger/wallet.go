package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// dbQuerier is satisfied by both pgx.Tx and pgx.Conn/pgxpool.Pool, so balance
// logic can run either as a standalone read or inside a caller-supplied
// transaction.
type dbQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type WalletService struct {
	engine *Engine
}

func NewWalletService(engine *Engine) *WalletService {
	return &WalletService{engine: engine}
}

func accountBalance(ctx context.Context, q dbQuerier, accountID string, lockForUpdate bool, atTime *time.Time) (balance int64, accType string, err error) {
	// 1. Хэрэв түгжих шаардлагатай бол эхлээд accounts мөрийг FOR UPDATE-аар түгжинэ
	if lockForUpdate {
		var dummy int
		if err := q.QueryRow(ctx, "SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE", accountID).Scan(&dummy); err != nil {
			return 0, "", fmt.Errorf("account not found or lock failed: %w", err)
		}
	}

	timeClause := ""
	args := []any{accountID}
	if atTime != nil {
		timeClause = "AND t.posted_at <= $2"
		args = append(args, *atTime)
	}

	// 2. Баланс бодох query дээрээ FOR UPDATE заалтыг хасна
	query := fmt.Sprintf(`
		SELECT
			a.type,
			COALESCE(SUM(CASE WHEN e.direction = 'DEBIT' THEN e.amount ELSE 0 END), 0) AS total_debits,
			COALESCE(SUM(CASE WHEN e.direction = 'CREDIT' THEN e.amount ELSE 0 END), 0) AS total_credits
		FROM accounts a
		LEFT JOIN entries e ON e.account_id = a.id
		LEFT JOIN transactions t ON e.transaction_id = t.id %s
		WHERE a.id = $1
		GROUP BY a.type;
	`, timeClause)

	var totalDebits, totalCredits int64
	if err := q.QueryRow(ctx, query, args...).Scan(&accType, &totalDebits, &totalCredits); err != nil {
		return 0, "", fmt.Errorf("account not found: %w", err)
	}

	switch accType {
	case "ASSET", "EXPENSE":
		balance = totalDebits - totalCredits
	case "LIABILITY", "EQUITY", "REVENUE":
		balance = totalCredits - totalDebits
	default:
		return 0, accType, fmt.Errorf("unknown account type: %s", accType)
	}

	return balance, accType, nil
}

// GetAccountBalance supporting Historical Balance Query (if atTime is provided)
func (s *WalletService) GetAccountBalance(ctx context.Context, accountID string, atTime *time.Time) (int64, error) {
	balance, _, err := accountBalance(ctx, s.engine.db, accountID, false, atTime)
	return balance, err
}

func (s *WalletService) Deposit(ctx context.Context, cashAssetAccountID, walletAccountID string, amount int64, idempotencyKey string, description string) error {
	if amount <= 0 {
		return errors.New("deposit amount must be positive")
	}

	req := TransactionRequest{
		IdempotencyKey: &idempotencyKey,
		Description:    description,
		Postings: []Posting{
			{AccountID: walletAccountID, Direction: Debit, Amount: amount},
			{AccountID: cashAssetAccountID, Direction: Credit, Amount: amount},
		},
	}

	return s.engine.PostTransaction(ctx, req)
}

func (s *WalletService) Withdraw(ctx context.Context, walletAccountID, cashAssetAccountID string, amount int64, idempotencyKey string, description string) error {
	if amount <= 0 {
		return errors.New("withdrawal amount must be positive")
	}

	tx, err := s.engine.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	balance, _, err := accountBalance(ctx, tx, walletAccountID, true, nil)
	if err != nil {
		return err
	}
	if balance < amount {
		return errors.New("insufficient funds: withdrawal would result in negative balance")
	}

	req := TransactionRequest{
		IdempotencyKey: &idempotencyKey,
		Description:    description,
		Postings: []Posting{
			{AccountID: cashAssetAccountID, Direction: Debit, Amount: amount},
			{AccountID: walletAccountID, Direction: Credit, Amount: amount},
		},
	}

	if err := s.engine.PostTransactionTx(ctx, tx, req); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Transfer transfers funds between two wallets safely
func (s *WalletService) Transfer(ctx context.Context, fromWalletID, toWalletID string, amount int64, idempotencyKey string, description string) error {
	if amount <= 0 {
		return errors.New("transfer amount must be positive")
	}
	if fromWalletID == toWalletID {
		return errors.New("cannot transfer to the same wallet")
	}

	tx, err := s.engine.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock sending wallet and check balance
	balance, _, err := accountBalance(ctx, tx, fromWalletID, true, nil)
	if err != nil {
		return err
	}
	if balance < amount {
		return errors.New("insufficient funds: transfer would result in negative balance")
	}

	req := TransactionRequest{
		IdempotencyKey: &idempotencyKey,
		Description:    description,
		Postings: []Posting{
			{AccountID: toWalletID, Direction: Debit, Amount: amount},
			{AccountID: fromWalletID, Direction: Credit, Amount: amount},
		},
	}

	if err := s.engine.PostTransactionTx(ctx, tx, req); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
