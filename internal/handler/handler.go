package handler

import (
	"encoding/json"
	"net/http"
	"strings"

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
	mux.HandleFunc("/accounts/", h.handleAccountByID)
	mux.HandleFunc("/deposits", h.handleDeposit)
	mux.HandleFunc("/withdrawals", h.handleWithdraw)
	mux.HandleFunc("/transactions", h.handleTransactions)
}

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

func (h *Handler) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/accounts/")
	parts := strings.Split(path, "/")

	accountID := parts[0]
	if accountID == "" {
		http.Error(w, "Account ID is required", http.StatusBadRequest)
		return
	}

	// GET /accounts/{id}/transactions
	if len(parts) > 1 && parts[1] == "transactions" {
		history, err := h.engine.GetAccountTransactions(r.Context(), accountID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
		return
	}

	// GET /accounts/{id}
	bal, err := h.walletSvc.GetAccountBalance(r.Context(), accountID, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"account_id": accountID,
		"balance":    bal,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (h *Handler) handleDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SourceAccountID      string `json:"source_account_id"`
		DestinationAccountID string `json:"destination_account_id"`
		Amount               int64  `json:"amount"`
		IdempotencyKey       string `json:"idempotency_key"`
		Description          string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	err := h.walletSvc.Deposit(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (h *Handler) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SourceAccountID      string `json:"source_account_id"`
		DestinationAccountID string `json:"destination_account_id"`
		Amount               int64  `json:"amount"`
		IdempotencyKey       string `json:"idempotency_key"`
		Description          string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	err := h.walletSvc.Withdraw(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (h *Handler) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "transaction created"})
}
