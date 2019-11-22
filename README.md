# confi

##### Quick and dirty CLI argument and config file parser. Pass it a `struct` of arguments, your app name, and the desired prefix.

## Syntax Example
```
app --hostname=nic.ac --port= 80 --port 8080 --interfaces eth0 vmbr0 \
--filters="eth0: src host 1.1.1.1" "eth0: dst host 1.1.1.1" --filters "vmbr0: host dst 10.0.0"
```

## Quick Example
```
package main

import (
	"confi"
	"fmt"
)

var (
	cfg arguments
)

type arguments struct {
	Hostname string
	Port []int
	Interfaces []string
	Filters []string
}

func main() {
	settings := confi.NewCoreSettings("app", "--")
	confi.Initialize(*confi.NewCoreArgument(&cfg), *settings)
	fmt.Printf("%s listening on ports %d\non interfaces %s\nwith BPF filters %q\n", 
		cfg.Hostname, cfg.Port, cfg.Interfaces, cfg.Filters)
}
```
```
greg@twotone:~/$ app --hostname=nic.ac --port= 80 --port 8080 --interfaces eth0 vmbr0 \
--filters="eth0: src host 1.1.1.1" "eth0: dst host 1.1.1.1" --filters "vmbr0: host dst 10.0.0"
nic.ac listening on ports [80 8080]
on interfaces [eth0 vmbr0]
with BPF filters ["eth0: src host 1.1.1.1" "eth0: dst host 1.1.1.1" "vmbr0: host dst 10.0.0"]
```
## Supported Primitives
*	Bool
*	String
*	Int8 ... Int64
*	Float32 ... Float64

## Supported Composites
*	Slices

## Supported Files
*	YAML (Todo)
*	JSON (Todo)