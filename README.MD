# General Ledger and Wallet Service in Go

A double-entry general ledger engine with a wallet layer built in Go and PostgreSQL.

## Features & Invariants
- **Double-Entry Accounting:** Strictly enforces Debits = Credits for all entries.
- **Immutability:** Entries are read-only and never updated or deleted.
- **Idempotency:** Accepts an `idempotency_key` per operation to prevent duplicate execution.
- **Concurrency Safety:** Uses PostgreSQL row-level locks (`SELECT ... FOR UPDATE`) to prevent overdrafts.
- **Historical Balances:** Supports querying balances at any historical timestamp (`at` query param).

## Accounting Mapping (Why Debits & Credits?)
- **Deposit:**
  - `DEBIT` Asset (Cash Clearing) — Increases platform cash.
  - `CREDIT` Liability (User Wallet) — Increases what the platform owes the user.
- **Withdrawal:**
  - `DEBIT` Liability (User Wallet) — Decreases what the platform owes the user.
  - `CREDIT` Asset (Cash Clearing) — Decreases platform cash.
- **Transfer:**
  - `DEBIT` Liability (Sender Wallet) — Decreases sender balance.
  - `CREDIT` Liability (Receiver Wallet) — Increases receiver balance.

## How to Run
```bash
docker compose up --build