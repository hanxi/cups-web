package ipp

import "fmt"

const (
	SidesOneSided  = "one-sided"
	SidesLongEdge  = "two-sided-long-edge"
	SidesShortEdge = "two-sided-short-edge"
)

// ResolveSides validates an explicit sides keyword, falling back to the legacy
// duplex flag only when no keyword was provided.
func ResolveSides(sides string, isDuplex bool) (string, error) {
	switch sides {
	case SidesOneSided, SidesLongEdge, SidesShortEdge:
		return sides, nil
	case "":
		if isDuplex {
			return SidesLongEdge, nil
		}
		return SidesOneSided, nil
	default:
		return "", fmt.Errorf("invalid sides: %q", sides)
	}
}

// SidesForPageSet keeps manual duplex single-sided (issue #109). Callers must
// use the original page set before even-reverse is consumed by PDF reordering.
func SidesForPageSet(sides, pageSet string) string {
	switch normalizePageSet(pageSet) {
	case "odd", "even", "even-reverse":
		return SidesOneSided
	default:
		return sides
	}
}
