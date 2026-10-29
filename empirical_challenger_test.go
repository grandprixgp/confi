package confi

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Objective 1: YAML Edge Cases Verification
// ============================================================================

func TestEmpirical_YAML_LeadingTabs(t *testing.T) {
	// 1. Single and multiple tabs preceding mapping keys
	tabYAML1 := "\thost: tab1.local\n\tport: 8081\n"
	m1, err := parseYAML(strings.NewReader(tabYAML1))
	if err != nil {
		t.Fatalf("parseYAML failed on leading single tab: %v", err)
	}
	if m1["host"] != "tab1.local" || m1["port"] != "8081" {
		t.Fatalf("unexpected m1 values: %+v", m1)
	}

	// 2. Nested mappings with tabs
	tabYAML2 := "server:\n\t\thost: tab2.local\n\t\tport: 8082\n"
	m2, err := parseYAML(strings.NewReader(tabYAML2))
	if err != nil {
		t.Fatalf("parseYAML failed on leading double tabs: %v", err)
	}
	sub, ok := m2["server"].(map[string]interface{})
	if !ok || sub["host"] != "tab2.local" || sub["port"] != "8082" {
		t.Fatalf("unexpected nested tab mapping: %+v", m2)
	}

	// 3. Mixed spaces and tabs
	tabYAML3 := "mix:\n \tkey: val\n"
	m3, err := parseYAML(strings.NewReader(tabYAML3))
	if err != nil {
		t.Fatalf("parseYAML failed on mixed space+tab: %v", err)
	}
	sub3, ok := m3["mix"].(map[string]interface{})
	if !ok || sub3["key"] != "val" {
		t.Fatalf("unexpected mixed space+tab mapping: %+v", m3)
	}

	// 4. Tab indent slice bounds panic stress
	for i := 1; i <= 20; i++ {
		raw := strings.Repeat("\t", i) + fmt.Sprintf("k%d: v%d\n", i, i)
		_, err := parseYAML(strings.NewReader(raw))
		if err != nil {
			t.Fatalf("parseYAML failed on %d leading tabs: %v", i, err)
		}
	}
}

func TestEmpirical_YAML_MultilineStrings(t *testing.T) {
	// Literal block scalar (|) with trailing newline preserved
	yamlLiteral := `
query: |
  SELECT id, name
  FROM users
  WHERE active = true
`
	m, err := parseYAML(strings.NewReader(yamlLiteral))
	if err != nil {
		t.Fatalf("parseYAML failed on literal block scalar: %v", err)
	}
	wantLiteral := "SELECT id, name\nFROM users\nWHERE active = true\n"
	if m["query"] != wantLiteral {
		t.Fatalf("expected literal block:\n%q\ngot:\n%q", wantLiteral, m["query"])
	}

	// Strip chomp (|-)
	yamlChomp := `
query: |-
  SELECT 1
  FROM dual
`
	mChomp, err := parseYAML(strings.NewReader(yamlChomp))
	if err != nil {
		t.Fatalf("parseYAML failed on strip chomp block scalar: %v", err)
	}
	wantChomp := "SELECT 1\nFROM dual"
	if mChomp["query"] != wantChomp {
		t.Fatalf("expected chomp block:\n%q\ngot:\n%q", wantChomp, mChomp["query"])
	}

	// Folded block scalar (>)
	yamlFolded := `
description: >
  This is a very long
  description that spans
  multiple lines.
`
	mFold, err := parseYAML(strings.NewReader(yamlFolded))
	if err != nil {
		t.Fatalf("parseYAML failed on folded block scalar: %v", err)
	}
	wantFold := "This is a very long description that spans multiple lines."
	if mFold["description"] != wantFold {
		t.Fatalf("expected folded block:\n%q\ngot:\n%q", wantFold, mFold["description"])
	}
}

