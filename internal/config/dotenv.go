package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// envFiles are searched in order. First file that exists is applied; later files do not override keys already set.
// OILCHANGE_ENV can point at another path. Process environment always wins over the file.
func envFiles() []string {
	return []string{
		os.Getenv("OILCHANGE_ENV"),
		"oilchange.env",
		"secrets/oilchange.env",
		".env",
	}
}

// loadDotEnv pulls KEY=value from the local secrets file so operators can paste creds without exporting a shell.
func loadDotEnv() error {
	var last error
	loaded := false
	for _, p := range envFiles() {
		if p == "" {
			continue
		}
		if err := applyEnvFile(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			last = err
			continue
		}
		loaded = true
		break
	}
	if loaded {
		return nil
	}
	return last
}

// applyEnvFile sets os env from a dotenv file. Existing process env is left alone so tests and CI stay in charge.
// PEM blocks (BEGIN/END) may span lines; KEY=value lines after a PEM must still load.
func applyEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	var pemKey string
	var pemVal strings.Builder
	flushPEM := func() error {
		if pemKey == "" {
			return nil
		}
		k := pemKey
		v := strings.TrimSpace(pemVal.String())
		pemKey = ""
		pemVal.Reset()
		return setEnvIfEmpty(k, v)
	}
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if pemKey != "" {
			pemVal.WriteByte('\n')
			pemVal.WriteString(line)
			if strings.Contains(line, "-----END ") {
				if err := flushPEM(); err != nil {
					return fmt.Errorf("%s:%d: %w", path, lineNo, err)
				}
			}
			continue
		}
		key, val, ok, err := parseDotEnvLine(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if !ok {
			continue
		}
		if strings.Contains(val, "-----BEGIN ") && !strings.Contains(val, "-----END ") {
			pemKey = key
			pemVal.Reset()
			pemVal.WriteString(val)
			continue
		}
		if err := setEnvIfEmpty(key, val); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if pemKey != "" {
		return fmt.Errorf("%s: unterminated PEM for %s", path, pemKey)
	}
	return sc.Err()
}

// setEnvIfEmpty never overwrites a process env value so CI and tests stay in charge.
func setEnvIfEmpty(key, val string) error {
	if os.Getenv(key) != "" {
		return nil
	}
	return os.Setenv(key, val)
}

// parseDotEnvLine accepts KEY=value, export KEY=value, quotes, and # comments. Blank/comment lines are skipped.
func parseDotEnvLine(line string) (key, val string, ok bool, err error) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", "", false, nil
	}
	if strings.HasPrefix(s, "export ") {
		s = strings.TrimSpace(s[len("export "):])
	}
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return "", "", false, fmt.Errorf("expected KEY=value")
	}
	key = strings.TrimSpace(s[:eq])
	if key == "" {
		return "", "", false, fmt.Errorf("empty key")
	}
	val = strings.TrimSpace(s[eq+1:])
	if i := strings.Index(val, " #"); i >= 0 && !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `'`) {
		val = strings.TrimSpace(val[:i])
	}
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	return key, val, true, nil
}
