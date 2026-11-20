package session_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/memstore"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

func TestStartDoesNotPersistSessionWhenRandomSourceIsShort(t *testing.T) {
	cases := []struct {
		name  string
		bytes int
	}{
		{name: "session ID", bytes: 15},
		{name: "display name", bytes: 23},
		{name: "token", bytes: 55},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := memstore.New()
			svc := session.New(store, session.WithRand(bytes.NewReader(make([]byte, tc.bytes))))

			sess, token, err := svc.Start(t.Context())
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("Start: got %v, want %v", err, io.ErrUnexpectedEOF)
			}
			if sess != (session.Session{}) || token != "" || len(store.TokenHashes()) != 0 {
				t.Fatal("Start must neither persist a token nor return a partial session")
			}
		})
	}
}

func TestResolveAtVisitAndExpirationBoundaries(t *testing.T) {
	const ttl = 2 * time.Hour
	cases := []struct {
		name     string
		elapsed  time.Duration
		wantSeen time.Duration
		wantErr  error
	}{
		{name: "at the refresh interval", elapsed: time.Hour},
		{name: "past the refresh interval", elapsed: time.Hour + time.Nanosecond, wantSeen: time.Hour + time.Nanosecond},
		{name: "at the TTL", elapsed: ttl, wantSeen: ttl},
		{name: "past the TTL", elapsed: ttl + time.Nanosecond, wantErr: session.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			c := clock{t: start}
			store := memstore.New()
			svc := session.New(store, session.WithClock(c.now), session.WithTTL(ttl))
			created, token, err := svc.Start(ctx)
			if err != nil {
				t.Fatal(err)
			}

			c.t = start.Add(tc.elapsed)
			resolved, err := svc.Resolve(ctx, token)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Resolve: got %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && resolved.ID != created.ID {
				t.Fatalf("Resolve returned session %q, want %q", resolved.ID, created.ID)
			}
			stored, err := store.SessionByToken(ctx, session.HashToken(token))
			if err != nil {
				t.Fatal(err)
			}
			if want := start.Add(tc.wantSeen); !stored.TokenLastSeen.Equal(want) {
				t.Fatalf("stored last visit: got %v, want %v", stored.TokenLastSeen, want)
			}
		})
	}
}
