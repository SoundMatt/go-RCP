package gpio

// applyValue combines operand with current under sem, clamped to active
// (the endpoint's active-pin mask). sem must not be SemanticReconfigure —
// that semantic replaces Config.Direction rather than a pin value, and is
// handled separately by Endpoint.applyWrite.
func applyValue(current, operand uint32, sem WriteSemantic, active uint32) (uint32, error) {
	switch sem {
	case SemanticReplace:
		return operand & active, nil
	case SemanticOr:
		return (current | operand) & active, nil
	case SemanticAnd:
		return (current & operand) & active, nil
	case SemanticAndNot:
		return (current &^ operand) & active, nil
	case SemanticXor:
		return (current ^ operand) & active, nil
	case SemanticSaturatingAdd:
		sum := uint64(current) + uint64(operand)
		if sum > uint64(active) {
			sum = uint64(active)
		}
		return uint32(sum), nil
	case SemanticSaturatingSubtract:
		if operand >= current {
			return 0, nil
		}
		return (current - operand) & active, nil
	default:
		return 0, ErrInvalidSemantic
	}
}
