package certgap

// This file is ROADMAP.md Milestone 58 (v0.71.0)'s ADAPT content: the
// generic Registry/Requirement/Analyze gap-analysis engine certgap.go
// implements carries over unchanged, but BuildRequirementFamilies below
// regenerates what it describes — a rollup of the actual REQ-* families
// Phases 13-16 (Milestones 44-57) produced, replacing what would otherwise
// still describe the retired REQ-ZONE/REQ-CMD/REQ-PRI-era requirement set
// this package held no populated content for at all before this milestone
// (the retired families are deliberately excluded here: they describe an
// API surface Milestone 59's Phase 18 cutover removes, not a durable part
// of this program's requirement set — see ROADMAP.md's own Phase 18
// section).
//
// Each FamilyRequirement below rolls up one //fusa:req prefix (e.g.
// "REQ-RCS" for every REQ-RCS-NNN tag in server/doc.go) into a single
// certgap.Requirement — this is a family-level gap report, not a
// requirement-by-requirement one; `gofusa trace` is the tool of record for
// the individual-requirement view (see the package doc comment's own note
// on why this package doesn't duplicate that). A family is included only
// if go-FuSa's traceability gate (100% required by this repository's own
// CI, see .github/workflows/*.yml) already confirms every requirement
// under that prefix is both traced and tested — so every FamilyRequirement
// here is marked Met unconditionally; the ASIL-D uplift gap this package
// reports is entirely carried by StandardASILDGaps' generic items, not by
// anything incomplete in this requirement set itself.
//
// Category assignments below are this implementation's own reasoned
// classification (no ISO 26262 clause mandates a specific mapping from
// go-RCP's own package names to Category): endpoint-type packages that
// interface a literal hardware signal (gpio/spi/i2c/uart/adc/pwm/lin/can/
// iseled/mdio) are CategoryHardware; the core protocol/dispatch/safety
// packages (server+lifecycle+regmap+discovery, request, e2e, wakeup,
// powerstate, redundancy) are CategoryFunctionalSafety; test-double and
// verification tooling (sim, faultinject, observe, record) is
// CategoryVerification; everything else is CategorySoftware.

// FamilyRequirement pairs a //fusa:req prefix with the certgap.Requirement
// summarizing it.
type FamilyRequirement struct {
	// Prefix is the //fusa:req tag prefix this family rolls up (e.g.
	// "REQ-RCS" covers every REQ-RCS-NNN tag).
	Prefix string
	Req    Requirement
}

