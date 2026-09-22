package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type testConfig struct {
	Listen   string        `json:"listen" doc:"The address to serve on."`
	Database string        `json:"databaseDSN" doc:"The database to open."`
	Token    string        `json:"token" secret:"true" doc:"The bearer token."`
	Timeout  time.Duration `json:"timeout" doc:"How long a request may take."`
	Verbose  bool          `json:"verbose" doc:"Log every request."`
	Workers  int           `json:"workers" doc:"How many workers run."`
	Origins  []string      `json:"origins" doc:"Origins allowed to call the API."`
	Iroh     testIroh      `json:"iroh" doc:"The iroh endpoint."`
	OTel     string        `json:"otelEndpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT" doc:"Where traces go."`
	Internal string        `json:"-"`
}

type testIroh struct {
	LogLevel slog.Level `json:"logLevel" doc:"The iroh log level."`
	Relay    string     `json:"relay" doc:"The relay URL."`
}

var testSpec = Spec{Name: "demo-server", Prefix: "DEMO", Ignore: []string{"DEMO_SERVER"}}

func defaults() testConfig {
	return testConfig{
		Listen:   "127.0.0.1:8080",
		Database: "demo.db",
		Timeout:  30 * time.Second,
		Workers:  4,
		Origins:  []string{"https://a.example"},
		Iroh:     testIroh{LogLevel: slog.LevelWarn, Relay: "https://relay.example"},
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWithNoFileOrEnvironmentKeepsDefaults(t *testing.T) {
	cfg := defaults()
	if err := testSpec.Load("", nil, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg, defaults()) {
		t.Errorf("cfg = %+v, want the defaults", cfg)
	}
}

func TestLoadAppliesFileThenEnvironment(t *testing.T) {
	path := writeFile(t, "c.yaml", `
listen: 0.0.0.0:9000
databaseDSN: postgres://db
timeout: 5s
verbose: true
workers: 8
origins: [https://b.example, https://c.example]
iroh:
  logLevel: debug
`)
	cfg := defaults()
	env := []string{
		"DEMO_LISTEN=:7000",
		"DEMO_IROH_RELAY=https://other.example",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://collector",
		"DEMO_SERVER=http://ignored",
		"HOME=/root",
	}
	if err := testSpec.Load(path, env, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := defaults()
	want.Listen = ":7000"
	want.Database = "postgres://db"
	want.Timeout = 5 * time.Second
	want.Verbose = true
	want.Workers = 8
	want.Origins = []string{"https://b.example", "https://c.example"}
	want.Iroh = testIroh{LogLevel: slog.LevelDebug, Relay: "https://other.example"}
	want.OTel = "http://collector"
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

func TestLoadRejectsUnknownKeysWithTheirPaths(t *testing.T) {
	path := writeFile(t, "c.yaml", "listne: x\niroh:\n  lvl: debug\n")
	cfg := defaults()
	err := testSpec.Load(path, nil, &cfg)
	if err == nil {
		t.Fatal("Load succeeded, want an error")
	}
	for _, want := range []string{`c.yaml:1: unknown key "listne"`, `c.yaml:3: unknown key "iroh.lvl"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestLoadRejectsAPrefixedVariableThatMatchesNoSetting(t *testing.T) {
	cfg := defaults()
	err := testSpec.Load("", []string{"DEMO_LISTNE=:1"}, &cfg)
	if err == nil || !strings.Contains(err.Error(), "DEMO_LISTNE matches no setting") {
		t.Fatalf("Load error = %v, want DEMO_LISTNE rejected", err)
	}
}

func TestLoadRejectsTheDerivedNameOfAnExplicitlyNamedSetting(t *testing.T) {
	cfg := defaults()
	err := testSpec.Load("", []string{"DEMO_OTEL_ENDPOINT=http://x"}, &cfg)
	if err == nil {
		t.Fatal("Load succeeded, want DEMO_OTEL_ENDPOINT rejected")
	}
}

func TestLoadReportsBadValuesWithTheirSource(t *testing.T) {
	path := writeFile(t, "c.yaml", "workers: many\n")
	cfg := defaults()
	err := testSpec.Load(path, []string{"DEMO_VERBOSE=sometimes"}, &cfg)
	if err == nil {
		t.Fatal("Load succeeded, want an error")
	}
	for _, want := range []string{"c.yaml:1: workers", "DEMO_VERBOSE (verbose)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestLoadRejectsDuplicateKeys(t *testing.T) {
	path := writeFile(t, "c.yaml", "listen: a\nlisten: b\n")
	cfg := defaults()
	if err := testSpec.Load(path, nil, &cfg); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load error = %v, want a duplicate key", err)
	}
}

func TestLoadReadsASecretFromAFile(t *testing.T) {
	secret := writeFile(t, "token", "s3cret\n")
	cfg := defaults()
	if err := testSpec.Load("", []string{"DEMO_TOKEN=file:" + secret}, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Token != "s3cret" {
		t.Errorf("Token = %q, want %q", cfg.Token, "s3cret")
	}
}

func TestLoadLeavesFileValuesOfNonSecretsAlone(t *testing.T) {
	cfg := defaults()
	if err := testSpec.Load("", []string{"DEMO_DATABASE_DSN=file:demo.db?mode=ro"}, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database != "file:demo.db?mode=ro" {
		t.Errorf("Database = %q, want the SQLite URI unchanged", cfg.Database)
	}
}

func TestLoadAcceptsJSON(t *testing.T) {
	path := writeFile(t, "c.json", `{"listen": ":1", "iroh": {"relay": "r"}}`)
	cfg := defaults()
	if err := testSpec.Load(path, nil, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":1" || cfg.Iroh.Relay != "r" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadingTheExampleChangesNothing(t *testing.T) {
	example, err := testSpec.Example(defaults())
	if err != nil {
		t.Fatalf("Example: %v", err)
	}
	for i, line := range strings.Split(string(example), "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("example line %d has trailing whitespace: %q", i+1, line)
		}
	}
	path := writeFile(t, "demo.example.yaml", string(example))
	var cfg testConfig
	if err := testSpec.Load(path, nil, &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := defaults()
	want.Token, want.OTel = "", "" // empty defaults read back empty
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

func TestSchemaDocumentsEveryKey(t *testing.T) {
	schema, err := testSpec.Schema(defaults())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	for _, want := range []string{
		`"additionalProperties": false`,
		`"title": "demo-server configuration"`,
		`Environment: DEMO_IROH_LOG_LEVEL.`,
		`"default": "30s"`,
		`"default": 4`,
	} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("schema does not contain %q:\n%s", want, schema)
		}
	}
}

func TestGenerationRequiresADocOnEveryKey(t *testing.T) {
	type undocumented struct {
		Listen string `json:"listen"`
	}
	if _, err := testSpec.Schema(undocumented{}); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("Schema error = %v, want the undocumented key named", err)
	}
}

func TestSettingsSharingAVariableAreRejected(t *testing.T) {
	type clash struct {
		AB struct {
			C string `json:"c"`
		} `json:"aB"`
		A struct {
			BC string `json:"bC"`
		} `json:"a"`
	}
	var cfg clash
	if err := testSpec.Load("", nil, &cfg); err == nil || !strings.Contains(err.Error(), "DEMO_A_B_C") {
		t.Fatalf("Load error = %v, want the shared variable named", err)
	}
}

func TestUpperSnakeKeepsAcronymsTogether(t *testing.T) {
	for key, want := range map[string]string{
		"listen":      "LISTEN",
		"logLevel":    "LOG_LEVEL",
		"databaseDSN": "DATABASE_DSN",
		"tlsCAFile":   "TLS_CA_FILE",
		"v2Enabled":   "V2_ENABLED",
		"max-conns":   "MAX_CONNS",
	} {
		if got := upperSnake(key); got != want {
			t.Errorf("upperSnake(%q) = %q, want %q", key, got, want)
		}
	}
}
