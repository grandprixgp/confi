# confi

Zero-dependency command-line argument, environment variable, and configuration file (`JSON`, `YAML`, `.env`) parser for Go structs.

## Features

- **Zero External Dependencies** — Built entirely on the Go standard library.
- **Unified Precedence Hierarchy** — `Defaults < Config File (JSON / YAML / .env) < Environment Variables < CLI Flags`.
- **Flexible CLI Syntax** — Supports `--flag=val`, `--flag= val`, `--flag val`, multi-value slices (`--port 80 8080`, `--port 80,8080`, repeated `--port 80 --port 8080`), short flags (`-p 8080`), bundled short booleans (`-alp`), boolean negation (`--no-tls`), and `--` positional argument termination.
- **Rich Struct Tags** — Configure flag names, short aliases, defaults, environment variable bindings, required field validation, custom list separators, positional arguments, and help text via `confi:"..."` or standalone struct tags.
- **Rich Type Support** — All Go primitives (`bool`, `string`, `int`..`int64`, `uint`..`uint64`, `uintptr`, `float32`, `float64`), `time.Duration`, `time.Time`, `url.URL`, `regexp.Regexp`, `net.IP`, any `encoding.TextUnmarshaler`, pointers, slices, maps (`map[K]V`), and nested/embedded structs.
- **Auto-Generated `--help` / `-h`** — Formatted usage and flag documentation out of the box.

## Quick Example

```go
package main

import (
	"fmt"
	"time"

	"github.com/grandprixgp/confi"
)

type Config struct {
	Hostname   string            `confi:"hostname,short=H,default=localhost,env=APP_HOST,usage=Server hostname"`
	Port       []int             `confi:"port,short=p,default='80,8080',usage=Listening ports"`
	Interfaces []string          `confi:"interfaces,short=i,usage=Network interfaces"`
	Filters    []string          `confi:"filters,short=f,usage=BPF packet filters"`
	Timeout    time.Duration     `confi:"timeout,default=10s,usage=Request timeout"`
	Labels     map[string]string `confi:"labels,usage=Key-value metadata labels"`
}

func main() {
	var cfg Config
	confi.MustParse(
		&cfg,
		confi.WithName("app"),
		confi.WithDescription("Packet capture and monitoring service."),
		confi.WithOptionalConfigFile("config.yaml"),
		confi.WithConfigFlag("config"),
		confi.WithAutoEnv(),
	)

	fmt.Printf("%s listening on ports %v\non interfaces %v\nwith BPF filters %q (timeout %v)\n",
		cfg.Hostname, cfg.Port, cfg.Interfaces, cfg.Filters, cfg.Timeout)
}
```

```bash
$ app --hostname=nic.ac --port= 80 --port 8080 --interfaces eth0 vmbr0 \
  --filters="eth0: src host 1.1.1.1" "eth0: dst host 1.1.1.1" --filters "vmbr0: host dst 10.0.0"
nic.ac listening on ports [80 8080]
on interfaces [eth0 vmbr0]
with BPF filters ["eth0: src host 1.1.1.1" "eth0: dst host 1.1.1.1" "vmbr0: host dst 10.0.0"] (timeout 10s)
```

## Struct Tags

You can use either the consolidated `confi:"..."` struct tag or standalone tags:

| Option | Consolidated `confi` Tag | Standalone Tag | Description |
| :--- | :--- | :--- | :--- |
| Flag / Key Name | `confi:"hostname"` or `confi:"name=hostname"` | `flag:"hostname"` / `name:"hostname"` | Custom CLI flag and config key name (defaults to `kebab-case` of field name) |
| Short Flag | `confi:",short=H"` | `short:"H"` | Single-character short flag (`-H`) |
| Default Value | `confi:",default=8080"` | `default:"8080"` | Default value applied when field is zero |
| Environment Variable | `confi:",env=APP_PORT"` | `env:"APP_PORT"` | Environment variable name to bind |
| Required Validation | `confi:",required"` | `required:"true"` | Returns `*confi.ValidationError` if unset |
| Help Description | `confi:",usage='Port number'"` | `usage:"Port number"` / `help:"..."` | Description shown in `--help` output |
| List Separator | `confi:",sep=;"` | `sep:";"` | Custom delimiter for slice/map values (defaults to `,`) |
| Positional Argument | `confi:",positional"` | `positional:"true"` | Binds non-flag positional CLI arguments |
| Dynamic Config File | `confi:",config"` | `config:"true"` | Marks a `string` field as a config file path to load |
| Squash Nested Struct | `confi:",squash"` | — | Flattens nested struct fields into parent namespace |
| Ignore Field | `confi:"-"` | — | Skips the field completely |

## Configuration Files

`confi` natively parses **JSON**, **YAML**, and **`.env`** files without external dependencies:

```go
err := confi.Parse(
	&cfg,
	confi.WithConfigFile("config.yaml"),          // Required config file (.json, .yaml, .yml, .env)
	confi.WithOptionalConfigFile(".env"),         // Skipped if file does not exist
	confi.WithConfigFlag("config"),               // Allows --config <path> on the CLI
	confi.WithEnvPrefix("APP"),                   // Prefixes env lookups with APP_
	confi.WithAutoEnv(),                          // Automatically binds UPPER_SNAKE_CASE env vars
	confi.WithStrict(true),                       // Rejects unknown flags or config keys
)
```

## Supported Types

- **Primitives**: `bool`, `string`, `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `uintptr`, `float32`, `float64`
- **Standard Library & Custom Types**: `time.Duration`, `time.Time`, `url.URL`, `regexp.Regexp`, `net.IP`, and any type implementing `encoding.TextUnmarshaler`
- **Composites**: Slices (`[]T`), Maps (`map[K]V`), Pointers (`*T`), and Nested / Embedded Structs
