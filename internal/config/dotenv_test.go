package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	key, val, ok, err := parseDotEnvLine(`EFLEETS_USERNAME=zach`)
	if err != nil || !ok || key != "EFLEETS_USERNAME" || val != "zach" {
		t.Fatalf("%s %s %v %v", key, val, ok, err)
	}
	_, _, ok, err = parseDotEnvLine(`# comment`)
	if err != nil || ok {
		t.Fatal("comment")
	}
	key, val, ok, err = parseDotEnvLine(`export ONESTEP_API_TOKEN="abc 123"`)
	if err != nil || !ok || key != "ONESTEP_API_TOKEN" || val != "abc 123" {
		t.Fatalf("quoted %s %q", key, val)
	}
	key, val, ok, err = parseDotEnvLine(`ONESTEP_API_PUBLIC_KEY= -----BEGIN PUBLIC KEY-----`)
	if err != nil || !ok || key != "ONESTEP_API_PUBLIC_KEY" || !strings.Contains(val, "BEGIN PUBLIC KEY") {
		t.Fatalf("pem start %s %q %v", key, val, err)
	}
}

func TestEnvReportOmitsValues(t *testing.T) {
	c := Config{EFleetsPass: "secret-pass", OneStepToken: "secret-token", SupabaseAnonKey: "sb_publishable_secret"}
	for _, line := range c.EnvReport() {
		if strings.Contains(line, "secret-pass") || strings.Contains(line, "secret-token") || strings.Contains(line, "sb_publishable_secret") {
			t.Fatalf("leaked %q", line)
		}
	}
}

