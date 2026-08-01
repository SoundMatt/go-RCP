// Package l2 implements a native layer-2 (raw Ethernet) transport for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), carrying AVTPDUs
// directly over Ethernet with EtherType 0x22F0, per TC18 §10.1: "[IEEE1722]
// can be used as a layer-2 protocol, which is independent from the physical
// layer below... an AVTPDU is marked by an EtherType value of 0x22F0."
//
// This finally delivers on ROADMAP.md Milestone 44's originally-stated
// primary transport target. Milestone 44 (avtp/doc.go's "Explicit
// non-goal") scoped its AVTPDU header work to "Ethernet-carried AVTPDUs
// first" and named 1722-over-UDP/IP as "an alternative to raw Ethernet
// framing" — language that, read on its own, could suggest raw Ethernet
// framing was the default/primary case and UDP/IP a secondary option. No
// native-Ethernet transport was ever built to go with that header work:
// every transport this repo shipped through this package's own addition
// (the `udp` package, ROADMAP.md Milestone 54) carried AVTPDUs over UDP/IP
// only. This package is that missing native-Ethernet transport, landing
// alongside a udp package fix (see udp/annexj.go) that makes the existing
// UDP/IP path itself real IEEE 1722-2016 Annex J framing for the first
// time (it previously used net.DialUDP/net.ListenUDP with no Annex J
// encapsulation sequence number and no standard port). Both transports are
// permanent, equally-supported, first-class options going forward — UDP/IP
// is not being deprecated in favor of this package, and this package is
// not a fallback: TC18 §10.1 documents them as two genuinely different,
// independent wire framings for the same AVTPDU payload, and a caller
// picks whichever fits its network (raw L2 on a dedicated in-vehicle
// Ethernet segment where every node speaks 1722 natively; UDP/IP wherever
// the traffic instead needs to ride ordinary IP routing/switching).
//
// # Wire frame shape
//
// EncodeFrame/DecodeFrame implement this package's own wire format: a
// standard Ethernet II header (destination MAC, 6 bytes; source MAC, 6
// bytes; EtherType, 2 bytes big-endian, always EtherTypeAVTP) directly
// followed by the AVTPDU bytes — no trailer, no length field (the Ethernet
// frame's own boundary delimits it), and critically no encapsulation
// sequence number: that field is specific to the sibling udp package's
// Annex J UDP/IP framing (see udp/annexj.go) and has no equivalent at
// layer 2. The two transports' wire bytes are genuinely different framings
// of the same AVTPDU payload, not the same bytes over a different socket
// API.
//
// # Destination addressing is caller-supplied, not derived
//
// TC18 only says a sender "select[s] a (multicast) destination address
// depending on the identification of the stream" — it does not, in the
// portion of the specification available to this implementation, specify
// the actual derivation algorithm from a stream_id (or anything else) to a
// multicast MAC address; that detail lives in the base IEEE 1722 standard,
// which this implementation also does not have access to (see the
// provenance note below). Rather than guess at an unverified derivation
// rule, NewTransport's Send method takes a caller-supplied destination MAC
// (unicast or multicast) for every frame, exactly the way the sibling udp
// package already lets a caller supply its own destination *net.UDPAddr
// rather than deriving one itself.
//
// # Linux only, and a real runtime privilege requirement
//
// This package's real implementation (transport_linux.go) opens an
// AF_PACKET/SOCK_RAW socket, which the Linux kernel only permits to a
// process holding CAP_NET_RAW (or running as root) — this is a genuine
// runtime requirement, not a formality: NewTransport fails on a process
// without it, exactly as the kernel itself would refuse the underlying
// socket(2) call. AF_PACKET/SOCK_RAW is Linux-specific; no portable
// cross-platform equivalent exists in the Go standard library. On any
// other GOOS, transport_other.go's build-tagged stub Transport type
// satisfies the same exported API but NewTransport always returns
// ErrL2UnsupportedPlatform — a clear, explicit, immediate failure rather
// than a silent no-op or a whole-repo build failure, so callers on any
// platform can import and reference this package unconditionally (e.g. a
// caller that only decides which transport to actually construct at
// runtime, based on its own platform detection or configuration).
//
// # Shape relative to the sibling udp package
//
// This package deliberately does not implement a shared interface with
// udp.Controller/udp.Server: those types are request/response-shaped (they
// own transaction correlation, timeouts, and Router dispatch) where this
// package's Transport is a raw frame send/receive primitive with no
// notion of a request at all — forcing them into one interface would
// misrepresent one shape as the other. Transport's own Send/Recv/Close
// method shapes are deliberately minimal and parallel to what a future
// shared abstraction could adopt if one ever proves useful, per this
// repo's general preference for concrete types over premature interfaces.
//
// # Provenance (Guiding Principle 10)
//
// EtherType 0x22F0 is quoted directly from TC18 §10.1's own text, which is
// available to this implementation (unlike the base IEEE 1722-2016
// standard itself, which is paywalled and not directly consulted here) —
// that value is a real primary-source citation, not a secondary-source
// inference. The multicast-MAC-derivation gap described above, by
// contrast, is a genuine unknown this implementation does not guess at.
package l2

//fusa:req REQ-L2-001
//fusa:req REQ-L2-002
//fusa:req REQ-L2-003
//fusa:req REQ-L2-004
//fusa:req REQ-L2-005
//fusa:req REQ-L2-006