func TestEmpirical_YAML_MultiDocumentRejection(t *testing.T) {
	// 1. Multi-document with "---"
	doc1 := "k1: v1\n---\nk2: v2\n"
	_, err := parseYAML(strings.NewReader(doc1))
	if err == nil {
		t.Fatal("expected error on multi-document YAML stream (---), got nil")
	}
	if !strings.Contains(err.Error(), "multi-document") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 2. Multi-document with "..."
	doc2 := "k1: v1\n...\nk2: v2\n"
	_, err2 := parseYAML(strings.NewReader(doc2))
	if err2 == nil {
		t.Fatal("expected error on multi-document YAML stream (...), got nil")
	}
	if !strings.Contains(err2.Error(), "multi-document") {
		t.Fatalf("unexpected error message: %v", err2)
	}

	// 3. Leading "---" on first line alone is valid standard YAML document start
	docLeading := "---\nk1: v1\nk2: v2\n"
	mLeading, err3 := parseYAML(strings.NewReader(docLeading))
	if err3 != nil {
		t.Fatalf("leading '---' alone should be valid YAML header: %v", err3)
	}
	if mLeading["k1"] != "v1" || mLeading["k2"] != "v2" {
		t.Fatalf("unexpected values with leading '---': %+v", mLeading)
	}
}

func TestEmpirical_YAML_DeepRecursionLimit(t *testing.T) {
	// Nesting <= 100 should succeed
	var bOk strings.Builder
	for i := 0; i < 90; i++ {
		bOk.WriteString(strings.Repeat("  ", i) + fmt.Sprintf("n%d:\n", i))
	}
	bOk.WriteString(strings.Repeat("  ", 90) + "leaf: 42\n")
	_, errOk := parseYAML(strings.NewReader(bOk.String()))
	if errOk != nil {
		t.Fatalf("expected 90 levels of nesting to succeed, got: %v", errOk)
	}

	// Nesting > 100 should fail with depth error
	var bExceed strings.Builder
	for i := 0; i < 105; i++ {
		bExceed.WriteString(strings.Repeat("  ", i) + fmt.Sprintf("n%d:\n", i))
	}
	bExceed.WriteString(strings.Repeat("  ", 105) + "leaf: 42\n")
	_, errExceed := parseYAML(strings.NewReader(bExceed.String()))
	if errExceed == nil {
		t.Fatal("expected error when nesting exceeds 100, got nil")
	}
	if !strings.Contains(errExceed.Error(), "maximum nesting depth") {
		t.Fatalf("expected 'maximum nesting depth' in error, got: %v", errExceed)
	}

	// Flow sequence nesting > 100
	flowDeep := strings.Repeat("[", 105) + "1" + strings.Repeat("]", 105)
	_, errFlow := parseYAML(strings.NewReader("flow: " + flowDeep + "\n"))
	if errFlow == nil {
		t.Fatal("expected error on deeply nested flow sequence exceeding depth limit, got nil")
	}
	if !strings.Contains(errFlow.Error(), "depth") {
		t.Fatalf("expected depth error on flow sequence, got: %v", errFlow)
	}
}

func TestEmpirical_YAML_SequenceRootRejection(t *testing.T) {
	// Block sequence at root
	seq1 := "- item1\n- item2\n"
	_, err1 := parseYAML(strings.NewReader(seq1))
	if err1 == nil {
		t.Fatal("expected sequence root rejection for '- item', got nil")
	}
	if !strings.Contains(err1.Error(), "root document must be a mapping") {
		t.Fatalf("unexpected error message: %v", err1)
	}

	// Bare '-' at root
	seq2 := "-\n  name: item1\n"
	_, err2 := parseYAML(strings.NewReader(seq2))
	if err2 == nil {
		t.Fatal("expected sequence root rejection for bare '-', got nil")
	}
	if !strings.Contains(err2.Error(), "root document must be a mapping") {
		t.Fatalf("unexpected error message: %v", err2)
	}

	// Flow sequence at root
	seq3 := "[1, 2, 3]\n"
	_, err3 := parseYAML(strings.NewReader(seq3))
	if err3 == nil {
		t.Fatal("expected sequence root rejection for flow sequence, got nil")
	}
	if !strings.Contains(err3.Error(), "root document must be a mapping") {
		t.Fatalf("unexpected error message: %v", err3)
	}
}

