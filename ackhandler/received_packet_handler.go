package ackhandler

import (
	"errors"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
)

var errInvalidPacketNumber = errors.New("ReceivedPacketHandler: Invalid packet number")

type receivedPacketHandler struct {
	largestObserved             protocol.PacketNumber
	lowerLimit                  protocol.PacketNumber
	largestObservedReceivedTime time.Time

	packetHistory *receivedPacketHistory

	ackSendDelay time.Duration

	packetsReceivedSinceLastAck                int
	retransmittablePacketsReceivedSinceLastAck int
	ackQueued                                  bool
	ackAlarm                                   time.Time
	lastAck                                    *wire.AckFrame

	version protocol.VersionNumber

	packets uint64

	//czy
	packetsHasDeadline  uint64
	packetsMeetDeadline uint64

	packetsMeetDeadlineSinceLastAck   uint16
	packetNotMeetDeadlineSinceLastAck uint16

	curNotSent uint16
	alpha      uint16
}

// NewReceivedPacketHandler creates a new receivedPacketHandler
func NewReceivedPacketHandler(version protocol.VersionNumber) ReceivedPacketHandler {
	return &receivedPacketHandler{
		packetHistory: newReceivedPacketHistory(),
		ackSendDelay:  protocol.AckSendDelay,
		version:       version,
	}
}

func (h *receivedPacketHandler) GetStatistics() (uint64, uint64, uint64) {
	return h.packets, h.packetsHasDeadline, h.packetsMeetDeadline
}

func (h *receivedPacketHandler) ReceivedPacket(packetNumber protocol.PacketNumber, shouldInstigateAck bool) error {
	if packetNumber == 0 {
		return errInvalidPacketNumber
	}

	// A new packet was received on that path and passes checks, so count it for stats
	h.packets++

	if packetNumber > h.largestObserved {
		h.largestObserved = packetNumber
		h.largestObservedReceivedTime = time.Now()
	}

	if packetNumber <= h.lowerLimit {
		return nil
	}

	if err := h.packetHistory.ReceivedPacket(packetNumber); err != nil {
		return err
	}
	h.maybeQueueAck(packetNumber, shouldInstigateAck)
	return nil
}

// SetLowerLimit sets a lower limit for acking packets.
// Packets with packet numbers smaller or equal than p will not be acked.
func (h *receivedPacketHandler) SetLowerLimit(p protocol.PacketNumber) {
	h.lowerLimit = p
	h.packetHistory.DeleteUpTo(p)
}

func (h *receivedPacketHandler) maybeQueueAck(packetNumber protocol.PacketNumber, shouldInstigateAck bool) {
	h.packetsReceivedSinceLastAck++

	if shouldInstigateAck {
		h.retransmittablePacketsReceivedSinceLastAck++
	}

	// always ack the first packet
	if h.lastAck == nil {
		h.ackQueued = true
	}

	if h.version < protocol.Version39 {
		// Always send an ack every 20 packets in order to allow the peer to discard
		// information from the SentPacketManager and provide an RTT measurement.
		// From QUIC 39, this is not needed anymore, since the peer will regularly send a retransmittable packet.
		if h.packetsReceivedSinceLastAck >= protocol.MaxPacketsReceivedBeforeAckSend {
			h.ackQueued = true
		}
	}

	// if the packet number is smaller than the largest acked packet, it must have been reported missing with the last ACK
	// note that it cannot be a duplicate because they're already filtered out by ReceivedPacket()
	if h.lastAck != nil && packetNumber < h.lastAck.LargestAcked {
		h.ackQueued = true
	}

	// check if a new missing range above the previously was created
	if h.lastAck != nil && h.packetHistory.GetHighestAckRange().First > h.lastAck.LargestAcked {
		h.ackQueued = true
	}

	if !h.ackQueued && shouldInstigateAck {
		if h.retransmittablePacketsReceivedSinceLastAck >= protocol.RetransmittablePacketsBeforeAck {
			h.ackQueued = true
		} else {
			if h.ackAlarm.IsZero() {
				h.ackAlarm = time.Now().Add(h.ackSendDelay)
			}
		}
	}

	if h.ackQueued {
		// cancel the ack alarm
		h.ackAlarm = time.Time{}
	}
}

func (h *receivedPacketHandler) GetAckFrame() *wire.AckFrame {
	if !h.ackQueued && (h.ackAlarm.IsZero() || h.ackAlarm.After(time.Now())) {
		return nil
	}

	ackRanges := h.packetHistory.GetAckRanges()
	ack := &wire.AckFrame{
		LargestAcked:       h.largestObserved,
		LowestAcked:        ackRanges[len(ackRanges)-1].First,
		PacketReceivedTime: h.largestObservedReceivedTime,
		NumMeetDeadline:    h.packetsMeetDeadlineSinceLastAck,
		NumHasDeadline:     h.packetNotMeetDeadlineSinceLastAck,
		CurNotSent:         h.curNotSent,
		Alpha:              h.alpha,
	}

	if len(ackRanges) > 1 {
		ack.AckRanges = ackRanges
	}

	h.lastAck = ack
	h.ackAlarm = time.Time{}
	h.ackQueued = false
	h.packetsReceivedSinceLastAck = 0
	h.retransmittablePacketsReceivedSinceLastAck = 0
	h.packetsMeetDeadlineSinceLastAck = 0
	h.packetNotMeetDeadlineSinceLastAck = 0

	return ack
}

func (h *receivedPacketHandler) GetClosePathFrame() *wire.ClosePathFrame {
	ackRanges := h.packetHistory.GetAckRanges()
	frame := &wire.ClosePathFrame{
		LargestAcked: h.largestObserved,
		LowestAcked:  ackRanges[len(ackRanges)-1].First,
	}

	if len(ackRanges) > 1 {
		frame.AckRanges = ackRanges
	}

	return frame
}

func (h *receivedPacketHandler) GetAlarmTimeout() time.Time { return h.ackAlarm }

func (h *receivedPacketHandler) StatisticPacketMeet(hdr *wire.PublicHeader, rcvTime time.Time) error {
	if !hdr.Deadline.IsZero() {
		meetTime := hdr.Deadline.Sub(rcvTime) //Deadline - RcvTime
		if meetTime > 0 {
			//meet deadline
			h.packetsHasDeadline++
			h.packetsMeetDeadline++

			h.packetsMeetDeadlineSinceLastAck++
			h.packetNotMeetDeadlineSinceLastAck++
		} else { //not meet deadline
			h.packetsHasDeadline++
			h.packetNotMeetDeadlineSinceLastAck++
		}
	}
	return nil
}

func (h *receivedPacketHandler) UpdateCurNotSent(curNotSent uint16) {
	h.curNotSent = curNotSent
}

func (h *receivedPacketHandler) UpdateAlpha(alpha uint16) {
	// Default alpha value is 10
	if alpha != 0 {
		h.alpha = alpha
	}
}
