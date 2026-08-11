package ledger

import (
	"context"
	"fmt"
	"time"
)

type WalletService struct {
	engine *Engine
}

func NewWalletService(engine *Engine) *WalletService {
	return &WalletService{engine: engine}
}

func (w *WalletService) Deposit(ctx context.Context, sourceAccountID, destAccountID string, amount int64, idempotencyKey, description string) error {
	if amount <= 0 {
		return fmt.Errorf("deposit amount must be positive")
	}

	req := TransactionRequest{
		IdempotencyKey: idempotencyKey,
		Description:    description,
		Postings: []Entry{
			{
				AccountID: destAccountID,
				Direction: Debit,
				Amount:    amount,
			},
			{
				AccountID: sourceAccountID,
				Direction: Credit,
				Amount:    amount,
			},
		},
	}

	return w.engine.PostTransaction(ctx, req)
}

func (w *WalletService) Withdraw(ctx context.Context, sourceAccountID, destAccountID string, amount int64, idempotencyKey, description string) error {
	if amount <= 0 {
		return fmt.Errorf("withdrawal amount must be positive")
	}

	req := TransactionRequest{
		IdempotencyKey: idempotencyKey,
		Description:    description,
		Postings: []Entry{
			{
				AccountID: destAccountID,
				Direction: Debit,
				Amount:    amount,
			},
			{
				AccountID: sourceAccountID,
				Direction: Credit,
				Amount:    amount,
			},
		},
	}

	return w.engine.PostTransaction(ctx, req)
}

func (w *WalletService) GetAccountBalance(ctx context.Context, accountID string, at *time.Time) (int64, error) {
	var query string
	var args []interface{}

	if at != nil {
		query = `
			SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE -amount END), 0)
			FROM entries e
			JOIN transactions t ON e.transaction_id = t.id
			WHERE e.account_id = $1 AND t.posted_at <= $2;
		`
		args = append(args, accountID, *at)
	} else {
		query = `
			SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE -amount END), 0)
			FROM entries
			WHERE account_id = $1;
		`
		args = append(args, accountID)
	}

	var balance int64
	err := w.engine.pool.QueryRow(ctx, query, args...).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("failed to get account balance: %w", err)
	}

	return balance, nil
}