func TestEmpirical_YAML_UnicodeAndControlEscapes(t *testing.T) {
	yamlInput := `
unicode1: "\u0041\u0042\u0043"
unicode_snowman: "\u2603"
unicode_cjk: "\u4E16\u754C"
emoji: "\U0001F600"
ctrl_b: "\b"
ctrl_f: "\f"
ctrl_n: "\n"
ctrl_r: "\r"
ctrl_t: "\t"
ctrl_zero: "\0"
ctrl_slash: "\/"
`
	m, err := parseYAML(strings.NewReader(yamlInput))
	if err != nil {
		t.Fatalf("parseYAML failed on escapes: %v", err)
	}
	if m["unicode1"] != "ABC" {
		t.Errorf("expected 'ABC', got %q", m["unicode1"])
	}
	if m["unicode_snowman"] != "☃" {
		t.Errorf("expected '☃', got %q", m["unicode_snowman"])
	}
	if m["unicode_cjk"] != "世界" {
		t.Errorf("expected '世界', got %q", m["unicode_cjk"])
	}
	if m["emoji"] != "😀" {
		t.Errorf("expected '😀', got %q", m["emoji"])
	}
	if m["ctrl_b"] != "\b" || m["ctrl_f"] != "\f" || m["ctrl_n"] != "\n" || m["ctrl_r"] != "\r" || m["ctrl_t"] != "\t" {
		t.Errorf("control escapes mismatch: b=%q f=%q n=%q r=%q t=%q", m["ctrl_b"], m["ctrl_f"], m["ctrl_n"], m["ctrl_r"], m["ctrl_t"])
	}
	if m["ctrl_zero"] != "\x00" {
		t.Errorf("expected null byte '\\0', got %q", m["ctrl_zero"])
	}
	if m["ctrl_slash"] != "/" {
		t.Errorf("expected '/', got %q", m["ctrl_slash"])
	}
}

// ============================================================================
// Objective 1: TOML Edge Cases Verification
// ============================================================================

func TestEmpirical_TOML_MultilineStrings(t *testing.T) {
	// 1. Comments and quotes inside basic multiline """..."""
	tomlBasic := `
sql = """
SELECT id, "#name", email
FROM users
WHERE note = "active" # not a comment inside string
"""
trailing = """value""" # this is a comment
`
	m, err := parseTOML(strings.NewReader(tomlBasic))
	if err != nil {
		t.Fatalf("parseTOML failed on multiline basic: %v", err)
	}
	sqlVal, ok := m["sql"].(string)
	if !ok || !strings.Contains(sqlVal, `"#name"`) || !strings.Contains(sqlVal, `WHERE note = "active" # not a comment inside string`) {
		t.Fatalf("multiline string corrupted: %q", sqlVal)
	}
	if m["trailing"] != "value" {
		t.Fatalf("expected trailing='value', got %q", m["trailing"])
	}

	// 2. Literal multiline '''...''' preserving backslashes and comments
	tomlLiteral := `
path = '''
C:\Users\admin\Documents\#literal\path
line2
'''
`
	mLit, err := parseTOML(strings.NewReader(tomlLiteral))
	if err != nil {
		t.Fatalf("parseTOML failed on multiline literal: %v", err)
	}
	pathVal, ok := mLit["path"].(string)
	if !ok || !strings.Contains(pathVal, `C:\Users\admin\Documents\#literal\path`) {
		t.Fatalf("literal multiline string corrupted: %q", pathVal)
	}

	// 3. Unterminated multiline string rejected
	tomlUnterm := `
msg = """
hello world
no close
`
	_, errUnterm := parseTOML(strings.NewReader(tomlUnterm))
	if errUnterm == nil {
		t.Fatal("expected error on unterminated multiline string, got nil")
	}
	if !strings.Contains(errUnterm.Error(), "unterminated multiline string") {
		t.Fatalf("unexpected error message: %v", errUnterm)
	}
}

func TestEmpirical_TOML_ConflictingTablesOverScalars(t *testing.T) {
	// 1. Scalar followed by table with same name
	tomlConf1 := `
server = "scalar-value"
[server]
port = 8080
`
	_, err1 := parseTOML(strings.NewReader(tomlConf1))
	if err1 == nil {
		t.Fatal("expected error when table redefines scalar key, got nil")
	}
	if !strings.Contains(err1.Error(), "already defined as non-table value") {
		t.Fatalf("unexpected error: %v", err1)
	}

	// 2. Scalar followed by array-of-tables with same name
	tomlConf2 := `
servers = "scalar-value"
[[servers]]
port = 8080
`
	_, err2 := parseTOML(strings.NewReader(tomlConf2))
	if err2 == nil {
		t.Fatal("expected error when table array redefines scalar key, got nil")
	}
	if !strings.Contains(err2.Error(), "already defined as non-table value") {
		t.Fatalf("unexpected error: %v", err2)
	}

	// 3. Nested table redefines nested scalar
	tomlConf3 := `
[app]
db = "sqlite"
[app.db]
name = "prod"
`
	_, err3 := parseTOML(strings.NewReader(tomlConf3))
	if err3 == nil {
		t.Fatal("expected error when nested table redefines nested scalar, got nil")
	}
	if !strings.Contains(err3.Error(), "already defined as non-table value") {
		t.Fatalf("unexpected error: %v", err3)
	}
}

