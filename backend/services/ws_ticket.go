package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

const DefaultWSTicketTTL = 30 * time.Second

var (
	ErrWSTicketInvalid       = errors.New("websocket ticket invalid")
	ErrWSTicketScopeMismatch = errors.New("websocket ticket scope mismatch")
)

type WSTicketPurpose string

const (
	WSTicketServer          WSTicketPurpose = "server"
	WSTicketMonitor         WSTicketPurpose = "monitor"
	WSTicketTerminal        WSTicketPurpose = "terminal"
	WSTicketServerList      WSTicketPurpose = "server_list"
	WSTicketLifeProbeList   WSTicketPurpose = "life_probe_list"
	WSTicketLifeProbeDetail WSTicketPurpose = "life_probe_detail"
)

type WSTicketClaims struct {
	AdminID        uint
	SessionVersion uint64
	Purpose        WSTicketPurpose
	ServerID       uint
	LifeProbeID    uint
	SessionID      string
	ExpiresAt      time.Time
}

type WSTicketStore struct {
	mu      sync.Mutex
	tickets map[[32]byte]WSTicketClaims
	ttl     time.Duration
	now     func() time.Time
}

func NewWSTicketStore(ttl time.Duration, now func() time.Time) *WSTicketStore {
	if ttl <= 0 {
		ttl = DefaultWSTicketTTL
	}
	if now == nil {
		now = time.Now
	}
	return &WSTicketStore{
		tickets: make(map[[32]byte]WSTicketClaims),
		ttl:     ttl,
		now:     now,
	}
}

func (s *WSTicketStore) Issue(claims WSTicketClaims) (string, time.Time, error) {
	if claims.AdminID == 0 || claims.SessionVersion == 0 {
		return "", time.Time{}, fmt.Errorf("ticket administrator identity is required")
	}
	if !claims.Purpose.valid() {
		return "", time.Time{}, fmt.Errorf("unsupported websocket ticket purpose")
	}

	for {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", time.Time{}, fmt.Errorf("generate websocket ticket: %w", err)
		}
		digest := sha256.Sum256(raw)
		now := s.now()
		expiresAt := now.Add(s.ttl)
		claims.ExpiresAt = expiresAt

		s.mu.Lock()
		s.deleteExpiredLocked(now)
		if _, exists := s.tickets[digest]; exists {
			s.mu.Unlock()
			continue
		}
		s.tickets[digest] = claims
		s.mu.Unlock()

		return base64.RawURLEncoding.EncodeToString(raw), expiresAt, nil
	}
}

func (s *WSTicketStore) Consume(ticket string, expected WSTicketClaims) (WSTicketClaims, error) {
	raw, err := base64.RawURLEncoding.DecodeString(ticket)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != ticket {
		return WSTicketClaims{}, ErrWSTicketInvalid
	}

	digest := sha256.Sum256(raw)
	now := s.now()

	s.mu.Lock()
	claims, ok := s.tickets[digest]
	if ok {
		delete(s.tickets, digest)
	}
	s.deleteExpiredLocked(now)
	if !ok || !now.Before(claims.ExpiresAt) {
		s.mu.Unlock()
		return WSTicketClaims{}, ErrWSTicketInvalid
	}
	if !claims.matches(expected) {
		s.mu.Unlock()
		return WSTicketClaims{}, ErrWSTicketScopeMismatch
	}
	s.mu.Unlock()
	return claims, nil
}

func (s *WSTicketStore) deleteExpiredLocked(now time.Time) {
	for digest, claims := range s.tickets {
		if !now.Before(claims.ExpiresAt) {
			delete(s.tickets, digest)
		}
	}
}

func (p WSTicketPurpose) valid() bool {
	switch p {
	case WSTicketServer,
		WSTicketMonitor,
		WSTicketTerminal,
		WSTicketServerList,
		WSTicketLifeProbeList,
		WSTicketLifeProbeDetail:
		return true
	default:
		return false
	}
}

func (claims WSTicketClaims) matches(expected WSTicketClaims) bool {
	if claims.Purpose != expected.Purpose ||
		claims.ServerID != expected.ServerID ||
		claims.LifeProbeID != expected.LifeProbeID ||
		claims.SessionID != expected.SessionID {
		return false
	}
	if expected.AdminID != 0 && claims.AdminID != expected.AdminID {
		return false
	}
	if expected.SessionVersion != 0 && claims.SessionVersion != expected.SessionVersion {
		return false
	}
	return true
}
