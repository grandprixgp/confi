package confi

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Completion generates a shell completion script ("bash", "zsh", "fish", or "powershell") for target.
func Completion(shell string, target interface{}, opts ...Option) (string, error) {
	p := NewParser("", opts...)
	return p.Completion(shell, target)
}

// WriteCompletion writes a shell completion script ("bash", "zsh", "fish", or "powershell") for target to w.
func WriteCompletion(w io.Writer, shell string, target interface{}, opts ...Option) error {
	p := NewParser("", opts...)
	return p.WriteCompletion(w, shell, target)
}

// Completion returns a shell completion script for target using this Parser's settings.
func (p *Parser) Completion(shell string, target interface{}) (string, error) {
	var buf bytes.Buffer
	if err := p.WriteCompletion(&buf, shell, target); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// WriteCompletion writes a shell completion script for target to w using this Parser's settings.
func (p *Parser) WriteCompletion(w io.Writer, shell string, target interface{}) error {
	schema, _, err := extractSchema(target)
	if err != nil {
		return err
	}
	return p.writeSchemaCompletion(w, shell, schema)
}

func (p *Parser) writeSchemaCompletion(w io.Writer, shell string, schema *structSchema) error {
	appName := p.name
	if appName == "" {
		appName = "app"
	}
	safeFunc := sanitizeShellIdentifier(appName)

	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash":
		return p.writeBashCompletion(w, appName, safeFunc, schema)
	case "zsh":
		return p.writeZshCompletion(w, appName, safeFunc, schema)
	case "fish":
		return p.writeFishCompletion(w, appName, schema)
	case "powershell", "pwsh":
		return p.writePowerShellCompletion(w, appName, schema)
	default:
		return &ParseError{Message: fmt.Sprintf("unsupported shell %q (supported: bash, zsh, fish, powershell)", shell)}
	}
}

func (p *Parser) collectFlagTokens(schema *structSchema) []string {
	var tokens []string
	for _, meta := range schema.fields {
		if meta.isPositional {
			continue
		}
		tokens = append(tokens, p.prefix+meta.flagName)
		if meta.short != "" && p.shortPrefix != "" {
			tokens = append(tokens, p.shortPrefix+meta.short)
		}
	}
	if p.configFlag != "" {
		tokens = append(tokens, p.prefix+p.configFlag)
	}
	if p.completionFlag != "" {
		tokens = append(tokens, p.prefix+p.completionFlag)
	}
	if p.helpEnabled {
		tokens = append(tokens, p.prefix+"help")
		if p.shortPrefix != "" {
			tokens = append(tokens, p.shortPrefix+"h")
		}
	}
	return tokens
}