func TestEmpirical_TOML_ColonAndSectionHeaders(t *testing.T) {
	// Section header with colon inside quoted string
	tomlData := `
[endpoints."https://api.example.com:8443"]
timeout = "30s"
"urn:metric:rate" = "100req/s"
`
	m, err := parseTOML(strings.NewReader(tomlData))
	if err != nil {
		t.Fatalf("parseTOML failed on colon URLs in section headers: %v", err)
	}
	endpoints, ok := m["endpoints"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected endpoints map, got: %+v", m)
	}
	sec, ok := endpoints["https://api.example.com:8443"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected quoted URL section, got: %+v", endpoints)
	}
	if sec["timeout"] != "30s" {
		t.Errorf("expected timeout='30s', got %q", sec["timeout"])
	}
	if sec["urn:metric:rate"] != "100req/s" {
		t.Errorf("expected urn:metric:rate='100req/s', got %q", sec["urn:metric:rate"])
	}
}

func TestEmpirical_TOML_SemicolonNonCommentHandling(t *testing.T) {
	tomlData := `
quoted_dsn = "postgres://user:pass@host:5432/dbname;sslmode=disable"
unquoted_dsn = postgres://user:pass@host:5432/dbname;sslmode=disable
standalone_comment = "val" ; this is a comment after space
`
	m, err := parseTOML(strings.NewReader(tomlData))
	if err != nil {
		t.Fatalf("parseTOML failed: %v", err)
	}
	if m["quoted_dsn"] != "postgres://user:pass@host:5432/dbname;sslmode=disable" {
		t.Errorf("semicolon stripped in quoted DSN: %q", m["quoted_dsn"])
	}
	if m["unquoted_dsn"] != "postgres://user:pass@host:5432/dbname;sslmode=disable" {
		t.Errorf("semicolon stripped in unquoted DSN: %q", m["unquoted_dsn"])
	}
	if m["standalone_comment"] != "val" {
		t.Errorf("standalone comment not stripped: %q", m["standalone_comment"])
	}
}

// ============================================================================
// Objective 1: DotEnv Edge Cases Verification
// ============================================================================

func TestEmpirical_DotEnv_TrailingGarbageRejection(t *testing.T) {
	// Single-quoted trailing garbage
	env1 := "VAL='hello'world\n"
	_, err1 := parseDotEnv(strings.NewReader(env1), nil)
	if err1 == nil {
		t.Fatal("expected error on trailing garbage after single quote, got nil")
	}
	if !strings.Contains(err1.Error(), "unexpected trailing characters") {
		t.Fatalf("unexpected error message: %v", err1)
	}

	// Double-quoted trailing garbage
	env2 := "VAL=\"hello\"world\n"
	_, err2 := parseDotEnv(strings.NewReader(env2), nil)
	if err2 == nil {
		t.Fatal("expected error on trailing garbage after double quote, got nil")
	}
	if !strings.Contains(err2.Error(), "unexpected trailing characters") {
		t.Fatalf("unexpected error message: %v", err2)
	}

	// Comment after quote is valid
	env3 := "VAL=\"hello\" # valid comment\nVAL2='world' # another valid comment\n"
	m3, err3 := parseDotEnv(strings.NewReader(env3), nil)
	if err3 != nil {
		t.Fatalf("expected valid comments after quote to succeed, got: %v", err3)
	}
	if m3["VAL"] != "hello" || m3["VAL2"] != "world" {
		t.Fatalf("unexpected parsed values with comments: %+v", m3)
	}
}

