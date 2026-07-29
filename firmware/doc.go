// Package firmware provides OTA firmware delivery as an OEM-layer
// convenience riding on top of a raw-byte endpoint transport.
//
// An Updater chunks a firmware image, delivers it via a caller-supplied
// TransportFunc, and verifies installation with a CRC-32 checksum. Delivery
// uses a simple stop-and-wait protocol: each chunk is sent, the response is
// checked, and only then is the next chunk sent.
//
// # Explicitly out of TC18 RCP's own scope
//
// The OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC
// covers low-level interface access, not application/firmware
// distribution — this package's byte layout, chunking, and verification
// scheme are this repo's own design, not a transcription of anything the
// specification defines. Per Phase 17's disposition table, this package is
// ADAPT-flagged specifically because it remains useful riding on top of a
// raw-byte endpoint (e.g. uart, spi) or the udsbr bridge, even though
// nothing about it is spec-mandated.
//
// # Rebuilt at Milestone 57 (ROADMAP.md, v0.70.0)
//
// The chunking and CRC-32 integrity-verification logic carries over
// unchanged in shape; only the transport call underneath it is rebuilt.
// TransportFunc replaces the retired rcp.Controller.Send call: a caller
// adapts whichever concrete transport it is using (a *udp.Controller
// addressing a declared UART/SPI endpoint, or a udsbr transfer call — both
// explicitly named as candidate transports by Milestone 56's own udsbr
// rebuild) to this shape, rather than this package importing uart, spi, or
// udsbr directly and picking one as "the" transport. The retired
// CmdUpdate rcp.CommandType and rcp.Priority fields have no equivalent
// here: a raw-byte endpoint has no command-type space to reserve a code
// in, and request prioritization is a concern of whichever Kind/transport
// call the caller's own TransportFunc closure makes, not this package's.
//
// # Explicit non-goal
//
// This package does not implement rollback: the pre-TC18 implementation it
// replaces did not either (only chunking and post-install CRC
// verification), and rebuilding rollback logic from scratch is out of this
// milestone's scope. A caller wanting rollback composes its own retry/
// re-flash logic around a failed Update call.
package firmware

//fusa:req REQ-FW-001
//fusa:req REQ-FW-002
//fusa:req REQ-FW-003
//fusa:req REQ-FW-004
//fusa:req REQ-FW-005
//fusa:req REQ-FW-006
//fusa:req REQ-FW-007
//fusa:req REQ-FW-008
