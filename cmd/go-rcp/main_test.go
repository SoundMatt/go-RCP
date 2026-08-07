package main

//fusa:test REQ-CLI-001
//fusa:test REQ-CLI-002
//fusa:test REQ-CLI-003
//fusa:test REQ-CLI-004
//fusa:test REQ-CLI-005

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	rcp "github.com/SoundMatt/go-RCP/v9"
	"github.com/SoundMatt/go-RCP/v9/mock"
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

func newTestFixture(t *testing.T) *mock.Client {
	t.Helper()
	fx, ctrl := newDemoFixture()
	t.Cleanup(func() { _ = fx.Close() })
	return ctrl
}

// TestConvert_RoundTrip pins convert's output to rcp.ResponseToMessage's
// own conversion for a representative addressed rcp.Message (spec §15.5;
// see spec/schemas/rcp-message.json). control=80 (0x50) is RELAY's
// Response(0x10)|Read(0x40) bit pair.
func TestConvert_RoundTrip(t *testing.T) {
	const input = `{"byte_bus_id":1,"transaction_num":7,"control":80,"read_size_or_segment":2,"body":"AQ=="}`

	var out, errBuf bytes.Buffer
	code := cmdConvert([]string{"--protocol", "RCP", "--format", "json"}, strings.NewReader(input), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if doc["id"] != rcp.EndpointIDString(1) {
		t.Errorf("id = %v, want %q", doc["id"], rcp.EndpointIDString(1))
	}
	if doc["payload"] != "AQ==" {
		t.Errorf("payload = %v, want AQ==", doc["payload"])
	}
	meta, _ := doc["meta"].(map[string]any)
	if meta["rcp.error"] != "false" {
		t.Errorf("meta[rcp.error] = %v, want false", meta["rcp.error"])
	}
	if meta["rcp.op"] != "read" {
		t.Errorf("meta[rcp.op] = %v, want read", meta["rcp.op"])
	}
	if meta["rcp.transaction_num"] != "7" {
		t.Errorf("meta[rcp.transaction_num] = %v, want 7", meta["rcp.transaction_num"])
	}
	if meta["rcp.read_size_or_segment"] != "2" {
		t.Errorf("meta[rcp.read_size_or_segment] = %v, want 2", meta["rcp.read_size_or_segment"])
	}
}

func TestConvert_InvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing required field", `{"body":"AQ=="}`},
		{"missing control", `{"byte_bus_id":1}`},
		{"byte_bus_id out of range", `{"byte_bus_id":999,"control":16}`},
		{"control out of range", `{"byte_bus_id":1,"control":999}`},
		{"unknown field", `{"byte_bus_id":1,"control":16,"x":1}`},
		{"malformed json", `not json`},
		{"bad base64 body", `{"byte_bus_id":1,"body":"!!!!","control":16}`},
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
			code := cmdConvert(tc.args, strings.NewReader(`{"byte_bus_id":1,"control":16}`), &out, &errBuf)
			if code != 2 {
				t.Errorf("exit code = %d, want 2 (stderr: %s)", code, errBuf.String())
			}
		})
	}
}

