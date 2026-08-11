package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledger-service/internal/handler"
	"ledger-service/internal/ledger"
)

// Автоматаар хүснэгт үүсгэх функц
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
	CREATE TABLE IF NOT EXISTS accounts (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL,
		currency VARCHAR(10) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id SERIAL PRIMARY KEY,
		idempotency_key VARCHAR(255) UNIQUE,
		description TEXT,
		posted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS entries (
		id SERIAL PRIMARY KEY,
		transaction_id INT REFERENCES transactions(id) ON DELETE CASCADE,
		account_id VARCHAR(255) REFERENCES accounts(id),
		direction VARCHAR(10) NOT NULL,
		amount BIGINT NOT NULL
	);
	`
	_, err := pool.Exec(ctx, query)
	return err
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://tergel:tergelgod@localhost:5432/mydatabase?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v\n", err)
	}

	log.Println("Connected to PostgreSQL successfully.")

	// --- ЭНД АВТОМАТ МИГРАЦИ АЖИЛЛАНА (hope) ---
	if err := runMigrations(ctx, pool); err != nil {
		log.Fatalf("Failed to run database migrations: %v\n", err)
	}
	log.Println("Database migrations applied successfully.")

	engine := ledger.NewEngine(pool)
	walletSvc := ledger.NewWalletService(engine)
	h := handler.NewServer(walletSvc, engine)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s...", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v\n", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting successfully.")
}
