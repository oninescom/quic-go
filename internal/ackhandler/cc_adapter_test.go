package ackhandler

import (
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/oninescom/quic-go/congestion"
	"github.com/oninescom/quic-go/internal/monotime"
	"github.com/oninescom/quic-go/internal/protocol"
	"github.com/oninescom/quic-go/internal/utils"
	"github.com/oninescom/quic-go/internal/wire"
	"github.com/stretchr/testify/require"
)

type recordingCongestionControl struct {
	id            int
	cwnd          congestion.ByteCount
	acked         []congestion.PacketNumber
	lost          []congestion.PacketNumber
	priorInFlight congestion.ByteCount
	eventTime     time.Time
	ackedBatch    []congestion.AckedPacketInfo
	lostBatch     []congestion.LostPacketInfo
	events        []string
}

func (c *recordingCongestionControl) TimeUntilSend(congestion.ByteCount) time.Time {
	return time.Time{}
}
func (c *recordingCongestionControl) HasPacingBudget(time.Time) bool { return true }
func (c *recordingCongestionControl) OnPacketSent(time.Time, congestion.ByteCount, congestion.PacketNumber, congestion.ByteCount, bool) {
}
func (c *recordingCongestionControl) CanSend(congestion.ByteCount) bool { return true }
func (c *recordingCongestionControl) MaybeExitSlowStart()               {}
func (c *recordingCongestionControl) OnPacketAcked(pn congestion.PacketNumber, _ congestion.ByteCount, _ congestion.ByteCount, _ time.Time) {
	c.acked = append(c.acked, pn)
}
func (c *recordingCongestionControl) OnCongestionEvent(pn congestion.PacketNumber, _ congestion.ByteCount, _ congestion.ByteCount) {
	c.lost = append(c.lost, pn)
}
func (c *recordingCongestionControl) OnSpuriousLoss(congestion.PacketNumber) {
	c.events = append(c.events, "spurious_loss")
}
func (c *recordingCongestionControl) OnRetransmissionTimeout(bool) {}
func (c *recordingCongestionControl) SetMaxDatagramSize(size congestion.ByteCount) {
	if c.cwnd == 0 {
		c.cwnd = size * 10
	}
}
func (c *recordingCongestionControl) InSlowStart() bool { return false }
func (c *recordingCongestionControl) InRecovery() bool  { return false }
func (c *recordingCongestionControl) GetCongestionWindow() congestion.ByteCount {
	if c.cwnd == 0 {
		return 12000
	}
	return c.cwnd
}
func (c *recordingCongestionControl) OnCongestionEventEx(priorInFlight congestion.ByteCount, eventTime time.Time, acked []congestion.AckedPacketInfo, lost []congestion.LostPacketInfo) {
	c.events = append(c.events, "batch")
	c.priorInFlight = priorInFlight
	c.eventTime = eventTime
	c.ackedBatch = append(c.ackedBatch[:0], acked...)
	c.lostBatch = append(c.lostBatch[:0], lost...)
}

var _ congestion.BatchCongestionControl = (*recordingCongestionControl)(nil)

func TestFactoryCreatesIndependentControllersAndPassesConnectionInfo(t *testing.T) {
	remoteAddr := &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 443}
	rttStats := utils.NewRTTStats()
	var infos []congestion.ConnectionInfo
	var controllers []*recordingCongestionControl
	factory := func(info congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		infos = append(infos, info)
		controller := &recordingCongestionControl{}
		controllers = append(controllers, controller)
		return controller, nil
	}
	newHandler := func() *sentPacketHandler {
		handler, err := NewSentPacketHandlerWithFactory(
			0,
			1200,
			rttStats,
			&utils.ConnectionStats{},
			true,
			false,
			nil,
			protocol.PerspectiveClient,
			nil,
			utils.DefaultLogger,
			factory,
			remoteAddr,
		)
		require.NoError(t, err)
		return handler.(*sentPacketHandler)
	}

	h1 := newHandler()
	h2 := newHandler()
	require.Len(t, infos, 2)
	require.Len(t, controllers, 2)
	require.NotSame(t, controllers[0], controllers[1])
	require.Same(t, controllers[0], h1.congestion.(*ccAdapterEx).CC)
	require.Same(t, controllers[1], h2.congestion.(*ccAdapterEx).CC)
	for _, info := range infos {
		require.Equal(t, remoteAddr, info.RemoteAddr)
		require.Equal(t, congestion.ByteCount(1200), info.InitialMaxDatagramSize)
		require.Equal(t, congestion.ByteCount(protocol.MaxCongestionWindowPackets*1200), info.MaxCongestionWindow)
		require.Same(t, rttStats, info.RTTStats)
	}
}

