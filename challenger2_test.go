package confi

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// OBJECTIVE 1: Negative numbers as positional arguments vs short flags
// =============================================================================

type Objective1PosConfig struct {
	Flag   string  `confi:"flag,short=f"`
	Offset int     `confi:"offset"`
	IntPos int     `confi:"intpos,pos"`
	FltPos float64 `confi:"fltpos,pos"`
}

func TestChallenger2_NegativeNumbers_PositionalAndFlags(t *testing.T) {
	// 1a: Negative integers and floats as positional args in strict mode
	t.Run("strict_negative_positionals", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"-42", "-3.1415"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("strict mode rejected negative positionals: %v", err)
		}
		if cfg.IntPos != -42 {
			t.Errorf("expected IntPos -42, got %d", cfg.IntPos)
		}
		if math.Abs(cfg.FltPos-(-3.1415)) > 1e-6 {
			t.Errorf("expected FltPos -3.1415, got %f", cfg.FltPos)
		}
	})

	// 1b: Leading dot negative float "-.75"
	t.Run("leading_dot_negative_float", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"-100", "-.75"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed to parse leading-dot negative float: %v", err)
		}
		if cfg.IntPos != -100 {
			t.Errorf("expected IntPos -100, got %d", cfg.IntPos)
		}
		if math.Abs(cfg.FltPos-(-0.75)) > 1e-6 {
			t.Errorf("expected FltPos -0.75, got %f", cfg.FltPos)
		}
	})

	// 1c: Negative number as flag argument followed by negative positional
	t.Run("negative_flag_arg_and_negative_pos", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"--offset", "-99", "-50", "-2.5"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on negative flag and positionals: %v", err)
		}
		if cfg.Offset != -99 {
			t.Errorf("expected Offset -99, got %d", cfg.Offset)
		}
		if cfg.IntPos != -50 {
			t.Errorf("expected IntPos -50, got %d", cfg.IntPos)
		}
		if math.Abs(cfg.FltPos-(-2.5)) > 1e-6 {
			t.Errorf("expected FltPos -2.5, got %f", cfg.FltPos)
		}
	})

	// 1d: Short flag followed by negative positional
	t.Run("short_flag_then_negative_pos", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"-f", "alpha", "-7", "-0.123"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on short flag followed by negative pos: %v", err)
		}
		if cfg.Flag != "alpha" {
			t.Errorf("expected Flag 'alpha', got %q", cfg.Flag)
		}
		if cfg.IntPos != -7 {
			t.Errorf("expected IntPos -7, got %d", cfg.IntPos)
		}
		if math.Abs(cfg.FltPos-(-0.123)) > 1e-6 {
			t.Errorf("expected FltPos -0.123, got %f", cfg.FltPos)
		}
	})

	// 1e: End of flags delimiter "--" followed by negative numbers
	t.Run("end_of_flags_delimiter_then_negatives", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"--", "-10", "-20.5"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed with -- delimiter: %v", err)
		}
		if cfg.IntPos != -10 {
			t.Errorf("expected IntPos -10, got %d", cfg.IntPos)
		}
		if math.Abs(cfg.FltPos-(-20.5)) > 1e-6 {
			t.Errorf("expected FltPos -20.5, got %f", cfg.FltPos)
		}
	})

	// 1f: True unknown short flag (not a number) MUST be rejected in strict mode
	t.Run("strict_rejects_unknown_short_flag", func(t *testing.T) {
		var cfg Objective1PosConfig
		args := []string{"-z", "-10", "-20.5"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err == nil {
			t.Fatalf("expected strict mode to reject unknown short flag -z, but got nil error")
		}
		if !strings.Contains(err.Error(), "unknown short flag -z") {
			t.Errorf("expected error message mentioning unknown short flag -z, got: %v", err)
		}
	})
}

// =============================================================================
// OBJECTIVE 2: Empty inline flags (`--flag=`)
// =============================================================================

type Objective2InlineConfig struct {
	Str     string   `confi:"str,short=s"`
	Slice   []string `confi:"slice"`
	BoolVal bool     `confi:"boolval,short=b"`
	Pos     string   `confi:"pos,pos"`
}

