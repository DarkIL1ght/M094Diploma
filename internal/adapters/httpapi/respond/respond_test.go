package respond

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrFollowsContract(t *testing.T) {
	rec := httptest.NewRecorder()
	Err(rec, http.StatusNotFound, "not_found", "Не найдено")
	var e Error
	if rec.Code != http.StatusNotFound || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Code != "not_found" || e.Message != "Не найдено" {
		t.Fatalf("код %d, тело %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type %q", ct)
	}
}

func TestJSONWritesStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]int{"n": 1})
	if rec.Code != http.StatusCreated || strings.TrimSpace(rec.Body.String()) != `{"n":1}` {
		t.Fatalf("код %d, тело %q", rec.Code, rec.Body.String())
	}
}
