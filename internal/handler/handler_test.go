package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledger-service/internal/handler"
)

func TestHandleAccounts_Validation(t *testing.T) {
	h := handler.NewServer(nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Invalid JSON body to test 400 response
	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBuffer([]byte(`invalid json`)))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