func (p *Parser) writeBashCompletion(w io.Writer, appName string, safeFunc string, schema *structSchema) error {
	globalFlags := p.collectFlagTokens(schema)
	var subNames []string
	for _, sub := range schema.subcommandList {
		subNames = append(subNames, sub.name)
	}

	fmt.Fprintf(w, "# bash completion for %s\n", appName)
	fmt.Fprintf(w, "_%s_completions() {\n", safeFunc)
	fmt.Fprintf(w, "    local cur prev words cword\n")
	fmt.Fprintf(w, "    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	fmt.Fprintf(w, "    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n\n")

	// Enum completions for flags with oneof choices
	allFields := append([]*fieldMeta(nil), schema.fields...)
	for _, sub := range schema.subcommandList {
		allFields = append(allFields, sub.schema.fields...)
	}
	hasEnumCases := false
	for _, meta := range allFields {
		if len(meta.oneOf) > 0 {
			if !hasEnumCases {
				fmt.Fprintf(w, "    case \"$prev\" in\n")
				hasEnumCases = true
			}
			pat := p.prefix + meta.flagName
			if meta.short != "" && p.shortPrefix != "" {
				pat += "|" + p.shortPrefix + meta.short
			}
			fmt.Fprintf(w, "        %s)\n", pat)
			fmt.Fprintf(w, "            COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n", strings.Join(meta.oneOf, " "))
			fmt.Fprintf(w, "            return 0\n")
			fmt.Fprintf(w, "            ;;\n")
		}
	}
	if hasEnumCases {
		fmt.Fprintf(w, "    esac\n\n")
	}

	if len(schema.subcommandList) > 0 {
		fmt.Fprintf(w, "    local subcmd=\"\"\n")
		fmt.Fprintf(w, "    local i\n")
		fmt.Fprintf(w, "    for ((i=1; i < COMP_CWORD; i++)); do\n")
		fmt.Fprintf(w, "        case \"${COMP_WORDS[i]}\" in\n")
		fmt.Fprintf(w, "            %s)\n", strings.Join(subNames, "|"))
		fmt.Fprintf(w, "                subcmd=\"${COMP_WORDS[i]}\"\n")
		fmt.Fprintf(w, "                break\n")
		fmt.Fprintf(w, "                ;;\n")
		fmt.Fprintf(w, "        esac\n")
		fmt.Fprintf(w, "    done\n\n")
		fmt.Fprintf(w, "    case \"$subcmd\" in\n")
		for _, sub := range schema.subcommandList {
			subFlags := p.collectFlagTokens(sub.schema)
			fmt.Fprintf(w, "        %s)\n", sub.name)
			fmt.Fprintf(w, "            COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n", strings.Join(subFlags, " "))
			fmt.Fprintf(w, "            return 0\n")
			fmt.Fprintf(w, "            ;;\n")
		}
		fmt.Fprintf(w, "    esac\n\n")
	}

	topWords := append(subNames, globalFlags...)
	fmt.Fprintf(w, "    COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n", strings.Join(topWords, " "))
	fmt.Fprintf(w, "}\n")
	fmt.Fprintf(w, "complete -F _%s_completions %q\n", safeFunc, appName)
	return nil
}

func (p *Parser) writeZshCompletion(w io.Writer, appName string, safeFunc string, schema *structSchema) error {
	escapeZsh := func(s string) string {
		return strings.NewReplacer(":", `\:`, "[", `\[`, "]", `\]`, "'", `\'`).Replace(s)
	}

	fmt.Fprintf(w, "#compdef %s\n\n", appName)
	fmt.Fprintf(w, "_%s() {\n", safeFunc)
	fmt.Fprintf(w, "    local -a opts\n")
	fmt.Fprintf(w, "    opts=(\n")
	for _, meta := range schema.fields {
		if meta.isPositional {
			continue
		}
		desc := escapeZsh(meta.usage)
		if len(meta.oneOf) > 0 {
			fmt.Fprintf(w, "        '%s%s[%s]:value:(%s)'\n", p.prefix, meta.flagName, desc, strings.Join(meta.oneOf, " "))
			if meta.short != "" {
				fmt.Fprintf(w, "        '-%s[%s]:value:(%s)'\n", meta.short, desc, strings.Join(meta.oneOf, " "))
			}
		} else {
			fmt.Fprintf(w, "        '%s%s[%s]'\n", p.prefix, meta.flagName, desc)
			if meta.short != "" {
				fmt.Fprintf(w, "        '-%s[%s]'\n", meta.short, desc)
			}
		}
	}
	if p.configFlag != "" {
		fmt.Fprintf(w, "        '%s%s[Path to config file]'\n", p.prefix, p.configFlag)
	}
	if p.completionFlag != "" {
		fmt.Fprintf(w, "        '%s%s[Generate shell completion script]:shell:(bash zsh fish)'\n", p.prefix, p.completionFlag)
	}
	fmt.Fprintf(w, "        '--help[Show help and usage information]'\n")
	fmt.Fprintf(w, "        '-h[Show help and usage information]'\n")

	for _, sub := range schema.subcommandList {
		desc := escapeZsh(sub.usage)
		fmt.Fprintf(w, "        '%s:%s'\n", sub.name, desc)
		if sub.schema != nil {
			for _, meta := range sub.schema.fields {
				if meta.isPositional {
					continue
				}
				sDesc := escapeZsh(meta.usage)
				if len(meta.oneOf) > 0 {
					fmt.Fprintf(w, "        '%s%s[%s]:value:(%s)'\n", p.prefix, meta.flagName, sDesc, strings.Join(meta.oneOf, " "))
					if meta.short != "" {
						fmt.Fprintf(w, "        '-%s[%s]:value:(%s)'\n", meta.short, sDesc, strings.Join(meta.oneOf, " "))
					}
				} else {
					fmt.Fprintf(w, "        '%s%s[%s]'\n", p.prefix, meta.flagName, sDesc)
					if meta.short != "" {
						fmt.Fprintf(w, "        '-%s[%s]'\n", meta.short, sDesc)
					}
				}
			}
		}
	}
	fmt.Fprintf(w, "    )\n")
	fmt.Fprintf(w, "    _describe '%s' opts\n", appName)
	fmt.Fprintf(w, "}\n")
	fmt.Fprintf(w, "compdef _%s %q\n", safeFunc, appName)
	return nil
}

func (p *Parser) writeFishCompletion(w io.Writer, appName string, schema *structSchema) error {
	fmt.Fprintf(w, "# fish completion for %s\n", appName)
	for _, sub := range schema.subcommandList {
		fmt.Fprintf(w, "complete -c %q -n '__fish_use_subcommand' -a %q -d %q\n", appName, sub.name, sub.usage)
		for _, meta := range sub.schema.fields {
			if meta.isPositional {
				continue
			}
			line := fmt.Sprintf("complete -c %q -n '__fish_seen_subcommand_from %s' -l %s", appName, sub.name, meta.flagName)
			if meta.short != "" {
				line += fmt.Sprintf(" -s %s", meta.short)
			}
			if len(meta.oneOf) > 0 {
				line += fmt.Sprintf(" -r -a %q", strings.Join(meta.oneOf, " "))
			}
			if meta.usage != "" {
				line += fmt.Sprintf(" -d %q", meta.usage)
			}
			fmt.Fprintln(w, line)
		}
	}
	for _, meta := range schema.fields {
		if meta.isPositional {
			continue
		}
		line := fmt.Sprintf("complete -c %q -l %s", appName, meta.flagName)
		if meta.short != "" {
			line += fmt.Sprintf(" -s %s", meta.short)
		}
		if len(meta.oneOf) > 0 {
			line += fmt.Sprintf(" -r -a %q", strings.Join(meta.oneOf, " "))
		}
		if meta.usage != "" {
			line += fmt.Sprintf(" -d %q", meta.usage)
		}
		fmt.Fprintln(w, line)
	}
	return nil
}

func (p *Parser) writePowerShellCompletion(w io.Writer, appName string, schema *structSchema) error {
	globalFlags := p.collectFlagTokens(schema)

	quoteTokens := func(tokens []string) string {
		var quoted []string
		for _, t := range tokens {
			quoted = append(quoted, fmt.Sprintf("'%s'", strings.Replace(t, "'", "''", -1)))
		}
		return strings.Join(quoted, ", ")
	}

	fmt.Fprintf(w, "# PowerShell completion for %s\n", appName)
	fmt.Fprintf(w, "Register-ArgumentCompleter -Native -CommandName '%s' -ScriptBlock {\n", strings.Replace(appName, "'", "''", -1))
	fmt.Fprintf(w, "    param($wordToComplete, $commandAst, $cursorPosition)\n")

	if len(schema.subcommandList) == 0 {
		fmt.Fprintf(w, "    $candidates = @(%s)\n", quoteTokens(globalFlags))
	} else {
		var subNames []string
		for _, sub := range schema.subcommandList {
			subNames = append(subNames, sub.name)
		}

		fmt.Fprintf(w, "    $subcmd = ''\n")
		fmt.Fprintf(w, "    if ($commandAst.Elements) {\n")
		fmt.Fprintf(w, "        $tokens = $commandAst.Elements | ForEach-Object { $_.Extent.Text }\n")
		fmt.Fprintf(w, "        foreach ($t in $tokens) {\n")
		fmt.Fprintf(w, "            switch ($t) {\n")
		for _, sub := range schema.subcommandList {
			fmt.Fprintf(w, "                '%s' { $subcmd = '%s'; break }\n", sub.name, sub.name)
			for _, al := range sub.aliases {
				if al != sub.name {
					fmt.Fprintf(w, "                '%s' { $subcmd = '%s'; break }\n", al, sub.name)
				}
			}
		}
		fmt.Fprintf(w, "            }\n")
		fmt.Fprintf(w, "        }\n")
		fmt.Fprintf(w, "    }\n")

		fmt.Fprintf(w, "    switch ($subcmd) {\n")
		for _, sub := range schema.subcommandList {
			subFlags := append([]string(nil), globalFlags...)
			subFlags = append(subFlags, p.collectFlagTokens(sub.schema)...)
			fmt.Fprintf(w, "        '%s' {\n", sub.name)
			fmt.Fprintf(w, "            $candidates = @(%s)\n", quoteTokens(subFlags))
			fmt.Fprintf(w, "        }\n")
		}
		fmt.Fprintf(w, "        default {\n")
		rootTokens := append([]string(nil), subNames...)
		rootTokens = append(rootTokens, globalFlags...)
		fmt.Fprintf(w, "            $candidates = @(%s)\n", quoteTokens(rootTokens))
		fmt.Fprintf(w, "        }\n")
		fmt.Fprintf(w, "    }\n")
	}

	fmt.Fprintf(w, "    $candidates | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n")
	fmt.Fprintf(w, "        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)\n")
	fmt.Fprintf(w, "    }\n")
	fmt.Fprintf(w, "}\n")
	return nil
}

func sanitizeShellIdentifier(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9' && out.Len() > 0) || ch == '_' {
			out.WriteByte(ch)
		} else {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "app"
	}
	return out.String()
}
