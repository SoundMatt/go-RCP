// go-rcp is the CLI for go-RCP RC Servers.
//
// Mandatory RELAY commands (spec §11.1):
//
//	go-rcp version [--format text|json]   — tool and spec version
//	go-rcp capabilities                   — capabilities document (JSON)
//	go-rcp status [--format text|json]    — self-assessed health
//
// Additional RCP commands, run against an in-process demo RC Server (a
// mock.Fixture with a handful of endpoints registered — this binary dials
// no real network transport on its own; wire your own *udp.Controller/
// *udp.Server pair for that):
//
//	go-rcp discover                       — read the demo server's EP0 register map
//	go-rcp send <byte_bus_id>              — write a demo payload to an endpoint
//	go-rcp send --format json             — streaming relay.Message NDJSON sink (crossbar spoke, §11.2)
//	go-rcp monitor                        — poll every demo endpoint on an interval
//
// RELAY interop driver (spec §11.2):
//
//	go-rcp convert --protocol RCP [--format json]
//	    — read one rcp.Message-shaped value (RELAY spec §15.5 canonical
//	      type: byte_bus_id, transaction_num, control, read_size_or_segment,
//	      body — see spec/schemas/rcp-message.json) as JSON on stdin, run it
//	      through rcp.ResponseToMessage (the §15.7.5-analogue conversion
//	      this package uses at runtime on the Call direction), and write the
//	      resulting relay.Message as JSON on stdout. Exit 0 converted, 1
//	      invalid input, 2 invalid args.
package main

//fusa:req REQ-CLI-001
//fusa:req REQ-CLI-002
//fusa:req REQ-CLI-003
//fusa:req REQ-CLI-004
//fusa:req REQ-CLI-005

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	rcp "github.com/SoundMatt/go-RCP/v9"
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/mock"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

const (
	toolName    = "go-rcp"
	protocol    = "RCP"
	protocolInt = 5
)

// demoStream/demoAddrs seed the in-process demo RC Server discover/send/
// monitor address a Fixture with no real network transport — a caller
// wanting a live server dials *udp.NewServer/*udp.NewController directly
// (see udp/doc.go); this binary's job is only to demonstrate Adapt/the
// RELAY commands against something that exists without one.
var demoAddrs = []avtp.ByteBusID{1, 2, 3, 4, 5}

func demoStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x67, 0x6f, 0x2d, 0x72, 0x63}, 1) // "go-rc" in the low 5 MAC bytes
}