func TestChallenger2_EmptyInlineFlags(t *testing.T) {
	// 2a: Empty inline long flag `--str=` followed by positional argument
	t.Run("empty_inline_long_string_flag", func(t *testing.T) {
		var cfg Objective2InlineConfig
		args := []string{"--str=", "my_positional"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("unexpected error on --str=: %v", err)
		}
		if cfg.Str != "" {
			t.Errorf("expected Str to be empty string, got %q", cfg.Str)
		}
		if cfg.Pos != "my_positional" {
			t.Errorf("expected Pos to be 'my_positional', got %q", cfg.Pos)
		}
	})

	// 2b: Empty inline short flag `-s=` followed by positional argument
	t.Run("empty_inline_short_string_flag", func(t *testing.T) {
		var cfg Objective2InlineConfig
		args := []string{"-s=", "pos_from_short"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("unexpected error on -s=: %v", err)
		}
		if cfg.Str != "" {
			t.Errorf("expected Str to be empty string, got %q", cfg.Str)
		}
		if cfg.Pos != "pos_from_short" {
			t.Errorf("expected Pos to be 'pos_from_short', got %q", cfg.Pos)
		}
	})

	// 2c: Empty inline boolean flag `--boolval=`
	t.Run("empty_inline_bool_flag", func(t *testing.T) {
		var cfg Objective2InlineConfig
		args := []string{"--boolval=", "pos_bool"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("unexpected error on --boolval=: %v", err)
		}
		if !cfg.BoolVal {
			t.Errorf("expected BoolVal to be true for --boolval=, got false")
		}
		if cfg.Pos != "pos_bool" {
			t.Errorf("expected Pos to be 'pos_bool', got %q", cfg.Pos)
		}
	})

	// 2d: Inline boolean flag with explicit false `--boolval=false`
	t.Run("explicit_false_inline_bool_flag", func(t *testing.T) {
		var cfg Objective2InlineConfig
		args := []string{"--boolval=false", "pos_bool_false"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("unexpected error on --boolval=false: %v", err)
		}
		if cfg.BoolVal {
			t.Errorf("expected BoolVal to be false, got true")
		}
		if cfg.Pos != "pos_bool_false" {
			t.Errorf("expected Pos to be 'pos_bool_false', got %q", cfg.Pos)
		}
	})
}

// =============================================================================
// OBJECTIVE 3: Strict mode rejection of trailing positional tokens after string flags
// =============================================================================

type Objective3NoPosConfig struct {
	Msg    string `confi:"msg,short=m"`
	Number int    `confi:"num,short=n"`
}

