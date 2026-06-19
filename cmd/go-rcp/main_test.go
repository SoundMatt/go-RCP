package main

//fusa:test REQ-CLI-004

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

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
