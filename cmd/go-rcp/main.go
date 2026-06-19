// go-rcp is the CLI for go-RCP zone controllers.
//
// Mandatory RELAY commands (spec §11.1):
//
//	go-rcp version [--format text|json]   — tool and spec version
//	go-rcp capabilities                   — capabilities document (JSON)
//	go-rcp status [--format text|json]    — self-assessed health
//
// Additional RCP commands:
//
//	go-rcp discover                       — list all registered zones
//	go-rcp send <zone>                    — send CmdSet to a zone
//	go-rcp monitor                        — stream status from all zones
//
// RELAY interop driver (spec §11.2):
//
//	go-rcp convert --protocol RCP [--format json]
//	    — read an rcp.Status as JSON on stdin, run it through Status.ToMessage()
//	      (the §15.7.5 canonical conversion), and write the relay.Message as JSON
//	      on stdout. Exit 0 converted, 1 invalid input, 2 invalid args.
package main

//fusa:req REQ-CLI-001
//fusa:req REQ-CLI-002
//fusa:req REQ-CLI-003
//fusa:req REQ-CLI-004

import (
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

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

const (
	toolName    = "go-rcp"
	toolVersion = "0.49.0"
	protocol    = "RCP"
	protocolInt = 5
)

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
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		cmdDiscover(reg, os.Stdout)
	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: go-rcp send <zone>")
			os.Exit(2)
		}
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		os.Exit(cmdSend(reg, os.Args[2], os.Stdout, os.Stderr))
	case "monitor":
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		cmdMonitor(ctx, reg, os.Stdout)
	case "convert":
		os.Exit(cmdConvert(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go-rcp <version|capabilities|status|discover|send <zone>|monitor|convert>")
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
		Transports:         []string{"virtual", "grpc", "rest", "tcp", "uds"},
		Features:           []string{"loaning"},
		Interfaces:         []string{"Controller", "Registry"},
		OptionalInterfaces: []string{"LoaningController", "HealthProvider", "MetricsProvider", "Drainer"},
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

func cmdDiscover(reg *mock.Registry, w io.Writer) {
	for _, ctrl := range reg.Controllers() {
		_, _ = fmt.Fprintf(w, "zone %-12s  controller=%T\n", ctrl.Zone(), ctrl)
	}
}

// cmdSend sends a CmdGet to zoneName and prints the response as JSON. It returns
// the process exit code: 0 on success, 1 on an unknown zone or send failure.
func cmdSend(reg *mock.Registry, zoneName string, w, errw io.Writer) int {
	zone, err := rcp.ZoneFromString(zoneName)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "unknown zone %q: %v\n", zoneName, err)
		return 1
	}
	ctrl, err := reg.Lookup(zone)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "zone %q not found: %v\n", zoneName, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{
		ID:       1,
		Zone:     zone,
		Type:     rcp.CmdGet,
		Priority: rcp.PriorityNormal,
	})
	if err != nil {
		_, _ = fmt.Fprintf(errw, "send: %v\n", err)
		return 1
	}
	b, _ := json.MarshalIndent(map[string]any{
		"command_id": resp.CommandID,
		"zone":       resp.Zone.String(),
		"status":     resp.Status.String(),
		"payload":    string(resp.Payload),
	}, "", "  ")
	_, _ = fmt.Fprintln(w, string(b))
	return 0
}

func cmdMonitor(ctx context.Context, reg *mock.Registry, w io.Writer) {
	for _, ctrl := range reg.Controllers() {
		mc, ok := ctrl.(*mock.Controller)
		if !ok {
			continue
		}
		ch, err := mc.Subscribe(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(w, "subscribe zone %s: %v\n", mc.Zone(), err)
			continue
		}
		go func(z rcp.Zone, ch <-chan *rcp.Status) {
			for s := range ch {
				_, _ = fmt.Fprintf(w, "[%s] seq=%d healthy=%v payload=%s\n", z, s.Seq, s.Healthy, string(s.Payload))
			}
		}(mc.Zone(), ch)

		go func(mc *mock.Controller) {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					mc.Publish([]byte(`{"heartbeat":true}`))
				}
			}
		}(mc)
	}

	_, _ = fmt.Fprintln(w, "monitoring all zones — press Ctrl+C to stop")
	<-ctx.Done()
}

// ── RELAY interop driver (spec §11.2) ─────────────────────────────────────────

// errInvalidInput is the sentinel name written to stderr when convert receives
// input that fails this implementation's validator (spec §11.2 / §5).
var errInvalidInput = errors.New("ErrInvalidInput")

// cmdConvert implements `convert --protocol RCP [--format json]` (spec §11.2).
// It reads one rcp.Status as JSON on stdin, converts it via Status.ToMessage()
// — the same code path used at runtime on the Subscribe direction (§15.7.5) —
// and writes the resulting relay.Message as JSON on stdout. The timestamp is
// zeroed so interop comparisons are deterministic.
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
	out, err := convertRCPStatus(raw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error()) // sentinel name (§5)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, string(out))
	return 0
}

// convertRCPStatus validates raw against the rcp.Status canonical schema
// (spec/schemas/rcp-status.json) and returns Status.ToMessage() as JSON with a
// zeroed timestamp. It returns errInvalidInput for any input the validator
// rejects. Pointer fields distinguish "absent" from a zero value so the schema's
// required set (zone, seq, healthy) is enforced.
func convertRCPStatus(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // schema additionalProperties: false
	var in struct {
		Zone    *int    `json:"zone"`
		Seq     *uint32 `json:"seq"`
		Healthy *bool   `json:"healthy"`
		Payload []byte  `json:"payload"` // base64-decoded by encoding/json
	}
	if err := dec.Decode(&in); err != nil {
		return nil, errInvalidInput
	}
	if in.Zone == nil || in.Seq == nil || in.Healthy == nil {
		return nil, errInvalidInput
	}
	if *in.Zone < int(rcp.ZoneUnknown) || *in.Zone > int(rcp.ZoneCentral) {
		return nil, errInvalidInput
	}

	s := &rcp.Status{
		Zone:    rcp.Zone(*in.Zone),
		Seq:     *in.Seq,
		Healthy: *in.Healthy,
		Payload: in.Payload,
	}
	msg := s.ToMessage()
	msg.Timestamp = time.Time{} // deterministic interop output
	return json.Marshal(msg)
}
