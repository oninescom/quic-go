package ackhandler

import (
	"github.com/oninescom/quic-go/congestion"
	cgInternal "github.com/oninescom/quic-go/internal/congestion"
	"github.com/oninescom/quic-go/internal/monotime"
	"github.com/oninescom/quic-go/internal/protocol"
)

var (
	_ cgInternal.SendAlgorithmWithDebugInfos      = (*ccAdapter)(nil)
	_ cgInternal.SendAlgorithmWithCongestionEvent = (*ccAdapterEx)(nil)
)

type ccAdapter struct {
	CC congestion.CongestionControl
}

func newCCAdapter(cc congestion.CongestionControl) cgInternal.SendAlgorithmWithDebugInfos {
	adapter := &ccAdapter{CC: cc}
	if batch, ok := cc.(congestion.BatchCongestionControl); ok {
		return &ccAdapterEx{ccAdapter: adapter, batch: batch}
	}
	return adapter
}

func (a *ccAdapter) TimeUntilSend(bytesInFlight protocol.ByteCount) monotime.Time {
	return monotime.FromTime(a.CC.TimeUntilSend(congestion.ByteCount(bytesInFlight)))
}

func (a *ccAdapter) HasPacingBudget(now monotime.Time) bool {
	return a.CC.HasPacingBudget(now.ToTime())
}

func (a *ccAdapter) OnPacketSent(sentTime monotime.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) {
	a.CC.OnPacketSent(sentTime.ToTime(), congestion.ByteCount(bytesInFlight), congestion.PacketNumber(packetNumber), congestion.ByteCount(bytes), isRetransmittable)
}

func (a *ccAdapter) CanSend(bytesInFlight protocol.ByteCount) bool {
	return a.CC.CanSend(congestion.ByteCount(bytesInFlight))
}

func (a *ccAdapter) MaybeExitSlowStart() { a.CC.MaybeExitSlowStart() }

func (a *ccAdapter) OnPacketAcked(number protocol.PacketNumber, ackedBytes protocol.ByteCount, priorInFlight protocol.ByteCount, eventTime monotime.Time) {
	a.CC.OnPacketAcked(congestion.PacketNumber(number), congestion.ByteCount(ackedBytes), congestion.ByteCount(priorInFlight), eventTime.ToTime())
}

func (a *ccAdapter) OnCongestionEvent(number protocol.PacketNumber, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount) {
	a.CC.OnCongestionEvent(congestion.PacketNumber(number), congestion.ByteCount(lostBytes), congestion.ByteCount(priorInFlight))
}

func (a *ccAdapter) OnSpuriousLoss(number protocol.PacketNumber) {
	a.CC.OnSpuriousLoss(congestion.PacketNumber(number))
}

func (a *ccAdapter) OnRetransmissionTimeout(packetsRetransmitted bool) {
	a.CC.OnRetransmissionTimeout(packetsRetransmitted)
}

func (a *ccAdapter) SetMaxDatagramSize(size protocol.ByteCount) {
	a.CC.SetMaxDatagramSize(congestion.ByteCount(size))
}

func (a *ccAdapter) InSlowStart() bool { return a.CC.InSlowStart() }
func (a *ccAdapter) InRecovery() bool  { return a.CC.InRecovery() }

func (a *ccAdapter) GetCongestionWindow() protocol.ByteCount {
	return protocol.ByteCount(a.CC.GetCongestionWindow())
}

type ccAdapterEx struct {
	*ccAdapter
	batch congestion.BatchCongestionControl
}

func (a *ccAdapterEx) OnCongestionEventEx(priorInFlight protocol.ByteCount, eventTime monotime.Time, ackedPackets []congestion.AckedPacketInfo, lostPackets []congestion.LostPacketInfo) {
	a.batch.OnCongestionEventEx(congestion.ByteCount(priorInFlight), eventTime.ToTime(), ackedPackets, lostPackets)
}