// TestConvert_EmptyBody confirms a response without a body converts
// cleanly. control=24 (0x18) is RELAY's Response(0x10)|Error(0x08) bit pair.
func TestConvert_EmptyBody(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := cmdConvert([]string{"--protocol", "RCP"}, strings.NewReader(`{"byte_bus_id":5,"control":24}`), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if obj["id"] != rcp.EndpointIDString(5) {
		t.Errorf("id = %v, want %q", obj["id"], rcp.EndpointIDString(5))
	}
	if meta, _ := obj["meta"].(map[string]any); meta["rcp.error"] != "true" {
		t.Errorf("meta[rcp.error] = %v, want true", meta["rcp.error"])
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
	ctrl := newTestFixture(t)
	var w, errw bytes.Buffer
	if code := cmdDiscover(ctrl, &w, &errw); code != 0 {
		t.Fatalf("cmdDiscover exit = %d, want 0 (stderr: %s)", code, errw.String())
	}
	if !strings.Contains(w.String(), "register map:") {
		t.Errorf("discover output = %q, want it to mention the register map", w.String())
	}
}

func TestCmdSend_Success(t *testing.T) {
	ctrl := newTestFixture(t)
	var w, errw bytes.Buffer
	if code := cmdSend(ctrl, "1", &w, &errw); code != 0 {
		t.Fatalf("cmdSend exit = %d, want 0 (stderr: %s)", code, errw.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Bytes(), &doc); err != nil {
		t.Fatalf("send output not valid JSON: %v", err)
	}
	byteBusID, ok := doc["byte_bus_id"].(float64)
	if !ok || int(byteBusID) != 1 {
		t.Errorf("byte_bus_id = %v, want 1", doc["byte_bus_id"])
	}
	if doc["error"] != false {
		t.Errorf("error = %v, want false", doc["error"])
	}
}

func TestCmdSend_UnparseableAddr(t *testing.T) {
	ctrl := newTestFixture(t)
	var w, errw bytes.Buffer
	if code := cmdSend(ctrl, "nowhere", &w, &errw); code != 1 {
		t.Errorf("cmdSend(unparseable) exit = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "unknown byte_bus_id") {
		t.Errorf("stderr = %q, want it to mention unknown byte_bus_id", errw.String())
	}
}

func TestCmdMonitor_ReturnsOnContextCancel(t *testing.T) {
	ctrl := newTestFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var w bytes.Buffer
	go func() {
		cmdMonitor(ctx, ctrl, &w)
		close(done)
	}()
	select {
	case <-done:
		if !strings.Contains(w.String(), "monitoring demo endpoints") {
			t.Errorf("monitor output = %q, want monitoring header", w.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cmdMonitor did not return after context cancellation")
	}
}

// ── §11.2 streaming send sink (crossbar spoke) ────────────────────────────────

func TestSendStream_PublishesMessages(t *testing.T) {
	ctrl := newTestFixture(t)
	in := ndjson(t,
		relay.Message{Protocol: relay.RCP, ID: "1", Meta: map[string]string{"rcp.op": "read"}},
		relay.Message{Protocol: relay.RCP, ID: "2", Meta: map[string]string{"rcp.op": "write"}},
	)
	var w, errw bytes.Buffer
	if code := cmdSendStream(ctrl, strings.NewReader(in), &w, &errw); code != 0 {
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
	ctrl := newTestFixture(t)
	// malformed JSON, an unparseable ID, a blank line, then one good message.
	in := "not json\n" +
		`{"protocol":5,"id":"nowhere"}` + "\n" +
		"\n" +
		ndjson(t, relay.Message{Protocol: relay.RCP, ID: "3", Meta: map[string]string{"rcp.op": "read"}})
	var w, errw bytes.Buffer
	if code := cmdSendStream(ctrl, strings.NewReader(in), &w, &errw); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(w.String(), "published 1") {
		t.Errorf("stdout = %q, want 1 published", w.String())
	}
	if !strings.Contains(errw.String(), "malformed") || !strings.Contains(errw.String(), "nowhere") {
		t.Errorf("stderr = %q, want malformed + unparseable-id warnings", errw.String())
	}
}

func TestSendStream_EmptyInput(t *testing.T) {
	ctrl := newTestFixture(t)
	var w, errw bytes.Buffer
	if code := cmdSendStream(ctrl, strings.NewReader(""), &w, &errw); code != 0 {
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
	if code := cmdConvert([]string{"--protocol", "RCP"}, strings.NewReader(`{"byte_bus_id":1,"control":16}`), &conv, &bytes.Buffer{}); code != 0 {
		t.Fatalf("convert exit = %d", code)
	}
	ctrl := newTestFixture(t)
	var w, errw bytes.Buffer
	if code := cmdSendStream(ctrl, bytes.NewReader(conv.Bytes()), &w, &errw); code != 0 {
		t.Fatalf("send sink exit = %d (stderr: %s)", code, errw.String())
	}
	if !strings.Contains(w.String(), "published 1") {
		t.Errorf("stdout = %q, want 1 published from convert output", w.String())
	}
}
