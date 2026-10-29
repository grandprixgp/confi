//go:build go1.18
// +build go1.18

package confi

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// FuzzParseCLI fuzz tests the command-line tokenizer, flag matcher, short flag
// bundler, and value unmarshaler against arbitrary strings.
func FuzzParseCLI(f *testing.F) {
	seeds := []string{
		"",
		"--host localhost --port 8080 --debug",
		"-h 127.0.0.1 -p 9000 -d",
		"--host=nic.ac --port=80,8080 --debug",
		"--timeout=1m30s --tags=alpha,beta,gamma",
		"--values k1=v1,k2=v2",
		"-10 -- -20",
		"--prefix= data.csv",
		"-fa",
		"-αβ",
		"--debug --no-debug",
		"--unknown-flag=val",
		"pos1 pos2 pos3",
		"=",
		"--",
		"-",
		"--host=\"quoted host\" --tags=\"a, b, c\"",
		"--port=-500 --rate=3.1415",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, cliInput string) {
		type CLIConfig struct {
			Host    string            `confi:"host,short=h"`
			Port    int               `confi:"port,short=p"`
			Debug   bool              `confi:"debug,short=d"`
			Alpha   bool              `confi:"alpha,short=a"`
			Fast    bool              `confi:"fast,short=f"`
			Rune1   bool              `confi:"rune1,short=α"`
			Rune2   bool              `confi:"rune2,short=β"`
			Timeout string            `confi:"timeout"`
			Rate    float64           `confi:"rate"`
			Tags    []string          `confi:"tags"`
			Values  map[string]string `confi:"values"`
			Pos     string            `confi:"pos,pos"`
		}

		tokens := strings.Fields(cliInput)

		// Fuzz with non-strict mode
		var cfg1 CLIConfig
		_ = Parse(&cfg1, WithName("fuzzapp"), WithArgs(tokens), WithStrict(false))

		// Fuzz with strict mode
		var cfg2 CLIConfig
		_ = Parse(&cfg2, WithName("fuzzapp"), WithArgs(tokens), WithStrict(true))
	})
}

// FuzzParseYAML fuzz tests the native YAML subset scanner, mapping/sequence
// decoders, and recursion guards against arbitrary byte payloads.
func FuzzParseYAML(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("host: localhost\nport: 8080\n"),
		[]byte("\t\thost: localhost\n"),
		[]byte("database:\n  host: 127.0.0.1\n  port: 5432\n"),
		[]byte("items:\n  - a\n  - b\n  - c\n"),
		[]byte("items:\n  - nested:\n      host: db1.local\n    name: primary\n"),
		[]byte("- item1\n- item2\n"),
		[]byte("---\nhost: primary\n---\nhost: secondary\n"),
		[]byte("quoted: \"hello \\u0041 \\b\\f world\"\nemoji: \"\\U0001F600\"\n"),
		[]byte("single: 'escaped '' quotes'\n"),
		[]byte("flow_list: [1, 2, 3]\n"),
		[]byte("flow_map: {a: 1, b: 2}\n"),
		[]byte("unclosed_list: [1, 2, \n"),
		[]byte("unclosed_map: {a: 1, \n"),
		[]byte("invalid: \x00\xff\n"),
		[]byte(strings.Repeat("nest:\n  ", 60) + "val: 1\n"),
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Low-level parser call
		_, _ = parseYAML(bytes.NewReader(data))

		// High-level struct binding
		type YAMLConfig struct {
			Host     string            `confi:"host"`
			Port     int               `confi:"port"`
			Items    []string          `confi:"items"`
			Settings map[string]string `confi:"settings"`
		}
		var cfg YAMLConfig
		_ = Parse(&cfg, WithConfigReader(bytes.NewReader(data), FormatYAML), WithArgs(nil))
	})
}

// FuzzParseTOML fuzz tests the native TOML/INI scanner, bracket balancing,
// multiline arrays, multiline strings, and table nesting against arbitrary bytes.
func FuzzParseTOML(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("title = \"TOML Example\"\n"),
		[]byte("port = 8080\nenabled = true\nrate = 1.25\n"),
		[]byte("[database]\nserver = \"192.168.1.1\"\nports = [ 8001, 8002 ]\n"),
		[]byte("[[servers]]\nhost = \"srv1\"\nport = 8080\n\n[[servers]]\nhost = \"srv2\"\nport = 8081\n"),
		[]byte("query = \"\"\"\nSELECT * FROM users\nWHERE id = 1 # comment\n\"\"\"\n"),
		[]byte("note = \"\"\"He said \"hello world\"\"\"\"\n"),
		[]byte("raw = '''\nC:\\Path # literal comment\n'''\n"),
		[]byte("dsn = \"postgres://user:pass@localhost:5432/db;sslmode=disable\"\n"),
		[]byte("[servers.\"us.east\"]\n\"urn:endpoint\" = \"https://us-east.example.com\"\n"),
		[]byte("unicode = \"\\u0041\\u0042\\u0043\"\nctrl = \"\\b\\f\"\n"),
		[]byte("unclosed_arr = [1, 2, \n"),
		[]byte("negative_bracket = ]\n"),
		[]byte("redefine = \"scalar\"\n[redefine]\nval = 1\n"),
		[]byte("arr = [ \"a\", \"b\", \"c\" ]\n"),
		[]byte("invalid: \x00\xff\n"),
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Low-level parser call
		_, _ = parseTOML(bytes.NewReader(data))

		// High-level struct binding
		type TOMLConfig struct {
			Title    string                 `confi:"title"`
			Port     int                    `confi:"port"`
			Enabled  bool                   `confi:"enabled"`
			Ports    []int                  `confi:"ports"`
			Database map[string]interface{} `confi:"database"`
		}
		var cfg TOMLConfig
		_ = Parse(&cfg, WithConfigReader(bytes.NewReader(data), FormatTOML), WithArgs(nil))
	})
}

// FuzzParseDotEnv fuzz tests the native POSIX .env lexer, variable expansion,
// multiline strings, and escape sequences against arbitrary bytes.
func FuzzParseDotEnv(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("HOST=localhost\nPORT=8080\n"),
		[]byte("EXPORT HOST=127.0.0.1\nexport PORT=9090\n"),
		[]byte("CERT=\"line1\nline2\nline3\"\n"),
		[]byte("LITERAL_DOLLAR=\"\\$foo\"\nESCAPED_SLASH=\"\\\\$foo\"\n"),
		[]byte("REF=$VAR_${NOT_EXIST:-fallback}\n"),
		[]byte("PLAIN=hello $CONFI_FUZZ_TEST_VAR\n"),
		[]byte("TRAILING='val'extra\n"),
		[]byte("TRAILING_DBL=\"val\"extra\n"),
		[]byte("UNTERMINATED=\"string\n"),
		[]byte("UNTERMINATED_SINGLE='string\n"),
		[]byte("# full line comment\nKEY=val # inline comment\n"),
		[]byte("EMPTY=\nEMPTY_QUOTED=\"\"\n"),
		[]byte("invalid: \x00\xff\n"),
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Low-level parser call
		_, _ = parseDotEnv(bytes.NewReader(data), os.LookupEnv)

		// High-level struct binding
		type DotEnvConfig struct {
			Host string `confi:"host"`
			Port int    `confi:"port"`
			Cert string `confi:"cert"`
			Ref  string `confi:"ref"`
		}
		var cfg DotEnvConfig
		_ = Parse(&cfg, WithConfigReader(bytes.NewReader(data), FormatEnv), WithArgs(nil))
	})
}
