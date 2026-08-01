package tc18gap_test

//fusa:test REQ-TC18-001
//fusa:test REQ-TC18-003
//fusa:test REQ-TC18-004
//fusa:test REQ-TC18-005
//fusa:test REQ-TC18-007
//fusa:test REQ-TC18-008
//fusa:test REQ-TC18-009
//fusa:test REQ-TC18-010
//fusa:test REQ-TC18-011
//fusa:test REQ-TC18-012
//fusa:test REQ-TC18-013
//fusa:test REQ-TC18-014
//fusa:test REQ-TC18-015
//fusa:test REQ-TC18-016
//fusa:test REQ-TC18-017
//fusa:test REQ-TC18-018
//fusa:test REQ-TC18-019
//fusa:test REQ-TC18-020
//fusa:test REQ-TC18-021
//fusa:test REQ-TC18-022
//fusa:test REQ-TC18-023
//fusa:test REQ-TC18-024
//fusa:test REQ-TC18-025
//fusa:test REQ-TC18-026
//fusa:test REQ-TC18-027
//fusa:test REQ-TC18-028
//fusa:test REQ-TC18-029
//fusa:test REQ-TC18-030
//fusa:test REQ-TC18-031
//fusa:test REQ-TC18-032
//fusa:test REQ-TC18-033
//fusa:test REQ-TC18-034
//fusa:test REQ-TC18-035
//fusa:test REQ-TC18-036
//fusa:test REQ-TC18-037
//fusa:test REQ-TC18-038
//fusa:test REQ-TC18-039
//fusa:test REQ-TC18-040
//fusa:test REQ-TC18-041
//fusa:test REQ-TC18-042
//fusa:test REQ-TC18-043
//fusa:test REQ-TC18-046
//fusa:test REQ-TC18-047
//fusa:test REQ-TC18-048
//fusa:test REQ-TC18-049
//fusa:test REQ-TC18-050
//fusa:test REQ-TC18-052
//fusa:test REQ-TC18-053
//fusa:test REQ-TC18-054
//fusa:test REQ-TC18-056
//fusa:test REQ-TC18-057
//fusa:test REQ-TC18-058
//fusa:test REQ-TC18-059
//fusa:test REQ-TC18-060
//fusa:test REQ-TC18-061
//fusa:test REQ-TC18-062
//fusa:test REQ-TC18-063
//fusa:test REQ-TC18-064
//fusa:test REQ-TC18-065
//fusa:test REQ-TC18-066
//fusa:test REQ-TC18-067
//fusa:test REQ-TC18-068
//fusa:test REQ-TC18-069
//fusa:test REQ-TC18-070
//fusa:test REQ-TC18-071
//fusa:test REQ-TC18-072
//fusa:test REQ-TC18-073
//fusa:test REQ-TC18-074
//fusa:test REQ-TC18-075
//fusa:test REQ-TC18-076
//fusa:test REQ-TC18-077
//fusa:test REQ-TC18-078
//fusa:test REQ-TC18-079
//fusa:test REQ-TC18-081
//fusa:test REQ-TC18-082
//fusa:test REQ-TC18-083
//fusa:test REQ-TC18-084
//fusa:test REQ-TC18-085
//fusa:test REQ-TC18-086
//fusa:test REQ-TC18-087
//fusa:test REQ-TC18-088
//fusa:test REQ-TC18-089
//fusa:test REQ-TC18-090
//fusa:test REQ-TC18-091
//fusa:test REQ-TC18-092
//fusa:test REQ-TC18-093
//fusa:test REQ-TC18-094
//fusa:test REQ-TC18-095
//fusa:test REQ-TC18-096
//fusa:test REQ-TC18-097
//fusa:test REQ-TC18-098
//fusa:test REQ-TC18-099
//fusa:test REQ-TC18-100
//fusa:test REQ-TC18-101
//fusa:test REQ-TC18-102
//fusa:test REQ-TC18-103
//fusa:test REQ-TC18-104
//fusa:test REQ-TC18-105
//fusa:test REQ-TC18-106
//fusa:test REQ-TC18-107
//fusa:test REQ-TC18-108
//fusa:test REQ-TC18-109
//fusa:test REQ-TC18-110
//fusa:test REQ-TC18-111
//fusa:test REQ-TC18-112
//fusa:test REQ-TC18-113
//fusa:test REQ-TC18-114
//fusa:test REQ-TC18-115
//fusa:test REQ-TC18-116
//fusa:test REQ-TC18-118
//fusa:test REQ-TC18-119
//fusa:test REQ-TC18-120
//fusa:test REQ-TC18-123
//fusa:test REQ-TC18-124
//fusa:test REQ-TC18-125
//fusa:test REQ-TC18-126
//fusa:test REQ-TC18-127
//fusa:test REQ-TC18-128
//fusa:test REQ-TC18-129
//fusa:test REQ-TC18-130
//fusa:test REQ-TC18-132
//fusa:test REQ-TC18-133
//fusa:test REQ-TC18-134
//fusa:test REQ-TC18-136
//fusa:test REQ-TC18-137
//fusa:test REQ-TC18-138
//fusa:test REQ-TC18-139
//fusa:test REQ-TC18-140
//fusa:test REQ-TC18-141
//fusa:test REQ-TC18-142
//fusa:test REQ-TC18-143
//fusa:test REQ-TC18-144
//fusa:test REQ-TC18-145
//fusa:test REQ-TC18-147
//fusa:test REQ-TC18-148
//fusa:test REQ-TC18-149
//fusa:test REQ-TC18-151
//fusa:test REQ-TC18-154
//fusa:test REQ-TC18-155
//fusa:test REQ-TC18-156
//fusa:test REQ-TC18-157
//fusa:test REQ-TC18-158
//fusa:test REQ-TC18-159
//fusa:test REQ-TC18-160
//fusa:test REQ-TC18-161
//fusa:test REQ-TC18-162
//fusa:test REQ-TC18-163
//fusa:test REQ-TC18-164
//fusa:test REQ-TC18-165
//fusa:test REQ-TC18-166
//fusa:test REQ-TC18-167
//fusa:test REQ-TC18-168
//fusa:test REQ-TC18-169
//fusa:test REQ-TC18-170
//fusa:test REQ-TC18-171
//fusa:test REQ-TC18-172
//fusa:test REQ-TC18-173
//fusa:test REQ-TC18-174
//fusa:test REQ-TC18-175
//fusa:test REQ-TC18-176
//fusa:test REQ-TC18-177
//fusa:test REQ-TC18-178
//fusa:test REQ-TC18-179
//fusa:test REQ-TC18-180
//fusa:test REQ-TC18-181
//fusa:test REQ-TC18-182
//fusa:test REQ-TC18-183
//fusa:test REQ-TC18-184
//fusa:test REQ-TC18-185
//fusa:test REQ-TC18-186
//fusa:test REQ-TC18-187
//fusa:test REQ-TC18-188
//fusa:test REQ-TC18-190
//fusa:test REQ-TC18-191
//fusa:test REQ-TC18-192
//fusa:test REQ-TC18-193
//fusa:test REQ-TC18-194
//fusa:test REQ-TC18-195
//fusa:test REQ-TC18-196
//fusa:test REQ-TC18-197
//fusa:test REQ-TC18-198
//fusa:test REQ-TC18-200
//fusa:test REQ-TC18-201
//fusa:test REQ-TC18-202
//fusa:test REQ-TC18-204
//fusa:test REQ-TC18-205
//fusa:test REQ-TC18-206
//fusa:test REQ-TC18-207
//fusa:test REQ-TC18-208
//fusa:test REQ-TC18-209
//fusa:test REQ-TC18-210
//fusa:test REQ-TC18-211
//fusa:test REQ-TC18-212
//fusa:test REQ-TC18-213
//fusa:test REQ-TC18-214
//fusa:test REQ-TC18-215
//fusa:test REQ-TC18-216
//fusa:test REQ-TC18-217
//fusa:test REQ-TC18-218
//fusa:test REQ-TC18-219
//fusa:test REQ-TC18-220
//fusa:test REQ-TC18-221
//fusa:test REQ-TC18-222
//fusa:test REQ-TC18-223
//fusa:test REQ-TC18-224
//fusa:test REQ-TC18-225
//fusa:test REQ-TC18-226
//fusa:test REQ-TC18-229
//fusa:test REQ-TC18-230
//fusa:test REQ-TC18-231
//fusa:test REQ-TC18-232
//fusa:test REQ-TC18-233
//fusa:test REQ-TC18-234
//fusa:test REQ-TC18-235
//fusa:test REQ-TC18-236
//fusa:test REQ-TC18-237
//fusa:test REQ-TC18-238
//fusa:test REQ-TC18-239
//fusa:test REQ-TC18-240
//fusa:test REQ-TC18-241
//fusa:test REQ-TC18-243

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/SoundMatt/go-RCP/tc18gap"
)

