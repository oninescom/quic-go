package congestion

import (
	"net"
	"time"
)

// ByteCount is a number of bytes in congestion-control state.
type ByteCount int64

// PacketNumber is a QUIC packet number.
type PacketNumber int64

// AckedPacketInfo describes one congestion-controlled packet acknowledged in an ACK event.
type AckedPacketInfo struct {
	PacketNumber PacketNumber
	BytesAcked   ByteCount
	ReceivedTime time.Time
}

// LostPacketInfo describes one congestion-controlled packet declared lost in an event.
type LostPacketInfo struct {
	PacketNumber PacketNumber
	BytesLost    ByteCount
}

// RTTStats exposes the read-only RTT measurements used by congestion controllers.
type RTTStats interface {
	MinRTT() time.Duration
	LatestRTT() time.Duration
	SmoothedRTT() time.Duration
	MeanDeviation() time.Duration
	MaxAckDelay() time.Duration
}

// ConnectionInfo contains the transport values needed to create one controller.
type ConnectionInfo struct {
	RemoteAddr             net.Addr
	InitialMaxDatagramSize ByteCount
	MaxCongestionWindow    ByteCount
	RTTStats               RTTStats
}

// Factory creates independent congestion-control state for a connection.
// It is called again if QUIC restarts congestion state, such as after path migration.
type Factory func(ConnectionInfo) (CongestionControl, error)

// CongestionControl implements quic-go's send-algorithm event contract.
type CongestionControl interface {
	TimeUntilSend(bytesInFlight ByteCount) time.Time
	HasPacingBudget(now time.Time) bool
	OnPacketSent(sentTime time.Time, bytesInFlight ByteCount, packetNumber PacketNumber, bytes ByteCount, isRetransmittable bool)
	CanSend(bytesInFlight ByteCount) bool
	MaybeExitSlowStart()
	OnPacketAcked(number PacketNumber, ackedBytes ByteCount, priorInFlight ByteCount, eventTime time.Time)
	OnCongestionEvent(number PacketNumber, lostBytes ByteCount, priorInFlight ByteCount)
	OnSpuriousLoss(number PacketNumber)
	OnRetransmissionTimeout(packetsRetransmitted bool)
	SetMaxDatagramSize(size ByteCount)
	InSlowStart() bool
	InRecovery() bool
	GetCongestionWindow() ByteCount
}

// BatchCongestionControl optionally receives the complete ACK/loss batch after
// quic-go has processed it and called the per-packet callbacks on
// CongestionControl. Implementations should use this batch as their ACK/loss
// accounting source instead of applying those per-packet callbacks again. The
// event slices are only valid for the callback; copy them if they need to be retained.
type BatchCongestionControl interface {
	CongestionControl
	OnCongestionEventEx(priorInFlight ByteCount, eventTime time.Time, ackedPackets []AckedPacketInfo, lostPackets []LostPacketInfo)
}
