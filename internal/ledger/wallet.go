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

// Deposit: Дансанд орлого оруулах
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

// Withdraw: Данснаас зарлага гаргах
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

// Transfer: Хоёр хэтэвч/дансны хооронд мөнгө шилжүүлэх
func (w *WalletService) Transfer(ctx context.Context, sourceAccountID, destAccountID string, amount int64, idempotencyKey, description string) error {
	if amount <= 0 {
		return fmt.Errorf("transfer amount must be positive")
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

// GetAccountBalance: Одоогийн эсвэл тухайн хугацаан дээрх балансыг тооцоолох (Historical balance)
func (w *WalletService) GetAccountBalance(ctx context.Context, accountID string, at *time.Time) (int64, error) {
	var query string
	var args []interface{}

	if at != nil {
		query = `
			SELECT COALESCE(
				SUM(
					CASE 
						WHEN a.type IN ('ASSET', 'EXPENSE') THEN 
							CASE WHEN e.direction = 'DEBIT' THEN e.amount ELSE -e.amount END
						ELSE 
							CASE WHEN e.direction = 'CREDIT' THEN e.amount ELSE -e.amount END
					END
				), 0)
			FROM entries e
			JOIN transactions t ON e.transaction_id = t.id
			JOIN accounts a ON e.account_id = a.id
			WHERE e.account_id = $1 AND t.posted_at <= $2;
		`
		args = append(args, accountID, *at)
	} else {
		query = `
			SELECT COALESCE(
				SUM(
					CASE 
						WHEN a.type IN ('ASSET', 'EXPENSE') THEN 
							CASE WHEN e.direction = 'DEBIT' THEN e.amount ELSE -e.amount END
						ELSE 
							CASE WHEN e.direction = 'CREDIT' THEN e.amount ELSE -e.amount END
					END
				), 0)
			FROM entries e
			JOIN accounts a ON e.account_id = a.id
			WHERE e.account_id = $1;
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
