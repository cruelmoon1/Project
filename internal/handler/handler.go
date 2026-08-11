package handler

import (
	"encoding/json"
	"net/http"

	"ledger-service/internal/ledger"
)

type Handler struct {
	walletSvc *ledger.WalletService
	engine    *ledger.Engine
}

func NewServer(walletSvc *ledger.WalletService, engine *ledger.Engine) *Handler {
	return &Handler{
		walletSvc: walletSvc,
		engine:    engine,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/accounts", h.handleAccounts)
	mux.HandleFunc("/transactions", h.handleTransactions)
	mux.HandleFunc("/deposit", h.handleDeposit)
	mux.HandleFunc("/withdraw", h.handleWithdraw)
	mux.HandleFunc("/transfer", h.handleTransfer)
	mux.HandleFunc("/balance", h.handleBalance)
}

// 1. POST /accounts - Шинэ данс нээх
func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID       string             `json:"id"`
		Name     string             `json:"name"`
		Type     ledger.AccountType `json:"type"`
		Currency string             `json:"currency"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	acc, err := h.engine.CreateAccount(r.Context(), req.ID, req.Name, req.Type, req.Currency)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(acc)
}

// 2. POST /transactions & GET /transactions?account_id=X
func (h *Handler) handleTransactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req ledger.TransactionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		if err := h.engine.PostTransaction(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})

	case http.MethodGet:
		accountID := r.URL.Query().Get("account_id")
		if accountID == "" {
			http.Error(w, "account_id parameter is required", http.StatusBadRequest)
			return
		}

		history, err := h.engine.GetAccountTransactions(r.Context(), accountID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// 3. POST /deposit
func (h *Handler) handleDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CashAccountID  string `json:"cash_account_id"`
		WalletID       string `json:"wallet_id"`
		Amount         int64  `json:"amount"`
		IdempotencyKey string `json:"idempotency_key"`
		Description    string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	err := h.walletSvc.Deposit(r.Context(), req.CashAccountID, req.WalletID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// 4. POST /withdraw
func (h *Handler) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WalletID       string `json:"wallet_id"`
		CashAccountID  string `json:"cash_account_id"`
		Amount         int64  `json:"amount"`
		IdempotencyKey string `json:"idempotency_key"`
		Description    string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	err := h.walletSvc.Withdraw(r.Context(), req.WalletID, req.CashAccountID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// 5. POST /transfer
func (h *Handler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FromWalletID   string `json:"from_wallet_id"`
		ToWalletID     string `json:"to_wallet_id"`
		Amount         int64  `json:"amount"`
		IdempotencyKey string `json:"idempotency_key"`
		Description    string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	err := h.walletSvc.Transfer(r.Context(), req.FromWalletID, req.ToWalletID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// 6. GET /balance?account_id=X
func (h *Handler) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		http.Error(w, "account_id required", http.StatusBadRequest)
		return
	}

	bal, err := h.walletSvc.GetAccountBalance(r.Context(), accountID, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"account_id": accountID,
		"balance":    bal,
	})
}
