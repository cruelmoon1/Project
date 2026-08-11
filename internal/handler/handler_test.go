package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledger-service/internal/handler"
	"ledger-service/internal/ledger"
)

func setupTestServer(t *testing.T) (*handler.Handler, *pgxpool.Pool) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://tergel:tergelgod@localhost:5432/mydatabase?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	engine := ledger.NewEngine(pool)
	walletSvc := ledger.NewWalletService(engine)
	h := handler.NewServer(walletSvc, engine)

	return h, pool
}

func TestHandleAccounts_HTTP(t *testing.T) {
	h, pool := setupTestServer(t)
	defer pool.Close()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Test Account Creation via HTTP POST
	jsonBody := []byte(`{
		"id": "http-test-acc",
		"name": "HTTP Test Account",
		"type": "ASSET",
		"currency": "MNT"
	}`)

	req, err := http.NewRequest("POST", "/accounts", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusCreated)
	}
}