// toolVersion is the value reported by `version --format json` and
// `capabilities` (spec §12.1/§12.2). It is a var, not a const, so release
// builds can pin it to the actual git tag via
//
//	go build -ldflags "-X main.toolVersion=$(git describe --tags --always)"
//
// (done by docker/Dockerfile and .github/workflows/release.yml). A build
// invoked without that flag — e.g. `go run`, `go install`, a plain `go build`
// during development — reports "dev" rather than a stale, hand-edited
// version string that silently drifts from the actual release tag.
var toolVersion = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		cmdVersion(flagFormat(os.Args[2:]), os.Stdout)
	case "capabilities":
		cmdCapabilities(os.Stdout)
	case "status":
		cmdStatus(flagFormat(os.Args[2:]), os.Stdout)
	case "discover":
		fx, ctrl := newDemoFixture()
		defer func() { _ = fx.Close() }()
		os.Exit(cmdDiscover(ctrl, os.Stdout, os.Stderr))
	case "send":
		args := os.Args[2:]
		fx, ctrl := newDemoFixture()
		defer func() { _ = fx.Close() }()
		// `send --format json` is the streaming NDJSON sink / crossbar spoke
		// (§11.2); `send <byte_bus_id>` is the ad-hoc single-request form.
		if flagFormat(args) == "json" {
			os.Exit(cmdSendStream(ctrl, os.Stdin, os.Stdout, os.Stderr))
		}
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: go-rcp send <byte_bus_id> | send --format json")
			os.Exit(2)
		}
		os.Exit(cmdSend(ctrl, args[0], os.Stdout, os.Stderr))
	case "monitor":
		fx, ctrl := newDemoFixture()
		defer func() { _ = fx.Close() }()
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		cmdMonitor(ctx, ctrl, os.Stdout)
	case "convert":
		os.Exit(cmdConvert(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go-rcp <version|capabilities|status|discover|send <byte_bus_id>|monitor|convert>")
}

// flagFormat returns "text" or "json" from --format flag, defaulting to "text".
func flagFormat(args []string) string {
	for i, a := range args {
		if a == "--format" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "text"
}

// newDemoFixture builds the in-process demo RC Server discover/send/monitor
// address: an unconfigured mock.Fixture (see mock/fixture.go) with a
// mock.Endpoint registered at each of demoAddrs, each answering with a
// fixed acknowledgement payload. The caller must Close the returned
// *mock.Fixture.
func newDemoFixture() (*mock.Fixture, *mock.Client) {
	fx, err := mock.NewFixture(demoStream(), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-rcp: demo fixture: %v\n", err)
		os.Exit(1)
	}
	for _, addr := range demoAddrs {
		ep := mock.NewEndpoint(addr, func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
			return acf.Message{
				Kind:           req.Kind,
				ByteBusID:      req.ByteBusID,
				TransactionNum: req.TransactionNum,
				Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
				Body:           []byte(fmt.Sprintf(`{"ack":true,"byte_bus_id":%d}`, addr)),
			}, nil
		})
		if err := fx.Router.Register(addr, ep); err != nil {
			fmt.Fprintf(os.Stderr, "go-rcp: demo fixture: register %d: %v\n", addr, err)
			os.Exit(1)
		}
	}
	return fx, fx.Root
}

// ── RELAY mandatory commands (spec §11.1) ─────────────────────────────────────

func cmdVersion(format string, w io.Writer) {
	type versionDoc struct {
		Tool        string `json:"tool"`
		Protocol    string `json:"protocol"`
		ProtocolInt int    `json:"protocol_int"`
		Version     string `json:"version"`
		SpecVersion string `json:"spec_version"`
		Language    string `json:"language"`
		Runtime     string `json:"runtime"`
	}
	doc := versionDoc{
		Tool:        toolName,
		Protocol:    protocol,
		ProtocolInt: protocolInt,
		Version:     toolVersion,
		SpecVersion: rcp.SpecVersion,
		Language:    "go",
		Runtime:     runtime.Version(),
	}
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doc)
		return
	}
	_, _ = fmt.Fprintf(w, "%s %s (protocol %s, RELAY spec %s, %s)\n",
		doc.Tool, doc.Version, doc.Protocol, doc.SpecVersion, doc.Runtime)
}

func cmdCapabilities(w io.Writer) {
	type capDoc struct {
		Kind               string   `json:"kind"`
		Tool               string   `json:"tool"`
		Protocol           string   `json:"protocol"`
		ProtocolInt        int      `json:"protocol_int"`
		Version            string   `json:"version"`
		SpecVersion        string   `json:"spec_version"`
		Commands           []string `json:"commands"`
		Transports         []string `json:"transports"`
		Features           []string `json:"features"`
		Interfaces         []string `json:"interfaces"`
		OptionalInterfaces []string `json:"optional_interfaces"`
		Adapt              bool     `json:"adapt"`
	}
	doc := capDoc{
		Kind:               "capabilities",
		Tool:               toolName,
		Protocol:           protocol,
		ProtocolInt:        protocolInt,
		Version:            toolVersion,
		SpecVersion:        rcp.SpecVersion,
		Commands:           []string{"version", "capabilities", "status", "discover", "send", "monitor", "convert"},
		Transports:         []string{"virtual", "udp", "tsn", "shmem", "grpc", "rest"},
		Features:           []string{"loaning"},
		Interfaces:         []string{"Controller"},
		OptionalInterfaces: []string{"HealthProvider", "MetricsProvider", "Drainer"},
		Adapt:              true,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

func cmdStatus(format string, w io.Writer) {
	type statusDoc struct {
		Protocol  string         `json:"protocol"`
		Tool      string         `json:"tool"`
		Version   string         `json:"version"`
		Healthy   bool           `json:"healthy"`
		Connected bool           `json:"connected"`
		Endpoint  string         `json:"endpoint"`
		Details   map[string]any `json:"details"`
	}
	doc := statusDoc{
		Protocol:  protocol,
		Tool:      toolName,
		Version:   toolVersion,
		Healthy:   true,
		Connected: false,
		Endpoint:  "",
		Details:   map[string]any{},
	}
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doc)
		return
	}
	healthy := "unhealthy"
	if doc.Healthy {
		healthy = "healthy"
	}
	_, _ = fmt.Fprintf(w, "%s %s — %s\n", doc.Tool, doc.Version, healthy)
}

// ── RCP commands ──────────────────────────────────────────────────────────────

