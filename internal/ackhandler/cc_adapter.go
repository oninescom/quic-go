package ackhandler

import (
	"time"

	"github.com/oninescom/quic-go/congestion"
	cgInternal "github.com/oninescom/quic-go/internal/congestion"
	"github.com/oninescom/quic-go/internal/monotime"
	"github.com/oninescom/quic-go/internal/protocol"
)

var _ cgInternal.SendAlgorithm = (*ccAdapter)(nil)

type congestionControllerWithPacketInfo interface {
	OnPacketSentWithInfo(monotime.Time, protocol.ByteCount, protocol.PacketNumber, protocol.ByteCount, bool)
	OnPacketAckedWithInfo(protocol.PacketNumber, protocol.ByteCount, protocol.ByteCount, monotime.Time)
	OnCongestionEventWithInfo(protocol.PacketNumber, protocol.ByteCount, protocol.ByteCount)
}

type congestionControllerWithBatch interface {
	OnCongestionEventBatch(protocol.ByteCount, monotime.Time, []congestion.AckedPacketInfo, []congestion.LostPacketInfo)
}

type ccAdapter struct {
	CC            congestion.CongestionControl
	bytesInFlight protocol.ByteCount
	packetSizes   map[protocol.PacketNumber]protocol.ByteCount
}

func newCCAdapter(cc congestion.CongestionControl) cgInternal.SendAlgorithm {
	adapter := &ccAdapter{
		CC:          cc,
		packetSizes: make(map[protocol.PacketNumber]protocol.ByteCount),
	}
	if batch, ok := cc.(congestion.BatchCongestionControl); ok {
		return &ccAdapterEx{ccAdapter: adapter, batch: batch}
	}
	return adapter
}

func (a *ccAdapter) TimeUntilSend() monotime.Time {
	return monotime.FromTime(a.CC.TimeUntilSend(congestion.ByteCount(a.bytesInFlight)))
}

func (a *ccAdapter) SetBytesInFlight(bytesInFlight protocol.ByteCount) {
	a.bytesInFlight = bytesInFlight
}

func (a *ccAdapter) HasPacingBudget(now monotime.Time) bool {
	return a.CC.HasPacingBudget(now.ToTime())
}

func (a *ccAdapter) OnPacketSent(sentTime monotime.Time, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) {
	a.OnPacketSentWithInfo(sentTime, a.bytesInFlight, packetNumber, bytes, isRetransmittable)
}

func (a *ccAdapter) OnPacketSentWithInfo(sentTime monotime.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) {
	a.bytesInFlight = bytesInFlight
	a.packetSizes[packetNumber] = bytes
	a.CC.OnPacketSent(sentTime.ToTime(), congestion.ByteCount(bytesInFlight), congestion.PacketNumber(packetNumber), congestion.ByteCount(bytes), isRetransmittable)
	if isRetransmittable {
		a.bytesInFlight += bytes
	}
}

func (a *ccAdapter) CanSend(bytesInFlight protocol.ByteCount) bool {
	a.bytesInFlight = bytesInFlight
	return a.CC.CanSend(congestion.ByteCount(bytesInFlight))
}

func (a *ccAdapter) MaybeExitSlowStart() { a.CC.MaybeExitSlowStart() }

func (a *ccAdapter) OnPacketAcked(number protocol.PacketNumber, priorInFlight protocol.ByteCount) {
	a.OnPacketAckedWithInfo(number, a.packetSizes[number], priorInFlight, monotime.FromTime(time.Now()))
}

func (a *ccAdapter) OnPacketAckedWithInfo(number protocol.PacketNumber, ackedBytes, priorInFlight protocol.ByteCount, eventTime monotime.Time) {
	a.CC.OnPacketAcked(congestion.PacketNumber(number), congestion.ByteCount(ackedBytes), congestion.ByteCount(priorInFlight), eventTime.ToTime())
	delete(a.packetSizes, number)
}

func (a *ccAdapter) OnCongestionEvent(number protocol.PacketNumber) {
	a.OnCongestionEventWithInfo(number, 0, a.bytesInFlight)
}

func (a *ccAdapter) OnCongestionEventWithInfo(number protocol.PacketNumber, lostBytes, priorInFlight protocol.ByteCount) {
	a.CC.OnCongestionEvent(congestion.PacketNumber(number), congestion.ByteCount(lostBytes), congestion.ByteCount(priorInFlight))
	delete(a.packetSizes, number)
}

func (a *ccAdapter) OnSpuriousLoss(number protocol.PacketNumber) {
	a.CC.OnSpuriousLoss(congestion.PacketNumber(number))
}

func (a *ccAdapter) SetMaxDatagramSize(size protocol.ByteCount) {
	a.CC.SetMaxDatagramSize(congestion.ByteCount(size))
}

func (a *ccAdapter) GetCongestionWindow() protocol.ByteCount {
	return protocol.ByteCount(a.CC.GetCongestionWindow())
}

type ccAdapterEx struct {
	*ccAdapter
	batch congestion.BatchCongestionControl
}

func (a *ccAdapterEx) OnCongestionEventBatch(priorInFlight protocol.ByteCount, eventTime monotime.Time, ackedPackets []congestion.AckedPacketInfo, lostPackets []congestion.LostPacketInfo) {
	a.batch.OnCongestionEventEx(congestion.ByteCount(priorInFlight), eventTime.ToTime(), ackedPackets, lostPackets)
}
