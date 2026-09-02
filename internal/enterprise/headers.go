package enterprise

import "strings"

// canonHeader strips sheet decorations so we match by name, never by column letter I/J.
func canonHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "*")
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

// headerIndex maps canonical header -> column. Trailing spaces on eFleets "Cust Name " are why we trim.
func headerIndex(headers []string) map[string]int {
	m := make(map[string]int, len(headers))
	for i, h := range headers {
		m[canonHeader(h)] = i
	}
	return m
}

// col returns a cell by header name. Missing headers yield "" so a shifted column cannot silently become odometer.
func col(idx map[string]int, row []string, names ...string) string {
	for _, n := range names {
		i, ok := idx[canonHeader(n)]
		if !ok || i < 0 || i >= len(row) {
			continue
		}
		return strings.TrimSpace(row[i])
	}
	return ""
}

func requireHeaders(idx map[string]int, names ...string) error {
	var missing []string
	for _, n := range names {
		if _, ok := idx[canonHeader(n)]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &HeaderError{Missing: missing}
}

// HeaderError means the file is not the expected eFleets export. We refuse to guess by position.
type HeaderError struct {
	Missing []string
}

func (e *HeaderError) Error() string {
	return "eFleets file missing headers: " + strings.Join(e.Missing, ", ")
}
