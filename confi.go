// Package confi provides a zero-dependency command-line flag, subcommand, environment variable,
// and configuration file (JSON, YAML, TOML, INI, .env) parser for Go structs.
package confi

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// ErrHelp is returned by Parse when -h, --help, or a completion flag is invoked.
var ErrHelp = errors.New("confi: help requested")

type valueSource int

const (
	sourceNone valueSource = iota
	sourceDefault
	sourceConfig
	sourceEnv
	sourceCLI
)

// ParseError represents an error that occurred during schema extraction, file loading, or value parsing.
type ParseError struct {
	Field   string
	Message string
	Err     error
}

func (e *ParseError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("confi: field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("confi: %s", e.Message)
}

// Unwrap returns the underlying error.
func (e *ParseError) Unwrap() error {
	return e.Err
}

// ValidationError is returned when one or more required fields are missing after parsing all sources.
type ValidationError struct {
	MissingFields []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("confi: missing required field(s): %s", strings.Join(e.MissingFields, ", "))
}

// Parser holds configuration settings for parsing structs.
type Parser struct {
	name           string
	description    string
	prefix         string
	shortPrefix    string
	args           []string
	hasCustomArgs  bool
	envPrefix      string
	autoEnv        bool
	lookupEnv      func(string) (string, bool)
	configSources  []configSource
	configFlag     string
	completionFlag string
	output         io.Writer
	errOutput      io.Writer
	helpEnabled    bool
	strict         bool
	autoRun        bool
	watchInterval  time.Duration
	activeCommand  string
	exitFunc       func(int)
}

// NewParser creates a new Parser with the given application name and functional options.
func NewParser(name string, opts ...Option) *Parser {
	defaultName := name
	if defaultName == "" && len(os.Args) > 0 {
		defaultName = filepath.Base(os.Args[0])
	}
	p := &Parser{
		name:          defaultName,
		prefix:        "--",
		shortPrefix:   "-",
		lookupEnv:     os.LookupEnv,
		output:        os.Stdout,
		errOutput:     os.Stderr,
		helpEnabled:   true,
		watchInterval: 500 * time.Millisecond,
		exitFunc:      os.Exit,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// ActiveCommand returns the name of the subcommand invoked during the most recent Parse call,
// or an empty string if no subcommand was invoked.
func (p *Parser) ActiveCommand() string {
	return p.activeCommand
}

// Parse populates target (a non-nil pointer to a struct) from defaults, config files,
// environment variables, and command-line arguments according to the provided options.
func Parse(target interface{}, opts ...Option) error {
	p := NewParser("", opts...)
	return p.Parse(target)
}

// MustParse calls Parse and exits the process if an error occurs (exiting with code 0 on ErrHelp
// or code 1 on any other error).
func MustParse(target interface{}, opts ...Option) {
	p := NewParser("", opts...)
	if err := p.Parse(target); err != nil {
		if err == ErrHelp {
			p.exitFunc(0)
			return
		}
		errW := p.errOutput
		if errW == nil {
			errW = p.output
		}
		fmt.Fprintln(errW, err)
		p.exitFunc(1)
	}
}

// Parse populates target using the Parser's configured options and arguments.
func (p *Parser) Parse(target interface{}) error {
	args := p.args
	if !p.hasCustomArgs {
		if len(os.Args) > 1 {
			args = os.Args[1:]
		} else {
			args = nil
		}
	}
	return p.ParseArgs(target, args)
}

// ParseArgs populates target using the Parser's configured options and the explicit args slice.
func (p *Parser) ParseArgs(target interface{}, args []string) error {
	p.activeCommand = ""
	schema, rootVal, err := extractSchema(target)
	if err != nil {
		return err
	}

	if err := p.parseIntoSchema(schema, rootVal, args, "", true); err != nil {
		return err
	}

	if err := invokeCustomValidators(rootVal); err != nil {
		return err
	}

	if p.autoRun {
		return p.RunCommand(target)
	}
	return nil
}

// RunCommand executes the Run() error method on the active subcommand if it implements Runner,
// or on target itself if target implements Runner.
func (p *Parser) RunCommand(target interface{}) error {
	schema, rootVal, err := extractSchema(target)
	if err != nil {
		return err
	}

	if p.activeCommand != "" {
		if subMeta, ok := schema.subcommands[strings.ToLower(p.activeCommand)]; ok {
			if subVal, ok := resolveIndexPath(rootVal, subMeta.indexPath, false); ok {
				if r, ok := extractRunner(subVal); ok {
					return r.Run()
				}
				return fmt.Errorf("confi: subcommand %q does not implement Runner", p.activeCommand)
			}
			return nil
		}
		return nil
	}

	if r, ok := extractRunner(rootVal); ok {
		return r.Run()
	}
	return nil
}

func extractRunner(v reflect.Value) (Runner, bool) {
	if !v.IsValid() {
		return nil, false
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, false
		}
		if v.Type().Implements(runnerType) {
			return v.Interface().(Runner), true
		}
		v = v.Elem()
	}
	if v.CanAddr() && v.Addr().Type().Implements(runnerType) {
		return v.Addr().Interface().(Runner), true
	}
	if v.Type().Implements(runnerType) {
		return v.Interface().(Runner), true
	}
	return nil, false
}

func (p *Parser) parseIntoSchema(schema *structSchema, rootVal reflect.Value, args []string, subcmdName string, allowConfigFiles bool) error {
	cliRes, err := p.parseCLI(args, schema)
	if err != nil {
		return err
	}

	if cliRes.completionShell != "" {
		if err := p.writeSchemaCompletion(p.output, cliRes.completionShell, schema); err != nil {
			return err
		}
		return ErrHelp
	}

	if cliRes.helpRequested {
		desc := p.description
		if subcmdName != "" {
			desc = ""
		}
		_ = p.writeSchemaUsage(p.output, schema, subcmdName, desc)
		return ErrHelp
	}

	sources := make(map[*fieldMeta]valueSource)

	// 1. Apply defaults for fields that are currently zero-valued
	for _, meta := range schema.fields {
		if !meta.hasDefault {
			continue
		}
		fv, ok := meta.fieldValue(rootVal, false)
		if ok && !isZeroValue(fv) {
			continue
		}
		fv, _ = meta.fieldValue(rootVal, true)
		if err := assignFromString(fv, meta.defaultValue, meta.sep, false); err != nil {
			return &ParseError{
				Field:   meta.fieldName,
				Message: fmt.Sprintf("invalid default value %q: %v", meta.defaultValue, err),
				Err:     err,
			}
		}
		sources[meta] = sourceDefault
	}

	// 2. Collect and load config sources (for root struct or subcommand)
	if allowConfigFiles {
		var allSources []configSource
		if subcmdName == "" {
			allSources = make([]configSource, len(p.configSources))
			copy(allSources, p.configSources)
		}

		if cliRes.configFilePath != "" {
			allSources = append(allSources, configSource{
				path:   cliRes.configFilePath,
				format: FormatAuto,
			})
		}

		if schema.configFileField != nil && cliRes.configFilePath == "" {
			if dynPath := p.resolveConfigFieldPath(schema.configFileField, rootVal, cliRes); dynPath != "" {
				allSources = append(allSources, configSource{
					path:   dynPath,
					format: FormatAuto,
				})
			}
		}

		for _, src := range allSources {
			if err := p.loadConfigSource(src, schema, rootVal, sources); err != nil {
				return err
			}
		}
	}

	// 3. Apply environment variables
	for _, meta := range schema.fields {
		candidates := p.envCandidatesForField(meta, p.autoEnv)
		for _, envKey := range candidates {
			if val, ok := p.lookupEnv(envKey); ok {
				fv, _ := meta.fieldValue(rootVal, true)
				if err := assignFromString(fv, val, meta.sep, false); err != nil {
					return &ParseError{
						Field:   meta.fieldName,
						Message: fmt.Sprintf("invalid environment variable %s=%q: %v", envKey, val, err),
						Err:     err,
					}
				}
				sources[meta] = sourceEnv
				break
			}
		}
	}

	// 4. Apply CLI flags
	for _, pf := range cliRes.flags {
		if pf.meta == nil {
			continue
		}
		fv, _ := pf.meta.fieldValue(rootVal, true)
		appendMode := sources[pf.meta] == sourceCLI
		if err := assignFromStrings(fv, pf.values, pf.meta.sep, appendMode); err != nil {
			return &ParseError{
				Field:   pf.meta.fieldName,
				Message: fmt.Sprintf("invalid value for flag %q: %v", pf.name, err),
				Err:     err,
			}
		}
		sources[pf.meta] = sourceCLI
	}

	// 5. Apply positional CLI arguments
	if err := p.applyPositionalArgs(cliRes.positional, schema, rootVal, sources); err != nil {
		return err
	}

	// 6. If a subcommand was invoked, parse into the subcommand struct first (so subcommand --help works even if global required fields are checked or after)
	if cliRes.subcommand != nil {
		p.activeCommand = cliRes.subcommand.name
		subVal, _ := resolveIndexPath(rootVal, cliRes.subcommand.indexPath, true)
		if subVal.Kind() == reflect.Ptr {
			if subVal.IsNil() {
				subVal.Set(reflect.New(subVal.Type().Elem()))
			}
			subVal = subVal.Elem()
		}
		if err := p.parseIntoSchema(cliRes.subcommand.schema, subVal, cliRes.subcommandArgs, cliRes.subcommand.name, true); err != nil {
			return err
		}
	}

	// 7. Validate required fields, constraint tags (oneof, min, max, pattern), and custom Validator
	return validateSchemaFields(schema, rootVal, sources)
}

func (p *Parser) resolveConfigFieldPath(meta *fieldMeta, rootVal reflect.Value, cliRes *cliParseResult) string {
	for i := len(cliRes.flags) - 1; i >= 0; i-- {
		if cliRes.flags[i].meta == meta && len(cliRes.flags[i].values) > 0 {
			return cliRes.flags[i].values[len(cliRes.flags[i].values)-1]
		}
	}
	for _, envKey := range p.envCandidatesForField(meta, p.autoEnv) {
		if val, ok := p.lookupEnv(envKey); ok && val != "" {
			return val
		}
	}
	if fv, ok := meta.fieldValue(rootVal, false); ok && fv.Kind() == reflect.String {
		return fv.String()
	}
	return ""
}

func (p *Parser) envCandidatesForField(meta *fieldMeta, includeAuto bool) []string {
	var out []string
	add := func(k string) {
		if k == "" {
			return
		}
		for _, existing := range out {
			if existing == k {
				return
			}
		}
		out = append(out, k)
	}

	if meta.env != "" {
		if p.envPrefix != "" && !strings.HasPrefix(strings.ToUpper(meta.env), strings.ToUpper(p.envPrefix)+"_") {
			add(strings.ToUpper(p.envPrefix) + "_" + meta.env)
		}
		add(meta.env)
	}
	if includeAuto && meta.autoEnvName != "" {
		if p.envPrefix != "" {
			add(strings.ToUpper(p.envPrefix) + "_" + meta.autoEnvName)
		}
		add(meta.autoEnvName)
	}
	return out
}

func (p *Parser) applyPositionalArgs(posArgs []string, schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	if len(posArgs) == 0 {
		return nil
	}
	if len(schema.positionalFields) == 0 {
		if p.strict {
			return &ParseError{Message: fmt.Sprintf("unexpected positional argument(s): %s", strings.Join(posArgs, " "))}
		}
		return nil
	}

	argIdx := 0
	for _, meta := range schema.positionalFields {
		if argIdx >= len(posArgs) {
			break
		}
		fv, _ := meta.fieldValue(rootVal, true)
		if isMultiValueField(meta) {
			if err := assignFromStrings(fv, posArgs[argIdx:], meta.sep, false); err != nil {
				return &ParseError{Field: meta.fieldName, Message: err.Error(), Err: err}
			}
			sources[meta] = sourceCLI
			argIdx = len(posArgs)
			break
		}
		if err := assignScalar(fv, posArgs[argIdx]); err != nil {
			return &ParseError{Field: meta.fieldName, Message: err.Error(), Err: err}
		}
		sources[meta] = sourceCLI
		argIdx++
	}

	if p.strict && argIdx < len(posArgs) {
		return &ParseError{Message: fmt.Sprintf("unexpected positional argument(s): %s", strings.Join(posArgs[argIdx:], " "))}
	}
	return nil
}
