package controllers

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

func ValidateDockerStreamID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return fmt.Errorf("日志流ID无效")
	}
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("-_:.", char) {
			continue
		}
		return fmt.Errorf("日志流ID无效")
	}
	return nil
}

var (
	errDockerStreamExists         = errors.New("Docker log stream already exists")
	errDockerStreamNotFound       = errors.New("Docker log stream not found")
	errDockerStreamOwnerMismatch  = errors.New("Docker log stream owner mismatch")
	errDockerStreamServerMismatch = errors.New("Docker log stream server mismatch")
)

type dockerStreamEntry struct {
	streamID string
	serverID uint
	owner    *SafeConn
}

type dockerStreamRegistry struct {
	mu      sync.Mutex
	entries map[string]dockerStreamEntry
}

func newDockerStreamRegistry() *dockerStreamRegistry {
	return &dockerStreamRegistry{entries: make(map[string]dockerStreamEntry)}
}

func (r *dockerStreamRegistry) start(streamID string, serverID uint, owner *SafeConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[streamID]; exists {
		return errDockerStreamExists
	}
	r.entries[streamID] = dockerStreamEntry{streamID: streamID, serverID: serverID, owner: owner}
	return nil
}

func (r *dockerStreamRegistry) stop(streamID string, serverID uint, owner *SafeConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.entries[streamID]
	if !exists {
		return errDockerStreamNotFound
	}
	if entry.serverID != serverID {
		return errDockerStreamServerMismatch
	}
	if entry.owner != owner {
		return errDockerStreamOwnerMismatch
	}
	delete(r.entries, streamID)
	return nil
}

func (r *dockerStreamRegistry) current(streamID string, serverID uint) (dockerStreamEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[streamID]
	return entry, ok && entry.serverID == serverID
}

func (r *dockerStreamRegistry) end(streamID string, serverID uint) (dockerStreamEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[streamID]
	if !ok || entry.serverID != serverID {
		return dockerStreamEntry{}, false
	}
	delete(r.entries, streamID)
	return entry, true
}

func (r *dockerStreamRegistry) cleanupOwner(owner *SafeConn) []dockerStreamEntry {
	return r.cleanup(func(entry dockerStreamEntry) bool { return entry.owner == owner })
}

func (r *dockerStreamRegistry) cleanupServer(serverID uint) []dockerStreamEntry {
	return r.cleanup(func(entry dockerStreamEntry) bool { return entry.serverID == serverID })
}

func (r *dockerStreamRegistry) cleanup(match func(dockerStreamEntry) bool) []dockerStreamEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]dockerStreamEntry, 0)
	for id, entry := range r.entries {
		if match(entry) {
			removed = append(removed, entry)
			delete(r.entries, id)
		}
	}
	return removed
}

func (r *dockerStreamRegistry) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.entries) }