// cmdDiscover reads the demo server's EP0 register map and prints its raw
// byte length. It returns the process exit code: 0 on success, 1 on error.
func cmdDiscover(ctrl *mock.Client, w, errw io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := ctrl.Discover(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "discover: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "register map: %d byte(s)\n", len(raw))
	if rm, err := regmap.DecodeRegisterMap(raw); err == nil {
		_, _ = fmt.Fprintf(w, "declared endpoints: %v\n", rm.Addresses())
	}
	return 0
}

// cmdSend writes a demo payload to addrStr (a decimal avtp.ByteBusID) and
// prints the response as JSON. It returns the process exit code: 0 on
// success, 1 on an unparseable address or request failure.
func cmdSend(ctrl *mock.Client, addrStr string, w, errw io.Writer) int {
	addr, err := rcp.ParseEndpointID(addrStr)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "unknown byte_bus_id %q: %v\n", addrStr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Write(ctx, addr, []byte(`{"cmd":"get"}`))
	if err != nil {
		_, _ = fmt.Fprintf(errw, "send: %v\n", err)
		return 1
	}
	b, _ := json.MarshalIndent(map[string]any{
		"byte_bus_id": addr,
		"error":       resp.Control.Has(acf.FlagError),
		"payload":     string(resp.Body),
	}, "", "  ")
	_, _ = fmt.Fprintln(w, string(b))
	return 0
}

// cmdSendStream is the streaming JSON sink (RELAY §11.2 / crossbar spoke). It
// reads relay.Message values as NDJSON on stdin (one per line) and dispatches
// each — via rcp.RequestFromMessage → Controller.Request — until EOF.
// Malformed or undeliverable lines are reported to errw and skipped so a
// single bad message does not tear down the crossbar route; only a stdin
// read error is fatal.
//
// Exit codes: 0 clean EOF, 1 stdin read error.
func cmdSendStream(ctrl *mock.Client, stdin io.Reader, w, errw io.Writer) int {
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // tolerate large messages
	sent := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg relay.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			_, _ = fmt.Fprintf(errw, "send: skipping malformed message: %v\n", err)
			continue
		}
		addr, control, body, err := rcp.RequestFromMessage(msg)
		if err != nil {
			_, _ = fmt.Fprintf(errw, "send: skipping message %q: %v\n", msg.ID, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = ctrl.Request(ctx, addr, control, body)
		cancel()
		if err != nil {
			_, _ = fmt.Fprintf(errw, "send: byte_bus_id %d: %v\n", addr, err)
			continue
		}
		sent++
	}
	if err := sc.Err(); err != nil {
		_, _ = fmt.Fprintf(errw, "send: read error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "published %d message(s)\n", sent)
	return 0
}

// cmdMonitor polls every demo endpoint on a fixed interval and prints each
// response — the TC18-model replacement for the retired push-based
// Subscribe monitor: the protocol has no server-initiated broadcast (see
// rcpAdapter.Subscribe's own doc comment in adapt.go), so a caller wanting
// a live view reads on a schedule instead.
func cmdMonitor(ctx context.Context, ctrl *mock.Client, w io.Writer) {
	_, _ = fmt.Fprintln(w, "monitoring demo endpoints — press Ctrl+C to stop")
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, addr := range demoAddrs {
				reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				resp, err := ctrl.Read(reqCtx, addr)
				cancel()
				if err != nil {
					_, _ = fmt.Fprintf(w, "[%d] read error: %v\n", addr, err)
					continue
				}
				_, _ = fmt.Fprintf(w, "[%d] error=%v payload=%s\n", addr, resp.Control.Has(acf.FlagError), string(resp.Body))
			}
		}
	}
}

// ── RELAY interop driver (spec §11.2) ─────────────────────────────────────────

// errInvalidInput is the sentinel name written to stderr when convert receives
// input that fails this implementation's validator (spec §11.2 / §5).
var errInvalidInput = errors.New("ErrInvalidInput")

// cmdConvert implements `convert --protocol RCP [--format json]` (spec §11.2).
// It reads one addressed endpoint response as JSON on stdin, converts it via
// rcp.ResponseToMessage — the same code path this binary's Call direction
// uses at runtime (§15.7.5) — and writes the resulting relay.Message as JSON
// on stdout. The timestamp is zeroed so interop comparisons are deterministic.
//
// Exit codes: 0 converted, 1 invalid input, 2 invalid args.
func cmdConvert(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	protocol := fs.String("protocol", "", "canonical protocol (must be RCP)")
	format := fs.String("format", "json", "output format (json)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *protocol != "RCP" {
		_, _ = fmt.Fprintln(stderr, "convert: --protocol RCP is required")
		return 2
	}
	if *format != "json" {
		_, _ = fmt.Fprintln(stderr, "convert: only --format json is supported")
		return 2
	}

	raw, err := io.ReadAll(stdin)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, errInvalidInput.Error())
		return 1
	}
	out, err := convertResponse(raw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error()) // sentinel name (§5)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, string(out))
	return 0
}

