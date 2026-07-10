package controllers

import "sync"

type ConnectionHandle struct {
	Conn       *SafeConn
	Generation uint64
}

type ConnectionRegistry[K comparable] struct {
	mu         sync.RWMutex
	generation uint64
	current    map[K]ConnectionHandle
}

func NewConnectionRegistry[K comparable]() *ConnectionRegistry[K] {
	return &ConnectionRegistry[K]{
		current: make(map[K]ConnectionHandle),
	}
}

func (r *ConnectionRegistry[K]) Replace(key K, conn *SafeConn) ConnectionHandle {
	r.mu.Lock()
	if r.current == nil {
		r.current = make(map[K]ConnectionHandle)
	}
	r.generation++
	handle := ConnectionHandle{
		Conn:       conn,
		Generation: r.generation,
	}
	previous := r.current[key]
	r.current[key] = handle
	r.mu.Unlock()

	if previous.Conn != nil && previous.Conn != conn {
		_ = previous.Conn.Close()
	}

	return handle
}

func (r *ConnectionRegistry[K]) Current(key K) (*SafeConn, bool) {
	r.mu.RLock()
	handle, ok := r.current[key]
	r.mu.RUnlock()
	if !ok || handle.Conn == nil {
		return nil, false
	}
	return handle.Conn, true
}

func (r *ConnectionRegistry[K]) DeleteIfCurrent(key K, handle ConnectionHandle) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.current[key]
	if !ok || current.Generation != handle.Generation || current.Conn != handle.Conn {
		return false
	}
	delete(r.current, key)
	return true
}

func (r *ConnectionRegistry[K]) CloseCurrent(key K) bool {
	r.mu.Lock()
	handle, ok := r.current[key]
	if ok {
		delete(r.current, key)
	}
	r.mu.Unlock()

	if !ok {
		return false
	}
	_ = handle.Conn.Close()
	return true
}

func (r *ConnectionRegistry[K]) ClearForTest() {
	r.mu.Lock()
	current := r.current
	r.current = make(map[K]ConnectionHandle)
	r.mu.Unlock()

	for _, handle := range current {
		_ = handle.Conn.Close()
	}
}

type SubscriberRegistry[K comparable, V comparable] struct {
	mu      sync.RWMutex
	entries map[K]map[V]struct{}
}

func NewSubscriberRegistry[K comparable, V comparable]() *SubscriberRegistry[K, V] {
	return &SubscriberRegistry[K, V]{
		entries: make(map[K]map[V]struct{}),
	}
}

func (r *SubscriberRegistry[K, V]) Add(key K, value V) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries == nil {
		r.entries = make(map[K]map[V]struct{})
	}
	values := r.entries[key]
	if values == nil {
		values = make(map[V]struct{})
		r.entries[key] = values
	}
	values[value] = struct{}{}
}

func (r *SubscriberRegistry[K, V]) Remove(key K, value V) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	values, ok := r.entries[key]
	if !ok {
		return false
	}
	if _, ok := values[value]; !ok {
		return false
	}
	delete(values, value)
	if len(values) == 0 {
		delete(r.entries, key)
	}
	return true
}

func (r *SubscriberRegistry[K, V]) Snapshot(key K) []V {
	r.mu.RLock()
	defer r.mu.RUnlock()

	values := r.entries[key]
	snapshot := make([]V, 0, len(values))
	for value := range values {
		snapshot = append(snapshot, value)
	}
	return snapshot
}

func (r *SubscriberRegistry[K, V]) RemoveAll(key K) []V {
	r.mu.Lock()
	values := r.entries[key]
	delete(r.entries, key)
	r.mu.Unlock()

	removed := make([]V, 0, len(values))
	for value := range values {
		removed = append(removed, value)
	}
	return removed
}
