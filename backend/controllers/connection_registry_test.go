package controllers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utils "github.com/user/server-ops-backend/internal/agenttransport"
)

func TestConnectionRegistryStaleHandleCannotDeleteReplacement(t *testing.T) {
	registry := NewConnectionRegistry[uint]()
	first := &SafeConn{}
	second := &SafeConn{}

	firstHandle := registry.Replace(7, first)
	secondHandle := registry.Replace(7, second)

	assert.False(t, registry.DeleteIfCurrent(7, firstHandle))
	current, ok := registry.Current(7)
	require.True(t, ok)
	assert.Same(t, second, current)
	assert.True(t, registry.DeleteIfCurrent(7, secondHandle))
	_, ok = registry.Current(7)
	assert.False(t, ok)
}

func TestConnectionRegistryConcurrentReplaceAndDelete(t *testing.T) {
	registry := NewConnectionRegistry[uint]()
	staleHandle := registry.Replace(7, &SafeConn{})

	const workers = 100
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			registry.Replace(7, &SafeConn{})
		}()
		go func() {
			defer wg.Done()
			registry.DeleteIfCurrent(7, staleHandle)
		}()
	}
	wg.Wait()

	finalConn := &SafeConn{}
	finalHandle := registry.Replace(7, finalConn)
	assert.False(t, registry.DeleteIfCurrent(7, staleHandle))

	current, ok := registry.Current(7)
	require.True(t, ok)
	assert.Same(t, finalConn, current)
	assert.True(t, registry.DeleteIfCurrent(7, finalHandle))
}

func TestConnectionRegistryCloseCurrentAndClearForTest(t *testing.T) {
	registry := NewConnectionRegistry[string]()

	registry.Replace("session-1", &SafeConn{})
	assert.True(t, registry.CloseCurrent("session-1"))
	_, ok := registry.Current("session-1")
	assert.False(t, ok)
	assert.False(t, registry.CloseCurrent("missing"))

	registry.Replace("session-2", &SafeConn{})
	registry.Replace("session-3", &SafeConn{})
	registry.ClearForTest()
	_, ok = registry.Current("session-2")
	assert.False(t, ok)
	_, ok = registry.Current("session-3")
	assert.False(t, ok)
}

func TestSafeConnCloseAllowsNilFixture(t *testing.T) {
	assert.NoError(t, (*SafeConn)(nil).Close())
	assert.NoError(t, (&SafeConn{}).Close())
}

func TestSubscriberRegistryRemoveDoesNotDropConcurrentSubscriber(t *testing.T) {
	for i := 0; i < 100; i++ {
		registry := NewSubscriberRegistry[uint, *SafeConn]()
		oldSubscriber := &SafeConn{}
		newSubscriber := &SafeConn{}
		registry.Add(7, oldSubscriber)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			registry.Remove(7, oldSubscriber)
		}()
		go func() {
			defer wg.Done()
			<-start
			registry.Add(7, newSubscriber)
		}()
		close(start)
		wg.Wait()

		assert.Contains(t, registry.Snapshot(7), newSubscriber)
	}
}

func TestSubscriberRegistryRemoveAllAtomicallyDetachesSubscribers(t *testing.T) {
	registry := NewSubscriberRegistry[uint, string]()
	registry.Add(7, "first")
	registry.Add(7, "second")

	removed := registry.RemoveAll(7)
	assert.ElementsMatch(t, []string{"first", "second"}, removed)
	assert.Empty(t, registry.Snapshot(7))
	assert.Empty(t, registry.RemoveAll(7))
	assert.False(t, registry.Remove(7, "missing"))
}

func TestAgentDisconnectFailsBrokerRequests(t *testing.T) {
	sent := make(chan utils.AgentCommandEnvelope, 1)
	utils.ConfigureAgentCommandSender(func(_ uint, command utils.AgentCommandEnvelope) error {
		sent <- command
		return nil
	})
	t.Cleanup(func() {
		utils.ConfigureAgentCommandSender(sendAgentCommandEnvelope)
		utils.FailAgentRequests(7, errAgentConnectionClosed)
	})

	result := make(chan error, 1)
	go func() {
		_, err := utils.SendAgentCommand(
			context.Background(),
			7,
			"nginx_command",
			map[string]interface{}{"action": "nginx_status"},
			"nginx_success",
		)
		result <- err
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broker request")
	}

	failAgentRequestsForDisconnectedServer(7)

	select {
	case err := <-result:
		assert.ErrorIs(t, err, errAgentConnectionClosed)
	case <-time.After(time.Second):
		t.Fatal("Agent disconnect did not fail broker request")
	}
}
