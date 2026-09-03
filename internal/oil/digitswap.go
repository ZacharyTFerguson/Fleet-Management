package oil

import "strconv"

// digitSwapCandidates returns odometer values from swapping two digits.
// Repair is only applied when the punch is flagged and the result lands in-band — fat-fingers, not a search of all numbers.
func digitSwapCandidates(odo int) []int {
	if odo < 0 {
		return nil
	}
	s := strconv.Itoa(odo)
	seen := map[int]struct{}{odo: {}}
	var out []int
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		for j := i + 1; j < len(b); j++ {
			if b[i] == b[j] {
				continue
			}
			b[i], b[j] = b[j], b[i]
			n, err := strconv.Atoi(string(b))
			b[i], b[j] = b[j], b[i]
			if err != nil {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	return out
}

// repairDigitSwap returns a swapped odo in band when the punch is flagged. Unflagged rows are never repaired.
func repairDigitSwap(odo int, flagged bool, band Band) (int, bool) {
	if !flagged {
		return odo, false
	}
	if band.Contains(odo) {
		return odo, false
	}
	for _, cand := range digitSwapCandidates(odo) {
		if band.Contains(cand) {
			return cand, true
		}
	}
	return odo, false
}
