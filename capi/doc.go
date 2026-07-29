// Package capi provides a C-compatible, handle-based API surface for
// go-RCP RC clients targeting the OPEN Alliance TC18 Remote Control
// Protocol.
//
// RTOS firmware and bare-metal C code can interact with go-RCP by building
// this package with "-buildmode=c-shared" (shared library) or
// "-buildmode=c-archive" (static archive), then linking against the
// generated header and library.
//
// # Rebuilt at Milestone 57 (ROADMAP.md, v0.70.0)
//
// The pre-TC18 C ABI mirrored Controller/Command directly
// (rcp_new_controller(zone), rcp_send(handle, zone, type, payload, ...)).
// Per Phase 17's disposition table, that surface had to be redesigned, not
// just recompiled against new types, once Controller/Command themselves are
// being replaced: this package now exposes an Endpoint-request-shaped API —
// a handle addresses one RC client (identified by its own avtp.StreamID),
// and every request names an avtp.ByteBusID endpoint plus
// acf.ControlFlags/body rather than a Zone and CommandType. The
// Subscribe/PollStatus surface is dropped outright, not adapted: TC18 has
// no native server-push broadcast to subscribe to (the same "caller-driven,
// not server-push" framing Milestone 56's ddsbr/mqttbr/grpcbridge/
// restbridge rebuild already established for this repo), so there is
// nothing left for a C caller to poll.
//
// # C ABI surface (illustrative; no cgo //export directives in this
// milestone's tree — see the "Explicit non-goal" note below)
//
//	int32_t rcp_new_controller(const uint8_t stream_id[8],
//	                           const char *server_addr, int32_t addr_len);
//	int32_t rcp_request(int32_t handle, uint8_t addr, uint8_t control,
//	                    const uint8_t *body, int32_t body_len,
//	                    int32_t timeout_ms,
//	                    uint8_t *resp_control_out,
//	                    uint8_t *resp_body_out, int32_t resp_body_cap,
//	                    int32_t *resp_body_len_out);
//	void    rcp_close(int32_t handle);
//
// # Explicit non-goal
//
// This milestone's tree does not add a //go:build cgo file with actual
// //export directives or a generated rcp.h header — the pre-TC18 capi.go
// this package replaces did not either (its own doc comment described the
// intended C surface without generating it). Wiring an actual c-shared/
// c-archive build is left as a follow-on, unchanged in scope from before
// this milestone's rebuild.
package capi

//fusa:req REQ-CAPI-001
//fusa:req REQ-CAPI-002
//fusa:req REQ-CAPI-003
//fusa:req REQ-CAPI-004
//fusa:req REQ-CAPI-005
//fusa:req REQ-CAPI-006
//fusa:req REQ-CAPI-007
//fusa:req REQ-CAPI-008