// relayControlBit{Ack,Read,Write,Response,Error,MoreSegments} are the bit
// positions RELAY spec §15.5's rcp.Message.Control byte assigns to each
// flag (see spec/schemas/rcp-message.json's field description), used only
// to translate an incoming golden-vector/interop "control" value into this
// package's own acf.ControlFlags. The two types are independently-chosen
// Go-side convenience packings of the same semantic flags, not required to
// share bit positions — the same is true of acf.ControlFlags relative to
// the real wire bits (see its own doc comment) — so a numeric cast between
// them (as convertResponse used before this fix) silently produces the
// wrong flags.
const (
	relayControlBitAck          = 1 << 7
	relayControlBitRead         = 1 << 6
	relayControlBitWrite        = 1 << 5
	relayControlBitResponse     = 1 << 4
	relayControlBitError        = 1 << 3
	relayControlBitMoreSegments = 1 << 2
)

// acfControlFromRELAY translates a RELAY rcp.Message.Control byte value
// into acf.ControlFlags by semantic flag name, not bit position (see the
// relayControlBit* constants' doc comment). RELAY's Ack bit has no
// acf.ControlFlags equivalent — this package tracks a request-acknowledge
// flag via acf.Message.EVT instead (see EVT's doc comment) — and is
// intentionally dropped here, matching RELAY's own reference
// rcp.Message.ToMessage() (spec §15.7.5), which never surfaces Ack in a
// converted relay.Message either.
func acfControlFromRELAY(wire int) acf.ControlFlags {
	var c acf.ControlFlags
	if wire&relayControlBitRead != 0 {
		c |= acf.FlagRead
	}
	if wire&relayControlBitWrite != 0 {
		c |= acf.FlagWrite
	}
	if wire&relayControlBitResponse != 0 {
		c |= acf.FlagResponse
	}
	if wire&relayControlBitError != 0 {
		c |= acf.FlagError
	}
	if wire&relayControlBitMoreSegments != 0 {
		c |= acf.FlagMoreSegments
	}
	return c
}

// convertResponse decodes raw as an rcp.Message-shaped value per RELAY spec
// §15.5/spec/schemas/rcp-message.json ("byte_bus_id", "transaction_num",
// "control", "read_size_or_segment", "body" — RELAY's own reference
// rcp.Message.ToMessage() is direction-agnostic, so the same shape covers
// both a request and a response; the field name "convertResponse" predates
// that and is kept only for its established call sites) and returns
// rcp.ResponseToMessage's JSON with a zeroed timestamp. It returns
// errInvalidInput for any input the validator rejects. Pointer byte_bus_id/
// control fields distinguish "absent" (invalid — both are required per the
// schema) from an explicit 0. "control" is translated via
// acfControlFromRELAY rather than cast directly — RELAY's Control byte and
// this package's acf.ControlFlags pack the same flags at different bit
// positions.
func convertResponse(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var in struct {
		ByteBusID         *int   `json:"byte_bus_id"`
		TransactionNum    uint16 `json:"transaction_num"`
		Control           *int   `json:"control"`
		ReadSizeOrSegment uint16 `json:"read_size_or_segment"`
		Body              []byte `json:"body"` // base64-decoded by encoding/json
	}
	if err := dec.Decode(&in); err != nil {
		return nil, errInvalidInput
	}
	if in.ByteBusID == nil || *in.ByteBusID < 0 || *in.ByteBusID > 255 {
		return nil, errInvalidInput
	}
	if in.Control == nil || *in.Control < 0 || *in.Control > 255 {
		return nil, errInvalidInput
	}

	msg := rcp.ResponseToMessage(avtp.ByteBusID(*in.ByteBusID), acf.Message{
		TransactionNum:    avtp.TransactionNum(in.TransactionNum),
		Control:           acfControlFromRELAY(*in.Control),
		ReadSizeOrSegment: in.ReadSizeOrSegment,
		Body:              in.Body,
	})
	msg.Timestamp = time.Time{} // deterministic interop output
	return json.Marshal(msg)
}