func TestFactoryErrorsAndNilControllerAreReturned(t *testing.T) {
	newHandler := func(factory congestion.Factory) error {
		_, err := NewSentPacketHandlerWithFactory(
			0,
			1200,
			utils.NewRTTStats(),
			&utils.ConnectionStats{},
			true,
			false,
			nil,
			protocol.PerspectiveClient,
			nil,
			utils.DefaultLogger,
			factory,
			nil,
		)
		return err
	}

	factoryErr := errors.New("invalid congestion controller")
	require.ErrorIs(t, newHandler(func(congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		return nil, factoryErr
	}), factoryErr)
	require.ErrorContains(t, newHandler(func(congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		return nil, nil
	}), "returned a nil controller")
	var typedNil *recordingCongestionControl
	require.ErrorContains(t, newHandler(func(congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		return typedNil, nil
	}), "returned a nil controller")
}

func TestFactoryRecreatesControllerAfterPathMigration(t *testing.T) {
	initialAddr := &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 443}
	newAddr := &net.UDPAddr{IP: net.IPv4(192, 0, 2, 2), Port: 443}
	var infos []congestion.ConnectionInfo
	var controllers []*recordingCongestionControl
	factory := func(info congestion.ConnectionInfo) (congestion.CongestionControl, error) {
		controller := &recordingCongestionControl{id: len(controllers) + 1}
		infos = append(infos, info)
		controllers = append(controllers, controller)
		return controller, nil
	}
	handler, err := NewSentPacketHandlerWithFactory(
		0,
		1200,
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		true,
		false,
		nil,
		protocol.PerspectiveClient,
		nil,
		utils.DefaultLogger,
		factory,
		initialAddr,
	)
	require.NoError(t, err)

	require.NoError(t, handler.MigratedPath(monotime.Now(), 1280, newAddr))
	require.Len(t, infos, 2)
	require.Len(t, controllers, 2)
	require.NotSame(t, controllers[0], controllers[1])
	require.Equal(t, initialAddr, infos[0].RemoteAddr)
	require.Equal(t, newAddr, infos[1].RemoteAddr)
	require.Equal(t, congestion.ByteCount(1280), infos[1].InitialMaxDatagramSize)
	require.Same(t, controllers[1], handler.(*sentPacketHandler).congestion.(*ccAdapterEx).CC)
}

func TestFactoryMigrationErrorIsReturnedWithoutFallback(t *testing.T) {
	factoryErr := errors.New("migration controller unavailable")
	controller := &recordingCongestionControl{}
	handlerCreation := 0
	handler, err := NewSentPacketHandlerWithFactory(
		0,
		1200,
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		true,
		false,
		nil,
		protocol.PerspectiveClient,
		nil,
		utils.DefaultLogger,
		func(congestion.ConnectionInfo) (congestion.CongestionControl, error) {
			if handlerCreation == 0 {
				handlerCreation++
				return controller, nil
			}
			return nil, factoryErr
		},
		nil,
	)
	require.NoError(t, err)

	oldController := handler.(*sentPacketHandler).congestion
	err = handler.MigratedPath(monotime.Now(), 1200, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 3), Port: 443})
	require.ErrorIs(t, err, factoryErr)
	require.Same(t, oldController, handler.(*sentPacketHandler).congestion)
}

