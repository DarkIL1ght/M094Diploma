package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/memstore"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

type failingSessionLookup struct {
	session.Store
	err error
}

func (s failingSessionLookup) SessionByToken(context.Context, session.TokenHash) (session.Session, error) {
	return session.Session{}, s.err
}

func TestSessionRoutesReturnInternalErrorWhenLookupFails(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/me"},
		{method: http.MethodPost, path: "/api/session/recovery-key"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			storageErr := errors.New("internal storage failure")
			store := memstore.New()
			svc := session.New(failingSessionLookup{Store: store, err: storageErr})
			_, token, err := svc.Start(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			router := NewRouter(testLogger(&bytes.Buffer{}), Deps{Sessions: svc})
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil)
			req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			var response respond.Error
			decode(t, rec.Body.Bytes(), &response)
			if rec.Code != http.StatusInternalServerError || response.Code != "internal" {
				t.Fatalf("got HTTP %d with error code %q, want HTTP 500 with code internal", rec.Code, response.Code)
			}
			if strings.Contains(rec.Body.String(), storageErr.Error()) || rec.Header().Get("Set-Cookie") != "" {
				t.Fatal("response must neither expose storage details nor issue a replacement cookie")
			}
			if len(store.TokenHashes()) != 1 {
				t.Fatal("lookup failure must not create another session")
			}
		})
	}
}
