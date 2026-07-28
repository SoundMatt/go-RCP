package fragment

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/request"
)

// Submitter is the subset of *request.Dispatcher's own method set Gateway
// needs, used here without requiring the request package to import this
// one — the same "wrap, don't edit" retrofit request.Dispatcher itself
// uses around a Phase 14/16 endpoint's Handler (see request/dispatcher.go).
// *request.Dispatcher satisfies Submitter unmodified.
type Submitter interface {
	Submit(requester avtp.StreamID, req acf.Message) (request.TicketID, error)
}

// Gateway is the ROADMAP.md Milestone 52 integration point named in this
// package's own design brief: it sits in front of a request.Dispatcher (via
// Submitter) so a fragmented request participates in the same
// StateQueued->StateStarted->StateExecuting->StateFinalized lifecycle
// request/doc.go describes for every other Kind, rather than bypassing it.
// Gateway never touches request.Dispatcher's own state machine, admission
// policy, or Kind taxonomy — it only ever calls Submit once, with the
// already-fully-reassembled Message a caller's segments describe, the exact
// same shape Submit already accepts from an unfragmented sender today.
//
// Gateway is deliberately asymmetric: reassembly (Submit) is generic over
// any Submitter, matching how request.Dispatcher.Submit takes an
// acf.Message and returns a request.TicketID regardless of Kind. Segmenting
// an outgoing response (Response) needs no Dispatcher reference of its
// own — a caller supplies the already-resolved response Message itself
// (typically request.Dispatcher.Response's own return value), so Response
// is a thin convenience wrapper around Split, not a second Dispatcher
// dependency.
type Gateway struct {
	Dispatcher     Submitter
	Reassembler    *Reassembler
	MaxSegmentBody int
}

// NewGateway returns a Gateway wrapping dispatcher, reassembling inbound
// segments with re (create one with NewReassembler/NewReassemblerWithClock)
// and splitting outgoing responses at maxSegmentBody bytes (a non-positive
// value is replaced with DefaultMaxSegmentBody).
func NewGateway(dispatcher Submitter, re *Reassembler, maxSegmentBody int) *Gateway {
	if maxSegmentBody <= 0 {
		maxSegmentBody = DefaultMaxSegmentBody
	}
	return &Gateway{Dispatcher: dispatcher, Reassembler: re, MaxSegmentBody: maxSegmentBody}
}

// Submit feeds one inbound AVTPDU's Message, addressed to/from requester,
// through g.Reassembler. An ordinary unfragmented Message (or a sequence's
// terminal segment) is, once complete, handed to g.Dispatcher.Submit
// exactly as request.Dispatcher.Submit would receive it directly — the
// resulting request.TicketID is returned unchanged. A non-terminal segment
// is buffered and returns (0, ErrAwaitingSegments): there is no ticket yet,
// and the caller has nothing further to do with this particular segment
// until the sequence's remaining segments arrive. Any other error (see
// Reassembler.Add and Submitter.Submit) means the message was rejected
// outright and, for a reassembly failure, that the whole in-progress
// sequence was abandoned.
func (g *Gateway) Submit(requester avtp.StreamID, req acf.Message) (request.TicketID, error) {
	complete, err := g.Reassembler.Add(requester, req)
	if err != nil {
		return 0, err
	}
	if !complete {
		return 0, ErrAwaitingSegments
	}

	key := KeyOf(requester, req)
	combined, err := g.Reassembler.Finish(key)
	if err != nil {
		return 0, err
	}
	return g.Dispatcher.Submit(requester, combined)
}

// Response splits resp into the segment sequence Split defines when its
// Body exceeds g.MaxSegmentBody, for a caller to transmit as one AVTPDU per
// returned Message (e.g. once request.Dispatcher.Response has resolved a
// ticket Submit admitted). A resp that already fits within one segment is
// returned as []acf.Message{resp} unchanged, exactly as Split itself would.
func (g *Gateway) Response(resp acf.Message) ([]acf.Message, error) {
	return Split(resp, g.MaxSegmentBody)
}
