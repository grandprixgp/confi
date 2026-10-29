package confi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// customRole implements encoding.TextUnmarshaler.
type customRole struct {
	Name string
}

func (r *customRole) UnmarshalText(text []byte) error {
	r.Name = "role:" + strings.ToUpper(string(text))
	return nil
}

func TestReadmeExample(t *testing.T) {
	type arguments struct {
		Hostname   string
		Port       []int
		Interfaces []string
		Filters    []string
	}

	var cfg arguments
	args := []string{
		"--hostname=nic.ac",
		"--port=", "80",
		"--port", "8080",
		"--interfaces", "eth0", "vmbr0",
		"--filters=eth0: src host 1.1.1.1", "eth0: dst host 1.1.1.1",
		"--filters", "vmbr0: host dst 10.0.0",
	}

	err := Parse(&cfg, WithName("app"), WithArgs(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Hostname != "nic.ac" {
		t.Errorf("expected Hostname 'nic.ac', got %q", cfg.Hostname)
	}
	if !reflect.DeepEqual(cfg.Port, []int{80, 8080}) {
		t.Errorf("expected Port [80 8080], got %v", cfg.Port)
	}
	if !reflect.DeepEqual(cfg.Interfaces, []string{"eth0", "vmbr0"}) {
		t.Errorf("expected Interfaces [eth0 vmbr0], got %v", cfg.Interfaces)
	}
	wantFilters := []string{
		"eth0: src host 1.1.1.1",
		"eth0: dst host 1.1.1.1",
		"vmbr0: host dst 10.0.0",
	}
	if !reflect.DeepEqual(cfg.Filters, wantFilters) {
		t.Errorf("expected Filters %q, got %q", wantFilters, cfg.Filters)
	}
}

func TestPrimitivesAndSlices(t *testing.T) {
	type allTypes struct {
		B      bool
		S      string
		I      int
		I8     int8
		I16    int16
		I32    int32
		I64    int64
		U      uint
		U8     uint8
		U16    uint16
		U32    uint32
		U64    uint64
		UPtr   uintptr
		F32    float32
		F64    float64
		SliceI []int16
		SliceU []uint8
		SliceF []float64
		SliceB []bool
	}

	var cfg allTypes
	args := []string{
		"--b",
		"--s", "hello",
		"--i", "-42",
		"--i8", "-8",
		"--i16", "-1600",
		"--i32", "-32000",
		"--i64", "-6400000",
		"--u", "42",
		"--u8", "250",
		"--u16", "1600",
		"--u32", "32000",
		"--u64", "6400000",
		"--u-ptr", "0x10",
		"--f32", "3.5",
		"--f64", "-2.25",
		"--slice-i", "10", "-20", "30",
		"--slice-u", "1,2,3",
		"--slice-f", "1.5", "2.5",
		"--slice-b", "true", "false", "yes",
	}

	if err := Parse(&cfg, WithArgs(args)); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !cfg.B || cfg.S != "hello" || cfg.I != -42 || cfg.I8 != -8 || cfg.I16 != -1600 || cfg.I32 != -32000 || cfg.I64 != -6400000 {
		t.Errorf("signed/basic fields mismatch: %+v", cfg)
	}
	if cfg.U != 42 || cfg.U8 != 250 || cfg.U16 != 1600 || cfg.U32 != 32000 || cfg.U64 != 6400000 || cfg.UPtr != 16 {
		t.Errorf("unsigned fields mismatch: %+v", cfg)
	}
	if cfg.F32 != 3.5 || cfg.F64 != -2.25 {
		t.Errorf("float fields mismatch: f32=%v f64=%v", cfg.F32, cfg.F64)
	}
	if !reflect.DeepEqual(cfg.SliceI, []int16{10, -20, 30}) {
		t.Errorf("SliceI mismatch: %v", cfg.SliceI)
	}
	if !reflect.DeepEqual(cfg.SliceU, []uint8{1, 2, 3}) {
		t.Errorf("SliceU mismatch: %v", cfg.SliceU)
	}
	if !reflect.DeepEqual(cfg.SliceF, []float64{1.5, 2.5}) {
		t.Errorf("SliceF mismatch: %v", cfg.SliceF)
	}
	if !reflect.DeepEqual(cfg.SliceB, []bool{true, false, true}) {
		t.Errorf("SliceB mismatch: %v", cfg.SliceB)
	}
}

func TestRichTypes(t *testing.T) {
	type richConfig struct {
		Timeout   time.Duration     `confi:"timeout,default=5s"`
		StartDate time.Time         `confi:"start-date"`
		Endpoint  url.URL           `confi:"endpoint"`
		Proxy     *url.URL          `confi:"proxy"`
		Pattern   regexp.Regexp     `confi:"pattern"`
		IP        net.IP            `confi:"ip"`
		Role      customRole        `confi:"role"`
		OptCount  *int              `confi:"opt-count"`
		Labels    map[string]string `confi:"labels"`
		Limits    map[string]int    `confi:"limits"`
	}

	var cfg richConfig
	args := []string{
		"--timeout", "1m30s",
		"--start-date", "2026-10-26",
		"--endpoint", "https://api.example.com/v1",
		"--proxy", "http://localhost:8888",
		"--pattern", "^[a-z0-9]+$",
		"--ip", "192.168.1.50",
		"--role", "admin",
		"--opt-count", "99",
		"--labels", "env=prod", "tier=backend,region:us-east",
		"--limits", "cpu=4,mem=8192",
	}

	if err := Parse(&cfg, WithArgs(args)); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Timeout != 90*time.Second {
		t.Errorf("expected Timeout 90s, got %v", cfg.Timeout)
	}
	if cfg.StartDate.Year() != 2026 || cfg.StartDate.Month() != 10 || cfg.StartDate.Day() != 26 {
		t.Errorf("unexpected StartDate: %v", cfg.StartDate)
	}
	if cfg.Endpoint.String() != "https://api.example.com/v1" {
		t.Errorf("unexpected Endpoint: %v", cfg.Endpoint.String())
	}
	if cfg.Proxy == nil || cfg.Proxy.String() != "http://localhost:8888" {
		t.Errorf("unexpected Proxy: %v", cfg.Proxy)
	}
	if !cfg.Pattern.MatchString("abc123") || cfg.Pattern.MatchString("ABC!") {
		t.Errorf("Pattern regex did not compile as expected")
	}
	if !cfg.IP.Equal(net.ParseIP("192.168.1.50")) {
		t.Errorf("unexpected IP: %v", cfg.IP)
	}
	if cfg.Role.Name != "role:ADMIN" {
		t.Errorf("unexpected Role: %+v", cfg.Role)
	}
	if cfg.OptCount == nil || *cfg.OptCount != 99 {
		t.Errorf("unexpected OptCount: %v", cfg.OptCount)
	}
	wantLabels := map[string]string{"env": "prod", "tier": "backend", "region": "us-east"}
	if !reflect.DeepEqual(cfg.Labels, wantLabels) {
		t.Errorf("expected Labels %v, got %v", wantLabels, cfg.Labels)
	}
	wantLimits := map[string]int{"cpu": 4, "mem": 8192}
	if !reflect.DeepEqual(cfg.Limits, wantLimits) {
		t.Errorf("expected Limits %v, got %v", wantLimits, cfg.Limits)
	}
}

func TestShortFlagsBundlingAndNegation(t *testing.T) {
	type flags struct {
		All     bool   `confi:"all,short=a"`
		Long    bool   `confi:"long,short=l"`
		Human   bool   `confi:"human,short=H"`
		Port    int    `confi:"port,short=p"`
		TLS     bool   `confi:"tls,default=true"`
		Ignored string `confi:"-"`
	}

	var cfg flags
	args := []string{"-alp", "9090", "-H", "--no-tls"}
	if err := Parse(&cfg, WithArgs(args)); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !cfg.All || !cfg.Long || !cfg.Human {
		t.Errorf("expected bundled short bool flags to be true: %+v", cfg)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected Port 9090, got %d", cfg.Port)
	}
	if cfg.TLS {
		t.Errorf("expected --no-tls to set TLS=false, got true")
	}
}

func TestNestedStructsAndPositional(t *testing.T) {
	type Common struct {
		Debug bool `confi:"debug,short=d"`
	}
	type DBConfig struct {
		Host string `confi:"host,default=localhost"`
		Port int    `confi:"port,default=5432"`
	}
	type AppConfig struct {
		Common
		Database *DBConfig `confi:"database"`
		Command  string    `confi:"command,positional,required"`
		Files    []string  `confi:"files,positional"`
	}

	var cfg AppConfig
	args := []string{
		"-d",
		"--database.host", "db.internal",
		"--database-port=5433",
		"deploy",
		"--",
		"--literal-file.txt",
		"second.txt",
	}

	if err := Parse(&cfg, WithArgs(args)); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !cfg.Debug {
		t.Errorf("expected Debug=true from embedded struct")
	}
	if cfg.Database == nil || cfg.Database.Host != "db.internal" || cfg.Database.Port != 5433 {
		t.Errorf("unexpected Database config: %+v", cfg.Database)
	}
	if cfg.Command != "deploy" {
		t.Errorf("expected Command 'deploy', got %q", cfg.Command)
	}
	wantFiles := []string{"--literal-file.txt", "second.txt"}
	if !reflect.DeepEqual(cfg.Files, wantFiles) {
		t.Errorf("expected Files %v, got %v", wantFiles, cfg.Files)
	}
}

func TestPrecedenceChain(t *testing.T) {
	// Precedence: Defaults < Config File < Environment Variables < CLI Flags
	type ServerConfig struct {
		FromDefault string   `confi:"from-default,default=def_val"`
		FromConfig  string   `confi:"from-config,default=def_val"`
		FromEnv     string   `confi:"from-env,default=def_val,env=FROM_ENV"`
		FromCLI     string   `confi:"from-cli,default=def_val,env=FROM_CLI"`
		Ports       []int    `confi:"ports,default='80,81'"`
		Tags        []string `confi:"tags,default='t1,t2'"`
	}

	yamlContent := `
from_config: cfg_val
from_env: cfg_val
from_cli: cfg_val
ports:
  - 8000
  - 8001
tags: [cfg_tag]
`
	envMap := map[string]string{
		"APP_FROM_ENV": "env_val",
		"APP_FROM_CLI": "env_val",
		"APP_TAGS":     "env_tag1,env_tag2",
	}
	lookupEnv := func(k string) (string, bool) {
		v, ok := envMap[k]
		return v, ok
	}

	var cfg ServerConfig
	err := Parse(
		&cfg,
		WithConfigReader(strings.NewReader(yamlContent), FormatYAML),
		WithEnvPrefix("APP"),
		WithAutoEnv(),
		WithEnvLookup(lookupEnv),
		WithArgs([]string{"--from-cli", "cli_val", "--ports", "9000", "--ports", "9001"}),
	)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.FromDefault != "def_val" {
		t.Errorf("expected FromDefault='def_val', got %q", cfg.FromDefault)
	}
	if cfg.FromConfig != "cfg_val" {
		t.Errorf("expected FromConfig='cfg_val', got %q", cfg.FromConfig)
	}
	if cfg.FromEnv != "env_val" {
		t.Errorf("expected FromEnv='env_val', got %q", cfg.FromEnv)
	}
	if cfg.FromCLI != "cli_val" {
		t.Errorf("expected FromCLI='cli_val', got %q", cfg.FromCLI)
	}
	// CLI flags should replace config/default slice values, while appending multiple CLI flags
	if !reflect.DeepEqual(cfg.Ports, []int{9000, 9001}) {
		t.Errorf("expected Ports [9000 9001], got %v", cfg.Ports)
	}
	// Env slice should replace config/default slice values
	if !reflect.DeepEqual(cfg.Tags, []string{"env_tag1", "env_tag2"}) {
		t.Errorf("expected Tags [env_tag1 env_tag2], got %v", cfg.Tags)
	}
}

func TestConfigFormatsJSONYAMLAndDotEnv(t *testing.T) {
	type Nested struct {
		Host string `json:"host" yaml:"host"`
		Port int    `json:"port" yaml:"port"`
	}
	type Config struct {
		AppName     string            `confi:"app-name,env=APP_NAME"`
		Banner      string            `confi:"banner"`
		Enabled     bool              `confi:"enabled"`
		Rate        float64           `confi:"rate"`
		AllowedIPs  []string          `confi:"allowed-ips"`
		Annotations map[string]string `confi:"annotations"`
		DB          Nested            `confi:"db"`
	}

	tmpDir, err := ioutil.TempDir("", "confi_test_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Test JSON file
	jsonPath := filepath.Join(tmpDir, "config.json")
	jsonData := `{
		"app-name": "json-service",
		"enabled": true,
		"rate": 0.75,
		"allowed-ips": ["10.0.0.1", "10.0.0.2"],
		"annotations": {"team": "core", "env": "staging"},
		"db": {"host": "json.db", "port": 3306}
	}`
	if err := ioutil.WriteFile(jsonPath, []byte(jsonData), 0600); err != nil {
		t.Fatalf("WriteFile json failed: %v", err)
	}

	var jsonCfg Config
	if err := Parse(&jsonCfg, WithConfigFile(jsonPath), WithArgs(nil)); err != nil {
		t.Fatalf("JSON Parse failed: %v", err)
	}
	if jsonCfg.AppName != "json-service" || !jsonCfg.Enabled || jsonCfg.Rate != 0.75 || jsonCfg.DB.Host != "json.db" || jsonCfg.DB.Port != 3306 {
		t.Errorf("unexpected jsonCfg: %+v", jsonCfg)
	}
	if !reflect.DeepEqual(jsonCfg.AllowedIPs, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Errorf("unexpected AllowedIPs: %v", jsonCfg.AllowedIPs)
	}
	if jsonCfg.Annotations["team"] != "core" {
		t.Errorf("unexpected Annotations: %v", jsonCfg.Annotations)
	}

	// 2. Test YAML file with block scalar, flow map, comments, and dynamic --config flag
	yamlPath := filepath.Join(tmpDir, "config.yaml")
	yamlData := `
# Application YAML config
app_name: "yaml-service"
banner: |-
  Welcome to Confi
  Line Two
enabled: yes
rate: 1.25
allowed_ips:
  - "192.168.0.1"
  - '192.168.0.2'
annotations: {team: platform, region: eu-west}
db:
  host: yaml.db # inline comment
  port: 5432
`
	if err := ioutil.WriteFile(yamlPath, []byte(yamlData), 0600); err != nil {
		t.Fatalf("WriteFile yaml failed: %v", err)
	}

	var yamlCfg Config
	if err := Parse(&yamlCfg, WithConfigFlag("config"), WithArgs([]string{"--config", yamlPath})); err != nil {
		t.Fatalf("YAML Parse failed: %v", err)
	}
	if yamlCfg.AppName != "yaml-service" || !yamlCfg.Enabled || yamlCfg.Rate != 1.25 || yamlCfg.DB.Host != "yaml.db" || yamlCfg.DB.Port != 5432 {
		t.Errorf("unexpected yamlCfg: %+v", yamlCfg)
	}
	if yamlCfg.Banner != "Welcome to Confi\nLine Two" {
		t.Errorf("unexpected YAML block scalar Banner: %q", yamlCfg.Banner)
	}
	if yamlCfg.Annotations["region"] != "eu-west" {
		t.Errorf("unexpected YAML flow map Annotations: %v", yamlCfg.Annotations)
	}

	// 3. Test .env file with variable expansion and export keyword
	envPath := filepath.Join(tmpDir, ".env")
	envData := `
# Dotenv config
BASE_NAME=dotenv
export APP_NAME="${BASE_NAME}-service"
ENABLED=true
RATE=2.5
ALLOWED_IPS='127.0.0.1, 10.10.0.1' # comment
DB_HOST=${UNDEFINED_HOST:-dotenv.db}
DB_PORT=6379
`
	if err := ioutil.WriteFile(envPath, []byte(envData), 0600); err != nil {
		t.Fatalf("WriteFile .env failed: %v", err)
	}

	var envCfg Config
	if err := Parse(&envCfg, WithConfigFile(envPath), WithOptionalConfigFile(filepath.Join(tmpDir, "nonexistent.yaml")), WithArgs(nil)); err != nil {
		t.Fatalf(".env Parse failed: %v", err)
	}
	if envCfg.AppName != "dotenv-service" || !envCfg.Enabled || envCfg.Rate != 2.5 || envCfg.DB.Host != "dotenv.db" || envCfg.DB.Port != 6379 {
		t.Errorf("unexpected envCfg: %+v", envCfg)
	}
	if !reflect.DeepEqual(envCfg.AllowedIPs, []string{"127.0.0.1", "10.10.0.1"}) {
		t.Errorf("unexpected envCfg AllowedIPs: %v", envCfg.AllowedIPs)
	}
}

func TestValidationAndStrictErrors(t *testing.T) {
	type ReqStruct struct {
		APIKey string `confi:"api-key,required"`
		Port   int    `confi:"port,default=8080"`
	}

	var cfg ReqStruct
	err := Parse(&cfg, WithArgs(nil))
	if err == nil {
		t.Fatalf("expected ValidationError for missing required api-key")
	}
	valErr, ok := err.(*ValidationError)
	if !ok || len(valErr.MissingFields) != 1 || valErr.MissingFields[0] != "api-key" {
		t.Errorf("unexpected error type or missing fields: %T (%v)", err, err)
	}

	// Strict mode unknown CLI flag
	err = Parse(&cfg, WithStrict(true), WithArgs([]string{"--api-key=secret", "--unknown-flag=1"}))
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown flag error in strict mode, got: %v", err)
	}

	// Invalid target error
	if err := Parse(cfg); err == nil {
		t.Errorf("expected error when passing non-pointer struct to Parse")
	}
}

func TestHelpAndUsageOutput(t *testing.T) {
	type HelpStruct struct {
		Host    string   `confi:"host,short=H,default=0.0.0.0,env=HOST,usage=Address to bind"`
		Port    int      `confi:"port,short=p,required,usage=Port to listen on"`
		Verbose bool     `confi:"verbose,short=v,usage=Enable verbose logging"`
		Targets []string `confi:"targets,positional,usage=Target endpoints"`
	}

	var cfg HelpStruct
	var out bytes.Buffer
	err := Parse(
		&cfg,
		WithName("myservice"),
		WithDescription("High-performance network service."),
		WithConfigFlag("config"),
		WithOutput(&out),
		WithArgs([]string{"--help"}),
	)
	if err != ErrHelp {
		t.Fatalf("expected ErrHelp, got %v", err)
	}

	helpText := out.String()
	expectedSubstrings := []string{
		"Usage: myservice [flags] [<targets>...]",
		"High-performance network service.",
		"-H, --host <string>",
		"Address to bind",
		"[default: 0.0.0.0]",
		"[env: HOST]",
		"-p, --port <int>",
		"(required)",
		"-v, --verbose",
		"--config <path>",
		"-h, --help",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(helpText, sub) {
			t.Errorf("expected help output to contain %q, got:\n%s", sub, helpText)
		}
	}
}

func TestStandaloneTagsAndConfigField(t *testing.T) {
	type StandaloneCfg struct {
		ConfigPath string   `flag:"config-file" config:"true"`
		Items      []string `name:"items" short:"i" default:"a;b;c" sep:";" usage:"Semicolon items"`
		EnvOnly    string   `env:"SPECIAL_ENV" required:"true" help:"Special env"`
	}

	tmpDir, err := ioutil.TempDir("", "confi_standalone_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgFile := filepath.Join(tmpDir, "app.json")
	if err := ioutil.WriteFile(cfgFile, []byte(`{"items": ["x", "y"]}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var cfg StandaloneCfg
	err = Parse(
		&cfg,
		WithEnvLookup(func(k string) (string, bool) {
			if k == "SPECIAL_ENV" {
				return "ok_env", true
			}
			return "", false
		}),
		WithArgs([]string{"--config-file", cfgFile}),
	)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !reflect.DeepEqual(cfg.Items, []string{"x", "y"}) {
		t.Errorf("expected Items [x y] from dynamic config file, got %v", cfg.Items)
	}
	if cfg.EnvOnly != "ok_env" {
		t.Errorf("expected EnvOnly 'ok_env', got %q", cfg.EnvOnly)
	}

	usageStr, err := Usage(&cfg, WithName("standalone"))
	if err != nil || !strings.Contains(usageStr, "Semicolon items") {
		t.Errorf("unexpected Usage output: %q, err=%v", usageStr, err)
	}
}

func TestEdgeCasesAndErrorPaths(t *testing.T) {
	// Duplicate short flag detection
	type DupShort struct {
		First  string `confi:"first,short=f"`
		Second string `confi:"second,short=f"`
	}
	var dup DupShort
	if err := Parse(&dup, WithArgs(nil)); err == nil || !strings.Contains(err.Error(), "duplicate short flag") {
		t.Errorf("expected duplicate short flag error, got: %v", err)
	}

	// Strict config key error in YAML and .env
	type Simple struct {
		Name string `confi:"name"`
	}
	var s Simple
	err := Parse(&s, WithStrict(true), WithConfigReader(strings.NewReader("unknown_key: 123\n"), FormatYAML), WithArgs(nil))
	if err == nil || !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected strict unknown YAML key error, got: %v", err)
	}

	err = Parse(&s, WithStrict(true), WithConfigReader(strings.NewReader("UNKNOWN_ENV=123\n"), FormatEnv), WithArgs(nil))
	if err == nil || !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected strict unknown .env key error, got: %v", err)
	}

	// Custom prefix and folded YAML block scalar
	yamlFolded := "name: >\n  hello\n  world\n"
	err = Parse(&s, WithPrefix("+"), WithShortPrefix("/"), WithHelpFlag(false), WithConfigReader(strings.NewReader(yamlFolded), FormatAuto), WithArgs(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "hello world" {
		t.Errorf("expected folded YAML scalar 'hello world', got %q", s.Name)
	}
}

type serveSubcmd struct {
	Port   int  `confi:"port,short=p,default=8080,usage=HTTP listen port"`
	ran    bool
}

func (s *serveSubcmd) Run() error {
	s.ran = true
	return nil
}

type migrateSubcmd struct {
	Steps int    `confi:"steps,default=1,usage=Number of migrations"`
	Dir   string `confi:"dir,positional,required,usage=Migration directory"`
}

func TestSubcommandsAndRunner(t *testing.T) {
	type AppCLI struct {
		Verbose bool           `confi:"verbose,short=v,usage=Global verbose mode"`
		Serve   *serveSubcmd   `confi:"serve,subcommand,usage=Start the web server"`
		Migrate *migrateSubcmd `confi:"migrate,subcommand,usage=Run database migrations"`
	}

	// 1. Test `app -v serve -p 9090` with WithAutoRun(true)
	var cli1 AppCLI
	p1 := NewParser("app", WithAutoRun(true), WithArgs([]string{"-v", "serve", "-p", "9090"}))
	if err := p1.Parse(&cli1); err != nil {
		t.Fatalf("subcommand serve Parse failed: %v", err)
	}
	if p1.ActiveCommand() != "serve" {
		t.Errorf("expected ActiveCommand 'serve', got %q", p1.ActiveCommand())
	}
	if !cli1.Verbose || cli1.Serve == nil || cli1.Serve.Port != 9090 || !cli1.Serve.ran {
		t.Errorf("unexpected cli1 state: verbose=%v serve=%+v", cli1.Verbose, cli1.Serve)
	}
	if cli1.Migrate != nil {
		t.Errorf("expected unused subcommand Migrate to remain nil")
	}

	// 2. Test `app migrate --steps 3 ./migrations`
	var cli2 AppCLI
	p2 := NewParser("app", WithArgs([]string{"migrate", "--steps", "3", "./migrations"}))
	if err := p2.Parse(&cli2); err != nil {
		t.Fatalf("subcommand migrate Parse failed: %v", err)
	}
	if p2.ActiveCommand() != "migrate" || cli2.Migrate == nil || cli2.Migrate.Steps != 3 || cli2.Migrate.Dir != "./migrations" {
		t.Errorf("unexpected cli2 Migrate state: %+v", cli2.Migrate)
	}

	// 3. Test scoped subcommand `--help` (`app serve --help`)
	var cli3 AppCLI
	var helpBuf bytes.Buffer
	err := Parse(&cli3, WithName("app"), WithOutput(&helpBuf), WithArgs([]string{"serve", "--help"}))
	if err != ErrHelp {
		t.Fatalf("expected ErrHelp for subcommand help, got %v", err)
	}
	if !strings.Contains(helpBuf.String(), "Usage: app serve [flags]") || !strings.Contains(helpBuf.String(), "--port <int>") {
		t.Errorf("unexpected subcommand help output:\n%s", helpBuf.String())
	}

	// 4. Test root `--help` shows Commands section
	helpBuf.Reset()
	err = Parse(&cli3, WithName("app"), WithOutput(&helpBuf), WithArgs([]string{"--help"}))
	if err != ErrHelp {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	if !strings.Contains(helpBuf.String(), "Commands:") || !strings.Contains(helpBuf.String(), "serve") || !strings.Contains(helpBuf.String(), "migrate") {
		t.Errorf("expected root help output to list Commands:\n%s", helpBuf.String())
	}
}

type validatedStruct struct {
	Env      string   `confi:"env,oneof=dev|staging|prod,default=dev"`
	Port     int      `confi:"port,min=1024,max=65535,default=8080"`
	Username string   `confi:"username,min=3,max=16,pattern=^[a-z0-9_]+$,default=admin_1"`
	Tags     []string `confi:"tags,min=1,max=3,default='a,b'"`
	MaxConn  int      `confi:"max-conn,default=10"`
	MinConn  int      `confi:"min-conn,default=2"`
}

func (v *validatedStruct) Validate() error {
	if v.MinConn > v.MaxConn {
		return &ParseError{Field: "MinConn", Message: "min-conn cannot exceed max-conn"}
	}
	return nil
}

func TestValidationRulesAndCustomValidator(t *testing.T) {
	var validCfg validatedStruct
	if err := Parse(&validCfg, WithArgs(nil)); err != nil {
		t.Fatalf("expected valid default config to pass, got: %v", err)
	}

	// Test invalid oneof
	var c1 validatedStruct
	if err := Parse(&c1, WithArgs([]string{"--env=invalid"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("expected oneof error, got: %v", err)
	}

	// Test invalid numeric min/max
	var c2 validatedStruct
	if err := Parse(&c2, WithArgs([]string{"--port=80"})); err == nil || !strings.Contains(err.Error(), "less than minimum") {
		t.Errorf("expected min error for port=80, got: %v", err)
	}

	// Test invalid string length min
	var c3 validatedStruct
	if err := Parse(&c3, WithArgs([]string{"--username=ab"})); err == nil || !strings.Contains(err.Error(), "length 2 is less than minimum") {
		t.Errorf("expected string min length error, got: %v", err)
	}

	// Test invalid regex pattern
	var c4 validatedStruct
	if err := Parse(&c4, WithArgs([]string{"--username=INVALID-USER!"})); err == nil || !strings.Contains(err.Error(), "does not match pattern") {
		t.Errorf("expected regex pattern error, got: %v", err)
	}

	// Test invalid slice count max
	var c5 validatedStruct
	if err := Parse(&c5, WithArgs([]string{"--tags=a,b,c,d"})); err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected slice max count error, got: %v", err)
	}

	// Test custom Validator interface
	var c6 validatedStruct
	if err := Parse(&c6, WithArgs([]string{"--min-conn=50", "--max-conn=10"})); err == nil || !strings.Contains(err.Error(), "min-conn cannot exceed max-conn") {
		t.Errorf("expected custom Validator error, got: %v", err)
	}
}

func TestTOMLAndINIConfig(t *testing.T) {
	type ServerSection struct {
		Host  string   `toml:"host"`
		Port  int      `toml:"port"`
		Peers []string `toml:"peers"`
	}
	type TOMLConfig struct {
		Title       string            `confi:"title"`
		Description string            `confi:"description"`
		Server      ServerSection     `confi:"server"`
		Labels      map[string]string `confi:"labels"`
	}

	tomlContent := `
# Sample TOML configuration
title = "Confi TOML App"
description = """
Multi-line
TOML description"""
labels = { env = "prod", region = "us-west" }

[server]
host = "10.0.0.5" ; inline comment
port = 8443
peers = [
  "10.0.0.6",
  "10.0.0.7"
]
`
	var cfg TOMLConfig
	err := Parse(&cfg, WithConfigReader(strings.NewReader(tomlContent), FormatTOML), WithArgs(nil))
	if err != nil {
		t.Fatalf("TOML Parse failed: %v", err)
	}
	if cfg.Title != "Confi TOML App" || cfg.Description != "Multi-line\nTOML description" {
		t.Errorf("unexpected Title/Description: %+v", cfg)
	}
	if cfg.Server.Host != "10.0.0.5" || cfg.Server.Port != 8443 || !reflect.DeepEqual(cfg.Server.Peers, []string{"10.0.0.6", "10.0.0.7"}) {
		t.Errorf("unexpected Server section: %+v", cfg.Server)
	}
	if cfg.Labels["env"] != "prod" || cfg.Labels["region"] != "us-west" {
		t.Errorf("unexpected Labels: %v", cfg.Labels)
	}
}

func TestSecretRedactionAndSampleGeneration(t *testing.T) {
	type DB struct {
		Host     string `confi:"host,default=db.local,usage=Database host"`
		Password string `confi:"password,secret,required,env=DB_PASS,usage=Database password"`
	}
	type SampleApp struct {
		Env  string `confi:"env,default=prod,oneof=dev|staging|prod,usage=Deployment environment"`
		Port int    `confi:"port,default=8080,usage=HTTP port"`
		DB   DB     `confi:"db"`
	}

	cfg := SampleApp{
		Env:  "prod",
		Port: 8080,
		DB:   DB{Host: "db.local", Password: "super-secret-password"},
	}

	dumped := Sprint(&cfg)
	if strings.Contains(dumped, "super-secret-password") || !strings.Contains(dumped, "********") {
		t.Errorf("expected secret password to be masked in Sprint output:\n%s", dumped)
	}

	redactedMap, err := RedactMap(&cfg)
	if err != nil {
		t.Fatalf("RedactMap failed: %v", err)
	}
	dbMap := redactedMap["db"].(map[string]interface{})
	if dbMap["password"] != "********" {
		t.Errorf("expected redacted map password '********', got %v", dbMap["password"])
	}

	for _, fmtType := range []Format{FormatYAML, FormatJSON, FormatTOML, FormatEnv} {
		sample, err := GenerateSample(&cfg, fmtType, WithDescription("Sample App Configuration"))
		if err != nil {
			t.Fatalf("GenerateSample(%v) failed: %v", fmtType, err)
		}
		if strings.Contains(sample, "super-secret-password") {
			t.Errorf("GenerateSample(%v) leaked secret value:\n%s", fmtType, sample)
		}
		if len(sample) == 0 {
			t.Errorf("GenerateSample(%v) returned empty output", fmtType)
		}
	}
}

func TestShellCompletions(t *testing.T) {
	type Sub struct {
		Fast bool `confi:"fast,short=f,usage=Run fast"`
	}
	type CompStruct struct {
		Mode string `confi:"mode,short=m,oneof=auto|manual,usage=Execution mode"`
		Run  *Sub   `confi:"run,subcommand,usage=Execute job"`
	}

	var cfg CompStruct
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		script, err := Completion(sh, &cfg, WithName("mycli"))
		if err != nil {
			t.Fatalf("Completion(%q) failed: %v", sh, err)
		}
		if !strings.Contains(script, "mycli") || !strings.Contains(script, "mode") {
			t.Errorf("Completion(%q) missing expected tokens:\n%s", sh, script)
		}
	}

	// Test WithCompletionFlag
	var buf bytes.Buffer
	err := Parse(&cfg, WithName("mycli"), WithCompletionFlag("completion"), WithOutput(&buf), WithArgs([]string{"--completion", "bash"}))
	if err != ErrHelp {
		t.Fatalf("expected ErrHelp when invoking --completion, got %v", err)
	}
	if !strings.Contains(buf.String(), "_mycli_completions") {
		t.Errorf("unexpected completion flag output:\n%s", buf.String())
	}
}

func TestLiveConfigWatcher(t *testing.T) {
	type WatchedCfg struct {
		Host string `confi:"host,default=localhost"`
		Port int    `confi:"port,default=8080"`
	}

	tmpDir, err := ioutil.TempDir("", "confi_watch_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "watch.yaml")
	if err := ioutil.WriteFile(cfgPath, []byte("host: initial.local\nport: 8000\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var cfg WatchedCfg
	changeCh := make(chan error, 4)

	watcher, err := Watch(
		&cfg,
		func(reloadErr error) {
			changeCh <- reloadErr
		},
		WithConfigFile(cfgPath),
		WithWatchInterval(20*time.Millisecond),
		WithArgs(nil),
	)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer watcher.Close()

	if cfg.Host != "initial.local" || cfg.Port != 8000 {
		t.Errorf("unexpected initial watched config: %+v", cfg)
	}

	// Update config file on disk
	if err := ioutil.WriteFile(cfgPath, []byte("host: updated.local\nport: 9090\n"), 0600); err != nil {
		t.Fatalf("WriteFile update failed: %v", err)
	}

	select {
	case reloadErr := <-changeCh:
		if reloadErr != nil {
			t.Fatalf("unexpected reload error: %v", reloadErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Watch reload callback")
	}

	watcher.RLock()
	host, port := cfg.Host, cfg.Port
	watcher.RUnlock()

	if host != "updated.local" || port != 9090 {
		t.Errorf("expected reloaded config updated.local:9090, got %s:%d", host, port)
	}
}




func TestP0Fixes(t *testing.T) {
	// 1. YAML tab indentation panic (CRIT-01)
	type TabCfg struct {
		Host string `confi:"host"`
	}
	var tabCfg TabCfg
	err := Parse(&tabCfg, WithConfigReader(strings.NewReader("\t\thost: localhost\n"), FormatYAML), WithArgs(nil))
	if err != nil || tabCfg.Host != "localhost" {
		t.Fatalf("CRIT-01 failed: expected host=localhost from tab-indented YAML, got %q, err=%v", tabCfg.Host, err)
	}

	// 2. Strict mode nested map key bypass (CRIT-02)
	type DBConfig struct {
		Host string `confi:"host"`
	}
	type StrictCfg struct {
		DB DBConfig `confi:"db"`
	}
	var strictCfg StrictCfg
	yamlWithUnknownNested := "db:\n  host: localhost\n  unrecognized_setting: 1234\n"
	err = Parse(&strictCfg, WithConfigReader(strings.NewReader(yamlWithUnknownNested), FormatYAML), WithStrict(true), WithArgs(nil))
	if err == nil || !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("CRIT-02 failed: expected unknown config key error in strict mode for nested key, got: %v", err)
	}

	// 3. Cyclic struct stack overflow guard (CRIT-03)
	type Node struct {
		Name string `confi:"name"`
		Next *Node  `confi:"next"`
	}
	var node Node
	err = Parse(&node, WithArgs([]string{"--name", "root"}))
	if err != nil || node.Name != "root" {
		t.Fatalf("CRIT-03 failed: cyclic struct parsing failed: %v", err)
	}

	// 4. Watcher self-deadlock in onChange (CRIT-04)
	tmpFile, err := ioutil.TempFile("", "watch_crit_")
	if err != nil {
		t.Fatalf("TempFile failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("port: 8080\n")
	tmpFile.Close()

	type PortCfg struct {
		Port int `confi:"port"`
	}
	var portCfg PortCfg
	var w *Watcher
	stopped := make(chan struct{})
	w, err = Watch(&portCfg, func(reloadErr error) {
		w.Stop() // Must not deadlock
		close(stopped)
	}, WithConfigFile(tmpFile.Name()), WithWatchInterval(10*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	ioutil.WriteFile(tmpFile.Name(), []byte("port: 9090\n"), 0600)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatalf("CRIT-04 failed: Watcher deadlocked when Stop() called inside onChange")
	}

	// 5. Explicit zero values on required fields (CRIT-05)
	type ReqZero struct {
		Port    int     `confi:"port,required"`
		Rate    float64 `confi:"rate,required"`
		Verbose bool    `confi:"verbose,required"`
	}
	var reqZero ReqZero
	err = Parse(&reqZero, WithArgs([]string{"--port", "0", "--rate", "0.0", "--verbose=false"}))
	if err != nil {
		t.Fatalf("CRIT-05 failed: explicit zero values rejected on required fields: %v", err)
	}
	if reqZero.Port != 0 || reqZero.Rate != 0.0 || reqZero.Verbose {
		t.Fatalf("CRIT-05 failed: unexpected parsed zero values: %+v", reqZero)
	}
}


func TestP1Fixes(t *testing.T) {
	// 1. Negative positional number (MAJ-01)
	type Calc struct {
		Offset int `confi:"offset,pos"`
	}
	var c Calc
	err := Parse(&c, WithArgs([]string{"-10"}), WithStrict(true))
	if err != nil || c.Offset != -10 {
		t.Fatalf("MAJ-01 failed: negative positional parse failed: offset=%d, err=%v", c.Offset, err)
	}

	// 2. Empty inline flag (MAJ-02)
	type InlineCfg struct {
		Prefix string `confi:"prefix"`
		File   string `confi:"file,pos"`
	}
	var inlineCfg InlineCfg
	err = Parse(&inlineCfg, WithArgs([]string{"--prefix=", "data.csv"}))
	if err != nil || inlineCfg.Prefix != "" || inlineCfg.File != "data.csv" {
		t.Fatalf("MAJ-02 failed: expected prefix='', file='data.csv', got prefix=%q, file=%q, err=%v", inlineCfg.Prefix, inlineCfg.File, err)
	}

	// 3. Pointer map default & env (MAJ-03)
	type PtrMapCfg struct {
		Tags *map[string]string `confi:"tags,default='env=prod'"`
	}
	var ptrMap PtrMapCfg
	err = Parse(&ptrMap, WithArgs(nil))
	if err != nil || ptrMap.Tags == nil || (*ptrMap.Tags)["env"] != "prod" {
		t.Fatalf("MAJ-03 failed: pointer map default failed: tags=%v, err=%v", ptrMap.Tags, err)
	}

	// 4. Subcommand runner does not fall through to root (MAJ-04)
	type RootCmd struct {
		Sub struct{} `confi:"sub,subcommand"`
	}
	var rootCmd RootCmd
	err = Parse(&rootCmd, WithArgs([]string{"sub"}), WithAutoRun(true))
	if err == nil || !strings.Contains(err.Error(), "does not implement Runner") {
		t.Fatalf("MAJ-04 failed: expected error that subcommand does not implement Runner, got: %v", err)
	}

	// 5. CLI --config precedence over struct default config (MAJ-05)
	tmpDir, err := ioutil.TempDir("", "confi_maj5_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	defaultPath := filepath.Join(tmpDir, "default.yaml")
	overridePath := filepath.Join(tmpDir, "override.yaml")
	ioutil.WriteFile(defaultPath, []byte("host: default.local\n"), 0600)
	ioutil.WriteFile(overridePath, []byte("host: override.local\n"), 0600)

	type CfgPrecedence struct {
		ConfigFile string `confi:"config,config,default="`
		Host       string `confi:"host"`
	}
	var cfgPrec CfgPrecedence
	cfgPrec.ConfigFile = defaultPath
	err = Parse(&cfgPrec, WithConfigFlag("config"), WithArgs([]string{"--config", overridePath}))
	if err != nil || cfgPrec.Host != "override.local" {
		t.Fatalf("MAJ-05 failed: expected Host='override.local', got %q, err=%v", cfgPrec.Host, err)
	}

	// 6. TOML array of tables and unterminated array (MAJ-06..10)
	type ServerEntry struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	}
	type TOMLArrayCfg struct {
		Servers []ServerEntry `confi:"servers"`
	}
	tomlData := `
[[servers]]
host = "srv1.local"
port = 8001

[[servers]]
host = "srv2.local"
port = 8002
`
	var tomlArr CfgPrecedence
	_ = tomlArr
	var tomlCfg TOMLArrayCfg
	err = Parse(&tomlCfg, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil))
	if err != nil || len(tomlCfg.Servers) != 2 || tomlCfg.Servers[0].Host != "srv1.local" || tomlCfg.Servers[1].Host != "srv2.local" {
		t.Fatalf("MAJ-09 failed: expected 2 servers in table array, got: %+v, err=%v", tomlCfg.Servers, err)
	}

	// Test unterminated TOML array returns error
	type BadTOML struct {
		Ports []int `confi:"ports"`
	}
	var badTOML BadTOML
	err = Parse(&badTOML, WithConfigReader(strings.NewReader("ports = [ 80, 81\n"), FormatTOML), WithArgs(nil))
	if err == nil || !strings.Contains(err.Error(), "unterminated multiline array") {
		t.Fatalf("MAJ-07 failed: expected error on unterminated TOML array, got: %v", err)
	}

	// 7. Multiline .env support (MAJ-11)
	type MultiDotEnv struct {
		Cert string `confi:"cert"`
	}
	var mDot MultiDotEnv
	dotEnvData := "CERT=\"line1\nline2\nline3\"\n"
	err = Parse(&mDot, WithConfigReader(strings.NewReader(dotEnvData), FormatEnv), WithArgs(nil))
	if err != nil || mDot.Cert != "line1\nline2\nline3" {
		t.Fatalf("MAJ-11 failed: expected multiline cert, got %q, err=%v", mDot.Cert, err)
	}

	// 8. Duration min/max validation (MAJ-20)
	type DurCfg struct {
		Timeout time.Duration `confi:"timeout,min=100ms,max=5s,default=1s"`
	}
	var dur DurCfg
	err = Parse(&dur, WithArgs([]string{"--timeout=50ms"}))
	if err == nil || !strings.Contains(err.Error(), "less than minimum") {
		t.Fatalf("MAJ-20 failed: expected min duration error for 50ms, got: %v", err)
	}
	err = Parse(&dur, WithArgs([]string{"--timeout=10s"}))
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("MAJ-20 failed: expected max duration error for 10s, got: %v", err)
	}
}

func TestPhase3PerformanceAndWatcher(t *testing.T) {
	// 1. Memory limit on reader (MAJ-17)
	type SimpleCfg struct {
		Name string `confi:"name"`
	}
	var s SimpleCfg
	hugeReader := io.LimitReader(bytes.NewReader(bytes.Repeat([]byte("name=foo\n"), 2000000)), 11<<20)
	err := Parse(&s, WithConfigReader(hugeReader, FormatEnv), WithArgs(nil))
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum limit") {
		t.Fatalf("MAJ-17 failed: expected limit error, got: %v", err)
	}

	// 2. Schema caching (MAJ-26)
	var s1, s2 SimpleCfg
	if err := Parse(&s1, WithArgs([]string{"--name=alpha"})); err != nil {
		t.Fatalf("Parse s1 failed: %v", err)
	}
	if err := Parse(&s2, WithArgs([]string{"--name=beta"})); err != nil {
		t.Fatalf("Parse s2 failed: %v", err)
	}
	cached, ok := schemaCache.Load(reflect.TypeOf(s1))
	if !ok || cached == nil {
		t.Fatalf("MAJ-26 failed: schema was not cached in schemaCache")
	}

	// 3. Watcher programmatic defaults preservation (MAJ-19) & thread safety (MAJ-18)
	tmpDir, err := ioutil.TempDir("", "confi_watch_p3_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := ioutil.WriteFile(cfgPath, []byte(`{"workers": 5}`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type AppCfg struct {
		Workers         int           `confi:"workers"`
		ProgrammaticDef time.Duration // programmatic default without tag
	}

	app := AppCfg{
		ProgrammaticDef: 42 * time.Second,
	}

	ch := make(chan error, 1)
	watcher, err := Watch(&app, func(e error) {
		ch <- e
	}, WithConfigFile(cfgPath), WithWatchInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer watcher.Close()

	if app.Workers != 5 || app.ProgrammaticDef != 42*time.Second {
		t.Fatalf("initial watch parse mismatch: workers=%d, prog=%v", app.Workers, app.ProgrammaticDef)
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				watcher.RLock()
				_ = app.Workers
				_ = app.ProgrammaticDef
				watcher.RUnlock()
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	if err := ioutil.WriteFile(cfgPath, []byte(`{"workers": 10}`), 0644); err != nil {
		t.Fatalf("WriteFile update failed: %v", err)
	}

	select {
	case reloadErr := <-ch:
		if reloadErr != nil {
			t.Fatalf("reload reported error: %v", reloadErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for reload notification")
	}

	close(done)
	watcher.RLock()
	if app.Workers != 10 {
		t.Fatalf("expected workers=10, got %d", app.Workers)
	}
	if app.ProgrammaticDef != 42*time.Second {
		t.Fatalf("MAJ-19 failed: programmatic default erased! expected 42s, got %v", app.ProgrammaticDef)
	}
	watcher.RUnlock()
}

func TestPhase4ObservabilityAndErgonomics(t *testing.T) {
	// 1. Slice comma splitting with whitespace (MAJ-21)
	type TagsCfg struct {
		Tags []string `confi:"tags"`
	}
	var tagCfg TagsCfg
	err := Parse(&tagCfg, WithArgs([]string{"--tags", "apple, banana, cherry"}))
	if err != nil {
		t.Fatalf("Parse tags failed: %v", err)
	}
	if len(tagCfg.Tags) != 3 || tagCfg.Tags[0] != "apple" || tagCfg.Tags[1] != "banana" || tagCfg.Tags[2] != "cherry" {
		t.Fatalf("MAJ-21 failed: expected 3 split tags, got: %v", tagCfg.Tags)
	}

	// 2. Zsh completion short flags, subcommands, and escaping (MAJ-22 & MAJ-23)
	type ServeCmd struct {
		Port int `confi:"port,short=p,usage='Port [default: 8080]'"`
	}
	type RootApp struct {
		Verbose bool      `confi:"verbose,short=v,usage='Enable verbose [logging: debug]'"`
		Serve   *ServeCmd `confi:"serve,subcommand,usage='Start background server'"`
	}
	var root RootApp
	var zshBuf bytes.Buffer
	err = Parse(&root, WithName("testapp"), WithCompletionFlag("completion"), WithOutput(&zshBuf), WithArgs([]string{"--completion", "zsh"}))
	if err != nil && err != ErrHelp {
		t.Fatalf("Parse zsh completion failed: %v", err)
	}
	zshOut := zshBuf.String()
	if !strings.Contains(zshOut, `'-v[Enable verbose \[logging\: debug\]]'`) {
		t.Fatalf("MAJ-22/23 failed: expected escaped usage and short flag for -v, got:\n%s", zshOut)
	}
	if !strings.Contains(zshOut, `'serve:Start background server'`) {
		t.Fatalf("MAJ-22 failed: expected subcommand serve in zsh completion, got:\n%s", zshOut)
	}
	if !strings.Contains(zshOut, `'-p[Port \[default\: 8080\]]'`) {
		t.Fatalf("MAJ-22 failed: expected subcommand short flag -p in zsh completion, got:\n%s", zshOut)
	}

	// 3. YAML sample generator for sibling structs (MAJ-24)
	type DBNode struct {
		Host string `confi:"host,default=localhost"`
		Port int    `confi:"port,default=5432"`
	}
	type DBCluster struct {
		Primary DBNode `confi:"primary"`
		Replica DBNode `confi:"replica"`
	}
	type ClusterCfg struct {
		DB DBCluster `confi:"db"`
	}
	yamlSample, err := GenerateSample(&ClusterCfg{}, FormatYAML)
	if err != nil {
		t.Fatalf("GenerateSample YAML failed: %v", err)
	}
	lines := strings.Split(yamlSample, "\n")
	dbCount := 0
	for _, l := range lines {
		if strings.TrimRight(l, "\r") == "db:" {
			dbCount++
		}
	}
	if dbCount != 1 {
		t.Fatalf("MAJ-24 failed: expected 'db:' exactly once in YAML sample, got %d times in:\n%s", dbCount, yamlSample)
	}

	// 4. TOML sample generator array quoting (MAJ-25)
	type ListCfg struct {
		Fruits []string `confi:"fruits,default='apple,banana'"`
	}
	tomlSample, err := GenerateSample(&ListCfg{}, FormatTOML)
	if err != nil {
		t.Fatalf("GenerateSample TOML failed: %v", err)
	}
	if !strings.Contains(tomlSample, `fruits = ["apple", "banana"]`) {
		t.Fatalf("MAJ-25 failed: expected quoted TOML array, got:\n%s", tomlSample)
	}

	// 5. MustParse custom error output and exitFunc (MAJ-28)
	var errOutBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	exitCode := -1
	MustParse(&RootApp{},
		WithArgs([]string{"--unknown-flag"}),
		WithStrict(true),
		WithOutput(&stdoutBuf),
		WithErrOutput(&errOutBuf),
		WithExitFunc(func(code int) {
			exitCode = code
		}),
	)
	if exitCode != 1 {
		t.Fatalf("MAJ-28 failed: expected exit code 1, got %d", exitCode)
	}
	if stdoutBuf.Len() > 0 {
		t.Fatalf("MAJ-28 failed: stdout was polluted: %q", stdoutBuf.String())
	}
	if !strings.Contains(errOutBuf.String(), "unknown flag") {
		t.Fatalf("MAJ-28 failed: expected error in errOutBuf, got %q", errOutBuf.String())
	}
}

func TestPhase5VerificationAndStress(t *testing.T) {
	// 1. Concurrent race stress on Watcher (MAJ-30)
	tmpDir, err := ioutil.TempDir("", "confi_stress_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "stress.json")
	if err := ioutil.WriteFile(cfgPath, []byte(`{"counter": 1}`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type StressCfg struct {
		Counter int `confi:"counter"`
	}
	var stressCfg StressCfg

	watcher, err := Watch(&stressCfg, func(err error) {
		// on reload callback
	}, WithConfigFile(cfgPath), WithWatchInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer watcher.Close()

	stopStress := make(chan struct{})
	var wg sync.WaitGroup

	// Readers
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopStress:
					return
				default:
					watcher.RLock()
					_ = stressCfg.Counter
					watcher.RUnlock()
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Writers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				select {
				case <-stopStress:
					return
				default:
					_ = ioutil.WriteFile(cfgPath, []byte(fmt.Sprintf(`{"counter": %d}`, j+writerID*100)), 0644)
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(i)
	}

	time.Sleep(150 * time.Millisecond)
	close(stopStress)
	wg.Wait()

	// 2. Fuzz simulation on zero-dependency parsers (MAJ-29)
	r := rand.New(rand.NewSource(1337))

	// Fuzz CLI Lexer
	type FuzzStruct struct {
		Alpha string   `confi:"alpha,short=a"`
		Num   int      `confi:"num,short=n"`
		Flag  bool     `confi:"flag,short=f"`
		List  []string `confi:"list"`
	}
	sampleTokens := []string{
		"--alpha", "-a", "val", "--num", "-n", "42", "-42", "--flag", "-f", "-fa",
		"--list", "a,b,c", "", "-", "--", "--unknown", "pos1", "pos2",
		"=", "--alpha=", "--num=invalid", "\x00", "\n", "\t", "true", "false",
	}
	for i := 0; i < 300; i++ {
		numTokens := r.Intn(8)
		tokens := make([]string, numTokens)
		for j := 0; j < numTokens; j++ {
			tokens[j] = sampleTokens[r.Intn(len(sampleTokens))]
		}
		var fTarget FuzzStruct
		_ = Parse(&fTarget, WithArgs(tokens), WithStrict(false))
	}

	// Fuzz YAML parser
	yamlCorpus := [][]byte{
		[]byte(""),
		[]byte("\t\ta: 1\n"),
		[]byte("key: [1, 2, \n"),
		[]byte("key: {a: 1\n"),
		[]byte("foo: \"bar\n"),
		[]byte("deep:\n  nest:\n    val: 1\n"),
		[]byte("---\nfoo: bar\n---\nbaz: qux\n"),
		[]byte("list:\n  - a\n  - b\n"),
	}
	for i := 0; i < 300; i++ {
		var payload []byte
		if r.Intn(2) == 0 {
			base := yamlCorpus[r.Intn(len(yamlCorpus))]
			payload = make([]byte, len(base))
			copy(payload, base)
			if len(payload) > 0 {
				payload[r.Intn(len(payload))] = byte(r.Intn(256))
			}
		} else {
			payload = make([]byte, r.Intn(64))
			r.Read(payload)
		}
		_, _ = parseYAML(bytes.NewReader(payload))
	}

	// Fuzz TOML parser
	tomlCorpus := [][]byte{
		[]byte(""),
		[]byte("a = 1\n"),
		[]byte("[section]\nb = \"str\"\n"),
		[]byte("[[arr]]\nname = 1\n"),
		[]byte("unclosed = [1, 2\n"),
		[]byte("unclosed_str = \"no end\n"),
		[]byte("hash = # comment\n1\n"),
	}
	for i := 0; i < 300; i++ {
		var payload []byte
		if r.Intn(2) == 0 {
			base := tomlCorpus[r.Intn(len(tomlCorpus))]
			payload = make([]byte, len(base))
			copy(payload, base)
			if len(payload) > 0 {
				payload[r.Intn(len(payload))] = byte(r.Intn(256))
			}
		} else {
			payload = make([]byte, r.Intn(64))
			r.Read(payload)
		}
		_, _ = parseTOML(bytes.NewReader(payload))
	}

	// Fuzz .env parser
	dotEnvCorpus := [][]byte{
		[]byte(""),
		[]byte("A=B\n"),
		[]byte("FOO=\"bar\n"),
		[]byte("REF=$VAR_${NOT_EXIST:-def}\n"),
		[]byte("EXPORT FOO=BAR\n"),
		[]byte("MULTI=\"line1\nline2\"\n"),
	}
	for i := 0; i < 300; i++ {
		var payload []byte
		if r.Intn(2) == 0 {
			base := dotEnvCorpus[r.Intn(len(dotEnvCorpus))]
			payload = make([]byte, len(base))
			copy(payload, base)
			if len(payload) > 0 {
				payload[r.Intn(len(payload))] = byte(r.Intn(256))
			}
		} else {
			payload = make([]byte, r.Intn(64))
			r.Read(payload)
		}
		_, _ = parseDotEnv(bytes.NewReader(payload), os.LookupEnv)
	}

	// 3. Zero-coverage APIs and error paths
	var buf bytes.Buffer
	if err := WriteCompletion(&buf, "bash", &FuzzStruct{}); err != nil {
		t.Fatalf("WriteCompletion failed: %v", err)
	}
	buf.Reset()
	if err := WriteUsage(&buf, &FuzzStruct{}); err != nil {
		t.Fatalf("WriteUsage failed: %v", err)
	}

	innerErr := errors.New("underlying cause")
	parseErr := &ParseError{Field: "foo", Message: "failed", Err: innerErr}
	if parseErr.Unwrap() != innerErr {
		t.Fatalf("ParseError.Unwrap mismatch: got %v", parseErr.Unwrap())
	}

	valErr := &ValidationError{MissingFields: []string{"Host", "Port"}}
	if !strings.Contains(valErr.Error(), "Host, Port") {
		t.Fatalf("ValidationError.Error mismatch: got %s", valErr.Error())
	}

	os.Setenv("CONFI_TEST_USER_99", "admin99")
	defer os.Unsetenv("CONFI_TEST_USER_99")
	envMap, err := parseDotEnv(strings.NewReader("GREETING=hello $CONFI_TEST_USER_99!\n"), os.LookupEnv)
	if err != nil || envMap["GREETING"] != "hello admin99!" {
		t.Fatalf("DotEnv $VAR expansion failed: %v, got %q", err, envMap["GREETING"])
	}

	type MapFieldCfg struct {
		Metadata map[string]string `confi:"metadata"`
	}
	var mapCfg MapFieldCfg
	err = Parse(&mapCfg, WithConfigReader(strings.NewReader(`{"metadata": {"key1": "val1", "key2": "val2"}}`), FormatJSON), WithStrict(true), WithArgs(nil))
	if err != nil || mapCfg.Metadata["key1"] != "val1" {
		t.Fatalf("Parse map field failed in strict mode: %v", err)
	}
}

func TestReviewBugFixes(t *testing.T) {
	// 1. Watcher captures pre-parse initial state and reverts removed config keys to defaults
	tmpDir, err := ioutil.TempDir("", "confi_review_watch_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "watch.yaml")
	if err := ioutil.WriteFile(cfgPath, []byte("host: override.local\nport: 9000\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type WatchRevertCfg struct {
		Host string            `confi:"host,default=default.local"`
		Port int               `confi:"port,default=8080"`
		Tags map[string]string `confi:"tags"`
	}
	watchCfg := WatchRevertCfg{
		Tags: map[string]string{"init": "yes"},
	}
	reloadCh := make(chan error, 2)
	watcher, err := Watch(&watchCfg, func(e error) {
		reloadCh <- e
	}, WithConfigFile(cfgPath), WithWatchInterval(15*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer watcher.Close()

	if watchCfg.Host != "override.local" || watchCfg.Port != 9000 {
		t.Fatalf("unexpected initial watchCfg: %+v", watchCfg)
	}

	// Remove `host` from config file; reload should revert Host to default ("default.local")
	time.Sleep(25 * time.Millisecond)
	if err := ioutil.WriteFile(cfgPath, []byte("port: 9001\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	select {
	case rErr := <-reloadCh:
		if rErr != nil {
			t.Fatalf("reload failed: %v", rErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for reload")
	}

	watcher.RLock()
	gotHost, gotPort, gotInitTag := watchCfg.Host, watchCfg.Port, watchCfg.Tags["init"]
	watcher.RUnlock()
	if gotHost != "default.local" || gotPort != 9001 || gotInitTag != "yes" {
		t.Fatalf("expected Host='default.local', Port=9001, Tags[init]='yes', got host=%q port=%d tag=%q", gotHost, gotPort, gotInitTag)
	}

	// 2. *time.Duration pointer with min/max tags
	type PtrDurCfg struct {
		Timeout *time.Duration `confi:"timeout,min=100ms,max=5s"`
	}
	var ptrDur PtrDurCfg
	if err := Parse(&ptrDur, WithArgs([]string{"--timeout=500ms"})); err != nil {
		t.Fatalf("expected valid *time.Duration to pass, got: %v", err)
	}
	if ptrDur.Timeout == nil || *ptrDur.Timeout != 500*time.Millisecond {
		t.Fatalf("unexpected *time.Duration value: %v", ptrDur.Timeout)
	}
	if err := Parse(&ptrDur, WithArgs([]string{"--timeout=20ms"})); err == nil || !strings.Contains(err.Error(), "less than minimum") {
		t.Fatalf("expected min error on *time.Duration, got: %v", err)
	}

	// 3. Strict mode rejects stray positional token after string flag
	type StrictStrCfg struct {
		Host string `confi:"host"`
	}
	var strictStr StrictStrCfg
	if err := Parse(&strictStr, WithStrict(true), WithArgs([]string{"--host", "example.com", "stray_positional"})); err == nil || !strings.Contains(err.Error(), "unexpected positional argument") {
		t.Fatalf("expected strict mode error on stray positional token after string flag, got: %v (Host=%q)", err, strictStr.Host)
	}

	// 4. YAML sequence item starting with nested mapping key (`- nested:`)
	type NestedNode struct {
		Host string `yaml:"host"`
	}
	type SeqItem struct {
		Nested NestedNode `yaml:"nested"`
		Name   string     `yaml:"name"`
	}
	type SeqCfg struct {
		Items []SeqItem `confi:"items"`
	}
	yamlSeqData := "items:\n  - nested:\n      host: db1.local\n    name: primary\n"
	var seqCfg SeqCfg
	if err := Parse(&seqCfg, WithConfigReader(strings.NewReader(yamlSeqData), FormatYAML), WithArgs(nil)); err != nil {
		t.Fatalf("YAML sequence with nested mapping failed: %v", err)
	}
	if len(seqCfg.Items) != 1 || seqCfg.Items[0].Nested.Host != "db1.local" || seqCfg.Items[0].Name != "primary" {
		t.Fatalf("unexpected YAML sequence items: %+v", seqCfg.Items)
	}

	// 5. TOML quoted dots in section names and colons before '='
	type TOMLQuotedCfg struct {
		Servers map[string]map[string]string `confi:"servers"`
	}
	tomlQuoted := "[servers.\"us.east\"]\n\"urn:endpoint\" = \"https://us-east.example.com:8443\"\n"
	var tqCfg TOMLQuotedCfg
	if err := Parse(&tqCfg, WithConfigReader(strings.NewReader(tomlQuoted), FormatTOML), WithArgs(nil)); err != nil {
		t.Fatalf("TOML quoted section parse failed: %v", err)
	}
	if tqCfg.Servers["us.east"]["urn:endpoint"] != "https://us-east.example.com:8443" {
		t.Fatalf("unexpected TOML quoted section result: %+v", tqCfg.Servers)
	}
}

func TestFormatParserBugFixes(t *testing.T) {
	// MAJ-08: TOML multiline triple-quoted strings with comments, quotes, and escapes
	t.Run("MAJ-08_TOMLMultilineStrings", func(t *testing.T) {
		type Cfg struct {
			Query string `confi:"query"`
			Note  string `confi:"note"`
			Raw   string `confi:"raw"`
		}
		tomlData := `
query = """
SELECT "#1", note FROM users
WHERE name = "Alice" # not a comment inside string
"""
note = """He said "hello world"""" # trailing comment
raw = '''
C:\Users\admin\Documents # literal comment preserved
'''
`
		var cfg Cfg
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatalf("failed to parse TOML multiline strings: %v", err)
		}
		if !strings.Contains(cfg.Query, `"#1"`) || !strings.Contains(cfg.Query, `WHERE name = "Alice" # not a comment inside string`) {
			t.Fatalf("multiline query corrupted by comment stripper: %q", cfg.Query)
		}
		if cfg.Note != `He said "hello world"` {
			t.Fatalf("multiline note mismatch: expected %q, got %q", `He said "hello world"`, cfg.Note)
		}
		if !strings.Contains(cfg.Raw, `C:\Users\admin\Documents # literal comment preserved`) {
			t.Fatalf("literal multiline raw mismatch: %q", cfg.Raw)
		}
	})

	// MAJ-10: TOML section header overwriting prior scalar key without error
	t.Run("MAJ-10_TOMLTableRedefiningScalarError", func(t *testing.T) {
		type Cfg struct {
			Server string `confi:"server"`
		}
		tomlData := "server = \"localhost\"\n[server]\nport = 8080\n"
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil))
		if err == nil {
			t.Fatal("expected syntax error when TOML table redefines an existing scalar, got nil")
		}
		if !strings.Contains(err.Error(), "already defined") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// MIN-16: Semicolon in TOML URIs/DSNs not stripped as comment
	t.Run("MIN-16_TOMLDSNWithSemicolon", func(t *testing.T) {
		type Cfg struct {
			DSN    string `confi:"dsn"`
			RawDSN string `confi:"raw_dsn"`
		}
		tomlData := "dsn = \"postgres://user:pass@localhost:5432/db;sslmode=disable\"\nraw_dsn = postgres://user:pass@localhost:5432/db;sslmode=disable\n"
		var cfg Cfg
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if cfg.DSN != "postgres://user:pass@localhost:5432/db;sslmode=disable" {
			t.Fatalf("semicolon was stripped from quoted DSN: got %q", cfg.DSN)
		}
		if cfg.RawDSN != "postgres://user:pass@localhost:5432/db;sslmode=disable" {
			t.Fatalf("semicolon was stripped from unquoted DSN: got %q", cfg.RawDSN)
		}
	})

	// MAJ-12: Top-level YAML sequence returns error
	t.Run("MAJ-12_YAMLRootSequenceError", func(t *testing.T) {
		type Cfg struct {
			Items []string `confi:"items"`
		}
		yamlData := "- first\n- second\n- third\n"
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader(yamlData), FormatYAML), WithArgs(nil))
		if err == nil {
			t.Fatal("expected error when YAML root document is a sequence, got nil")
		}
		if !strings.Contains(err.Error(), "root document must be a mapping") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// MAJ-13: Multi-document YAML stream rejected
	t.Run("MAJ-13_YAMLMultiDocumentStream", func(t *testing.T) {
		type Cfg struct {
			Env string `confi:"env"`
		}
		yamlData := "env: production\n---\nenv: development\n"
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader(yamlData), FormatYAML), WithArgs(nil))
		if err == nil {
			t.Fatal("expected error for multi-document YAML stream, got nil")
		}
		if !strings.Contains(err.Error(), "multi-document") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// MAJ-14: Unbounded recursion depth in YAML parser guarded
	t.Run("MAJ-14_YAMLRecursionLimit", func(t *testing.T) {
		type Cfg struct {
			Data interface{} `confi:"data"`
		}
		deepFlow := strings.Repeat("[", 250) + "1" + strings.Repeat("]", 250)
		yamlData := "data: " + deepFlow + "\n"
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader(yamlData), FormatYAML), WithArgs(nil))
		if err == nil {
			t.Fatal("expected error on deeply nested YAML structure exceeding depth limit, got nil")
		}
		if !strings.Contains(err.Error(), "depth") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// MIN-14: Unicode and control escape sequences in YAML and TOML
	t.Run("MIN-14_UnicodeAndControlEscapes", func(t *testing.T) {
		type Cfg struct {
			Unicode string `confi:"unicode"`
			Ctrl    string `confi:"ctrl"`
			Emoji   string `confi:"emoji"`
		}
		yamlData := "unicode: \"\\u0041\\u0042\\u0043\"\nctrl: \"\\b\\f\"\nemoji: \"\\U0001F600\"\n"
		var cfg Cfg
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(yamlData), FormatYAML), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if cfg.Unicode != "ABC" {
			t.Fatalf("expected 'ABC', got %q", cfg.Unicode)
		}
		if cfg.Ctrl != "\b\f" {
			t.Fatalf("expected '\\b\\f', got %q", cfg.Ctrl)
		}
		if cfg.Emoji != "😀" {
			t.Fatalf("expected '😀', got %q", cfg.Emoji)
		}

		tomlData := "unicode = \"\\u0041\\u0042\\u0043\"\nctrl = \"\\b\\f\"\nemoji = \"\\U0001F600\"\n"
		var tomlCfg Cfg
		if err := Parse(&tomlCfg, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if tomlCfg.Unicode != "ABC" || tomlCfg.Ctrl != "\b\f" || tomlCfg.Emoji != "😀" {
			t.Fatalf("unexpected TOML escapes: %+v", tomlCfg)
		}
	})

	// MIN-15: Flawed isQuotedYAML detection
	t.Run("MIN-15_IsQuotedYAML", func(t *testing.T) {
		if isQuotedYAML(`"foo" and "bar"`) {
			t.Fatal(`expected false for '"foo" and "bar"'`)
		}
		if isQuotedYAML(`'foo' and 'bar'`) {
			t.Fatal(`expected false for "'foo' and 'bar'"`)
		}
		if !isQuotedYAML(`"foo \"bar\""`) {
			t.Fatal(`expected true for valid escaped double-quoted string`)
		}
		if !isQuotedYAML(`'foo ''bar'''`) {
			t.Fatal(`expected true for valid escaped single-quoted string`)
		}
	})

	// MIN-17: Trailing garbage rejected in .env
	t.Run("MIN-17_DotEnvTrailingGarbage", func(t *testing.T) {
		type Cfg struct {
			Val string `confi:"val"`
		}
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader("VAL='test'trailing\n"), FormatEnv), WithArgs(nil))
		if err == nil {
			t.Fatal("expected error on trailing characters after single-quoted .env value, got nil")
		}
		err2 := Parse(&cfg, WithConfigReader(strings.NewReader("VAL=\"test\"trailing\n"), FormatEnv), WithArgs(nil))
		if err2 == nil {
			t.Fatal("expected error on trailing characters after double-quoted .env value, got nil")
		}
	})

	// MIN-18: Double unescaping defect \\$ in .env
	t.Run("MIN-18_DotEnvEscapedBackslashDollar", func(t *testing.T) {
		type Cfg struct {
			LiteralDollar string `confi:"literal_dollar"`
			EscapedSlash  string `confi:"escaped_slash"`
		}
		dotEnvData := "LITERAL_DOLLAR=\"\\$foo\"\nESCAPED_SLASH=\"\\\\$foo\"\n"
		var cfg Cfg
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(dotEnvData), FormatEnv), WithArgs(nil)); err != nil {
			t.Fatalf("failed to parse .env: %v", err)
		}
		if cfg.LiteralDollar != "$foo" {
			t.Fatalf("expected '$foo', got %q", cfg.LiteralDollar)
		}
		if cfg.EscapedSlash != "\\$foo" {
			t.Fatalf("expected '\\$foo', got %q", cfg.EscapedSlash)
		}
	})

	// ADD-DEF-01: Case-insensitive export prefix
	t.Run("ADD-DEF-01_DotEnvExportCaseInsensitivity", func(t *testing.T) {
		type Cfg struct {
			Host string `confi:"host"`
			Port int    `confi:"port"`
		}
		var cfg Cfg
		envData := "EXPORT HOST=127.0.0.1\nexport PORT=8080\n"
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(envData), FormatEnv), WithArgs(nil)); err != nil {
			t.Fatalf("failed to parse EXPORT prefix: %v", err)
		}
		if cfg.Host != "127.0.0.1" || cfg.Port != 8080 {
			t.Fatalf("expected 127.0.0.1:8080, got %s:%d", cfg.Host, cfg.Port)
		}
	})

	// MAJ-15: Auto-detection of TOML with top-level keys
	t.Run("MAJ-15_DetectFormatTopLevelTOML", func(t *testing.T) {
		type ServerCfg struct {
			Host string `confi:"host"`
			Port int    `confi:"port"`
		}
		type Cfg struct {
			Title  string    `confi:"title"`
			Server ServerCfg `confi:"server"`
		}
		tomlData := "title = \"My Service\"\n[server]\nhost = \"127.0.0.1\"\nport = 8080\n"
		var cfg Cfg
		err := Parse(&cfg, WithConfigReader(strings.NewReader(tomlData), FormatAuto), WithArgs(nil))
		if err != nil {
			t.Fatalf("failed to auto-detect and parse TOML with top-level key: %v", err)
		}
		if cfg.Title != "My Service" || cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 8080 {
			t.Fatalf("unexpected parsed result: %+v", cfg)
		}
	})

	// MAJ-16 & MIN-12: Deterministic key lookup in findKeyInMap
	t.Run("MAJ-16_MIN-12_DeterministicMapKeyBinding", func(t *testing.T) {
		type Cfg struct {
			UserID string `confi:"user_id"`
		}
		m := map[string]interface{}{
			"userId": "camelCaseVal",
			"userid": "lowerVal",
		}
		var firstVal string
		for i := 0; i < 50; i++ {
			var cfg Cfg
			p := NewParser("test")
			schema, rootVal, err := extractSchema(&cfg)
			if err != nil {
				t.Fatalf("extractSchema failed: %v", err)
			}
			err = p.applyStructuredMap(m, schema, rootVal, make(map[*fieldMeta]valueSource))
			if err != nil {
				t.Fatalf("applyStructuredMap failed: %v", err)
			}
			if i == 0 {
				firstVal = cfg.UserID
			} else if cfg.UserID != firstVal {
				t.Fatalf("non-deterministic binding on iteration %d: got %q, expected %q", i, cfg.UserID, firstVal)
			}
		}
	})

	// MIN-13: Slice allocation in assignInterfaceValue
	t.Run("MIN-13_SliceAssignmentCorrectness", func(t *testing.T) {
		type Cfg struct {
			Endpoints []string `confi:"endpoints"`
		}
		yamlData := "endpoints:\n  - ep1\n  - ep2\n  - ep3\n"
		var cfg Cfg
		if err := Parse(&cfg, WithConfigReader(strings.NewReader(yamlData), FormatYAML), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if len(cfg.Endpoints) != 3 || cfg.Endpoints[0] != "ep1" || cfg.Endpoints[1] != "ep2" || cfg.Endpoints[2] != "ep3" {
			t.Fatalf("unexpected endpoints: %+v", cfg.Endpoints)
		}
	})
}

