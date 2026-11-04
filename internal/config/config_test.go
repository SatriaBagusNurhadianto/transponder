package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "transponder.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" || c.Store.SQLite != "./transponder.db" || c.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoadFile(t *testing.T) {
	p := writeFile(t, `
listen_addr: "127.0.0.1:9090"
store:
  sqlite: /data/t.db
secrets_key: abc
otel:
  endpoint: http://otel:4318
log_level: debug
`)
	c, err := Load(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Bootstrap{
		ListenAddr: "127.0.0.1:9090",
		Store:      Store{SQLite: "/data/t.db"},
		SecretsKey: "abc",
		OTel:       OTel{Endpoint: "http://otel:4318"},
		LogLevel:   "debug",
	}
	if *c != want {
		t.Fatalf("got %+v, want %+v", *c, want)
	}
}

func TestLoadExpandsVars(t *testing.T) {
	c, err := Load(writeFile(t, "secrets_key: ${K}\n"), []string{"K=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretsKey != "abc" {
		t.Fatalf("SecretsKey = %q", c.SecretsKey)
	}
	_, err = Load(writeFile(t, "secrets_key: ${MISSING}\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "MISSING") {
		t.Fatalf("err = %v, want it to name MISSING", err)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	p := writeFile(t, "listen_addr: \":8081\"\nstore:\n  sqlite: a.db\n")
	c, err := Load(p, []string{"TRANSPONDER_LISTEN_ADDR=:9000", "transponder_store__sqlite=/tmp/x.db", "PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9000" || c.Store.SQLite != "/tmp/x.db" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		environ []string
		path    string
	}{
		{name: "unknown file key", body: "listen_adr: x\n"},
		{name: "both stores", body: "store:\n  sqlite: a.db\n  postgres: postgres://x\n"},
		{name: "bad log level", body: "log_level: loud\n"},
		{name: "missing file", path: filepath.Join(t.TempDir(), "nope.yaml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.path
			if p == "" {
				p = writeFile(t, tt.body)
			}
			if _, err := Load(p, tt.environ); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadIgnoresUnknownEnv(t *testing.T) {
	environ := []string{
		"TRANSPONDER_PORT=tcp://10.0.0.1:8080",
		"TRANSPONDER_SERVICE_HOST=10.0.0.1",
		"TRANSPONDER_PORT_8080_TCP_ADDR=10.0.0.1",
		"TRANSPONDER_STORE=oops",
		"TRANSPONDER_STORE__NOPE=1",
		"TRANSPONDER_LOG_LEVEL=warn",
	}
	c, err := Load("", environ)
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel != "warn" || c.Store.SQLite != "./transponder.db" {
		t.Fatalf("config = %+v", c)
	}
}