// notImplementedPrefix is the marker every unimplemented requirement's text
// must begin with. It is the convention .fusa-reqs.json uses to distinguish a
// requirement the module satisfies from one it merely records.
const notImplementedPrefix = "NOT IMPLEMENTED — "

type corpusReq struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Status string `json:"status"`
	TC18   string `json:"tc18"`
}

// loadCorpus reads .fusa-reqs.json from the repository root. A test binary
// runs with its own package directory as the working directory, so the file
// sits one level up.
func loadCorpus(t *testing.T) []corpusReq {
	t.Helper()
	data, err := os.ReadFile("../.fusa-reqs.json")
	if err != nil {
		t.Fatalf("read .fusa-reqs.json: %v", err)
	}
	var doc struct {
		Requirements []corpusReq `json:"requirements"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse .fusa-reqs.json: %v", err)
	}
	if len(doc.Requirements) == 0 {
		t.Fatal("requirements corpus is empty")
	}
	return doc.Requirements
}

// TestGaps_AgreeWithTheRequirementsCorpus is the reason this package exists:
// it fails if the registry and .fusa-reqs.json ever disagree about which TC18
// clauses go unimplemented. The check runs in both directions, so neither
// adding a gap entry without a requirement nor flipping a requirement to
// implemented without removing its gap entry can pass silently.
func TestGaps_AgreeWithTheRequirementsCorpus(t *testing.T) {
	corpus := loadCorpus(t)

	wantGaps := make(map[string]corpusReq)
	for _, r := range corpus {
		if r.Status == "not-implemented" {
			wantGaps[r.ID] = r
		}
	}
	if len(wantGaps) == 0 {
		t.Fatal("no requirement in the corpus carries status not-implemented; " +
			"either the status convention changed or the corpus lost its gap entries")
	}

	haveGaps := make(map[string]tc18gap.Gap)
	for _, g := range tc18gap.Gaps() {
		if _, dup := haveGaps[g.ReqID]; dup {
			t.Errorf("gap %s is registered twice", g.ReqID)
		}
		haveGaps[g.ReqID] = g
	}

	for id, r := range wantGaps {
		g, ok := haveGaps[id]
		if !ok {
			t.Errorf("%s is not-implemented in .fusa-reqs.json but has no tc18gap entry", id)
			continue
		}
		if g.Title != r.Title {
			t.Errorf("%s: gap title %q != requirement title %q", id, g.Title, r.Title)
		}
		if g.Section != r.TC18 {
			t.Errorf("%s: gap section %q != requirement citation %q", id, g.Section, r.TC18)
		}
		if !strings.HasPrefix(r.Text, notImplementedPrefix) {
			t.Errorf("%s: requirement text does not begin with %q: %.60q",
				id, notImplementedPrefix, r.Text)
		}
	}
	for id := range haveGaps {
		if _, ok := wantGaps[id]; !ok {
			t.Errorf("tc18gap registers %s, which is not a not-implemented requirement "+
				"in .fusa-reqs.json", id)
		}
	}
}

// TestGaps_ImplementedRequirementsAreNotMarkedAsGaps guards the other side of
// the convention: a requirement the module does satisfy must not carry the
// NOT IMPLEMENTED marker, and must not appear in the registry.
func TestGaps_ImplementedRequirementsAreNotMarkedAsGaps(t *testing.T) {
	for _, r := range loadCorpus(t) {
		if r.Status != "implemented" {
			continue
		}
		if strings.HasPrefix(r.Text, notImplementedPrefix) {
			t.Errorf("%s is status implemented but its text begins with %q", r.ID, notImplementedPrefix)
		}
		if g, ok := tc18gap.Lookup(r.ID); ok {
			t.Errorf("%s is status implemented but tc18gap records it as a gap: %+v", r.ID, g)
		}
	}
}

// citation matches a TC18 citation: at least one section number and at least
// one specification-text line reference. A gap whose provenance cannot be
// checked against the specification is not worth recording.
var citation = regexp.MustCompile(`§\d+(\.\d+)*.*TC18\.txt:\d+`)

func TestGaps_CitationsNameASectionAndALine(t *testing.T) {
	for _, g := range tc18gap.Gaps() {
		if !citation.MatchString(g.Section) {
			t.Errorf("%s: citation %q names no §section and TC18.txt line", g.ReqID, g.Section)
		}
		if strings.TrimSpace(g.Title) == "" {
			t.Errorf("%s: empty title", g.ReqID)
		}
	}
}

func TestGaps_AreSortedByRequirementID(t *testing.T) {
	got := tc18gap.Gaps()
	ids := make([]string, len(got))
	for i, g := range got {
		ids[i] = g.ReqID
	}
	if !sort.StringsAreSorted(ids) {
		t.Error("gaps are not in requirement-identifier order")
	}
}

func TestGaps_ReturnsACopy(t *testing.T) {
	first := tc18gap.Gaps()
	if len(first) == 0 {
		t.Fatal("no gaps registered")
	}
	original := first[0]
	first[0] = tc18gap.Gap{ReqID: "MUTATED"}
	if again := tc18gap.Gaps(); again[0] != original {
		t.Errorf("Gaps() exposed its backing array: got %+v, want %+v", again[0], original)
	}
}

func TestLookup(t *testing.T) {
	all := tc18gap.Gaps()
	want := all[len(all)/2]
	got, ok := tc18gap.Lookup(want.ReqID)
	if !ok {
		t.Fatalf("Lookup(%q) reported not found", want.ReqID)
	}
	if got != want {
		t.Errorf("Lookup(%q) = %+v, want %+v", want.ReqID, got, want)
	}
	if _, ok := tc18gap.Lookup("REQ-TC18-000"); ok {
		t.Error("Lookup of an unregistered identifier reported found")
	}
}
