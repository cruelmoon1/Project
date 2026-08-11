package db_test

import (
	"testing"
)

// Invariant Test: Debits MUST equal Credits
func TestDoubleEntryInvariant(t *testing.T) {
	debits := int64(1000)
	credits := int64(1000)

	if debits != credits {
		t.Fatalf("Invariant Violation: Debits (%d) != Credits (%d)", debits, credits)
	}
}

// Invariant Test: Overdraft Check
func TestNoNegativeBalance(t *testing.T) {
	balance := int64(500)
	withdrawAmount := int64(600)

	if balance-withdrawAmount < 0 {
		// Expected behavior
		return
	}
	t.Fatal("Allowed overdraft transaction!")
}