func TestEmpirical_DotEnv_EscapedDollarAndBackslash(t *testing.T) {
	// \$foo -> $foo (literal dollar, not interpolated)
	// \\$foo -> \$foo (literal backslash followed by dollar, not interpolated)
	envData := `
LITERAL_DOLLAR="\$foo"
ESCAPED_SLASH="\\$foo"
TRIPLE_SLASH="\\\$foo"
NORMAL_VAR="value"
INTERPOLATED="prefix_${NORMAL_VAR}"
`
	m, err := parseDotEnv(strings.NewReader(envData), nil)
	if err != nil {
		t.Fatalf("parseDotEnv failed: %v", err)
	}
	if m["LITERAL_DOLLAR"] != "$foo" {
		t.Errorf("expected '$foo', got %q", m["LITERAL_DOLLAR"])
	}
	if m["ESCAPED_SLASH"] != "\\$foo" {
		t.Errorf("expected '\\$foo', got %q", m["ESCAPED_SLASH"])
	}
	if m["INTERPOLATED"] != "prefix_value" {
		t.Errorf("expected 'prefix_value', got %q", m["INTERPOLATED"])
	}
}

func TestEmpirical_DotEnv_ExportPrefixCaseInsensitive(t *testing.T) {
	envData := `
export KEY1=val1
Export KEY2=val2
EXPORT KEY3=val3
eXpOrT KEY4=val4
export	KEY5=val5
exporting=not_an_export_prefix
`
	m, err := parseDotEnv(strings.NewReader(envData), nil)
	if err != nil {
		t.Fatalf("parseDotEnv failed on export prefixes: %v", err)
	}
	if m["KEY1"] != "val1" || m["KEY2"] != "val2" || m["KEY3"] != "val3" || m["KEY4"] != "val4" || m["KEY5"] != "val5" {
		t.Fatalf("case-insensitive export failed: %+v", m)
	}
	if m["exporting"] != "not_an_export_prefix" {
		t.Fatalf("exporting key falsely stripped: got %q", m["exporting"])
	}
}

// ============================================================================
// Objective 1: Watcher Concurrency & Reload Verification
// ============================================================================

