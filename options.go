package confi

import (
	"io"
)

// Format represents a configuration file format.
type Format int

const (
	// FormatAuto automatically detects the configuration format from the file extension or content.
	FormatAuto Format = iota
	// FormatJSON parses configuration as JSON.
	FormatJSON
	// FormatYAML parses configuration as YAML.
	FormatYAML
	// FormatEnv parses configuration as a .env (dotenv) file.
	FormatEnv
)

type configSource struct {
	path     string
	reader   io.Reader
	format   Format
	optional bool
}

// Option configures a Parser instance.
type Option func(*Parser)

// WithName sets the application name displayed in usage/help output.
func WithName(name string) Option {
	return func(p *Parser) {
		p.name = name
	}
}

// WithDescription sets the application description displayed in usage/help output.
func WithDescription(description string) Option {
	return func(p *Parser) {
		p.description = description
	}
}

// WithPrefix sets the long CLI flag prefix (defaults to "--").
func WithPrefix(prefix string) Option {
	return func(p *Parser) {
		if prefix != "" {
			p.prefix = prefix
		}
	}
}

// WithShortPrefix sets the short CLI flag prefix (defaults to "-").
func WithShortPrefix(prefix string) Option {
	return func(p *Parser) {
		p.shortPrefix = prefix
	}
}

// WithArgs overrides the command-line arguments to parse (defaults to os.Args[1:]).
func WithArgs(args []string) Option {
	return func(p *Parser) {
		argsCopy := make([]string, len(args))
		copy(argsCopy, args)
		p.args = argsCopy
		p.hasCustomArgs = true
	}
}

// WithEnvPrefix sets a prefix prepended to environment variable lookups (e.g. "APP" -> "APP_PORT").
func WithEnvPrefix(prefix string) Option {
	return func(p *Parser) {
		p.envPrefix = prefix
	}
}

// WithAutoEnv enables automatic environment variable lookup for all struct fields
// using their UPPER_SNAKE_CASE path (with EnvPrefix prepended if configured),
// even when no explicit `env` struct tag is present.
func WithAutoEnv() Option {
	return func(p *Parser) {
		p.autoEnv = true
	}
}

// WithEnvLookup overrides the function used to look up environment variables (defaults to os.LookupEnv).
func WithEnvLookup(fn func(string) (string, bool)) Option {
	return func(p *Parser) {
		if fn != nil {
			p.lookupEnv = fn
		}
	}
}

// WithConfigFile adds a required configuration file (JSON, YAML, or .env) to load.
func WithConfigFile(path string) Option {
	return func(p *Parser) {
		p.configSources = append(p.configSources, configSource{
			path:     path,
			format:   FormatAuto,
			optional: false,
		})
	}
}

// WithOptionalConfigFile adds an optional configuration file (JSON, YAML, or .env) to load.
// If the file does not exist, it is silently skipped.
func WithOptionalConfigFile(path string) Option {
	return func(p *Parser) {
		p.configSources = append(p.configSources, configSource{
			path:     path,
			format:   FormatAuto,
			optional: true,
		})
	}
}

// WithConfigReader adds a configuration stream with the given format to load.
func WithConfigReader(r io.Reader, format Format) Option {
	return func(p *Parser) {
		p.configSources = append(p.configSources, configSource{
			reader: r,
			format: format,
		})
	}
}

// WithConfigFlag specifies a CLI flag name (e.g. "config") that users can pass on the command line
// (e.g. `--config app.yaml`) to dynamically load a configuration file before applying env and CLI overrides.
func WithConfigFlag(flagName string) Option {
	return func(p *Parser) {
		p.configFlag = flagName
	}
}

// WithOutput sets the writer used for printing help/usage messages (defaults to os.Stdout).
func WithOutput(w io.Writer) Option {
	return func(p *Parser) {
		if w != nil {
			p.output = w
		}
	}
}

// WithHelpFlag enables or disables automatic handling of `-h` and `--help` flags (enabled by default).
func WithHelpFlag(enabled bool) Option {
	return func(p *Parser) {
		p.helpEnabled = enabled
	}
}

// WithStrict enables or disables strict mode. When enabled, unknown CLI flags and unknown
// configuration keys return an error instead of being ignored.
func WithStrict(strict bool) Option {
	return func(p *Parser) {
		p.strict = strict
	}
}