func TestEnvReportGrokBuildKey(t *testing.T) {
	c := Config{SupabaseAnonKey: "sb_publishable_secret"}
	found := false
	for _, line := range c.EnvReport() {
		if strings.Contains(line, "sb_publishable_secret") {
			t.Fatalf("leaked %q", line)
		}
		if strings.HasPrefix(line, "SUPABASE_GROK_BUILD_KEY:") {
			found = true
			if !strings.Contains(line, "set") || !strings.Contains(line, "publishable") {
				t.Fatalf("expected presence note: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("SUPABASE_GROK_BUILD_KEY line missing")
	}
}

func TestDSNPrefersSQLiteWhenBothSet(t *testing.T) {
	c := Config{SQLitePath: "./oilchange.sqlite", DatabaseURL: "postgres://u:p@ep-x.us-east-2.aws.neon.tech/db"}
	driver, dsn, err := c.DSN()
	if err != nil {
		t.Fatal(err)
	}
	if driver != "sqlite" || dsn != "./oilchange.sqlite" {
		t.Fatalf("dsn %s %s", driver, dsn)
	}
}

func TestEnvReportDatabaseURLIsBackup(t *testing.T) {
	c := Config{DatabaseURL: "postgres://secret-user:secret-pass@ep-x.neon.tech/db"}
	found := false
	for _, line := range c.EnvReport() {
		if strings.Contains(line, "secret-pass") || strings.Contains(line, "secret-user") {
			t.Fatalf("leaked %q", line)
		}
		if strings.HasPrefix(line, "DATABASE_URL:") {
			found = true
			if !strings.Contains(line, "backup") {
				t.Fatalf("expected backup note: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("DATABASE_URL line missing")
	}
}

func TestApplyEnvFileDoesNotOverrideProcessEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "oilchange.env")
	if err := os.WriteFile(p, []byte("OILCHANGE_TEST_KEY=fromfile\nOILCHANGE_TEST_KEEP=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OILCHANGE_TEST_KEY", "fromproc")
	t.Setenv("OILCHANGE_TEST_KEEP", "")
	_ = os.Unsetenv("OILCHANGE_TEST_KEEP")
	if err := applyEnvFile(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OILCHANGE_TEST_KEY") != "fromproc" {
		t.Fatal("process env must win")
	}
	if os.Getenv("OILCHANGE_TEST_KEEP") != "file" {
		t.Fatal("file fills empty process env")
	}
}

func TestApplyEnvFilePEMThenLaterKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "oilchange.env")
	body := "EFLEETS_USERNAME=envuser\nONESTEP_API_PRIVATEKEY=-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\nONE_STEP_FULL_API_KEY=fromfile\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("EFLEETS_USERNAME")
	_ = os.Unsetenv("ONESTEP_API_PRIVATEKEY")
	_ = os.Unsetenv("ONE_STEP_FULL_API_KEY")
	t.Cleanup(func() {
		_ = os.Unsetenv("EFLEETS_USERNAME")
		_ = os.Unsetenv("ONESTEP_API_PRIVATEKEY")
		_ = os.Unsetenv("ONE_STEP_FULL_API_KEY")
	})
	if err := applyEnvFile(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ONE_STEP_FULL_API_KEY") != "fromfile" {
		t.Fatalf("key after PEM %q", os.Getenv("ONE_STEP_FULL_API_KEY"))
	}
	if !strings.Contains(os.Getenv("ONESTEP_API_PRIVATEKEY"), "BEGIN PRIVATE KEY") {
		t.Fatalf("pem %q", os.Getenv("ONESTEP_API_PRIVATEKEY"))
	}
}

func TestLoadPEMPath(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	pemPath := filepath.Join(dir, "client.pem")
	if err := os.WriteFile(pemPath, []byte("-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("oilchange.env", []byte("ONESTEP_PEM_PATH="+pemPath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ONESTEP_API_PRIVATEKEY", "ONESTEP_API_PRIVATE_KEY", "ONESTEP_PEM_PATH", "OILCHANGE_ENV"} {
		_ = os.Unsetenv(k)
	}
	cfg := Load()
	if !strings.Contains(cfg.OneStepPrivateKey, "BEGIN PRIVATE KEY") {
		t.Fatalf("pem path %q", cfg.OneStepPrivateKey)
	}
}

func TestLoadTokenAliases(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.WriteFile("oilchange.env", []byte("ONE_STEP_FULL_API_KEY=alias-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ONESTEP_API_TOKEN", "ONESTEP_API_KEY", "ONE_STEP_FULL_API_KEY", "OILCHANGE_ENV"} {
		_ = os.Unsetenv(k)
	}
	cfg := Load()
	if cfg.OneStepToken != "alias-key" {
		t.Fatalf("token %q", cfg.OneStepToken)
	}
}

func TestLoadReadsOilchangeEnv(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	body := "EFLEETS_USERNAME=envuser\nEFLEETS_CUST_NUM=999999\nOILCHANGE_DB=./fromenv.sqlite\n"
	if err := os.WriteFile("oilchange.env", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OILCHANGE_ENV", "")
	_ = os.Unsetenv("OILCHANGE_ENV")
	unsetEFleetsEnv(t)
	_ = os.Unsetenv("OILCHANGE_DB")
	_ = os.Unsetenv("ONESTEP_API_TOKEN")
	_ = os.Unsetenv("ONESTEP_API_KEY")
	_ = os.Unsetenv("ONE_STEP_FULL_API_KEY")
	cfg := Load()
	if cfg.EFleetsUser != "envuser" {
		t.Fatalf("user %q", cfg.EFleetsUser)
	}
	if cfg.EFleetsCust != "999999" {
		t.Fatalf("cust %q", cfg.EFleetsCust)
	}
	if cfg.SQLitePath != "./fromenv.sqlite" {
		t.Fatalf("db %q", cfg.SQLitePath)
	}
}

func TestLoadEFleetsCloudAliases(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	unsetEFleetsEnv(t)
	_ = os.Unsetenv("OILCHANGE_ENV")
	t.Setenv("EFleetsUsername", "portal-user")
	t.Setenv("EFleetsPassword", "portal-pass")
	t.Setenv("EFleetsCustNum", "583424")
	cfg := Load()
	if cfg.EFleetsUser != "portal-user" {
		t.Fatalf("user %q", cfg.EFleetsUser)
	}
	if cfg.EFleetsPass != "portal-pass" {
		t.Fatalf("pass %q", cfg.EFleetsPass)
	}
	if cfg.EFleetsCust != "583424" {
		t.Fatalf("cust %q", cfg.EFleetsCust)
	}
}

func TestEnvReportEFleetsHintOmitsValues(t *testing.T) {
	c := Config{EFleetsUser: "portal-user", EFleetsPass: "portal-pass", EFleetsCust: "583424"}
	foundUser, foundPass := false, false
	for _, line := range c.EnvReport() {
		if strings.Contains(line, "portal-user") || strings.Contains(line, "portal-pass") {
			t.Fatalf("leaked %q", line)
		}
		if strings.HasPrefix(line, "EFLEETS_USERNAME:") {
			foundUser = true
			if !strings.Contains(line, "Cloud Agent") || !strings.Contains(line, "never chat") {
				t.Fatalf("expected secrets hint: %q", line)
			}
		}
		if strings.HasPrefix(line, "EFLEETS_PASSWORD:") {
			foundPass = true
			if !strings.Contains(line, "set") {
				t.Fatalf("expected presence: %q", line)
			}
		}
	}
	if !foundUser || !foundPass {
		t.Fatal("eFleets env report lines missing")
	}
}

func unsetEFleetsEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"EFLEETS_USERNAME", "EFLEETS_USER", "EFleetsUsername", "EFleetsUser",
		"EFLEETS_PASSWORD", "EFLEETS_PASS", "EFleetsPassword", "EFleetsPass",
		"EFLEETS_CUST_NUM", "EFLEETS_CUSTOMER", "EFleetsCustNum",
		"EFLEETS_DETAILS_URL", "EFleetsDetailsURL",
		"EFLEETS_MAINT_URL", "EFleetsMaintURL",
		"EFLEETS_FLEETSUMMARY_URL", "EFleetsFleetSummaryURL",
	} {
		_ = os.Unsetenv(k)
	}
}
