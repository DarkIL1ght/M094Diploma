package memstore

import (
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session/sessiontest"
)

func TestStoreContract(t *testing.T) {
	sessiontest.Run(t, func(*testing.T) session.Store { return New() })
}