func TestNilFactoryKeepsNativeCongestionController(t *testing.T) {
	handler, err := newSentPacketHandler(
		0,
		1200,
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		true,
		false,
		nil,
		protocol.PerspectiveClient,
		nil,
		utils.DefaultLogger,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "cubicSender", reflect.TypeOf(handler.congestion).Elem().Name())
}

func TestBatchControllerReceivesACKAndLossesAsOneEvent(t *testing.T) {
	controller := &recordingCongestionControl{}
	handler, err := NewSentPacketHandlerWithFactory(
		0,
		1200,
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		true,
		false,
		nil,
		protocol.PerspectiveClient,
		nil,
		utils.DefaultLogger,
		func(congestion.ConnectionInfo) (congestion.CongestionControl, error) { return controller, nil },
		nil,
	)
	require.NoError(t, err)

	var pns []protocol.PacketNumber
	now := monotime.Now()
	for range 4 {
		pn := handler.PopPacketNumber(protocol.PacketNumberSpaceAppData)
		handler.SentPacket(now, pn, protocol.InvalidPacketNumber, nil, []Frame{{Frame: &wire.PingFrame{}}}, protocol.Encryption1RTT, protocol.ECNNon, 1200, false, false)
		pns = append(pns, pn)
	}
	ackTime := now.Add(time.Millisecond)
	_, err = handler.ReceivedAck(&wire.AckFrame{AckRanges: ackRanges(pns[3])}, protocol.Encryption1RTT, ackTime)
	require.NoError(t, err)

	require.Equal(t, []congestion.PacketNumber{congestion.PacketNumber(pns[3])}, controller.acked)
	require.Equal(t, []congestion.PacketNumber{congestion.PacketNumber(pns[0])}, controller.lost)
	require.Equal(t, congestion.ByteCount(4*1200), controller.priorInFlight)
	require.True(t, controller.eventTime.Equal(ackTime.ToTime()))
	require.Equal(t, []congestion.AckedPacketInfo{{
		PacketNumber: congestion.PacketNumber(pns[3]),
		BytesAcked:   1200,
		ReceivedTime: ackTime.ToTime(),
	}}, controller.ackedBatch)
	require.Equal(t, []congestion.LostPacketInfo{{PacketNumber: congestion.PacketNumber(pns[0]), BytesLost: 1200}}, controller.lostBatch)
}

func TestBatchCongestionEventPrecedesSpuriousLoss(t *testing.T) {
	controller := &recordingCongestionControl{}
	handler, err := NewSentPacketHandlerWithFactory(
		0,
		1200,
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		true,
		false,
		nil,
		protocol.PerspectiveClient,
		nil,
		utils.DefaultLogger,
		func(congestion.ConnectionInfo) (congestion.CongestionControl, error) { return controller, nil },
		nil,
	)
	require.NoError(t, err)

	now := monotime.Now()
	var pns []protocol.PacketNumber
	for range 8 {
		pn := handler.PopPacketNumber(protocol.PacketNumberSpaceAppData)
		handler.SentPacket(now, pn, protocol.InvalidPacketNumber, nil, []Frame{{Frame: &wire.PingFrame{}}}, protocol.Encryption1RTT, protocol.ECNNon, 1200, false, false)
		pns = append(pns, pn)
	}
	_, err = handler.ReceivedAck(&wire.AckFrame{AckRanges: ackRanges(pns[3])}, protocol.Encryption1RTT, now.Add(time.Millisecond))
	require.NoError(t, err)
	require.Equal(t, []congestion.LostPacketInfo{{PacketNumber: congestion.PacketNumber(pns[0]), BytesLost: 1200}}, controller.lostBatch)

	controller.events = nil
	_, err = handler.ReceivedAck(&wire.AckFrame{AckRanges: ackRanges(pns[7], pns[0])}, protocol.Encryption1RTT, now.Add(2*time.Millisecond))
	require.NoError(t, err)
	require.Equal(t, []string{"batch", "spurious_loss"}, controller.events)
}
