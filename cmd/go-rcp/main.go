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
package main

//fusa:req REQ-CLI-001
//fusa:req REQ-CLI-002
//fusa:req REQ-CLI-003

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	toolVersion = "0.46.0"
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
		cmdVersion(flagFormat(os.Args[2:]))
	case "capabilities":
		cmdCapabilities()
	case "status":
		cmdStatus(flagFormat(os.Args[2:]))
	case "discover":
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		cmdDiscover(reg)
	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: go-rcp send <zone>")
			os.Exit(2)
		}
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		cmdSend(reg, os.Args[2])
	case "monitor":
		reg := mock.NewRegistry()
		defer reg.Close() //nolint:errcheck
		cmdMonitor(reg)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go-rcp <version|capabilities|status|discover|send <zone>|monitor>")
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

func cmdVersion(format string) {
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doc)
		return
	}
	fmt.Printf("%s %s (protocol %s, RELAY spec %s, %s)\n",
		doc.Tool, doc.Version, doc.Protocol, doc.SpecVersion, doc.Runtime)
}

func cmdCapabilities() {
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
		Commands:           []string{"version", "capabilities", "status", "discover", "send", "monitor"},
		Transports:         []string{"virtual", "grpc", "rest", "tcp", "uds"},
		Features:           []string{"loaning"},
		Interfaces:         []string{"Controller", "Registry"},
		OptionalInterfaces: []string{"LoaningController", "HealthProvider", "MetricsProvider", "Drainer"},
		Adapt:              true,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

func cmdStatus(format string) {
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doc)
		return
	}
	healthy := "unhealthy"
	if doc.Healthy {
		healthy = "healthy"
	}
	fmt.Printf("%s %s — %s\n", doc.Tool, doc.Version, healthy)
}

// ── RCP commands ──────────────────────────────────────────────────────────────

func cmdDiscover(reg *mock.Registry) {
	for _, ctrl := range reg.Controllers() {
		fmt.Printf("zone %-12s  controller=%T\n", ctrl.Zone(), ctrl)
	}
}

func cmdSend(reg *mock.Registry, zoneName string) {
	zone, err := rcp.ZoneFromString(zoneName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unknown zone %q: %v\n", zoneName, err)
		os.Exit(1)
	}
	ctrl, err := reg.Lookup(zone)
	if err != nil {
		log.Fatalf("zone %q not found: %v", zoneName, err)
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
		log.Fatalf("send: %v", err)
	}
	b, _ := json.MarshalIndent(map[string]any{
		"command_id": resp.CommandID,
		"zone":       resp.Zone.String(),
		"status":     resp.Status.String(),
		"payload":    string(resp.Payload),
	}, "", "  ")
	fmt.Println(string(b))
}

func cmdMonitor(reg *mock.Registry) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for _, ctrl := range reg.Controllers() {
		mc, ok := ctrl.(*mock.Controller)
		if !ok {
			continue
		}
		ch, err := mc.Subscribe(ctx)
		if err != nil {
			log.Printf("subscribe zone %s: %v", mc.Zone(), err)
			continue
		}
		go func(z rcp.Zone, ch <-chan *rcp.Status) {
			for s := range ch {
				fmt.Printf("[%s] seq=%d healthy=%v payload=%s\n", z, s.Seq, s.Healthy, string(s.Payload))
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

	fmt.Println("monitoring all zones — press Ctrl+C to stop")
	<-ctx.Done()
}