func TestChallenger2_StrictModeTrailingPositionalTokens(t *testing.T) {
	// 3a: Strict mode MUST reject trailing tokens after string flags when no positional fields exist
	t.Run("strict_rejection", func(t *testing.T) {
		var cfg Objective3NoPosConfig
		args := []string{"--msg", "hello", "extra_token"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err == nil {
			t.Fatalf("expected strict mode error for unexpected positional 'extra_token', got nil")
		}
		if !strings.Contains(err.Error(), "unexpected positional argument(s): extra_token") {
			t.Errorf("unexpected error text: %v", err)
		}
	})

	// 3b: Non-strict mode should join greedy trailing string arguments
	t.Run("non_strict_joins_string_args", func(t *testing.T) {
		var cfg Objective3NoPosConfig
		args := []string{"--msg", "hello", "extra_token", "world"}
		err := Parse(&cfg, WithArgs(args), WithStrict(false))
		if err != nil {
			t.Fatalf("non-strict mode returned unexpected error: %v", err)
		}
		if cfg.Msg != "hello extra_token world" {
			t.Errorf("expected Msg to be 'hello extra_token world', got %q", cfg.Msg)
		}
	})
}

// =============================================================================
// OBJECTIVE 4: Dynamic config loading on subcommands and subcommand flag isolation
// =============================================================================

type Objective4Subcmd struct {
	ConfigFile  string `confi:"config,config"`
	WorkerCount int    `confi:"workers,default=2"`
	SubFile     string `confi:"subfile"`
}

type Objective4RootConfig struct {
	Host   string           `confi:"host,default=localhost"`
	RunSub Objective4Subcmd `confi:"run,subcommand"`
}

func TestChallenger2_SubcommandDynamicConfigAndIsolation(t *testing.T) {
	// 4a: Dynamic config file provided directly on subcommand
	t.Run("subcommand_dynamic_config_loading", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "confi_challenger_")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		subYAML := []byte("workers: 42\nsubfile: loaded_by_sub\n")
		tmpFile := filepath.Join(tmpDir, "subconfig.yaml")
		if err := ioutil.WriteFile(tmpFile, subYAML, 0644); err != nil {
			t.Fatalf("failed to write tmp subconfig: %v", err)
		}

		var cfg Objective4RootConfig
		args := []string{"run", "--config", tmpFile}
		err = Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed to parse subcommand with dynamic config: %v", err)
		}
		if cfg.RunSub.WorkerCount != 42 {
			t.Errorf("expected WorkerCount 42 from subcommand config, got %d", cfg.RunSub.WorkerCount)
		}
		if cfg.RunSub.SubFile != "loaded_by_sub" {
			t.Errorf("expected SubFile 'loaded_by_sub', got %q", cfg.RunSub.SubFile)
		}
		// Root should remain default
		if cfg.Host != "localhost" {
			t.Errorf("expected Host 'localhost', got %q", cfg.Host)
		}
	})

	// 4b: Flag isolation: subcommand flags should not be accepted at root in strict mode
	t.Run("subcommand_flag_isolation_at_root", func(t *testing.T) {
		var cfg Objective4RootConfig
		args := []string{"--workers", "10", "run"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err == nil {
			t.Fatalf("expected error when passing subcommand flag at root in strict mode, got nil")
		}
		if !strings.Contains(err.Error(), "unknown flag --workers") {
			t.Errorf("expected unknown flag --workers error, got: %v", err)
		}
	})

	// 4c: Flag isolation: root flags should not be accepted after subcommand in strict mode
	t.Run("root_flag_isolation_in_subcommand", func(t *testing.T) {
		var cfg Objective4RootConfig
		args := []string{"run", "--host", "remote"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err == nil {
			t.Fatalf("expected error when passing root flag after subcommand in strict mode, got nil")
		}
		if !strings.Contains(err.Error(), "unknown flag --host") {
			t.Errorf("expected unknown flag --host error, got: %v", err)
		}
	})
}

// =============================================================================
// OBJECTIVE 5: 64-bit integer min/max precision (`uint64` values exceeding 2^53 up to `math.MaxUint64`)
// =============================================================================

type Objective5PrecisionConfig struct {
	// 2^53 = 9007199254740992
	// 2^53 + 1 = 9007199254740993
	LargeUint uint64 `confi:"large_uint,min=9007199254740993,max=18446744073709551615"`
	LargeInt  int64  `confi:"large_int,min=9007199254740993,max=9223372036854775807"`
}

func TestChallenger2_64BitIntegerPrecision(t *testing.T) {
	// 5a: Value exactly at min (2^53 + 1) should pass
	t.Run("uint64_exact_min_passes", func(t *testing.T) {
		var cfg Objective5PrecisionConfig
		args := []string{"--large_uint=9007199254740993", "--large_int=9007199254740993"}
		err := Parse(&cfg, WithArgs(args))
		if err != nil {
			t.Fatalf("unexpected validation error at exact min: %v", err)
		}
		if cfg.LargeUint != 9007199254740993 {
			t.Errorf("expected LargeUint 9007199254740993, got %d", cfg.LargeUint)
		}
	})

	// 5b: Value 1 below min (2^53 = 9007199254740992) MUST fail validation
	t.Run("uint64_below_min_fails", func(t *testing.T) {
		var cfg Objective5PrecisionConfig
		args := []string{"--large_uint=9007199254740992"}
		err := Parse(&cfg, WithArgs(args))
		if err == nil {
			t.Fatalf("expected validation failure for uint64 9007199254740992 < 9007199254740993")
		}
		if !strings.Contains(err.Error(), "less than minimum") {
			t.Errorf("expected 'less than minimum' error, got: %v", err)
		}
	})

	// 5c: Value at math.MaxUint64 (18446744073709551615) should pass max validation
	t.Run("uint64_max_passes", func(t *testing.T) {
		var cfg Objective5PrecisionConfig
		args := []string{"--large_uint=18446744073709551615"}
		err := Parse(&cfg, WithArgs(args))
		if err != nil {
			t.Fatalf("unexpected validation failure for math.MaxUint64: %v", err)
		}
		if cfg.LargeUint != math.MaxUint64 {
			t.Errorf("expected LargeUint %d, got %d", uint64(math.MaxUint64), cfg.LargeUint)
		}
	})

	// 5d: Int64 at math.MaxInt64 (9223372036854775807) should pass
	t.Run("int64_max_passes", func(t *testing.T) {
		var cfg Objective5PrecisionConfig
		args := []string{"--large_int=9223372036854775807"}
		err := Parse(&cfg, WithArgs(args))
		if err != nil {
			t.Fatalf("unexpected validation failure for math.MaxInt64: %v", err)
		}
		if cfg.LargeInt != math.MaxInt64 {
			t.Errorf("expected LargeInt %d, got %d", int64(math.MaxInt64), cfg.LargeInt)
		}
	})
}

// =============================================================================
// OBJECTIVE 6: `Validator` interface invocation on slices, arrays, maps, and custom scalar types
// =============================================================================

type ValidatingScalar int

func (v ValidatingScalar) Validate() error {
	if v < 1000 {
		return fmt.Errorf("validating scalar %d must be >= 1000", v)
	}
	return nil
}

type ValidatingItem struct {
	Name string `confi:"name"`
}

func (vi ValidatingItem) Validate() error {
	if strings.HasPrefix(vi.Name, "bad_") {
		return fmt.Errorf("invalid name %q: cannot start with bad_", vi.Name)
	}
	return nil
}

type Objective6ValidatorConfig struct {
	Scalar   ValidatingScalar          `confi:"scalar"`
	Items    []ValidatingItem          `confi:"items"`
	ItemArr  [2]ValidatingItem         `confi:"item_arr"`
	ItemMap  map[string]ValidatingItem `confi:"item_map"`
}

func TestChallenger2_ValidatorInterfaceInvocations(t *testing.T) {
	// 6a: Custom scalar type implementing Validator
	t.Run("custom_scalar_validation_failure", func(t *testing.T) {
		var cfg Objective6ValidatorConfig
		args := []string{"--scalar=500"}
		err := Parse(&cfg, WithArgs(args))
		if err == nil {
			t.Fatalf("expected error from scalar validator, got nil")
		}
		if !strings.Contains(err.Error(), "must be >= 1000") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("custom_scalar_validation_success", func(t *testing.T) {
		var cfg Objective6ValidatorConfig
		args := []string{"--scalar=1500"}
		err := Parse(&cfg, WithArgs(args))
		if err != nil {
			t.Fatalf("unexpected error for valid scalar: %v", err)
		}
		if cfg.Scalar != 1500 {
			t.Errorf("expected Scalar 1500, got %d", cfg.Scalar)
		}
	})

	// 6b: Slice elements implementing Validator
	t.Run("slice_element_validator", func(t *testing.T) {
		cfg := Objective6ValidatorConfig{
			Scalar: 1200,
			Items: []ValidatingItem{
				{Name: "good_1"},
				{Name: "bad_item"},
			},
		}
		err := Parse(&cfg, WithArgs([]string{}))
		if err == nil {
			t.Fatalf("expected slice element validator to fail on bad_item")
		}
		if !strings.Contains(err.Error(), "cannot start with bad_") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// 6c: Array elements implementing Validator
	t.Run("array_element_validator", func(t *testing.T) {
		cfg := Objective6ValidatorConfig{
			Scalar: 1200,
			ItemArr: [2]ValidatingItem{
				{Name: "good_array"},
				{Name: "bad_array"},
			},
		}
		err := Parse(&cfg, WithArgs([]string{}))
		if err == nil {
			t.Fatalf("expected array element validator to fail on bad_array")
		}
		if !strings.Contains(err.Error(), "cannot start with bad_") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// 6d: Map values implementing Validator
	t.Run("map_element_validator", func(t *testing.T) {
		cfg := Objective6ValidatorConfig{
			Scalar: 1200,
			ItemMap: map[string]ValidatingItem{
				"entry1": {Name: "good_map"},
				"entry2": {Name: "bad_map"},
			},
		}
		err := Parse(&cfg, WithArgs([]string{}))
		if err == nil {
			t.Fatalf("expected map element validator to fail on bad_map")
		}
		if !strings.Contains(err.Error(), "cannot start with bad_") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// =============================================================================
// OBJECTIVE 7: SubcommandUsage public API and empty string default formatting (`[default: ""]`)
// =============================================================================

type Objective7Sub struct {
	File    string `confi:"file,default=output.log,usage=Target output file"`
	EmptyDef string `confi:"empty_def,default=,usage=Optional empty default"`
}

type Objective7Root struct {
	Host     string        `confi:"host,default=localhost,usage=Server hostname"`
	EmptyStr string        `confi:"empty_str,default=\"\",usage=Empty string default"`
	Sub      Objective7Sub `confi:"subcmd,subcommand,usage=Subcommand demonstration"`
}

func TestChallenger2_SubcommandUsageAndEmptyDefaults(t *testing.T) {
	// 7a: Verify empty string default displays [default: ""] in root usage
	t.Run("empty_string_default_in_root_usage", func(t *testing.T) {
		usageStr, err := Usage(&Objective7Root{}, WithName("mytestapp"))
		if err != nil {
			t.Fatalf("Usage() returned error: %v", err)
		}
		if !strings.Contains(usageStr, `[default: ""]`) {
			t.Errorf("expected usage to contain `[default: \"\"]`, got:\n%s", usageStr)
		}
		if !strings.Contains(usageStr, `[default: localhost]`) {
			t.Errorf("expected usage to contain `[default: localhost]`, got:\n%s", usageStr)
		}
	})

	// 7b: SubcommandUsage public API function
	t.Run("subcommand_usage_api", func(t *testing.T) {
		subUsage, err := SubcommandUsage(&Objective7Root{}, "subcmd", WithName("mytestapp"))
		if err != nil {
			t.Fatalf("SubcommandUsage returned error: %v", err)
		}
		if !strings.Contains(subUsage, "Usage: mytestapp subcmd [flags]") {
			t.Errorf("expected usage line for subcommand, got:\n%s", subUsage)
		}
		if !strings.Contains(subUsage, `[default: ""]`) {
			t.Errorf("expected subcommand usage to format empty default as `[default: \"\"]`, got:\n%s", subUsage)
		}
		if !strings.Contains(subUsage, `[default: output.log]`) {
			t.Errorf("expected subcommand usage to format output.log default, got:\n%s", subUsage)
		}
	})

	// 7c: SubcommandUsage with Parser instance and WriteSubcommandUsage
	t.Run("parser_subcommand_usage_and_writer", func(t *testing.T) {
		p := NewParser("mytestapp")
		var buf bytes.Buffer
		err := p.WriteSubcommandUsage(&buf, &Objective7Root{}, "subcmd")
		if err != nil {
			t.Fatalf("WriteSubcommandUsage returned error: %v", err)
		}
		if buf.Len() == 0 {
			t.Errorf("expected non-empty output from WriteSubcommandUsage")
		}

		// Unknown subcommand should return error
		err = p.WriteSubcommandUsage(&buf, &Objective7Root{}, "nonexistent")
		if err == nil {
			t.Errorf("expected error for unknown subcommand, got nil")
		}
	})
}

// =============================================================================
// OBJECTIVE 8: Multi-shell completion script escaping and quoting for apps with spaces/dashes
// =============================================================================

type Objective8App struct {
	Host  string `confi:"host,short=h,usage=Target host"`
	Env   string `confi:"env,oneof=prod|staging|dev,usage=Target environment"`
	Sub   Objective7Sub `confi:"sub,subcommand,usage=Subcommand"`
}

func TestChallenger2_ShellCompletionEscapingAndQuoting(t *testing.T) {
	appName := "my-complex app's tool"

	// 8a: Bash completion quoting
	t.Run("bash_completion_quoting", func(t *testing.T) {
		script, err := Completion("bash", &Objective8App{}, WithName(appName))
		if err != nil {
			t.Fatalf("Completion(bash) failed: %v", err)
		}
		// Function identifier must be sanitized
		if !strings.Contains(script, "_my_complex_app_s_tool_completions()") {
			t.Errorf("expected sanitized bash function identifier, got:\n%s", script)
		}
		// Registration must quote the application name
		if !strings.Contains(script, fmt.Sprintf("complete -F _my_complex_app_s_tool_completions %q", appName)) {
			t.Errorf("expected quoted appName in bash complete registration, got:\n%s", script)
		}
	})

	// 8b: Zsh completion quoting
	t.Run("zsh_completion_quoting", func(t *testing.T) {
		script, err := Completion("zsh", &Objective8App{}, WithName(appName))
		if err != nil {
			t.Fatalf("Completion(zsh) failed: %v", err)
		}
		if !strings.Contains(script, fmt.Sprintf("compdef _my_complex_app_s_tool %q", appName)) {
			t.Errorf("expected quoted appName in zsh compdef registration, got:\n%s", script)
		}
	})

	// 8c: Fish completion quoting
	t.Run("fish_completion_quoting", func(t *testing.T) {
		script, err := Completion("fish", &Objective8App{}, WithName(appName))
		if err != nil {
			t.Fatalf("Completion(fish) failed: %v", err)
		}
		if !strings.Contains(script, fmt.Sprintf("complete -c %q", appName)) {
			t.Errorf("expected quoted appName in fish complete registration, got:\n%s", script)
		}
	})

	// 8d: PowerShell completion quoting
	t.Run("powershell_completion_quoting", func(t *testing.T) {
		script, err := Completion("powershell", &Objective8App{}, WithName(appName))
		if err != nil {
			t.Fatalf("Completion(powershell) failed: %v", err)
		}
		// PowerShell escapes single quotes as ''
		escaped := strings.Replace(appName, "'", "''", -1)
		expectedReg := fmt.Sprintf("Register-ArgumentCompleter -Native -CommandName '%s' -ScriptBlock {", escaped)
		if !strings.Contains(script, expectedReg) {
			t.Errorf("expected escaped appName in PowerShell Register-ArgumentCompleter, got:\n%s", script)
		}
	})
}

// =============================================================================
// ADVANCED ADVERSARIAL STRESS TEST SUITE
// =============================================================================

type AdvVector1Config struct {
	PosSlice []int   `confi:"pos_slice,pos"`
	Offset   int     `confi:"offset,short=o"`
	FloatVal float64 `confi:"float_val"`
}

func TestChallenger2_AdvVector1_NegativeNumbersAdversarial(t *testing.T) {
	// 1. Positional slice collecting multiple negative integers
	t.Run("positional_slice_multiple_negatives", func(t *testing.T) {
		var cfg AdvVector1Config
		args := []string{"-10", "-20", "-30", "-40"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on negative integers into positional slice: %v", err)
		}
		if len(cfg.PosSlice) != 4 || cfg.PosSlice[0] != -10 || cfg.PosSlice[1] != -20 || cfg.PosSlice[2] != -30 || cfg.PosSlice[3] != -40 {
			t.Errorf("unexpected PosSlice values: %v", cfg.PosSlice)
		}
	})

	// 2. Short flag with inline negative number vs spaced negative number
	t.Run("short_flag_inline_negative", func(t *testing.T) {
		var cfg AdvVector1Config
		args := []string{"-o=-77", "-15"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on short flag inline negative: %v", err)
		}
		if cfg.Offset != -77 {
			t.Errorf("expected Offset -77, got %d", cfg.Offset)
		}
		if len(cfg.PosSlice) != 1 || cfg.PosSlice[0] != -15 {
			t.Errorf("expected PosSlice [-15], got %v", cfg.PosSlice)
		}
	})

	// 3. "-0" and "-0.0"
	t.Run("negative_zero_values", func(t *testing.T) {
		var cfg AdvVector1Config
		args := []string{"--float_val=-0.0", "-0"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on -0 and -0.0: %v", err)
		}
		if len(cfg.PosSlice) != 1 || cfg.PosSlice[0] != 0 {
			t.Errorf("expected PosSlice [0], got %v", cfg.PosSlice)
		}
	})
}

type AdvVector2Config struct {
	S1  string `confi:"s1"`
	S2  string `confi:"s2"`
	S3  string `confi:"s3"`
	Num int    `confi:"num"`
	Pos string `confi:"pos,pos"`
}

func TestChallenger2_AdvVector2_ChainedEmptyInlineAndMalformed(t *testing.T) {
	// Chained empty inline flags
	t.Run("chained_empty_inline_flags", func(t *testing.T) {
		var cfg AdvVector2Config
		args := []string{"--s1=", "--s2=", "--s3=set_value", "my_positional"}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err != nil {
			t.Fatalf("failed on chained empty flags: %v", err)
		}
		if cfg.S1 != "" {
			t.Errorf("expected S1 '', got %q", cfg.S1)
		}
		if cfg.S2 != "" {
			t.Errorf("expected S2 '', got %q", cfg.S2)
		}
		if cfg.S3 != "set_value" {
			t.Errorf("expected S3 'set_value', got %q", cfg.S3)
		}
		if cfg.Pos != "my_positional" {
			t.Errorf("expected Pos 'my_positional', got %q", cfg.Pos)
		}
	})

	// Empty inline int flag (--num=) should return error
	t.Run("empty_inline_int_fails", func(t *testing.T) {
		var cfg AdvVector2Config
		args := []string{"--num="}
		err := Parse(&cfg, WithArgs(args), WithStrict(true))
		if err == nil {
			t.Fatalf("expected error on --num= for int field, got nil")
		}
	})
}

type AdvVector3Config struct {
	Msg string `confi:"msg"`
	Pos string `confi:"pos,pos"`
}

func TestChallenger2_AdvVector3_StrictModeExcessPositional(t *testing.T) {
	// 1 Positional configured, but 2 provided
	var cfg AdvVector3Config
	args := []string{"--msg", "hello", "first_pos", "second_pos"}
	err := Parse(&cfg, WithArgs(args), WithStrict(true))
	if err == nil {
		t.Fatalf("expected strict error for excess positional argument, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected positional argument(s): second_pos") {
		t.Errorf("unexpected error text: %v", err)
	}
}

type AdvVector4Subcmd struct {
	ConfigFile string `confi:"config,config"`
	Timeout    string `confi:"timeout,default=10s"`
}

type AdvVector4RootConfig struct {
	Timeout string           `confi:"timeout,default=5s"`
	Worker  AdvVector4Subcmd `confi:"worker,subcommand"`
}

func TestChallenger2_AdvVector4_SubcommandConfigStrictnessAndIndependence(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "confi_sub_adv_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rootCfgPath := filepath.Join(tmpDir, "root.json")
	if err := ioutil.WriteFile(rootCfgPath, []byte(`{"timeout": "3s"}`), 0644); err != nil {
		t.Fatalf("WriteFile root: %v", err)
	}

	subCfgPath := filepath.Join(tmpDir, "worker.yaml")
	if err := ioutil.WriteFile(subCfgPath, []byte("timeout: 99s\n"), 0644); err != nil {
		t.Fatalf("WriteFile worker: %v", err)
	}

	var cfg AdvVector4RootConfig
	args := []string{"worker", "--config", subCfgPath}
	err = Parse(&cfg, WithConfigFile(rootCfgPath), WithArgs(args), WithStrict(true))
	if err != nil {
		t.Fatalf("failed to parse with root and sub config: %v", err)
	}
	if cfg.Timeout != "3s" {
		t.Errorf("expected root Timeout '3s', got %q", cfg.Timeout)
	}
	if cfg.Worker.Timeout != "99s" {
		t.Errorf("expected worker Timeout '99s', got %q", cfg.Worker.Timeout)
	}

	// Subcommand config with unknown key in strict mode
	badSubCfgPath := filepath.Join(tmpDir, "bad_worker.yaml")
	if err := ioutil.WriteFile(badSubCfgPath, []byte("unknown_sub_key: true\n"), 0644); err != nil {
		t.Fatalf("WriteFile bad worker: %v", err)
	}
	var cfg2 AdvVector4RootConfig
	err = Parse(&cfg2, WithArgs([]string{"worker", "--config", badSubCfgPath}), WithStrict(true))
	if err == nil {
		t.Fatalf("expected strict error for unknown key in subcommand config, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected unknown config key error, got: %v", err)
	}
}

type AdvVector5Config struct {
	MinInt64 int64  `confi:"min_int64,min=-9223372036854775808,max=0"`
	MaxUint  uint64 `confi:"max_uint,min=18446744073709551614,max=18446744073709551615"`
	ZeroReq  uint64 `confi:"zero_req,required,min=0"`
}

func TestChallenger2_AdvVector5_ExtremesPrecisionAndExplicitZero(t *testing.T) {
	// min int64 (-9223372036854775808)
	t.Run("min_int64_exact", func(t *testing.T) {
		var cfg AdvVector5Config
		args := []string{"--min_int64=-9223372036854775808", "--max_uint=18446744073709551614", "--zero_req=0"}
		err := Parse(&cfg, WithArgs(args))
		if err != nil {
			t.Fatalf("unexpected validation error at min_int64 / explicit zero: %v", err)
		}
		if cfg.MinInt64 != math.MinInt64 {
			t.Errorf("expected MinInt64 %d, got %d", int64(math.MinInt64), cfg.MinInt64)
		}
		if cfg.ZeroReq != 0 {
			t.Errorf("expected ZeroReq 0, got %d", cfg.ZeroReq)
		}
	})

	// max_uint below min
	t.Run("max_uint_below_min_fails", func(t *testing.T) {
		var cfg AdvVector5Config
		args := []string{"--max_uint=18446744073709551613", "--zero_req=1"}
		err := Parse(&cfg, WithArgs(args))
		if err == nil {
			t.Fatalf("expected error for max_uint 18446744073709551613 < 18446744073709551614")
		}
	})
}

type AdvCyclicNode struct {
	Val  int            `confi:"val"`
	Next *AdvCyclicNode `confi:"-"`
}

func (n *AdvCyclicNode) Validate() error {
	if n.Val < 0 {
		return fmt.Errorf("node val cannot be negative: %d", n.Val)
	}
	return nil
}

type AdvVector6Config struct {
	PtrSlice []*ValidatingItem          `confi:"ptr_slice"`
	PtrMap   map[string]*ValidatingItem `confi:"ptr_map"`
	Node     *AdvCyclicNode             `confi:"-"`
}

func TestChallenger2_AdvVector6_NilPointersAndCyclicValidator(t *testing.T) {
	// Slice with nil pointers and valid items
	t.Run("slice_with_nil_pointers", func(t *testing.T) {
		cfg := AdvVector6Config{
			PtrSlice: []*ValidatingItem{
				nil,
				{Name: "good_1"},
				nil,
			},
		}
		err := Parse(&cfg, WithArgs([]string{}))
		if err != nil {
			t.Fatalf("unexpected error validating slice with nil pointers: %v", err)
		}
	})

	// Map with nil pointers
	t.Run("map_with_nil_pointers", func(t *testing.T) {
		cfg := AdvVector6Config{
			PtrMap: map[string]*ValidatingItem{
				"k1": nil,
				"k2": {Name: "good_2"},
			},
		}
		err := Parse(&cfg, WithArgs([]string{}))
		if err != nil {
			t.Fatalf("unexpected error validating map with nil pointers: %v", err)
		}
	})

	// Cyclic structure terminates cleanly without stack overflow
	t.Run("cyclic_struct_validator_terminates", func(t *testing.T) {
		nodeA := &AdvCyclicNode{Val: 1}
		nodeB := &AdvCyclicNode{Val: 2}
		nodeA.Next = nodeB
		nodeB.Next = nodeA

		cfg := AdvVector6Config{Node: nodeA}
		err := Parse(&cfg, WithArgs([]string{}))
		if err != nil {
			t.Fatalf("unexpected error on cyclic struct: %v", err)
		}

		// Cyclic structure with failing validation
		nodeFail := &AdvCyclicNode{Val: -5}
		nodeA.Next = nodeFail
		nodeFail.Next = nodeA
		err = Parse(&cfg, WithArgs([]string{}))
		if err == nil {
			t.Fatalf("expected error from cyclic node validation, got nil")
		}
		if !strings.Contains(err.Error(), "node val cannot be negative") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestChallenger2_AdvVector7_SubcommandUsageCaseInsensitive(t *testing.T) {
	var root Objective7Root
	// Mixed case and spaces in subcommand name
	usage, err := SubcommandUsage(&root, "  SUBCMD  ", WithName("myapp"))
	if err != nil {
		t.Fatalf("SubcommandUsage failed with whitespace/mixed case: %v", err)
	}
	if !strings.Contains(usage, "Usage: myapp subcmd [flags]") {
		t.Errorf("expected subcommand usage header, got:\n%s", usage)
	}
}

func TestChallenger2_AdvVector8_CompletionSpecialCharacters(t *testing.T) {
	appName := "test-tool $special [v1]"
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		script, err := Completion(shell, &Objective8App{}, WithName(appName))
		if err != nil {
			t.Errorf("Completion(%s) failed for special appName: %v", shell, err)
		}
		if len(script) == 0 {
			t.Errorf("Completion(%s) returned empty script", shell)
		}
	}
}


