package main

import (
	"fmt"
	"log"
	"net/http"

	"ledger-service/internal/db"
	"ledger-service/internal/handler"
	"ledger-service/internal/ledger"
)

func main() {
	// 1. Database Connection Pool үүсгэх
	pool, err := db.NewPool()
	if err != nil {
		log.Fatalf("Холбогдоход алдаа гарлаа: %v", err)
	}
	defer pool.Close()

	// 2. Ledger Engine болон Wallet Service-ээ эхлүүлэх
	engine := ledger.NewEngine(pool)
	walletSvc := ledger.NewWalletService(engine)

	// 3. Handler зангилаатай холбох (engine-ийг дамжуулна)
	h := handler.NewServer(walletSvc, engine)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	fmt.Println("Сервер 8080 порт дээр ажиллаж байна... (http://localhost:8080)")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Сервер зогслоо: %v", err)
	}
}
