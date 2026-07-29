// Package record provides always-on, in-memory black-box recording of RCP
// request/response traffic with ring-buffer semantics, a checksummed binary
// log format, and a replay mode for regression testing.
//
// # Captured frame type moved at Milestone 57 (ROADMAP.md, v0.70.0)
//
// Per Phase 17's disposition table, this package is ADAPT-flagged:
// append-only checksummed traffic recording does not care what is inside
// the frames it records, so the log format and replay engine carry over
// unchanged in shape — only what gets captured changes. The retired
// Command/Response/Status trio (three separate Go types, one of them a
// periodic server-push broadcast that no longer exists in this model —
// see Milestone 56's own "TC18 has no native server-push broadcast"
// framing) is replaced by a single Entry: the requester avtp.StreamID plus
// the request acf.Message plus either the response acf.Message or an error
// string, wrapping a request.Handler rather than the retired
// rcp.Controller.
package record

//fusa:req REQ-REC-001
//fusa:req REQ-REC-002
//fusa:req REQ-REC-003
//fusa:req REQ-REC-004
//fusa:req REQ-REC-005
//fusa:req REQ-REC-006
//fusa:req REQ-REC-007
//fusa:req REQ-REC-008
