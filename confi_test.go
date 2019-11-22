package confi

import (
	"bytes"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
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