// BuildRequirementFamilies returns one certgap.Requirement per REQ-* family
// Milestones 44-57 produced, each marked Met (see this file's own doc
// comment for why). ASIL is uniformly ASILB, matching this repository's
// own project-wide target (.fusa.json's "asil": "ASIL-B") — none of these
// families claims a higher per-requirement ASIL of its own.
func BuildRequirementFamilies() []FamilyRequirement {
	fam := func(prefix, id, desc string, cat Category) FamilyRequirement {
		return FamilyRequirement{
			Prefix: prefix,
			Req: Requirement{
				ID:          id,
				Description: desc,
				TargetASIL:  ASILB,
				Category:    cat,
				Met:         true,
			},
		}
	}

	return []FamilyRequirement{
		// Core protocol: transport framing, lifecycle/regmap/discovery
		// composition, conditional-request dispatch, and the safe-point/
		// safe-state watchdog mechanism.
		fam("REQ-AVTP", "REQFAM-AVTP", "AVTPDU/ACF wire framing (avtp, acf)", CategoryFunctionalSafety),
		fam("REQ-RCS", "REQFAM-RCS", "RC Server composition: lifecycle state machine, register map, discovery (server, lifecycle, regmap, discovery)", CategoryFunctionalSafety),
		fam("REQ-REQ", "REQFAM-REQ", "Conditional-request taxonomy, sequencer, and dispatch lifecycle (request)", CategoryFunctionalSafety),
		fam("REQ-CRC", "REQFAM-CRC", "CRC32 safe-point mechanism and automatic safe-state-entry watchdog (e2e)", CategoryFunctionalSafety),
		fam("REQ-WAKEUP", "REQFAM-WAKEUP", "Wakeup power-management endpoint (wakeup)", CategoryFunctionalSafety),
		fam("REQ-PWR", "REQFAM-PWR", "Wake-handshake retransmission pacing (powerstate)", CategoryFunctionalSafety),
		fam("REQ-RD", "REQFAM-RD", "Redundancy/failover handling (redundancy)", CategoryFunctionalSafety),
		fam("REQ-FRAG", "REQFAM-FRAG", "Multi-AVTPDU fragmentation and reassembly (fragment)", CategoryFunctionalSafety),

		// Hardware-facing endpoint types.
		fam("REQ-GPIO", "REQFAM-GPIO", "GPIO endpoint type (gpio)", CategoryHardware),
		fam("REQ-SPI", "REQFAM-SPI", "SPI endpoint type (spi)", CategoryHardware),
		fam("REQ-I2C", "REQFAM-I2C", "I2C endpoint type (i2c)", CategoryHardware),
		fam("REQ-UART", "REQFAM-UART", "UART endpoint type (uart)", CategoryHardware),
		fam("REQ-ADC", "REQFAM-ADC", "ADC endpoint type (adc)", CategoryHardware),
		fam("REQ-PWM", "REQFAM-PWM", "PWM endpoint type (pwm)", CategoryHardware),
		fam("REQ-LINEP", "REQFAM-LINEP", "LIN endpoint type (lin)", CategoryHardware),
		fam("REQ-CANEP", "REQFAM-CANEP", "CAN endpoint type (can)", CategoryHardware),
		fam("REQ-ISELED", "REQFAM-ISELED", "ISELED endpoint type (iseled)", CategoryHardware),
		fam("REQ-MDIO", "REQFAM-MDIO", "MDIO endpoint type (mdio)", CategoryHardware),

		// Protocol bridges.
		fam("REQ-CAN", "REQFAM-CANBR", "CAN protocol bridge (canbr)", CategorySoftware),
		fam("REQ-LIN", "REQFAM-LINBR", "LIN protocol bridge (linbr)", CategorySoftware),
		fam("REQ-DOIP", "REQFAM-DOIPBR", "DoIP protocol bridge (doipbr)", CategorySoftware),
		fam("REQ-UDS", "REQFAM-UDSBR", "UDS protocol bridge (udsbr)", CategorySoftware),
		fam("REQ-GRPC", "REQFAM-GRPCBRIDGE", "gRPC protocol bridge (grpcbridge)", CategorySoftware),
		fam("REQ-REST", "REQFAM-RESTBRIDGE", "REST protocol bridge (restbridge)", CategorySoftware),
		fam("REQ-MQTT", "REQFAM-MQTTBR", "MQTT protocol bridge (mqttbr)", CategorySoftware),
		fam("REQ-DDS", "REQFAM-DDSBR", "DDS protocol bridge (ddsbr)", CategorySoftware),

		// Transport, access control, and admission control.
		fam("REQ-UDP", "REQFAM-UDP", "AVTPDU/ACF-over-UDP/IP transport (udp)", CategorySoftware),
		fam("REQ-AZ", "REQFAM-AZ", "Client-side stream/endpoint access-control policy (authz)", CategorySoftware),
		fam("REQ-RL", "REQFAM-RL", "Per-endpoint token-bucket admission control (ratelimit)", CategorySoftware),
		fam("REQ-ADM", "REQFAM-ADM", "Administrative HTTP interface (admin)", CategorySoftware),

		// Multi-server/topology concerns.
		fam("REQ-FED", "REQFAM-FED", "Multi-server federation (federation)", CategorySoftware),
		fam("REQ-ZG", "REQFAM-ZG", "Zone-group addressing (zonegroup)", CategorySoftware),
		fam("REQ-DYN", "REQFAM-DYN", "Dynamic data distribution (dyndata)", CategorySoftware),
		fam("REQ-PQ", "REQFAM-PQ", "Priority queueing (prioqueue)", CategorySoftware),
		fam("REQ-SHMEM", "REQFAM-SHMEM", "Shared-memory transport (shmem)", CategorySoftware),

		// Tooling and verification infrastructure.
		fam("REQ-SIM", "REQFAM-SIM", "Realistic-timing simulator (sim)", CategoryVerification),
		fam("REQ-FI", "REQFAM-FI", "Fault-injection catalogue (faultinject)", CategoryVerification),
		fam("REQ-OB", "REQFAM-OB", "Runtime observability (observe)", CategoryVerification),
		fam("REQ-REC", "REQFAM-REC", "Record/replay (record)", CategoryVerification),
		fam("REQ-CAPI", "REQFAM-CAPI", "C ABI bindings (capi)", CategorySoftware),
		fam("REQ-CAA", "REQFAM-CODEGEN-CAA", "Code generation, access/authorization surface (codegen)", CategorySoftware),
		fam("REQ-CG", "REQFAM-CODEGEN-CG", "Code generation, general (codegen)", CategorySoftware),
		fam("REQ-FW", "REQFAM-FW", "Firmware-side reference bindings (firmware)", CategorySoftware),
	}
}

// BuildRegistry returns a certgap.Registry populated with one Requirement
// per BuildRequirementFamilies entry (every family Met, at the project's
// ASILB target) plus StandardASILDGaps (the reusable, protocol-independent
// ASIL-D uplift baseline — every one still Unmet, unchanged from before
// this milestone since nothing in Milestone 58's scope closes an ASIL-D
// gap). Analyzing this Registry with targetASIL="" (all) or ASILB reports
// full compliance; analyzing with ASILD reports exactly StandardASILDGaps'
// eight items as the gap, which is the intended, honest result — go-RCP
// targets ASIL-B, not ASIL-D, and this milestone does not change that.
func BuildRegistry() *Registry {
	r := NewRegistry()
	for _, f := range BuildRequirementFamilies() {
		r.Add(f.Req)
	}
	for _, gap := range StandardASILDGaps() {
		r.Add(gap)
	}
	return r
}
