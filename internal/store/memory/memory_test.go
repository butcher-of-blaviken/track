package memory_test

import (
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
	"github.com/butcher-of-blaviken/track/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) core.Store { return memory.New() })
}
