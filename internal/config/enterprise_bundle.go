package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// applyBundledEnterpriseSecrets copies username/password/cust out of a single
// Cloud Agent secret named efleets or Enterprise secrets (spaces/underscores ignored).
// Explicit EFLEETS_* / EFleetsUsername env already set wins (setEnvIfEmpty).
func applyBundledEnterpriseSecrets() {
	raw := bundledEnterpriseSecret()
	if raw == "" {
		return
	}
	got := parseEnterpriseBundle(raw)
	if firstEnv("EFLEETS_USERNAME", "EFLEETS_USER", "EFleetsUsername", "EFleetsUser") == "" {
		_ = setEnvIfEmpty("EFLEETS_USERNAME", got.user)
	}
	if firstEnv("EFLEETS_PASSWORD", "EFLEETS_PASS", "EFleetsPassword", "EFleetsPass") == "" {
		_ = setEnvIfEmpty("EFLEETS_PASSWORD", got.pass)
	}
	if firstEnv("EFLEETS_CUST_NUM", "EFLEETS_CUSTOMER", "EFleetsCustNum") == "" {
		_ = setEnvIfEmpty("EFLEETS_CUST_NUM", got.cust)
	}
	if firstEnv("EFLEETS_DETAILS_URL", "EFleetsDetailsURL") == "" {
		_ = setEnvIfEmpty("EFLEETS_DETAILS_URL", got.details)
	}
	if firstEnv("EFLEETS_MAINT_URL", "EFleetsMaintURL") == "" {
		_ = setEnvIfEmpty("EFLEETS_MAINT_URL", got.maint)
	}
	if firstEnv("EFLEETS_FLEETSUMMARY_URL", "EFleetsFleetSummaryURL") == "" {
		_ = setEnvIfEmpty("EFLEETS_FLEETSUMMARY_URL", got.fleet)
	}
}

func bundledEnterpriseSecret() string {
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !isBundledEnterpriseName(k) {
			continue
		}
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func isBundledEnterpriseName(k string) bool {
	switch normalizeSecretName(k) {
	case "efleets", "enterprise", "enterprisesecret", "enterprisesecrets",
		"efleetssecret", "efleetssecrets":
		return true
	default:
		return false
	}
}

func normalizeSecretName(k string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(k)) {
		if r == ' ' || r == '_' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

type enterpriseFields struct {
	user, pass, cust, details, maint, fleet string
}

func parseEnterpriseBundle(raw string) enterpriseFields {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return enterpriseFields{}
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		if got, ok := parseEnterpriseJSON(raw); ok {
			return got
		}
	}
	return parseEnterpriseDotEnv(raw)
}

func parseEnterpriseJSON(raw string) (enterpriseFields, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		var wrap []any
		if err := json.Unmarshal([]byte(raw), &wrap); err != nil {
			return enterpriseFields{}, false
		}
		obj = map[string]any{}
		for _, item := range wrap {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := jsonString(m, "name", "key", "id")
			val := jsonString(m, "value", "secret")
			if name != "" && val != "" {
				obj[name] = val
			}
		}
		if len(obj) == 0 {
			return enterpriseFields{}, false
		}
	}
	if inner, ok := nestedEnterpriseObject(obj); ok {
		obj = inner
	}
	got := enterpriseFields{
		user:    jsonString(obj, "EFLEETS_USERNAME", "EFLEETS_USER", "EFleetsUsername", "EFleetsUser", "username", "user"),
		pass:    jsonString(obj, "EFLEETS_PASSWORD", "EFLEETS_PASS", "EFleetsPassword", "EFleetsPass", "password", "pass"),
		cust:    jsonString(obj, "EFLEETS_CUST_NUM", "EFLEETS_CUSTOMER", "EFleetsCustNum", "cust_num", "custnum", "customer", "cust"),
		details: jsonString(obj, "EFLEETS_DETAILS_URL", "EFleetsDetailsURL", "details_url", "details"),
		maint:   jsonString(obj, "EFLEETS_MAINT_URL", "EFleetsMaintURL", "maint_url"),
		fleet:   jsonString(obj, "EFLEETS_FLEETSUMMARY_URL", "EFleetsFleetSummaryURL", "fleet_url"),
	}
	return got, got.user != "" || got.pass != "" || got.cust != ""
}

func nestedEnterpriseObject(obj map[string]any) (map[string]any, bool) {
	want := map[string]struct{}{"efleets": {}, "enterprise": {}, "secrets": {}, "value": {}}
	for k, v := range obj {
		if _, ok := want[normalizeSecretName(k)]; !ok {
			continue
		}
		m, ok := v.(map[string]any)
		if ok && len(m) > 0 {
			return m, true
		}
	}
	return nil, false
}

func jsonString(m map[string]any, names ...string) string {
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[normalizeSecretName(n)] = struct{}{}
	}
	for k, v := range m {
		if _, ok := want[normalizeSecretName(k)]; !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case float64:
			if t == float64(int64(t)) {
				return strconv.FormatInt(int64(t), 10)
			}
			return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
		case json.Number:
			if s := strings.TrimSpace(t.String()); s != "" {
				return s
			}
		}
	}
	return ""
}

func parseEnterpriseDotEnv(raw string) enterpriseFields {
	got := enterpriseFields{}
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok, err := parseDotEnvLine(line)
		if err != nil || !ok {
			continue
		}
		switch normalizeSecretName(k) {
		case "efleetsusername", "efleetsuser", "username", "user":
			if got.user == "" {
				got.user = v
			}
		case "efleetspassword", "efleetspass", "password", "pass":
			if got.pass == "" {
				got.pass = v
			}
		case "efleetscustnum", "efleetscustomer", "custnum", "customer", "cust":
			if got.cust == "" {
				got.cust = v
			}
		case "efleetsdetailsurl", "detailsurl":
			if got.details == "" {
				got.details = v
			}
		case "efleetsmainturl", "mainturl":
			if got.maint == "" {
				got.maint = v
			}
		case "efleetsfleetsummaryurl", "fleeturl":
			if got.fleet == "" {
				got.fleet = v
			}
		}
	}
	return got
}
