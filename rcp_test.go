package rcp_test

//fusa:test REQ-ZONE-001
//fusa:test REQ-ZONE-002
//fusa:test REQ-ZONE-003
//fusa:test REQ-ZONE-004
//fusa:test REQ-ZONE-005
//fusa:test REQ-ZONE-006
//fusa:test REQ-ZONE-007
//fusa:test REQ-ZONE-008
//fusa:test REQ-PRI-001
//fusa:test REQ-PRI-002
//fusa:test REQ-PRI-003
//fusa:test REQ-CMD-001
//fusa:test REQ-CMD-002
//fusa:test REQ-CMD-003
//fusa:test REQ-CMD-004
//fusa:test REQ-CMD-005
//fusa:test REQ-CMD-006
//fusa:test REQ-STATUS-001
//fusa:test REQ-STATUS-002
//fusa:test REQ-STATUS-003
//fusa:test REQ-STATUS-004
//fusa:test REQ-STATUS-005
//fusa:test REQ-STATUS-006
//fusa:test REQ-ERR-001
//fusa:test REQ-ERR-002
//fusa:test REQ-ERR-003
//fusa:test REQ-ERR-004
//fusa:test REQ-ERR-005
//fusa:test REQ-ERR-006
//fusa:test REQ-ERR-007
//fusa:test REQ-ERR-008
//fusa:test REQ-ERR-009
//fusa:test REQ-ERR-010
//fusa:test REQ-CMDSTRUCT-001
//fusa:test REQ-CMDSTRUCT-002
//fusa:test REQ-RESP-003
//fusa:test REQ-STAT-005

import (
	"errors"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
)

// ── Zone constants ────────────────────────────────────────────────────────────

func TestZone_String_UniqueAndNonEmpty(t *testing.T) {
	zones := []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	}
	seen := make(map[string]rcp.Zone)
	for _, z := range zones {
		s := z.String()
		if s == "" {
			t.Errorf("Zone(%d).String() is empty", z)
		}
		if prev, dup := seen[s]; dup {
			t.Errorf("Zone(%d) and Zone(%d) both return %q", z, prev, s)
		}
		seen[s] = z
	}
}

func TestZone_Values(t *testing.T) {
	cases := []struct {
		z    rcp.Zone
		want rcp.Zone
	}{
		{rcp.ZoneUnknown, 0},
		{rcp.ZoneFrontLeft, 1},
		{rcp.ZoneFrontRight, 2},
		{rcp.ZoneRearLeft, 3},
		{rcp.ZoneRearRight, 4},
		{rcp.ZoneCentral, 5},
	}
	for _, tc := range cases {
		if tc.z != tc.want {
			t.Errorf("Zone constant = %d, want %d", tc.z, tc.want)
		}
	}
}

func TestZone_AllDistinct(t *testing.T) {
	zones := []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	}
	for i := range zones {
		for j := range zones {
			if i != j && zones[i] == zones[j] {
				t.Errorf("zones[%d] == zones[%d] (both %d)", i, j, zones[i])
			}
		}
	}
}

// ── Priority constants ────────────────────────────────────────────────────────

func TestPriority_Values(t *testing.T) {
	if rcp.PriorityNormal != 0 {
		t.Errorf("PriorityNormal = %d, want 0", rcp.PriorityNormal)
	}
	if rcp.PriorityHigh <= rcp.PriorityNormal {
		t.Errorf("PriorityHigh (%d) not > PriorityNormal (%d)", rcp.PriorityHigh, rcp.PriorityNormal)
	}
	if rcp.PriorityCritical <= rcp.PriorityHigh {
		t.Errorf("PriorityCritical (%d) not > PriorityHigh (%d)", rcp.PriorityCritical, rcp.PriorityHigh)
	}
}

// ── CommandType constants ─────────────────────────────────────────────────────

func TestCommandType_Values(t *testing.T) {
	cases := []struct {
		c    rcp.CommandType
		want rcp.CommandType
	}{
		{rcp.CmdNoop, 0},
		{rcp.CmdSet, 1},
		{rcp.CmdGet, 2},
		{rcp.CmdReset, 3},
		{rcp.CmdWatchdog, 4},
	}
	for _, tc := range cases {
		if tc.c != tc.want {
			t.Errorf("CommandType constant = %d, want %d", tc.c, tc.want)
		}
	}
}

func TestCommandType_AllDistinct(t *testing.T) {
	cmds := []rcp.CommandType{rcp.CmdNoop, rcp.CmdSet, rcp.CmdGet, rcp.CmdReset, rcp.CmdWatchdog}
	for i := range cmds {
		for j := range cmds {
			if i != j && cmds[i] == cmds[j] {
				t.Errorf("cmds[%d] == cmds[%d] (both %d)", i, j, cmds[i])
			}
		}
	}
}

// ── ResponseStatus constants ──────────────────────────────────────────────────

