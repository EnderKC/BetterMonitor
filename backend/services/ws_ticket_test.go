package services

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTerminalTicketClaims() WSTicketClaims {
	return WSTicketClaims{
		AdminID:        1,
		SessionVersion: 2,
		Purpose:        WSTicketTerminal,
		ServerID:       7,
		SessionID:      "session-1",
	}
}

func TestWSTicketIssueStoresOnlyDigestAndConsumes(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	store := NewWSTicketStore(30*time.Second, func() time.Time { return now })

	ticket, expiresAt, err := store.Issue(testTerminalTicketClaims())
	require.NoError(t, err)
	assert.Equal(t, now.Add(30*time.Second), expiresAt)

	raw, err := base64.RawURLEncoding.DecodeString(ticket)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	digest := sha256.Sum256(raw)
	require.Len(t, store.tickets, 1)
	stored, ok := store.tickets[digest]
	require.True(t, ok)
	assert.Equal(t, expiresAt, stored.ExpiresAt)
	assert.NotContains(t, stored.SessionID, ticket)

	claims, err := store.Consume(ticket, testTerminalTicketClaims())
	require.NoError(t, err)
	assert.Equal(t, testTerminalTicketClaims().AdminID, claims.AdminID)
	assert.Equal(t, testTerminalTicketClaims().SessionVersion, claims.SessionVersion)
	assert.Equal(t, expiresAt, claims.ExpiresAt)
	assert.Empty(t, store.tickets)

	_, err = store.Consume(ticket, testTerminalTicketClaims())
	assert.ErrorIs(t, err, ErrWSTicketInvalid)
}

func TestWSTicketRejectsMalformedAndExpiredTickets(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	store := NewWSTicketStore(30*time.Second, func() time.Time { return now })

	_, err := store.Consume("not-a-valid-ticket", testTerminalTicketClaims())
	assert.ErrorIs(t, err, ErrWSTicketInvalid)

	ticket, _, err := store.Issue(testTerminalTicketClaims())
	require.NoError(t, err)
	now = now.Add(30 * time.Second)

	_, err = store.Consume(ticket, testTerminalTicketClaims())
	assert.ErrorIs(t, err, ErrWSTicketInvalid)
	assert.Empty(t, store.tickets)
}

func TestWSTicketScopeMismatchConsumesTicket(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		expected WSTicketClaims
	}{
		{
			name: "purpose",
			expected: WSTicketClaims{
				Purpose:   WSTicketServer,
				ServerID:  7,
				SessionID: "session-1",
			},
		},
		{
			name: "server",
			expected: WSTicketClaims{
				Purpose:   WSTicketTerminal,
				ServerID:  8,
				SessionID: "session-1",
			},
		},
		{
			name: "session",
			expected: WSTicketClaims{
				Purpose:   WSTicketTerminal,
				ServerID:  7,
				SessionID: "session-2",
			},
		},
		{
			name: "life probe",
			expected: WSTicketClaims{
				Purpose:     WSTicketTerminal,
				ServerID:    7,
				LifeProbeID: 9,
				SessionID:   "session-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewWSTicketStore(30*time.Second, func() time.Time { return now })
			ticket, _, err := store.Issue(testTerminalTicketClaims())
			require.NoError(t, err)

			_, err = store.Consume(ticket, tt.expected)
			assert.ErrorIs(t, err, ErrWSTicketScopeMismatch)

			_, err = store.Consume(ticket, testTerminalTicketClaims())
			assert.ErrorIs(t, err, ErrWSTicketInvalid)
		})
	}
}

func TestWSTicketConcurrentConsumeSucceedsOnce(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	store := NewWSTicketStore(30*time.Second, func() time.Time { return now })
	expected := testTerminalTicketClaims()
	ticket, _, err := store.Issue(expected)
	require.NoError(t, err)

	var success atomic.Int32
	var invalid atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Consume(ticket, expected); err == nil {
				success.Add(1)
			} else if errors.Is(err, ErrWSTicketInvalid) {
				invalid.Add(1)
			} else {
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), success.Load())
	assert.Equal(t, int32(19), invalid.Load())
	assert.Zero(t, unexpected.Load())
}

func TestWSTicketIssueRejectsMissingIdentityAndUnknownPurpose(t *testing.T) {
	store := NewWSTicketStore(30*time.Second, time.Now)

	claims := testTerminalTicketClaims()
	claims.AdminID = 0
	_, _, err := store.Issue(claims)
	assert.Error(t, err)

	claims = testTerminalTicketClaims()
	claims.SessionVersion = 0
	_, _, err = store.Issue(claims)
	assert.Error(t, err)

	claims = testTerminalTicketClaims()
	claims.Purpose = WSTicketPurpose("unknown")
	_, _, err = store.Issue(claims)
	assert.Error(t, err)
}
