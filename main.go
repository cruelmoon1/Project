package main

import (
	"context"
	"fmt"
	"log"

	"ledger-service/internal/db"
)

type Account struct {
	ID      int
	Name    string
	Balance float64
}

func main() {
	pool, err := db.NewPool()
	if err != nil {
		log.Fatalf("Холбогдоход алдаа гарлаа: %v", err)
	}
	defer pool.Close()

	var acc Account
	query := `SELECT id, name, balance FROM accounts WHERE name = $1`
	err = pool.QueryRow(context.Background(), query, "Alice").Scan(&acc.ID, &acc.Name, &acc.Balance)
	if err != nil {
		log.Fatalf("Пүрэв хийхэд алдаа гарлаа: %v", err)
	}

	fmt.Printf("Баазаас авсан мэдээлэл -> ID: %d, Нэр: %s, Үлдэгдэл: %.2f\n", acc.ID, acc.Name, acc.Balance)
}
