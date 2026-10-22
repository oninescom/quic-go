package quic

import (
	"testing"
	"time"

	"github.com/oninescom/quic-go/congestion"
	"github.com/stretchr/testify/require"
)

type factoryTestController struct{ id int }

func (*factoryTestController) TimeUntilSend(congestion.ByteCount) time.Time { return time.Time{} }
func (*factoryTestController) HasPacingBudget(time.Time) bool               { return true }
func (*factoryTestController) OnPacketSent(time.Time, congestion.ByteCount, congestion.PacketNumber, congestion.ByteCount, bool) {
}
func (*factoryTestController) CanSend(congestion.ByteCount) bool { return true }
func (*factoryTestController) MaybeExitSlowStart()               {}
func (*factoryTestController) OnPacketAcked(congestion.PacketNumber, congestion.ByteCount, congestion.ByteCount, time.Time) {
}
func (*factoryTestController) OnCongestionEvent(congestion.PacketNumber, congestion.ByteCount, congestion.ByteCount) {
}
func (*factoryTestController) OnSpuriousLoss(congestion.PacketNumber)    {}
func (*factoryTestController) OnRetransmissionTimeout(bool)              {}
func (*factoryTestController) SetMaxDatagramSize(congestion.ByteCount)   {}
func (*factoryTestController) InSlowStart() bool                         { return false }
func (*factoryTestController) InRecovery() bool                          { return false }
func (*factoryTestController) GetCongestionWindow() congestion.ByteCount { return 12000 }

var _ congestion.CongestionControl = (*factoryTestController)(nil)

func TestCongestionControlFactoryRunsForEachClientAndServerConnection(t *testing.T) {
	var infos []congestion.ConnectionInfo
	var controllers []*factoryTestController
	factory := func(info congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		controller := &factoryTestController{id: len(controllers) + 1}
		infos = append(infos, info)
		controllers = append(controllers, controller)
		return controller, nil
	}
	config := &Config{DisablePathMTUDiscovery: true, CongestionControlFactory: factory}

	client1 := newClientTestConnection(t, nil, config, false)
	client2 := newClientTestConnection(t, nil, config, false)
	server := newServerTestConnection(t, nil, config, false)

	require.Len(t, infos, 3)
	require.Len(t, controllers, 3)
	require.NotSame(t, controllers[0], controllers[1])
	require.NotSame(t, controllers[0], controllers[2])
	require.NotSame(t, controllers[1], controllers[2])
	require.Equal(t, client1.conn.conn.RemoteAddr(), infos[0].RemoteAddr)
	require.Equal(t, client2.conn.conn.RemoteAddr(), infos[1].RemoteAddr)
	require.Equal(t, server.remoteAddr, infos[2].RemoteAddr)
	for _, info := range infos {
		require.NotNil(t, info.RTTStats)
		require.GreaterOrEqual(t, info.InitialMaxDatagramSize, congestion.ByteCount(1200))
		require.GreaterOrEqual(t, info.MaxCongestionWindow, info.InitialMaxDatagramSize)
	}
}
