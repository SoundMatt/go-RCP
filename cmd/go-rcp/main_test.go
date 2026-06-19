package main

//fusa:test REQ-CLI-004

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

func ndjson(t *testing.T, msgs ...relay.Message) string {
	t.Helper()
	var b strings.Builder
	for _, m := range msgs {
		line, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestConvert_GoldenVector pins convert's output to the RELAY golden vector
// spec/vectors/rcp-status.json: the Status value must produce the exact
// relay.Message the cross-language interop oracle expects.
func TestConvert_GoldenVector(t *testing.T) {
	const input = `{"zone":1,"seq":3,"healthy":true,"payload":"AQ=="}`
	const want = `{"protocol":5,"version":{"major":0,"minor":0,"patch":0},"id":"FrontLeft","payload":"AQ==","timestamp":"0001-01-01T00:00:00Z","seq":3,"meta":{"rcp.healthy":"true"}}`

	var out, errBuf bytes.Buffer
	code := cmdConvert([]string{"--protocol", "RCP", "--format", "json"}, strings.NewReader(input), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	got := strings.TrimSpace(out.String())
	if got != want {
		t.Errorf("convert output mismatch:\n got: %s\nwant: %s", got, want)
	}
	// And it must be valid JSON parseable back into a generic object.
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestConvert_InvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing required field", `{"zone":1,"seq":3}`},
		{"zone out of range", `{"zone":9,"seq":3,"healthy":true}`},
		{"unknown field", `{"zone":1,"seq":3,"healthy":true,"x":1}`},
		{"malformed json", `not json`},
		{"bad base64 payload", `{"zone":1,"seq":3,"healthy":true,"payload":"!!!!"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := cmdConvert([]string{"--protocol", "RCP"}, strings.NewReader(tc.input), &out, &errBuf)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if got := strings.TrimSpace(errBuf.String()); got != errInvalidInput.Error() {
				t.Errorf("stderr = %q, want %q", got, errInvalidInput.Error())
			}
		})
	}
}

func TestConvert_InvalidArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"missing protocol", []string{}},
		{"wrong protocol", []string{"--protocol", "CAN"}},
		{"unsupported format", []string{"--protocol", "RCP", "--format", "yaml"}},
		{"undefined flag", []string{"--protocol", "RCP", "--bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := cmdConvert(tc.args, strings.NewReader(`{"zone":1,"seq":3,"healthy":true}`), &out, &errBuf)
			if code != 2 {
				t.Errorf("exit code = %d, want 2 (stderr: %s)", code, errBuf.String())
			}
		})
	}
}

// TestConvert_EmptyPayload confirms a Status without payload converts cleanly
// (the relay.Message marshals payload as null and omits the zero seq).
func TestConvert_EmptyPayload(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := cmdConvert([]string{"--protocol", "RCP"}, strings.NewReader(`{"zone":5,"seq":0,"healthy":false}`), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if obj["id"] != "Central" {
		t.Errorf("id = %v, want Central", obj["id"])
	}
	if meta, _ := obj["meta"].(map[string]any); meta["rcp.healthy"] != "false" {
		t.Errorf("meta[rcp.healthy] = %v, want false", meta["rcp.healthy"])
	}
}

// ── §11.1 mandatory commands ──────────────────────────────────────────────────

func TestFlagFormat(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--format", "json"}, "json"},
		{[]string{"--format", "text"}, "text"},
		{[]string{}, "text"},           // default
		{[]string{"--format"}, "text"}, // missing value → default
		{[]string{"other", "--format", "json"}, "json"},
	}
	for _, tc := range cases {
		if got := flagFormat(tc.args); got != tc.want {
			t.Errorf("flagFormat(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestCmdVersion_JSON(t *testing.T) {
	var w bytes.Buffer
	cmdVersion("json", &w)
	var doc map[string]any
	if err := json.Unmarshal(w.Bytes(), &doc); err != nil {
		t.Fatalf("version --format json not valid JSON: %v", err)
	}
	if doc["tool"] != toolName {
		t.Errorf("tool = %v, want %q", doc["tool"], toolName)
	}
	if doc["protocol"] != protocol {
		t.Errorf("protocol = %v, want %q", doc["protocol"], protocol)
	}
	if doc["spec_version"] != rcp.SpecVersion {
		t.Errorf("spec_version = %v, want %q", doc["spec_version"], rcp.SpecVersion)
	}
	if doc["language"] != "go" {
		t.Errorf("language = %v, want go", doc["language"])
	}
}

func TestCmdVersion_Text(t *testing.T) {
	var w bytes.Buffer
	cmdVersion("text", &w)
	out := w.String()
	if !strings.Contains(out, toolName) || !strings.Contains(out, "RELAY spec "+rcp.SpecVersion) {
		t.Errorf("text version missing expected fields: %q", out)
	}
}

func TestCmdCapabilities_JSON(t *testing.T) {
	var w bytes.Buffer
	cmdCapabilities(&w)
	var doc struct {
		Kind        string   `json:"kind"`
		Tool        string   `json:"tool"`
		ProtocolInt int      `json:"protocol_int"`
		Commands    []string `json:"commands"`
		SpecVersion string   `json:"spec_version"`
	}
	if err := json.Unmarshal(w.Bytes(), &doc); err != nil {
		t.Fatalf("capabilities not valid JSON: %v", err)
	}
	if doc.Kind != "capabilities" {
		t.Errorf("kind = %q, want capabilities", doc.Kind)
	}
	if doc.ProtocolInt != protocolInt {
		t.Errorf("protocol_int = %d, want %d", doc.ProtocolInt, protocolInt)
	}
	if doc.SpecVersion != rcp.SpecVersion {
		t.Errorf("spec_version = %q, want %q", doc.SpecVersion, rcp.SpecVersion)
	}
	// The convert interop driver must be advertised (§11.2).
	var hasConvert bool
	for _, c := range doc.Commands {
		if c == "convert" {
			hasConvert = true
		}
	}
	if !hasConvert {
		t.Errorf("capabilities commands %v missing \"convert\"", doc.Commands)
	}
}

func TestCmdStatus_JSON(t *testing.T) {
	var w bytes.Buffer
	cmdStatus("json", &w)
	var doc map[string]any
	if err := json.Unmarshal(w.Bytes(), &doc); err != nil {
		t.Fatalf("status not valid JSON: %v", err)
	}
	if doc["protocol"] != protocol {
		t.Errorf("protocol = %v, want %q", doc["protocol"], protocol)
	}
	if doc["healthy"] != true {
		t.Errorf("healthy = %v, want true", doc["healthy"])
	}
}

func TestCmdStatus_Text(t *testing.T) {
	var w bytes.Buffer
	cmdStatus("text", &w)
	if !strings.Contains(w.String(), "healthy") {
		t.Errorf("text status = %q, want it to contain \"healthy\"", w.String())
	}
}

// ── RCP commands ──────────────────────────────────────────────────────────────

func TestCmdDiscover(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	var w bytes.Buffer
	cmdDiscover(reg, &w)
	// mock.NewRegistry pre-populates all five standard zones.
	if n := strings.Count(w.String(), "zone "); n != 5 {
		t.Errorf("discover printed %d zone lines, want 5:\n%s", n, w.String())
	}
}

func TestCmdSend_Success(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	var w, errw bytes.Buffer
	if code := cmdSend(reg, "FrontLeft", &w, &errw); code != 0 {
		t.Fatalf("cmdSend exit = %d, want 0 (stderr: %s)", code, errw.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Bytes(), &doc); err != nil {
		t.Fatalf("send output not valid JSON: %v", err)
	}
	if doc["zone"] != "FrontLeft" {
		t.Errorf("zone = %v, want FrontLeft", doc["zone"])
	}
	if doc["status"] != "OK" {
		t.Errorf("status = %v, want OK", doc["status"])
	}
}

func TestCmdSend_UnknownZone(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	var w, errw bytes.Buffer
	if code := cmdSend(reg, "nowhere", &w, &errw); code != 1 {
		t.Errorf("cmdSend(unknown) exit = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "unknown zone") {
		t.Errorf("stderr = %q, want it to mention unknown zone", errw.String())
	}
}

func TestCmdMonitor_ReturnsOnContextCancel(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var w bytes.Buffer
	go func() {
		cmdMonitor(ctx, reg, &w)
		close(done)
	}()
	select {
	case <-done:
		if !strings.Contains(w.String(), "monitoring all zones") {
			t.Errorf("monitor output = %q, want monitoring header", w.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cmdMonitor did not return after context cancellation")
	}
}

// ── §11.2 streaming send sink (crossbar spoke) ────────────────────────────────

func TestSendStream_PublishesMessages(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	in := ndjson(t,
		relay.Message{Protocol: relay.RCP, ID: "FrontLeft", Meta: map[string]string{"rcp.cmd_type": "get"}},
		relay.Message{Protocol: relay.RCP, ID: "Central", Meta: map[string]string{"rcp.cmd_type": "set"}},
	)
	var w, errw bytes.Buffer
	if code := cmdSendStream(reg, strings.NewReader(in), &w, &errw); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errw.String())
	}
	if !strings.Contains(w.String(), "published 2") {
		t.Errorf("stdout = %q, want it to report 2 published", w.String())
	}
	if errw.Len() != 0 {
		t.Errorf("unexpected stderr: %s", errw.String())
	}
}

func TestSendStream_SkipsBadAndUndeliverableLines(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	// malformed JSON, an unknown zone, a blank line, then one good message.
	in := "not json\n" +
		`{"protocol":5,"id":"Nowhere"}` + "\n" +
		"\n" +
		ndjson(t, relay.Message{Protocol: relay.RCP, ID: "RearLeft", Meta: map[string]string{"rcp.cmd_type": "get"}})
	var w, errw bytes.Buffer
	if code := cmdSendStream(reg, strings.NewReader(in), &w, &errw); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(w.String(), "published 1") {
		t.Errorf("stdout = %q, want 1 published", w.String())
	}
	if !strings.Contains(errw.String(), "malformed") || !strings.Contains(errw.String(), "Nowhere") {
		t.Errorf("stderr = %q, want malformed + unknown-zone warnings", errw.String())
	}
}

func TestSendStream_EmptyInput(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	var w, errw bytes.Buffer
	if code := cmdSendStream(reg, strings.NewReader(""), &w, &errw); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(w.String(), "published 0") {
		t.Errorf("stdout = %q, want 0 published", w.String())
	}
}

// TestConvertSendRoundTrip exercises the crossbar identity path: convert emits a
// relay.Message that the send sink can re-publish.
func TestConvertSendRoundTrip(t *testing.T) {
	var conv bytes.Buffer
	if code := cmdConvert([]string{"--protocol", "RCP"}, strings.NewReader(`{"zone":1,"seq":3,"healthy":true}`), &conv, &bytes.Buffer{}); code != 0 {
		t.Fatalf("convert exit = %d", code)
	}
	reg := mock.NewRegistry()
	defer reg.Close() //nolint:errcheck
	var w, errw bytes.Buffer
	if code := cmdSendStream(reg, bytes.NewReader(conv.Bytes()), &w, &errw); code != 0 {
		t.Fatalf("send sink exit = %d (stderr: %s)", code, errw.String())
	}
	if !strings.Contains(w.String(), "published 1") {
		t.Errorf("stdout = %q, want 1 published from convert output", w.String())
	}
}