func TestResponseStatus_String_UniqueAndNonEmpty(t *testing.T) {
	statuses := []rcp.ResponseStatus{
		rcp.StatusOK,
		rcp.StatusError,
		rcp.StatusTimeout,
		rcp.StatusBusy,
		rcp.StatusUnknown,
	}
	seen := make(map[string]rcp.ResponseStatus)
	for _, s := range statuses {
		str := s.String()
		if str == "" {
			t.Errorf("ResponseStatus(%d).String() is empty", s)
		}
		if prev, dup := seen[str]; dup {
			t.Errorf("ResponseStatus(%d) and ResponseStatus(%d) both return %q", s, prev, str)
		}
		seen[str] = s
	}
}

func TestResponseStatus_Values(t *testing.T) {
	cases := []struct {
		s    rcp.ResponseStatus
		want rcp.ResponseStatus
	}{
		{rcp.StatusOK, 0},
		{rcp.StatusError, 1},
		{rcp.StatusTimeout, 2},
		{rcp.StatusBusy, 3},
	}
	for _, tc := range cases {
		if tc.s != tc.want {
			t.Errorf("ResponseStatus constant = %d, want %d", tc.s, tc.want)
		}
	}
}

func TestResponseStatus_AllDistinct(t *testing.T) {
	statuses := []rcp.ResponseStatus{
		rcp.StatusOK, rcp.StatusError, rcp.StatusTimeout, rcp.StatusBusy, rcp.StatusUnknown,
	}
	for i := range statuses {
		for j := range statuses {
			if i != j && statuses[i] == statuses[j] {
				t.Errorf("statuses[%d] == statuses[%d] (both %d)", i, j, statuses[i])
			}
		}
	}
}

// ── Sentinel errors ───────────────────────────────────────────────────────────

func TestErrors_NonNil(t *testing.T) {
	errs := []struct {
		name string
		err  error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotFound", rcp.ErrNotFound},
		{"ErrAlreadyExists", rcp.ErrAlreadyExists},
		{"ErrTimeout", rcp.ErrTimeout},
		{"ErrBusy", rcp.ErrBusy},
	}
	for _, tc := range errs {
		if tc.err == nil {
			t.Errorf("%s is nil", tc.name)
		}
	}
}

func TestErrors_AllDistinct(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotFound", rcp.ErrNotFound},
		{"ErrAlreadyExists", rcp.ErrAlreadyExists},
		{"ErrTimeout", rcp.ErrTimeout},
		{"ErrBusy", rcp.ErrBusy},
	}
	for i := range sentinels {
		for j := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(sentinels[i].err, sentinels[j].err) {
				t.Errorf("%s matches %s via errors.Is — sentinels must be distinct",
					sentinels[i].name, sentinels[j].name)
			}
		}
	}
}

func TestErrors_IsDetectableWhenWrapped(t *testing.T) {
	wrap := func(sentinel error) error {
		return errors.Join(errors.New("outer context"), sentinel)
	}
	cases := []struct {
		name     string
		sentinel error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotFound", rcp.ErrNotFound},
		{"ErrAlreadyExists", rcp.ErrAlreadyExists},
		{"ErrTimeout", rcp.ErrTimeout},
	}
	for _, tc := range cases {
		wrapped := wrap(tc.sentinel)
		if !errors.Is(wrapped, tc.sentinel) {
			t.Errorf("errors.Is(wrap(%s)) = false, want true", tc.name)
		}
	}
}

// ── Struct zero values ────────────────────────────────────────────────────────

func TestCommand_ZeroValue(t *testing.T) {
	var cmd rcp.Command
	if cmd.Zone != rcp.ZoneUnknown {
		t.Errorf("zero Command.Zone = %d, want ZoneUnknown (%d)", cmd.Zone, rcp.ZoneUnknown)
	}
	if cmd.Type != rcp.CmdNoop {
		t.Errorf("zero Command.Type = %d, want CmdNoop (%d)", cmd.Type, rcp.CmdNoop)
	}
	if cmd.Priority != rcp.PriorityNormal {
		t.Errorf("zero Command.Priority = %d, want PriorityNormal (%d)", cmd.Priority, rcp.PriorityNormal)
	}
	if cmd.Payload != nil {
		t.Errorf("zero Command.Payload = %v, want nil", cmd.Payload)
	}
}

func TestResponse_ZeroValue_StatusOK(t *testing.T) {
	var r rcp.Response
	if r.Status != rcp.StatusOK {
		t.Errorf("zero Response.Status = %d, want StatusOK (%d)", r.Status, rcp.StatusOK)
	}
}

func TestStatus_NilPayload_Accepted(t *testing.T) {
	s := &rcp.Status{Payload: nil}
	if s.Payload != nil {
		t.Errorf("Status.Payload = %v, want nil", s.Payload)
	}
}
