package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"ledger-service/internal/ledger"
)

type Server struct {
	walletSvc *ledger.WalletService
}

func NewServer(walletSvc *ledger.WalletService) *Server {
	return &Server{walletSvc: walletSvc}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /deposit", s.HandleDeposit)
	mux.HandleFunc("POST /withdraw", s.HandleWithdraw)
	mux.HandleFunc("POST /transfer", s.HandleTransfer)
	mux.HandleFunc("GET /balance", s.HandleGetBalance)
}

type DepositRequest struct {
	CashAccountID  string `json:"cash_account_id"`
	WalletID       string `json:"wallet_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Description    string `json:"description"`
}

type WithdrawRequest struct {
	WalletID       string `json:"wallet_id"`
	CashAccountID  string `json:"cash_account_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Description    string `json:"description"`
}

type TransferRequest struct {
	FromWalletID   string `json:"from_wallet_id"`
	ToWalletID     string `json:"to_wallet_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Description    string `json:"description"`
}

func (s *Server) HandleDeposit(w http.ResponseWriter, r *http.Request) {
	var req DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.walletSvc.Deposit(r.Context(), req.CashAccountID, req.WalletID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *Server) HandleWithdraw(w http.ResponseWriter, r *http.Request) {
	var req WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.walletSvc.Withdraw(r.Context(), req.WalletID, req.CashAccountID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *Server) HandleTransfer(w http.ResponseWriter, r *http.Request) {
	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.walletSvc.Transfer(r.Context(), req.FromWalletID, req.ToWalletID, req.Amount, req.IdempotencyKey, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *Server) HandleGetBalance(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		http.Error(w, "account_id is required", http.StatusBadRequest)
		return
	}

	var atTime *time.Time
	if atStr := r.URL.Query().Get("at"); atStr != "" {
		parsedTime, err := time.Parse(time.RFC3339, atStr)
		if err != nil {
			http.Error(w, "invalid timestamp format, use RFC3339", http.StatusBadRequest)
			return
		}
		atTime = &parsedTime
	}

	balance, err := s.walletSvc.GetAccountBalance(r.Context(), accountID, atTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"account_id": accountID,
		"balance":    balance,
	})
}