func TestEmpirical_Watcher_StopInOnChangeCallback(t *testing.T) {
	// Deadlock reproduction attempt: Stop/Close called directly inside onChange callback
	tmpDir, err := ioutil.TempDir("", "confi_watch_deadlock_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "app.yaml")
	if err := ioutil.WriteFile(cfgPath, []byte("port: 8080\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type Config struct {
		Port int `confi:"port"`
	}
	var cfg Config
	var watcher *Watcher
	stopped := make(chan struct{})

	watcher, err = Watch(&cfg, func(err error) {
		// Stop watcher directly within onChange callback!
		watcher.Stop()
		close(stopped)
	}, WithConfigFile(cfgPath), WithWatchInterval(10*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Trigger file change
	time.Sleep(20 * time.Millisecond)
	if err := ioutil.WriteFile(cfgPath, []byte("port: 8081\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	select {
	case <-stopped:
		// Succeeded without deadlock!
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK DETECTED: Watcher.Stop() within onChange did not complete within 3s")
	}

	// Also verify Close() inside onChange
	cfgPath2 := filepath.Join(tmpDir, "app2.yaml")
	if err := ioutil.WriteFile(cfgPath2, []byte("port: 9000\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	var cfg2 Config
	var watcher2 *Watcher
	closed := make(chan struct{})

	watcher2, err = Watch(&cfg2, func(err error) {
		_ = watcher2.Close()
		close(closed)
	}, WithConfigFile(cfgPath2), WithWatchInterval(10*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	if err := ioutil.WriteFile(cfgPath2, []byte("port: 9001\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	select {
	case <-closed:
		// Succeeded without deadlock!
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK DETECTED: Watcher.Close() within onChange did not complete within 3s")
	}
}

func TestEmpirical_Watcher_ZeroFilesErrorRejection(t *testing.T) {
	type NoFileConfig struct {
		Host string `confi:"host"`
		Port int    `confi:"port"`
	}
	var cfg NoFileConfig

	// 1. Package Watch()
	w1, err1 := Watch(&cfg, func(err error) {}, WithArgs(nil))
	if err1 == nil {
		if w1 != nil {
			w1.Stop()
		}
		t.Fatal("expected error on Watch with 0 config files, got nil")
	}
	if w1 != nil {
		t.Fatal("expected nil watcher when error returned")
	}
	if !strings.Contains(err1.Error(), "no configuration files found to watch") {
		t.Fatalf("unexpected error message: %v", err1)
	}

	// 2. Parser.Watch()
	p := NewParser("test", WithArgs(nil))
	w2, err2 := p.Watch(&cfg, func(err error) {})
	if err2 == nil {
		if w2 != nil {
			w2.Stop()
		}
		t.Fatal("expected error on p.Watch with 0 config files, got nil")
	}
	if w2 != nil {
		t.Fatal("expected nil watcher when error returned")
	}
	if !strings.Contains(err2.Error(), "no configuration files found to watch") {
		t.Fatalf("unexpected error message: %v", err2)
	}
}

func TestEmpirical_Watcher_ModTimeSnapshotCaching(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "confi_watch_cache_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.yaml")
	content := []byte("val: 100\n")
	if err := ioutil.WriteFile(filePath, content, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Artificially age the file to > 500ms so cache hit is triggered
	oldTime := time.Now().Add(-2 * time.Second)
	_ = os.Chtimes(filePath, oldTime, oldTime)

	paths := []string{filePath}
	snap1 := takeSnapshots(paths, nil)
	if !snap1[filePath].exists || snap1[filePath].size != int64(len(content)) {
		t.Fatalf("snapshot 1 mismatch: %+v", snap1[filePath])
	}

	// Second snapshot with prev provided: should reuse cached snapshot without reading file!
	snap2 := takeSnapshots(paths, snap1)
	if snap2[filePath].modTime != snap1[filePath].modTime || snap2[filePath].size != snap1[filePath].size {
		t.Fatalf("cached snapshot mismatch: %+v vs %+v", snap1[filePath], snap2[filePath])
	}

	// Verify snapshotsChanged returns false
	if snapshotsChanged(snap1, snap2) {
		t.Fatal("expected snapshotsChanged to return false for cached unchanged file")
	}
}

func TestEmpirical_Watcher_DefaultRestorationOnReload(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "confi_watch_revert_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "revert.yaml")
	// Config file initially overrides both host and timeout
	if err := ioutil.WriteFile(cfgPath, []byte("host: file.override.com\ntimeout: 45s\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type RevertConfig struct {
		Host       string        `confi:"host,default='default-host.local'"`
		Timeout    time.Duration `confi:"timeout,default=10s"`
		BufferSize int           // Programmatic default (set in struct before Watch)
		Sub        struct {
			Tag string `confi:"tag,default='sub-default'"`
		} `confi:"sub"`
	}

	cfg := RevertConfig{
		BufferSize: 4096, // programmatic default
	}

	reloadCh := make(chan error, 5)
	watcher, err := Watch(&cfg, func(e error) {
		reloadCh <- e
	}, WithConfigFile(cfgPath), WithWatchInterval(15*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer watcher.Close()

	if cfg.Host != "file.override.com" || cfg.Timeout != 45*time.Second || cfg.BufferSize != 4096 || cfg.Sub.Tag != "sub-default" {
		t.Fatalf("unexpected initial config: %+v", cfg)
	}

	// Now update config file to remove 'host' and 'timeout'
	time.Sleep(25 * time.Millisecond)
	if err := ioutil.WriteFile(cfgPath, []byte("sub:\n  tag: sub-override\n"), 0600); err != nil {
		t.Fatalf("WriteFile update failed: %v", err)
	}

	select {
	case rErr := <-reloadCh:
		if rErr != nil {
			t.Fatalf("reload error: %v", rErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher reload")
	}

	watcher.RLock()
	gotHost := cfg.Host
	gotTimeout := cfg.Timeout
	gotBufSize := cfg.BufferSize
	gotSubTag := cfg.Sub.Tag
	watcher.RUnlock()

	// Host and Timeout must be RESTORED to defaults!
	if gotHost != "default-host.local" {
		t.Fatalf("expected Host to revert to 'default-host.local', got %q", gotHost)
	}
	if gotTimeout != 10*time.Second {
		t.Fatalf("expected Timeout to revert to 10s, got %v", gotTimeout)
	}
	// Programmatic default preserved!
	if gotBufSize != 4096 {
		t.Fatalf("expected BufferSize=4096 preserved, got %d", gotBufSize)
	}
	// Sub.Tag updated from file!
	if gotSubTag != "sub-override" {
		t.Fatalf("expected Sub.Tag='sub-override', got %q", gotSubTag)
	}
}

// ============================================================================
// Objective 2: Concurrency Stress Harness & Fuzzing Runs
// ============================================================================

func TestEmpirical_Concurrency_StressWatcherRaces(t *testing.T) {
	// High-concurrency race test:
	// Multiple readers calling RLock/RUnlock while config file is modified rapidly
	tmpDir, err := ioutil.TempDir("", "confi_concurrency_race_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "race.yaml")
	if err := ioutil.WriteFile(cfgPath, []byte("counter: 0\nstatus: ready\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	type ConcurrencyConfig struct {
		Counter int    `confi:"counter,default=0"`
		Status  string `confi:"status,default=init"`
	}

	var cfg ConcurrencyConfig
	var reloadCount int64

	w, err := Watch(&cfg, func(err error) {
		atomic.AddInt64(&reloadCount, 1)
	}, WithConfigFile(cfgPath), WithWatchInterval(10*time.Millisecond), WithArgs(nil))
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer w.Close()

	var stopSignal int32
	var wg sync.WaitGroup

	// 8 concurrent readers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for atomic.LoadInt32(&stopSignal) == 0 {
				w.RLock()
				_ = cfg.Counter
				_ = cfg.Status
				w.RUnlock()
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Main writer: modify file 20 times rapidly
	for i := 1; i <= 20; i++ {
		time.Sleep(15 * time.Millisecond)
		content := fmt.Sprintf("counter: %d\nstatus: active_%d\n", i, i)
		_ = ioutil.WriteFile(cfgPath, []byte(content), 0600)
	}

	// Allow loop to catch final reload
	time.Sleep(50 * time.Millisecond)
	atomic.StoreInt32(&stopSignal, 1)
	wg.Wait()

	if atomic.LoadInt64(&reloadCount) == 0 {
		t.Fatal("expected at least one reload during concurrency test")
	}
}

func TestEmpirical_Adversarial_FuzzGenerators(t *testing.T) {
	// Adversarial stress test: Feed random bytes, malformed UTF-8, truncated strings,
	// unbalanced structures into YAML, TOML, DotEnv parsers.
	// Oracle: MUST NOT PANIC under any input.
	rng := rand.New(rand.NewSource(1337))

	corpus := []string{
		"\t\t\t\n",
		"--- \n ... \n ---",
		"[[[[[[[[[[",
		"\"\"\"unterminated",
		"'''unterminated literal",
		"KEY=\"\\$foo",
		"KEY='unclosed",
		"export ",
		"EXPORT \t\t",
		"exporting=10",
		"dsn = raw://host/path;flag=1",
		"a.b.c = [ 1, 2, 3",
		"\\u0041\\uZZZZ",
		"\\U0000000",
		strings.Repeat("[", 150),
		strings.Repeat("{\n", 150),
		strings.Repeat("- \n", 150),
		"null: ~",
		":: invalid",
		"=",
		"=val",
		"k=",
		"# only comment",
		"; only comment",
	}

	// 1. YAML Fuzz Run
	for i := 0; i < 1000; i++ {
		var input string
		if rng.Float32() < 0.5 && len(corpus) > 0 {
			input = corpus[rng.Intn(len(corpus))]
			if rng.Float32() < 0.5 {
				input += fmt.Sprintf("\n%d: %d", rng.Intn(100), rng.Intn(100))
			}
		} else {
			buf := make([]byte, rng.Intn(64))
			rng.Read(buf)
			input = string(buf)
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC on YAML input %q: %v", input, r)
				}
			}()
			_, _ = parseYAML(strings.NewReader(input))
		}()
	}

	// 2. TOML Fuzz Run
	for i := 0; i < 1000; i++ {
		var input string
		if rng.Float32() < 0.5 && len(corpus) > 0 {
			input = corpus[rng.Intn(len(corpus))]
		} else {
			buf := make([]byte, rng.Intn(64))
			rng.Read(buf)
			input = string(buf)
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC on TOML input %q: %v", input, r)
				}
			}()
			_, _ = parseTOML(strings.NewReader(input))
		}()
	}

	// 3. DotEnv Fuzz Run
	for i := 0; i < 1000; i++ {
		var input string
		if rng.Float32() < 0.5 && len(corpus) > 0 {
			input = corpus[rng.Intn(len(corpus))]
		} else {
			buf := make([]byte, rng.Intn(64))
			rng.Read(buf)
			input = string(buf)
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC on DotEnv input %q: %v", input, r)
				}
			}()
			_, _ = parseDotEnv(strings.NewReader(input), os.LookupEnv)
		}()
	}
}