func TestCoreEngineBugFixes(t *testing.T) {
	// MIN-02: Support dynamic config file loading on subcommands
	t.Run("MIN-02_SubcommandConfigFileLoading", func(t *testing.T) {
		type WorkerCmd struct {
			ConfigFile  string `confi:"config,config"`
			Concurrency int    `confi:"concurrency" default:"1"`
			Queue       string `confi:"queue" default:"default"`
		}
		type AppCfg struct {
			GlobalHost string    `confi:"host" default:"localhost"`
			Worker     WorkerCmd `confi:"worker,subcommand"`
		}

		tmpDir, err := ioutil.TempDir("", "confi_subcmd_cfg_")
		if err != nil {
			t.Fatalf("TempDir failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		workerYaml := filepath.Join(tmpDir, "worker.yaml")
		if err := ioutil.WriteFile(workerYaml, []byte("concurrency: 8\nqueue: critical\n"), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Test 1: passing --config to subcommand
		var app1 AppCfg
		err = Parse(&app1, WithStrict(true), WithArgs([]string{"worker", "--config", workerYaml}))
		if err != nil {
			t.Fatalf("Parse failed for subcommand with --config: %v", err)
		}
		if app1.Worker.Concurrency != 8 || app1.Worker.Queue != "critical" {
			t.Fatalf("expected concurrency 8 and queue critical, got: %+v", app1.Worker)
		}

		// Test 2: subcommand struct with default config file and root config isolation
		type ServerCmd struct {
			ConfigFile string `confi:"config,config"`
			Port       int    `confi:"port"`
		}
		type AppCfg2 struct {
			RootKey string    `confi:"root_key"`
			Server  ServerCmd `confi:"server,subcommand"`
		}
		serverJson := filepath.Join(tmpDir, "server.json")
		if err := ioutil.WriteFile(serverJson, []byte(`{"port": 9090}`), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		rootValidYaml := filepath.Join(tmpDir, "root_valid.yaml")
		if err := ioutil.WriteFile(rootValidYaml, []byte("root_key: root_val\n"), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		var app2 AppCfg2
		err = Parse(&app2, WithStrict(true), WithConfigFile(rootValidYaml), WithArgs([]string{"server", "--config", serverJson}))
		if err != nil {
			t.Fatalf("Parse failed with root config and subcommand config: %v", err)
		}
		if app2.RootKey != "root_val" {
			t.Errorf("expected RootKey 'root_val', got %q", app2.RootKey)
		}
		if app2.Server.Port != 9090 {
			t.Errorf("expected Server.Port 9090, got %d", app2.Server.Port)
		}
	})

	// MIN-03: Asymmetric empty prefix handling (WithPrefix(""))
	t.Run("MIN-03_EmptyPrefixHandling", func(t *testing.T) {
		type Cfg struct {
			Target string `confi:"target,pos"`
		}
		var cfg Cfg
		// WithPrefix(""): "--custom-pos" is not recognized as a long flag, so it becomes a positional argument
		err := Parse(&cfg, WithPrefix(""), WithArgs([]string{"--custom-pos"}))
		if err != nil {
			t.Fatalf("Parse with empty prefix failed: %v", err)
		}
		if cfg.Target != "--custom-pos" {
			t.Fatalf("expected Target '--custom-pos', got %q", cfg.Target)
		}

		// Custom prefix e.g. "/"
		type SlashCfg struct {
			Port int `confi:"port"`
		}
		var slashCfg SlashCfg
		err = Parse(&slashCfg, WithPrefix("/"), WithArgs([]string{"/port=8080"}))
		if err != nil {
			t.Fatalf("Parse with slash prefix failed: %v", err)
		}
		if slashCfg.Port != 8080 {
			t.Fatalf("expected Port 8080, got %d", slashCfg.Port)
		}
	})

	// INFO-01: WithAutoEnv boolean toggle
	t.Run("INFO-01_WithAutoEnvToggle", func(t *testing.T) {
		type Cfg struct {
			DbUser string
		}
		lookup := func(k string) (string, bool) {
			if k == "DB_USER" {
				return "admin", true
			}
			return "", false
		}

		// 1. WithAutoEnv() defaults to true
		var cfg1 Cfg
		if err := Parse(&cfg1, WithAutoEnv(), WithEnvLookup(lookup), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if cfg1.DbUser != "admin" {
			t.Fatalf("expected DbUser 'admin' with WithAutoEnv(), got %q", cfg1.DbUser)
		}

		// 2. WithAutoEnv(true) explicitly enables
		var cfg2 Cfg
		if err := Parse(&cfg2, WithAutoEnv(true), WithEnvLookup(lookup), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if cfg2.DbUser != "admin" {
			t.Fatalf("expected DbUser 'admin' with WithAutoEnv(true), got %q", cfg2.DbUser)
		}

		// 3. WithAutoEnv(false) explicitly disables
		var cfg3 Cfg
		if err := Parse(&cfg3, WithAutoEnv(false), WithEnvLookup(lookup), WithArgs(nil)); err != nil {
			t.Fatal(err)
		}
		if cfg3.DbUser != "" {
			t.Fatalf("expected DbUser empty with WithAutoEnv(false), got %q", cfg3.DbUser)
		}
	})

	// MIN-05: splitTagParts with double quotes and escapes
	t.Run("MIN-05_DoubleQuotedTagSplitting", func(t *testing.T) {
		type Cfg struct {
			Color   string `confi:"usage=\"Select: red, green, or blue\",default=\"red,green\""`
			Escaped string `confi:"usage=\"Press \\\"Enter\\\" to proceed\",default=\"a,b\""`
		}
		var cfg Cfg
		err := Parse(&cfg, WithArgs(nil))
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if cfg.Color != "red,green" {
			t.Errorf("expected Color default 'red,green', got %q", cfg.Color)
		}
		if cfg.Escaped != "a,b" {
			t.Errorf("expected Escaped default 'a,b', got %q", cfg.Escaped)
		}

		// Verify usage string
		usageStr, err := Usage(&cfg, WithName("testapp"))
		if err != nil {
			t.Fatalf("Usage failed: %v", err)
		}
		if !strings.Contains(usageStr, "Select: red, green, or blue") {
			t.Errorf("expected usage to contain 'Select: red, green, or blue', got:\n%s", usageStr)
		}
		if !strings.Contains(usageStr, `Press "Enter" to proceed`) {
			t.Errorf("expected usage to contain 'Press \"Enter\" to proceed', got:\n%s", usageStr)
		}
	})

	// MIN-06: Conflicting flag aliases across distinct fields
	t.Run("MIN-06_DuplicateLongFlagError", func(t *testing.T) {
		type DupCfg struct {
			Addr1 string `confi:"addr"`
			Addr2 string `confi:"addr"`
		}
		var cfg DupCfg
		err := Parse(&cfg, WithArgs(nil))
		if err == nil {
			t.Fatal("expected error on duplicate long flag alias, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate flag \"addr\"") {
			t.Fatalf("expected duplicate flag error message, got: %v", err)
		}
	})

	// MIN-07 (schema preparation): minStr and maxStr stored on fieldMeta
	t.Run("MIN-07_FieldMetaMinMaxStrings", func(t *testing.T) {
		type BoundsCfg struct {
			BigUint uint64 `confi:"min=9007199254740993,max=18446744073709551615"`
			BigInt  int64  `confi:"min=-9223372036854775807,max=9223372036854775806"`
		}
		schema, _, err := extractSchema(&BoundsCfg{})
		if err != nil {
			t.Fatalf("extractSchema failed: %v", err)
		}
		if len(schema.fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(schema.fields))
		}
		f0 := schema.fields[0]
		if f0.minStr != "9007199254740993" || f0.maxStr != "18446744073709551615" {
			t.Errorf("f0 minStr=%q, maxStr=%q", f0.minStr, f0.maxStr)
		}
		f1 := schema.fields[1]
		if f1.minStr != "-9223372036854775807" || f1.maxStr != "9223372036854775806" {
			t.Errorf("f1 minStr=%q, maxStr=%q", f1.minStr, f1.maxStr)
		}
	})

	// MIN-10: Inefficient struct isZeroValue via DeepEqual
	t.Run("MIN-10_ZeroAllocStructIsZeroValue", func(t *testing.T) {
		type Inner struct {
			A int
			B string
		}
		type Outer struct {
			X Inner
			Y float64
		}
		zeroVal := Outer{}
		nonZeroVal := Outer{X: Inner{A: 10}}

		// Verify correctness
		if !isZeroValue(reflect.ValueOf(zeroVal)) {
			t.Error("expected zeroVal to be identified as zero")
		}
		if isZeroValue(reflect.ValueOf(nonZeroVal)) {
			t.Error("expected nonZeroVal to NOT be identified as zero")
		}

		// Verify zero heap allocations
		rv := reflect.ValueOf(zeroVal)
		allocs := testing.AllocsPerRun(100, func() {
			_ = isZeroValue(rv)
		})
		if allocs > 0 {
			t.Errorf("expected 0 allocs for isZeroValue on struct, got %f", allocs)
		}
	})

	// MIN-11: Defensive isBoolValue / assignFromStrings handling
	t.Run("MIN-11_IsBoolValueDefensiveHandling", func(t *testing.T) {
		var b bool
		fvBool := reflect.ValueOf(&b).Elem()
		if !isBoolValue(fvBool) {
			t.Error("expected isBoolValue(bool) to be true")
		}
		// Passing empty raws to assignFromStrings on a boolean sets it to true
		if err := assignFromStrings(fvBool, nil, "", false); err != nil {
			t.Fatalf("assignFromStrings on bool with nil raws failed: %v", err)
		}
		if !b {
			t.Error("expected b to be set to true")
		}

		// Passing empty raws on a non-bool type returns missing value error
		var num int
		fvInt := reflect.ValueOf(&num).Elem()
		if isBoolValue(fvInt) {
			t.Error("expected isBoolValue(int) to be false")
		}
		err := assignFromStrings(fvInt, nil, "", false)
		if err == nil || !strings.Contains(err.Error(), "missing value for int") {
			t.Fatalf("expected 'missing value for int' error, got: %v", err)
		}

		// Invalid reflect value
		if isBoolValue(reflect.Value{}) {
			t.Error("expected isBoolValue(invalid) to be false")
		}
	})
}

type validatedTestPort int

func (p validatedTestPort) Validate() error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("invalid port %d", p)
	}
	return nil
}

type validatedTestSubItem struct {
	Val string `confi:"val"`
}

func (s *validatedTestSubItem) Validate() error {
	if s.Val == "forbidden" {
		return errors.New("forbidden item value")
	}
	return nil
}

func TestValidationAndWatcherBugFixes(t *testing.T) {
	// MIN-07: uint64 and int64 precision around 2^53 and boundary conditions
	t.Run("MIN-07_Uint64PrecisionAndLargeBounds", func(t *testing.T) {
		type BigUintCfg struct {
			Val uint64 `confi:"val,min=9007199254740993,max=9007199254740995"`
		}

		// Exact min boundary: 9007199254740993 passes
		var c1 BigUintCfg
		if err := Parse(&c1, WithArgs([]string{"--val=9007199254740993"})); err != nil {
			t.Fatalf("expected min boundary 9007199254740993 to pass, got: %v", err)
		}
		if c1.Val != 9007199254740993 {
			t.Fatalf("expected Val=9007199254740993, got %d", c1.Val)
		}

		// One below min: 9007199254740992 fails
		var c2 BigUintCfg
		err := Parse(&c2, WithArgs([]string{"--val=9007199254740992"}))
		if err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected 'less than minimum' error for 9007199254740992, got: %v", err)
		}

		// Exact max boundary: 9007199254740995 passes
		var c3 BigUintCfg
		if err := Parse(&c3, WithArgs([]string{"--val=9007199254740995"})); err != nil {
			t.Fatalf("expected max boundary 9007199254740995 to pass, got: %v", err)
		}

		// One above max: 9007199254740996 fails
		var c4 BigUintCfg
		err = Parse(&c4, WithArgs([]string{"--val=9007199254740996"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
			t.Fatalf("expected 'exceeds maximum' error for 9007199254740996, got: %v", err)
		}

		// math.MaxUint64 (18446744073709551615) supported without float overflow
		type MaxUint64Cfg struct {
			Val uint64 `confi:"val,min=18446744073709551610,max=18446744073709551615"`
		}
		var cMax MaxUint64Cfg
		if err := Parse(&cMax, WithArgs([]string{"--val=18446744073709551615"})); err != nil {
			t.Fatalf("expected math.MaxUint64 to pass, got: %v", err)
		}
		if cMax.Val != 18446744073709551615 {
			t.Fatalf("expected Val=18446744073709551615, got %d", cMax.Val)
		}

		var cMaxFail MaxUint64Cfg
		err = Parse(&cMaxFail, WithArgs([]string{"--val=18446744073709551609"}))
		if err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected less than minimum error for 18446744073709551609, got: %v", err)
		}

		// Big int64 (> 2^53)
		type BigIntCfg struct {
			Val int64 `confi:"val,min=-9007199254740995,max=9007199254740995"`
		}
		var ci1 BigIntCfg
		if err := Parse(&ci1, WithArgs([]string{"--val=-9007199254740995"})); err != nil {
			t.Fatalf("expected min int64 boundary to pass, got: %v", err)
		}
		var ci2 BigIntCfg
		err = Parse(&ci2, WithArgs([]string{"--val=-9007199254740996"}))
		if err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected less than minimum error for -9007199254740996, got: %v", err)
		}
		var ci3 BigIntCfg
		err = Parse(&ci3, WithArgs([]string{"--val=9007199254740996"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
			t.Fatalf("expected exceeds maximum error for 9007199254740996, got: %v", err)
		}

		// time.Duration and *time.Duration min/max comparisons preserved
		type DurCfg struct {
			Timeout    time.Duration  `confi:"timeout,min=100ms,max=5s"`
			PtrTimeout *time.Duration `confi:"ptr_timeout,min=50ms,max=2s"`
		}
		var durValid DurCfg
		if err := Parse(&durValid, WithArgs([]string{"--timeout=500ms", "--ptr_timeout=500ms"})); err != nil {
			t.Fatalf("expected valid durations to pass: %v", err)
		}
		var durMin DurCfg
		err = Parse(&durMin, WithArgs([]string{"--timeout=20ms", "--ptr_timeout=500ms"}))
		if err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected min duration error, got: %v", err)
		}
		var durMax DurCfg
		err = Parse(&durMax, WithArgs([]string{"--timeout=10s", "--ptr_timeout=500ms"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
			t.Fatalf("expected max duration error, got: %v", err)
		}
		var ptrMin DurCfg
		err = Parse(&ptrMin, WithArgs([]string{"--timeout=500ms", "--ptr_timeout=10ms"}))
		if err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected min *time.Duration error, got: %v", err)
		}
		var ptrMax DurCfg
		err = Parse(&ptrMax, WithArgs([]string{"--timeout=500ms", "--ptr_timeout=3s"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
			t.Fatalf("expected max *time.Duration error, got: %v", err)
		}
	})

	// MIN-21: oneOf validation heap optimization & primitive types
	t.Run("MIN-21_OneOfOptimizationAndPrimitives", func(t *testing.T) {
		type ModeCfg struct {
			Mode  string   `confi:"mode,oneof=read|write|admin,default=read"`
			Modes []string `confi:"modes,oneof=json|yaml|xml"`
			Level int      `confi:"level,oneof=1|2|3,default=1"`
		}

		var validCfg ModeCfg
		if err := Parse(&validCfg, WithArgs([]string{"--mode=write", "--modes=json,yaml", "--level=2"})); err != nil {
			t.Fatalf("expected valid oneof to pass, got: %v", err)
		}
		if validCfg.Mode != "write" || len(validCfg.Modes) != 2 || validCfg.Level != 2 {
			t.Fatalf("unexpected ModeCfg values: %+v", validCfg)
		}

		var invalidMode ModeCfg
		err := Parse(&invalidMode, WithArgs([]string{"--mode=execute"}))
		if err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected oneof error for invalid mode, got: %v", err)
		}

		var invalidSliceElem ModeCfg
		err = Parse(&invalidSliceElem, WithArgs([]string{"--modes=json,binary"}))
		if err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected oneof error for invalid slice element, got: %v", err)
		}

		var invalidInt ModeCfg
		err = Parse(&invalidInt, WithArgs([]string{"--level=5"}))
		if err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected oneof error for invalid int level, got: %v", err)
		}

		// Zero heap allocation check for string oneOf fast-path
		type AllocCheckCfg struct {
			Role string `confi:"role,oneof=user|admin|guest"`
		}
		var allocCfg AllocCheckCfg
		allocCfg.Role = "admin"
		schema, rootVal, err := extractSchema(&allocCfg)
		if err != nil {
			t.Fatalf("extractSchema failed: %v", err)
		}
		fv, ok := schema.fields[0].fieldValue(rootVal, false)
		if !ok {
			t.Fatal("fieldValue failed")
		}
		allocs := testing.AllocsPerRun(100, func() {
			_ = validateFieldConstraints(schema.fields[0], fv)
		})
		if allocs > 0 {
			t.Errorf("expected 0 allocs for string oneOf validation, got %f", allocs)
		}
	})

	// MIN-22: Collection & scalar Custom Validators traversal
	t.Run("MIN-22_CollectionAndScalarCustomValidators", func(t *testing.T) {
		// 1. Slice of scalar custom type implementing Validator
		type SliceCustomCfg struct {
			Ports []validatedTestPort `confi:"ports"`
		}
		var validPorts SliceCustomCfg
		if err := Parse(&validPorts, WithArgs([]string{"--ports=80,443"})); err != nil {
			t.Fatalf("expected valid ports to pass, got: %v", err)
		}
		if len(validPorts.Ports) != 2 || validPorts.Ports[0] != 80 || validPorts.Ports[1] != 443 {
			t.Fatalf("unexpected ports: %+v", validPorts.Ports)
		}

		var invalidPorts SliceCustomCfg
		err := Parse(&invalidPorts, WithArgs([]string{"--ports=80,99999"}))
		if err == nil || !strings.Contains(err.Error(), "invalid port 99999") {
			t.Fatalf("expected custom validator error for port 99999, got: %v", err)
		}

		// 2. Slice of pointer structs implementing Validator
		type SliceStructCfg struct {
			Items []*validatedTestSubItem `confi:"items"`
		}
		var validItems SliceStructCfg
		validItems.Items = []*validatedTestSubItem{{Val: "ok1"}, {Val: "ok2"}}
		if err := invokeCustomValidators(reflect.ValueOf(&validItems)); err != nil {
			t.Fatalf("expected valid items to pass custom validator: %v", err)
		}

		var invalidItems SliceStructCfg
		invalidItems.Items = []*validatedTestSubItem{{Val: "ok1"}, {Val: "forbidden"}}
		err = invokeCustomValidators(reflect.ValueOf(&invalidItems))
		if err == nil || !strings.Contains(err.Error(), "forbidden item value") {
			t.Fatalf("expected forbidden item value error, got: %v", err)
		}

		// 3. Array of custom type implementing Validator
		type ArrayCustomCfg struct {
			Ports [2]validatedTestPort `confi:"ports"`
		}
		var validArray ArrayCustomCfg
		validArray.Ports = [2]validatedTestPort{80, 443}
		if err := invokeCustomValidators(reflect.ValueOf(&validArray)); err != nil {
			t.Fatalf("expected valid array to pass: %v", err)
		}
		var invalidArray ArrayCustomCfg
		invalidArray.Ports = [2]validatedTestPort{80, 99999}
		err = invokeCustomValidators(reflect.ValueOf(&invalidArray))
		if err == nil || !strings.Contains(err.Error(), "invalid port 99999") {
			t.Fatalf("expected custom validator error on array element, got: %v", err)
		}

		// 4. Map of structs implementing Validator
		type MapValCfg struct {
			Nodes map[string]validatedTestSubItem `confi:"nodes"`
		}
		var validMap MapValCfg
		validMap.Nodes = map[string]validatedTestSubItem{"node1": {Val: "healthy"}}
		if err := invokeCustomValidators(reflect.ValueOf(&validMap)); err != nil {
			t.Fatalf("expected valid map values to pass: %v", err)
		}
		var invalidMap MapValCfg
		invalidMap.Nodes = map[string]validatedTestSubItem{"node1": {Val: "forbidden"}}
		err = invokeCustomValidators(reflect.ValueOf(&invalidMap))
		if err == nil || !strings.Contains(err.Error(), "forbidden item value") {
			t.Fatalf("expected custom validator error on map value, got: %v", err)
		}

		// 5. Cyclic pointer structures guard
		type CyclicNode struct {
			Next *CyclicNode
			Val  int
		}
		node1 := &CyclicNode{Val: 1}
		node2 := &CyclicNode{Val: 2, Next: node1}
		node1.Next = node2
		if err := invokeCustomValidators(reflect.ValueOf(node1)); err != nil {
			t.Fatalf("expected cyclic node validation to terminate cleanly, got: %v", err)
		}
	})

	// MIN-27: Watcher without configuration files returns descriptive error
	t.Run("MIN-27_WatchWithNoConfigFilesReturnsError", func(t *testing.T) {
		type NoConfigCfg struct {
			Host string `confi:"host"`
			Port int    `confi:"port"`
		}
		var noCfg NoConfigCfg

		// 1. Package Watch() without config files returns error
		w, err := Watch(&noCfg, func(err error) {}, WithArgs(nil))
		if err == nil {
			if w != nil {
				w.Stop()
			}
			t.Fatal("expected error when no config files are specified for Watch, got nil")
		}
		if w != nil {
			w.Stop()
			t.Fatal("expected Watcher to be nil on error")
		}
		if err.Error() != "confi: no configuration files found to watch" {
			t.Fatalf("unexpected error message: got %q, expected 'confi: no configuration files found to watch'", err.Error())
		}

		// 2. Parser.Watch() without config files returns error
		p := NewParser("testapp", WithArgs(nil))
		w2, err2 := p.Watch(&noCfg, func(err error) {})
		if err2 == nil {
			if w2 != nil {
				w2.Stop()
			}
			t.Fatal("expected error from p.Watch when no config files specified, got nil")
		}
		if w2 != nil {
			w2.Stop()
			t.Fatal("expected Watcher to be nil on error")
		}
		if err2.Error() != "confi: no configuration files found to watch" {
			t.Fatalf("unexpected error message from p.Watch: got %q", err2.Error())
		}
	})
}

func TestCLIPresentationBugFixes(t *testing.T) {
	// 1. MIN-19: SubcommandUsage and WriteSubcommandUsage
	t.Run("MIN-19_SubcommandUsage", func(t *testing.T) {
		type MigrateCmd struct {
			DryRun bool `confi:"dry-run,usage='Run migration in dry-run mode'"`
		}
		type RootApp struct {
			Verbose bool       `confi:"verbose,usage='Enable verbose logging'"`
			Migrate MigrateCmd `confi:"migrate,subcommand,usage='Execute database migrations'"`
		}

		app := RootApp{}

		// Test public SubcommandUsage
		usageStr, err := SubcommandUsage(&app, "migrate", WithName("myapp"))
		if err != nil {
			t.Fatalf("SubcommandUsage failed: %v", err)
		}
		if !strings.Contains(usageStr, "Usage: myapp migrate [flags]") {
			t.Errorf("expected usage header 'Usage: myapp migrate [flags]', got:\n%s", usageStr)
		}
		if !strings.Contains(usageStr, "--dry-run") {
			t.Errorf("expected '--dry-run' in subcommand usage, got:\n%s", usageStr)
		}
		if !strings.Contains(usageStr, "Run migration in dry-run mode") {
			t.Errorf("expected usage description in subcommand usage, got:\n%s", usageStr)
		}
		if strings.Contains(usageStr, "--verbose") {
			t.Errorf("subcommand usage should not contain root flag --verbose, got:\n%s", usageStr)
		}

		// Test WriteSubcommandUsage
		var buf bytes.Buffer
		if err := WriteSubcommandUsage(&buf, &app, "migrate", WithName("myapp")); err != nil {
			t.Fatalf("WriteSubcommandUsage failed: %v", err)
		}
		if buf.String() != usageStr {
			t.Errorf("WriteSubcommandUsage output mismatch: got %q, expected %q", buf.String(), usageStr)
		}

		// Test Parser methods
		p := NewParser("customapp")
		pUsage, err := p.SubcommandUsage(&app, "migrate")
		if err != nil {
			t.Fatalf("p.SubcommandUsage failed: %v", err)
		}
		if !strings.Contains(pUsage, "Usage: customapp migrate [flags]") {
			t.Errorf("expected custom parser app name, got:\n%s", pUsage)
		}

		var pBuf bytes.Buffer
		if err := p.WriteSubcommandUsage(&pBuf, &app, "migrate"); err != nil {
			t.Fatalf("p.WriteSubcommandUsage failed: %v", err)
		}
		if pBuf.String() != pUsage {
			t.Errorf("p.WriteSubcommandUsage output mismatch")
		}

		// Test case-insensitivity and trim
		ciUsage, err := SubcommandUsage(&app, "  MIGRATE  ", WithName("myapp"))
		if err != nil {
			t.Fatalf("case-insensitive SubcommandUsage failed: %v", err)
		}
		if !strings.Contains(ciUsage, "--dry-run") {
			t.Errorf("expected --dry-run in case-insensitive usage, got:\n%s", ciUsage)
		}

		// Test unknown subcommand returns descriptive ParseError
		_, err = SubcommandUsage(&app, "unknown")
		if err == nil {
			t.Fatal("expected error for unknown subcommand, got nil")
		}
		parseErr, ok := err.(*ParseError)
		if !ok {
			t.Fatalf("expected *ParseError for unknown subcommand, got %T: %v", err, err)
		}
		if !strings.Contains(parseErr.Error(), `unknown subcommand "unknown"`) {
			t.Fatalf("unexpected error message: %v", parseErr)
		}

		// Test nil target
		if _, err := SubcommandUsage(nil, "migrate"); err == nil {
			t.Fatal("expected error for nil target")
		}
	})

	// 2. MIN-20: Usage output with default empty string
	t.Run("MIN-20_EmptyDefaultUsage", func(t *testing.T) {
		type HelpDefCfg struct {
			Prefix string `confi:"prefix,default='',usage='Path prefix'"`
			Port   int    `confi:"port,default=8080,usage='Server port'"`
			Host   string `confi:"host,usage='Server host'"`
		}

		var cfg HelpDefCfg
		usageStr, err := Usage(&cfg)
		if err != nil {
			t.Fatalf("Usage failed: %v", err)
		}

		// Check prefix has [default: ""]
		if !strings.Contains(usageStr, `[default: ""]`) {
			t.Errorf("expected '[default: \"\"]' for empty default string, got:\n%s", usageStr)
		}
		// Check port has [default: 8080]
		if !strings.Contains(usageStr, "[default: 8080]") {
			t.Errorf("expected '[default: 8080]' for port, got:\n%s", usageStr)
		}
		// Check host does not have default tag
		for _, line := range strings.Split(usageStr, "\n") {
			if strings.Contains(line, "--host") && strings.Contains(line, "[default:") {
				t.Errorf("field without default should not have [default: ...], got line: %s", line)
			}
		}
	})

	// 3. MIN-24: Context-sensitive PowerShell completion
	t.Run("MIN-24_PowerShellContextSensitivity", func(t *testing.T) {
		type ServeCmd struct {
			Port int `confi:"port,short=p,usage='HTTP Port'"`
		}
		type MigrateCmd struct {
			DryRun bool `confi:"dry-run,usage='Dry run migration'"`
		}
		type RootApp struct {
			Verbose bool        `confi:"verbose,usage='Enable verbose output'"`
			Serve   *ServeCmd   `confi:"serve,subcommand,usage='Start server'"`
			Migrate *MigrateCmd `confi:"migrate,subcommand,usage='Run migration'"`
		}

		var app RootApp
		psScript, err := Completion("powershell", &app, WithName("mycli"))
		if err != nil {
			t.Fatalf("PowerShell completion failed: %v", err)
		}

		// Verify AST elements parsing
		if !strings.Contains(psScript, "$commandAst.Elements") {
			t.Errorf("expected AST elements inspection in PowerShell completion, got:\n%s", psScript)
		}

		// Split PowerShell script by candidates switch block to inspect context-specific candidates
		switchSubcmd := strings.Index(psScript, "switch ($subcmd)")
		if switchSubcmd == -1 {
			t.Fatalf("expected 'switch ($subcmd)' in script:\n%s", psScript)
		}
		subcmdBody := psScript[switchSubcmd:]
		serveIndex := strings.Index(subcmdBody, "'serve' {")
		migrateIndex := strings.Index(subcmdBody, "'migrate' {")
		defaultIndex := strings.Index(subcmdBody, "default {")

		if serveIndex == -1 || migrateIndex == -1 || defaultIndex == -1 {
			t.Fatalf("expected switch branches for 'serve', 'migrate', and default in script:\n%s", psScript)
		}

		serveSection := subcmdBody[serveIndex:migrateIndex]
		migrateSection := subcmdBody[migrateIndex:defaultIndex]
		defaultSection := subcmdBody[defaultIndex:]

		// Serve candidates must contain --port, -p, --verbose, but NOT --dry-run
		if !strings.Contains(serveSection, "'--port'") || !strings.Contains(serveSection, "'-p'") {
			t.Errorf("serve candidates missing serve flags: %s", serveSection)
		}
		if !strings.Contains(serveSection, "'--verbose'") {
			t.Errorf("serve candidates missing global flag --verbose: %s", serveSection)
		}
		if strings.Contains(serveSection, "'--dry-run'") {
			t.Errorf("serve candidates should NOT contain flags from migrate (--dry-run): %s", serveSection)
		}

		// Migrate candidates must contain --dry-run, --verbose, but NOT --port or -p
		if !strings.Contains(migrateSection, "'--dry-run'") {
			t.Errorf("migrate candidates missing migrate flags: %s", migrateSection)
		}
		if !strings.Contains(migrateSection, "'--verbose'") {
			t.Errorf("migrate candidates missing global flag --verbose: %s", migrateSection)
		}
		if strings.Contains(migrateSection, "'--port'") || strings.Contains(migrateSection, "'-p'") {
			t.Errorf("migrate candidates should NOT contain flags from serve (--port): %s", migrateSection)
		}

		// Default candidates must contain subcommand names and global flags
		if !strings.Contains(defaultSection, "'serve'") || !strings.Contains(defaultSection, "'migrate'") {
			t.Errorf("default candidates missing subcommands: %s", defaultSection)
		}
		if !strings.Contains(defaultSection, "'--verbose'") {
			t.Errorf("default candidates missing global flags: %s", defaultSection)
		}
	})

	// 4. MIN-25: collectFlagTokens appends completionFlag
	t.Run("MIN-25_CompletionFlagToken", func(t *testing.T) {
		type AppCfg struct {
			Env string `confi:"env"`
		}
		var cfg AppCfg

		// Bash completion includes --completion when WithCompletionFlag is set
		bashScript, err := Completion("bash", &cfg, WithName("mycli"), WithCompletionFlag("completion"))
		if err != nil {
			t.Fatalf("Bash completion failed: %v", err)
		}
		if !strings.Contains(bashScript, "--completion") {
			t.Errorf("Bash completion script missing --completion flag token:\n%s", bashScript)
		}

		// PowerShell completion includes '--completion' when WithCompletionFlag is set
		pwshScript, err := Completion("powershell", &cfg, WithName("mycli"), WithCompletionFlag("completion"))
		if err != nil {
			t.Fatalf("PowerShell completion failed: %v", err)
		}
		if !strings.Contains(pwshScript, "'--completion'") {
			t.Errorf("PowerShell completion script missing '--completion' flag token:\n%s", pwshScript)
		}
	})

	// 5. MIN-26: App name quoting in Bash, Zsh, and Fish
	t.Run("MIN-26_AppNameQuoting", func(t *testing.T) {
		type AppCfg struct {
			Host string `confi:"host"`
		}
		var cfg AppCfg

		for _, appName := range []string{"my-app", "my tool"} {
			// Bash
			bash, err := Completion("bash", &cfg, WithName(appName))
			if err != nil {
				t.Fatalf("Bash completion failed for %q: %v", appName, err)
			}
			expectedBash := fmt.Sprintf("complete -F _%s_completions %q", sanitizeShellIdentifier(appName), appName)
			if !strings.Contains(bash, expectedBash) {
				t.Errorf("Bash registration line mismatch for %q: expected %q in:\n%s", appName, expectedBash, bash)
			}

			// Zsh
			zsh, err := Completion("zsh", &cfg, WithName(appName))
			if err != nil {
				t.Fatalf("Zsh completion failed for %q: %v", appName, err)
			}
			expectedZsh := fmt.Sprintf("compdef _%s %q", sanitizeShellIdentifier(appName), appName)
			if !strings.Contains(zsh, expectedZsh) {
				t.Errorf("Zsh registration line mismatch for %q: expected %q in:\n%s", appName, expectedZsh, zsh)
			}

			// Fish
			fish, err := Completion("fish", &cfg, WithName(appName))
			if err != nil {
				t.Fatalf("Fish completion failed for %q: %v", appName, err)
			}
			expectedFish := fmt.Sprintf("complete -c %q", appName)
			if !strings.Contains(fish, expectedFish) {
				t.Errorf("Fish registration line mismatch for %q: expected %q in:\n%s", appName, expectedFish, fish)
			}
		}
	})

	// 6. MIN-23: Sample generation using populated runtime struct values
	t.Run("MIN-23_SampleGeneratorRuntimeValues", func(t *testing.T) {
		type SampleSeedCfg struct {
			Host    string        `confi:"host"`
			Port    int           `confi:"port"`
			Timeout time.Duration `confi:"timeout"`
			Tags    []string      `confi:"tags"`
			Secret  string        `confi:"secret,secret"`
		}

		seed := SampleSeedCfg{
			Host:    "cluster.internal",
			Port:    7000,
			Timeout: 5 * time.Second,
			Tags:    []string{"web", "prod"},
			Secret:  "dont-leak-secret",
		}

		// YAML sample contains populated non-zero values
		yamlSample, err := GenerateSample(&seed, FormatYAML)
		if err != nil {
			t.Fatalf("GenerateSample YAML failed: %v", err)
		}
		if !strings.Contains(yamlSample, `host: "cluster.internal"`) {
			t.Errorf("YAML sample missing populated host:\n%s", yamlSample)
		}
		if !strings.Contains(yamlSample, `port: 7000`) {
			t.Errorf("YAML sample missing populated port:\n%s", yamlSample)
		}
		if !strings.Contains(yamlSample, `timeout: "5s"`) {
			t.Errorf("YAML sample missing populated timeout:\n%s", yamlSample)
		}
		if !strings.Contains(yamlSample, `tags: ["web", "prod"]`) {
			t.Errorf("YAML sample missing populated tags:\n%s", yamlSample)
		}
		if strings.Contains(yamlSample, "dont-leak-secret") {
			t.Errorf("YAML sample leaked secret value:\n%s", yamlSample)
		}

		// TOML sample contains populated non-zero values
		tomlSample, err := GenerateSample(&seed, FormatTOML)
		if err != nil {
			t.Fatalf("GenerateSample TOML failed: %v", err)
		}
		if !strings.Contains(tomlSample, `host = "cluster.internal"`) {
			t.Errorf("TOML sample missing populated host:\n%s", tomlSample)
		}
		if !strings.Contains(tomlSample, `port = 7000`) {
			t.Errorf("TOML sample missing populated port:\n%s", tomlSample)
		}
		if !strings.Contains(tomlSample, `timeout = "5s"`) {
			t.Errorf("TOML sample missing populated timeout:\n%s", tomlSample)
		}
		if !strings.Contains(tomlSample, `tags = ["web", "prod"]`) {
			t.Errorf("TOML sample missing populated tags:\n%s", tomlSample)
		}
		if strings.Contains(tomlSample, "dont-leak-secret") {
			t.Errorf("TOML sample leaked secret value:\n%s", tomlSample)
		}

		// JSON sample contains populated non-zero values
		jsonSample, err := GenerateSample(&seed, FormatJSON)
		if err != nil {
			t.Fatalf("GenerateSample JSON failed: %v", err)
		}
		if !strings.Contains(jsonSample, `"host": "cluster.internal"`) {
			t.Errorf("JSON sample missing populated host:\n%s", jsonSample)
		}
		if !strings.Contains(jsonSample, `"port": 7000`) {
			t.Errorf("JSON sample missing populated port:\n%s", jsonSample)
		}
		if !strings.Contains(jsonSample, `"timeout": "5s"`) {
			t.Errorf("JSON sample missing populated timeout:\n%s", jsonSample)
		}
		if strings.Contains(jsonSample, "dont-leak-secret") {
			t.Errorf("JSON sample leaked secret value:\n%s", jsonSample)
		}

		// Unpopulated struct outputs hardcoded zeros/defaults
		var empty SampleSeedCfg
		emptyYAML, err := GenerateSample(&empty, FormatYAML)
		if err != nil {
			t.Fatalf("GenerateSample empty YAML failed: %v", err)
		}
		if !strings.Contains(emptyYAML, `host: ""`) {
			t.Errorf("empty YAML sample should have empty host string, got:\n%s", emptyYAML)
		}
		if !strings.Contains(emptyYAML, `port: 0`) {
			t.Errorf("empty YAML sample should have port 0, got:\n%s", emptyYAML)
		}
		if !strings.Contains(emptyYAML, `timeout: "0s"`) {
			t.Errorf("empty YAML sample should have timeout '0s', got:\n%s", emptyYAML)
		}
		if !strings.Contains(emptyYAML, `tags: []`) {
			t.Errorf("empty YAML sample should have empty tags slice, got:\n%s", emptyYAML)
		}
	})
}

func TestComprehensiveRegressionAndEdgeCases(t *testing.T) {
	// 1. MIN-08: Unicode short flag bundling and non-ASCII runes
	t.Run("MIN-08_UnicodeRuneShortFlagBundling", func(t *testing.T) {
		type RuneCfg struct {
			Verbose bool   `confi:"verbose,short=α"`
			Quiet   bool   `confi:"quiet,short=β"`
			Output  string `confi:"output,short=γ"`
		}
		var rCfg RuneCfg
		err := Parse(&rCfg, WithArgs([]string{"-αβ", "-γ=file.txt"}))
		if err != nil {
			t.Fatalf("MIN-08 failed: expected unicode short flags to parse cleanly: %v", err)
		}
		if !rCfg.Verbose || !rCfg.Quiet || rCfg.Output != "file.txt" {
			t.Fatalf("MIN-08 failed: expected Verbose=true, Quiet=true, Output='file.txt', got %+v", rCfg)
		}

		// Single unicode rune short flag
		var rCfg2 RuneCfg
		if err := Parse(&rCfg2, WithArgs([]string{"-α"})); err != nil || !rCfg2.Verbose {
			t.Fatalf("MIN-08 failed on single unicode short flag: %v", err)
		}
	})

	// 2. MAJ-06: TOML bracket balancing rejecting negative depth
	t.Run("MAJ-06_TOMLBracketBalancingNegativeDepth", func(t *testing.T) {
		if isBalancedBrackets("]") {
			t.Error("expected isBalancedBrackets to return false for lone closing bracket ']'")
		}
		if isBalancedBrackets("][") {
			t.Error("expected isBalancedBrackets to return false for inverted brackets ']['")
		}
		if isBalancedBrackets("[1, 2]]") {
			t.Error("expected isBalancedBrackets to return false for negative depth '[1, 2]]'")
		}
		if !isBalancedBrackets("[ [ 1 ] ]") {
			t.Error("expected isBalancedBrackets to return true for nested brackets '[ [ 1 ] ]'")
		}
		if !isBalancedBrackets(`[ "]" ]`) {
			t.Error("expected isBalancedBrackets to return true for quoted bracket '[ \"]\" ]'")
		}
		if !isBalancedBrackets(`[ '[' ]`) {
			t.Error("expected isBalancedBrackets to return true for single-quoted bracket")
		}

		type NegBracketCfg struct {
			Items []int `confi:"items"`
		}
		var negCfg NegBracketCfg
		err := Parse(&negCfg, WithConfigReader(strings.NewReader("items = ]\n"), FormatTOML), WithArgs(nil))
		if err == nil {
			t.Error("expected error for unbalanced bracket 'items = ]'")
		}
	})

	// 3. MAJ-27: Watcher modtime check avoiding redundant read
	t.Run("MAJ-27_WatcherModtimeOptimization", func(t *testing.T) {
		tmpFile, err := ioutil.TempFile("", "confi_modtime_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("val = 1\n")
		tmpFile.Close()

		snap1 := takeSnapshots([]string{tmpFile.Name()}, nil)
		if snap1[tmpFile.Name()].size == 0 || !snap1[tmpFile.Name()].exists {
			t.Fatalf("expected initial snapshot to exist with non-zero size")
		}

		// Subsequent snapshot without file modification should reuse previous snapshot struct
		snap2 := takeSnapshots([]string{tmpFile.Name()}, snap1)
		if !snap2[tmpFile.Name()].modTime.Equal(snap1[tmpFile.Name()].modTime) {
			t.Errorf("expected modTime equality")
		}
		if snapshotsChanged(snap1, snap2) {
			t.Errorf("expected snapshotsChanged to be false when modtime and size are identical")
		}
	})

	// 4. MIN-01: ParseError Unwrap and formatting
	t.Run("MIN-01_ParseErrorUnwrap", func(t *testing.T) {
		inner := errors.New("underlying syntax error")
		pe := &ParseError{Field: "database.port", Message: "invalid port syntax", Err: inner}
		if pe.Unwrap() != inner {
			t.Fatalf("expected Unwrap() to return inner error, got %v", pe.Unwrap())
		}
		if !strings.Contains(pe.Error(), "database.port") || !strings.Contains(pe.Error(), "invalid port syntax") {
			t.Fatalf("unexpected ParseError.Error(): %s", pe.Error())
		}
	})

	// 5. INFO-03: Completion error handling
	t.Run("INFO-03_CompletionErrorHandling", func(t *testing.T) {
		type CompCfg struct {
			Host string `confi:"host"`
		}
		var buf bytes.Buffer
		// Unsupported shell
		if err := WriteCompletion(&buf, "nosuchshell", &CompCfg{}); err == nil {
			t.Error("expected error for unsupported shell 'nosuchshell'")
		}
		// Nil target
		if err := WriteCompletion(&buf, "bash", nil); err == nil {
			t.Error("expected error for nil target")
		}
		if _, err := Completion("invalid", &CompCfg{}); err == nil {
			t.Error("expected error for invalid shell in Completion()")
		}
	})

	// 6. Format parsers dotted keys and multiline basic strings
	t.Run("FormatParsers_DottedKeysAndMultiline", func(t *testing.T) {
		// TOML dotted key with quotes
		type DottedCfg struct {
			Data map[string]map[string]string `confi:"data"`
		}
		tomlDotted := "data.\"api.v1\".endpoint = \"https://api.local/v1\"\n"
		var dotCfg DottedCfg
		if err := Parse(&dotCfg, WithConfigReader(strings.NewReader(tomlDotted), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatalf("failed to parse TOML dotted key: %v", err)
		}
		if dotCfg.Data["api.v1"]["endpoint"] != "https://api.local/v1" {
			t.Fatalf("unexpected dotted key structure: %+v", dotCfg.Data)
		}

		// TOML multiline basic string
		type MultilineBasicCfg struct {
			Bio string `confi:"bio"`
		}
		var mbCfg MultilineBasicCfg
		tomlMB := "bio = \"\"\"\nFirst line \\u0041\nSecond line \\n third line\n\"\"\"\n"
		if err := Parse(&mbCfg, WithConfigReader(strings.NewReader(tomlMB), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatalf("failed to parse TOML basic multiline string: %v", err)
		}
		if !strings.Contains(mbCfg.Bio, "First line A") || !strings.Contains(mbCfg.Bio, "Second line \n third line") {
			t.Fatalf("unexpected multiline basic string content: %q", mbCfg.Bio)
		}

		// DotEnv variable expansion fallback ${VAR:-default}
		dotEnvContent := "DEFINED=hello\nEXP1=${DEFINED:-fallback}\nEXP2=${UNDEFINED_999:-fallback_val}\nEXP3=${UNDEFINED_888:-raw_fallback}\n"
		envMap, err := parseDotEnv(strings.NewReader(dotEnvContent), os.LookupEnv)
		if err != nil {
			t.Fatalf("failed to parse .env fallbacks: %v", err)
		}
		if envMap["EXP1"] != "hello" || envMap["EXP2"] != "fallback_val" || envMap["EXP3"] != "raw_fallback" {
			t.Fatalf("unexpected .env expansions: %+v", envMap)
		}
	})

	// 7. Validation: multi-type oneOf, string/slice min/max length, and pattern constraints
	t.Run("Validation_MultiTypeOneOfAndRegexp", func(t *testing.T) {
		type ConstraintsCfg struct {
			IntOneOf   int           `confi:"int-one-of,oneof='10|20|30'"`
			FloatOneOf float64       `confi:"float-one-of,oneof='1.5|2.5'"`
			DurOneOf   time.Duration `confi:"dur-one-of,oneof='100ms|500ms|1s'"`
			Code       string        `confi:"code,pattern='^[A-Z]{3}-[0-9]{3}$'"`
			ShortStr   string        `confi:"short-str,min=2,max=5"`
		}
		var validC ConstraintsCfg
		err := Parse(&validC, WithArgs([]string{
			"--int-one-of=20",
			"--float-one-of=2.5",
			"--dur-one-of=500ms",
			"--code=ABC-123",
			"--short-str=hello",
		}))
		if err != nil {
			t.Fatalf("expected valid constraints to pass, got: %v", err)
		}

		// Test violations
		var invalidInt ConstraintsCfg
		if err := Parse(&invalidInt, WithArgs([]string{"--int-one-of=99"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected int oneOf error, got: %v", err)
		}

		var invalidFloat ConstraintsCfg
		if err := Parse(&invalidFloat, WithArgs([]string{"--float-one-of=9.9"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected float oneOf error, got: %v", err)
		}

		var invalidDur ConstraintsCfg
		if err := Parse(&invalidDur, WithArgs([]string{"--dur-one-of=2s"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected dur oneOf error, got: %v", err)
		}

		var invalidRegex ConstraintsCfg
		if err := Parse(&invalidRegex, WithArgs([]string{"--code=invalid-code"})); err == nil || !strings.Contains(err.Error(), "does not match pattern") {
			t.Fatalf("expected pattern error, got: %v", err)
		}

		var invalidLenMax ConstraintsCfg
		if err := Parse(&invalidLenMax, WithArgs([]string{"--short-str=too-long-string"})); err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
			t.Fatalf("expected max length error, got: %v", err)
		}

		var invalidLenMin ConstraintsCfg
		if err := Parse(&invalidLenMin, WithArgs([]string{"--short-str=a"})); err == nil || !strings.Contains(err.Error(), "less than minimum") {
			t.Fatalf("expected min length error, got: %v", err)
		}
	})

	// 8. CLI flags terminator and negation
	t.Run("CLI_FlagsTerminatorAndNegation", func(t *testing.T) {
		type TermCfg struct {
			Enabled bool   `confi:"enabled"`
			Verbose bool   `confi:"verbose"`
			File1   string `confi:"file1,pos"`
			File2   string `confi:"file2,pos"`
		}
		var tCfg TermCfg
		err := Parse(&tCfg, WithArgs([]string{"--enabled", "--verbose", "--no-verbose", "--", "--not-a-flag", "-file2"}))
		if err != nil {
			t.Fatalf("Parse with terminator failed: %v", err)
		}
		if !tCfg.Enabled || tCfg.Verbose || tCfg.File1 != "--not-a-flag" || tCfg.File2 != "-file2" {
			t.Fatalf("unexpected terminator result: %+v", tCfg)
		}
	})

	// 9. Scalar interface conversion in config loader
	t.Run("ScalarInterfaceConversionInConfig", func(t *testing.T) {
		type ScalarConvCfg struct {
			IntStr   string `confi:"int_str"`
			FloatStr string `confi:"float_str"`
			BoolStr  string `confi:"bool_str"`
			Role     customRole `confi:"role"`
		}
		jsonData := `{"int_str": 42, "float_str": 3.1415, "bool_str": true, "role": "editor"}`
		var sCfg ScalarConvCfg
		if err := Parse(&sCfg, WithConfigReader(strings.NewReader(jsonData), FormatJSON), WithArgs(nil)); err != nil {
			t.Fatalf("JSON scalar conversion failed: %v", err)
		}
		if sCfg.IntStr != "42" || sCfg.FloatStr != "3.1415" || sCfg.BoolStr != "true" || sCfg.Role.Name != "role:EDITOR" {
			t.Fatalf("unexpected scalar conversions: %+v", sCfg)
		}
	})

	// 10. TOML array of tables with rich fields
	t.Run("TOMLTableUnderArrayOfTables", func(t *testing.T) {
		type Server struct {
			Name    string `confi:"name"`
			Port    int    `confi:"port"`
			Enabled bool   `confi:"enabled"`
		}
		type Topology struct {
			Servers []Server `confi:"servers"`
		}
		tomlData := `
[[servers]]
name = "alpha"
port = 8001
enabled = true

[[servers]]
name = "beta"
port = 8002
enabled = false
`
		var top Topology
		if err := Parse(&top, WithConfigReader(strings.NewReader(tomlData), FormatTOML), WithArgs(nil)); err != nil {
			t.Fatalf("TOML array of tables failed: %v", err)
		}
		if len(top.Servers) != 2 || top.Servers[0].Name != "alpha" || top.Servers[1].Name != "beta" {
			t.Fatalf("unexpected server names: %+v", top.Servers)
		}
		if top.Servers[0].Port != 8001 || !top.Servers[0].Enabled || top.Servers[1].Port != 8002 || top.Servers[1].Enabled {
			t.Fatalf("unexpected server fields: %+v", top.Servers)
		}
	})

	// 11. OneOf boolean and unsigned integer validation
	t.Run("OneOfBooleanAndUnsignedInts", func(t *testing.T) {
		type BoolUintOneOf struct {
			Choice bool   `confi:"choice,oneof='true'"`
			Size   uint64 `confi:"size,oneof='1024|2048|4096'"`
		}
		var bu BoolUintOneOf
		if err := Parse(&bu, WithArgs([]string{"--choice=true", "--size=2048"})); err != nil {
			t.Fatalf("expected valid choice and size to pass, got: %v", err)
		}

		var invalidSize BoolUintOneOf
		if err := Parse(&invalidSize, WithArgs([]string{"--choice=true", "--size=9999"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected oneOf error on uint64, got: %v", err)
		}

		var invalidBool BoolUintOneOf
		if err := Parse(&invalidBool, WithArgs([]string{"--choice=false"})); err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected oneOf error on bool, got: %v", err)
		}
	})

	// 12. Watcher deep copy complex pointer and interface structures
	t.Run("WatcherDeepCopyComplexStructures", func(t *testing.T) {
		type InnerData struct {
			Count int
			Tags  []string
		}
		type OuterData struct {
			PtrInner *InnerData
			MapData  map[string]string
		}

		orig := OuterData{
			PtrInner: &InnerData{Count: 42, Tags: []string{"a", "b"}},
			MapData:  map[string]string{"k1": "v1"},
		}

		copyVal := deepCopyValue(reflect.ValueOf(orig))
		copied, ok := copyVal.Interface().(OuterData)
		if !ok {
			t.Fatalf("deepCopyValue type mismatch")
		}

		// Mutate original
		orig.PtrInner.Count = 100
		orig.PtrInner.Tags[0] = "mutated"
		orig.MapData["k1"] = "mutated"

		// Copied must be unaffected
		if copied.PtrInner.Count != 42 || copied.PtrInner.Tags[0] != "a" || copied.MapData["k1"] != "v1" {
			t.Fatalf("deepCopyValue was not a true deep copy: %+v", copied)
		}
	})
}

type e2eServeCmd struct {
	Port     int    `confi:"port,default=8080"`
	Host     string `confi:"host,default=0.0.0.0"`
	executed bool
}

func (s *e2eServeCmd) Run() error {
	s.executed = true
	return nil
}

type e2eMigrateCmd struct {
	ConfigFile string `confi:"config,config"`
	Steps      int    `confi:"steps,default=1"`
	executed   bool
}

func (m *e2eMigrateCmd) Run() error {
	m.executed = true
	return nil
}

func TestEndToEndIntegration(t *testing.T) {
	// 1. Full Precedence, Live Reload, Redaction, and Shell Completion Lifecycle
	t.Run("PrecedenceAndLiveReloadIntegration", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "confi_e2e_")
		if err != nil {
			t.Fatalf("TempDir failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		cfgPath := filepath.Join(tmpDir, "app.yaml")
		yamlContent := `
host: "config.local"
port: 7000
secret_key: "yaml-super-secret"
`
		if err := ioutil.WriteFile(cfgPath, []byte(yamlContent), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		type E2EAppConfig struct {
			Host       string            `confi:"host,min=3,default='default.local'"`
			Port       int               `confi:"port,min=1024,max=65535,default=3000"`
			SecretKey  string            `confi:"secret_key,secret"`
			Workers    int               // Programmatic default (no tag)
			Labels     map[string]string `confi:"labels"`
			ConfigFile string            `confi:"config,config"`
		}

		app := E2EAppConfig{
			Workers: 8, // programmatic default
		}

		// Environment variable overrides config file and struct default
		os.Setenv("E2E_HOST", "env.override.local")
		defer os.Unsetenv("E2E_HOST")

		// CLI flag overrides env, config file, and struct default
		cliArgs := []string{"--port=9999", "--labels", "region=us-west,env=prod"}

		reloadCh := make(chan error, 2)
		watcher, err := Watch(
			&app,
			func(e error) {
				reloadCh <- e
			},
			WithConfigFile(cfgPath),
			WithConfigFlag("config"),
			WithEnvPrefix("E2E"),
			WithAutoEnv(true),
			WithArgs(cliArgs),
			WithWatchInterval(15*time.Millisecond),
		)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}
		defer watcher.Close()

		// Verify precedence:
		// Port: CLI (9999) > Config (7000) > Default (3000)
		if app.Port != 9999 {
			t.Errorf("precedence failure for Port: expected 9999 from CLI, got %d", app.Port)
		}
		// Host: Env (env.override.local) > Config (config.local) > Default (default.local)
		if app.Host != "env.override.local" {
			t.Errorf("precedence failure for Host: expected env.override.local, got %q", app.Host)
		}
		// SecretKey: from Config ("yaml-super-secret")
		if app.SecretKey != "yaml-super-secret" {
			t.Errorf("SecretKey failure: expected yaml-super-secret, got %q", app.SecretKey)
		}
		// Programmatic default: preserved!
		if app.Workers != 8 {
			t.Errorf("Programmatic default failure: expected Workers=8, got %d", app.Workers)
		}
		// Labels map
		if app.Labels["region"] != "us-west" || app.Labels["env"] != "prod" {
			t.Errorf("Labels mismatch: %+v", app.Labels)
		}

		// Generate sample YAML and verify secret value is not leaked
		sampleYAML, err := GenerateSample(&app, FormatYAML)
		if err != nil {
			t.Fatalf("GenerateSample failed: %v", err)
		}
		if strings.Contains(sampleYAML, "yaml-super-secret") {
			t.Errorf("SecretKey was leaked in YAML sample:\n%s", sampleYAML)
		}
		if !strings.Contains(sampleYAML, `secret_key: ""`) {
			t.Errorf("Expected secret_key: \"\" in YAML sample:\n%s", sampleYAML)
		}

		// Dump masks secrets with ********
		dumpStr, err := Dump(&app)
		if err != nil {
			t.Fatalf("Dump failed: %v", err)
		}
		if strings.Contains(dumpStr, "yaml-super-secret") {
			t.Errorf("SecretKey was leaked in Dump:\n%s", dumpStr)
		}
		if !strings.Contains(dumpStr, "secret-key = ********") && !strings.Contains(dumpStr, "secret_key = ********") {
			t.Errorf("Expected secret key masked with ******** in Dump, got:\n%s", dumpStr)
		}

		// Generate completions for all supported shells
		for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
			script, err := Completion(shell, &app, WithName("e2eapp"))
			if err != nil {
				t.Fatalf("Completion failed for %s: %v", shell, err)
			}
			if len(script) == 0 {
				t.Fatalf("empty completion script for %s", shell)
			}
		}

		// Mutate config on disk
		time.Sleep(30 * time.Millisecond)
		updatedYaml := `
host: "config.local"
port: 7000
secret_key: "updated-secret"
`
		if err := ioutil.WriteFile(cfgPath, []byte(updatedYaml), 0600); err != nil {
			t.Fatalf("WriteFile update failed: %v", err)
		}

		select {
		case rErr := <-reloadCh:
			if rErr != nil {
				t.Fatalf("reload returned error: %v", rErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for watcher reload")
		}

		watcher.RLock()
		if app.SecretKey != "updated-secret" {
			t.Errorf("expected updated SecretKey 'updated-secret', got %q", app.SecretKey)
		}
		if app.Workers != 8 {
			t.Errorf("expected Workers=8 to be preserved after reload, got %d", app.Workers)
		}
		watcher.RUnlock()
	})

	// 2. Subcommands, Runners, Dynamic Subcommand Config Files, and Usage
	t.Run("SubcommandsEndToEndIntegration", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "confi_e2e_sub_")
		if err != nil {
			t.Fatalf("TempDir failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		migrateCfgFile := filepath.Join(tmpDir, "migrate.json")
		if err := ioutil.WriteFile(migrateCfgFile, []byte(`{"steps": 5}`), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		type RootApp struct {
			Verbose bool          `confi:"verbose,usage='Enable verbose output'"`
			Serve   e2eServeCmd   `confi:"serve,subcommand,usage='Start API server'"`
			Migrate e2eMigrateCmd `confi:"migrate,subcommand,usage='Run database migrations'"`
		}

		// Test 1: SubcommandUsage formatting
		var root RootApp
		usageStr, err := SubcommandUsage(&root, "serve", WithName("myapp"))
		if err != nil {
			t.Fatalf("SubcommandUsage failed: %v", err)
		}
		if !strings.Contains(usageStr, "Usage: myapp serve [flags]") || !strings.Contains(usageStr, "--port") {
			t.Fatalf("unexpected SubcommandUsage output:\n%s", usageStr)
		}

		// Test 2: Subcommand execution via RunCommand
		var root2 RootApp
		p := NewParser("myapp", WithArgs([]string{"serve", "--port=9090"}))
		if err := p.Parse(&root2); err != nil {
			t.Fatalf("Parse serve subcommand failed: %v", err)
		}
		if root2.Serve.Port != 9090 {
			t.Fatalf("expected Serve.Port 9090, got %d", root2.Serve.Port)
		}
		if err := p.RunCommand(&root2); err != nil {
			t.Fatalf("RunCommand failed: %v", err)
		}
		if !root2.Serve.executed {
			t.Fatal("expected serve.Run() to be executed")
		}

		// Test 3: Subcommand with dynamic config file
		var root3 RootApp
		p2 := NewParser("myapp", WithArgs([]string{"migrate", "--config", migrateCfgFile}))
		if err := p2.Parse(&root3); err != nil {
			t.Fatalf("Parse migrate subcommand with dynamic config failed: %v", err)
		}
		if root3.Migrate.Steps != 5 {
			t.Fatalf("expected Migrate.Steps=5 from JSON config, got %d", root3.Migrate.Steps)
		}
		if err := p2.RunCommand(&root3); err != nil {
			t.Fatalf("RunCommand failed: %v", err)
		}
		if !root3.Migrate.executed {
			t.Fatal("expected migrate.Run() to be executed")
		}
	})

	// 3. Cyclic structs with custom validation and schema extraction
	t.Run("CyclicAndComplexTypesIntegration", func(t *testing.T) {
		type RecursiveNode struct {
			Name  string         `confi:"name"`
			Child *RecursiveNode `confi:"child"`
		}

		var root RecursiveNode
		// Verify schema extraction and ParseArgs handles self-referencing types cleanly
		if err := Parse(&root, WithArgs([]string{"--name=root"})); err != nil {
			t.Fatalf("Parse on self-referential struct failed: %v", err)
		}
		if root.Name != "root" {
			t.Errorf("expected root.Name 'root', got %q", root.Name)
		}

		// Verify usage string generation does not loop infinitely
		usageStr, err := Usage(&root, WithName("cyclicapp"))
		if err != nil {
			t.Fatalf("Usage failed on cyclic struct: %v", err)
		}
		if !strings.Contains(usageStr, "--name") {
			t.Fatalf("Usage missing --name:\n%s", usageStr)
		}

		// Verify custom validator terminates cleanly on cyclic reference
		n1 := &RecursiveNode{Name: "n1"}
		n2 := &RecursiveNode{Name: "n2", Child: n1}
		n1.Child = n2
		if err := invokeCustomValidators(reflect.ValueOf(n1)); err != nil {
			t.Fatalf("invokeCustomValidators failed on cyclic pointers: %v", err)
		}
	})
}
