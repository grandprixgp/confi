package confi

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Usage returns the formatted help/usage message for target.
func Usage(target interface{}, opts ...Option) (string, error) {
	p := NewParser("", opts...)
	return p.Usage(target)
}

// Usage returns the formatted help/usage message for target using this Parser's settings.
func (p *Parser) Usage(target interface{}) (string, error) {
	var buf bytes.Buffer
	if err := p.WriteUsage(&buf, target); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// WriteUsage writes the formatted help/usage message for target to w.
func (p *Parser) WriteUsage(w io.Writer, target interface{}) error {
	schema, _, err := extractSchema(target)
	if err != nil {
		return err
	}
	return p.writeSchemaUsage(w, schema, "", p.description)
}

func (p *Parser) writeSchemaUsage(w io.Writer, schema *structSchema, subcmdName string, description string) error {
	appName := p.name
	if appName == "" {
		appName = "app"
	}
	if subcmdName != "" {
		appName = appName + " " + subcmdName
	}

	var posPlaceholders []string
	for _, pos := range schema.positionalFields {
		tok := "<" + pos.flagName + ">"
		if isMultiValueField(pos) {
			tok += "..."
		}
		if !pos.required {
			tok = "[" + tok + "]"
		}
		posPlaceholders = append(posPlaceholders, tok)
	}

	usageLine := fmt.Sprintf("Usage: %s [flags]", appName)
	if len(schema.subcommandList) > 0 {
		usageLine += " [command]"
	}
	if len(posPlaceholders) > 0 {
		usageLine += " " + strings.Join(posPlaceholders, " ")
	}
	fmt.Fprintln(w, usageLine)

	if description != "" {
		fmt.Fprintf(w, "\n%s\n", description)
	}

	type row struct {
		left  string
		right string
	}

	if len(schema.subcommandList) > 0 {
		fmt.Fprintln(w, "\nCommands:")
		var cmdRows []row
		maxLeft := 0
		for _, sub := range schema.subcommandList {
			left := "  " + sub.name
			if len(left) > maxLeft {
				maxLeft = len(left)
			}
			cmdRows = append(cmdRows, row{
				left:  left,
				right: sub.usage,
			})
		}
		for _, r := range cmdRows {
			if r.right != "" {
				fmt.Fprintf(w, "%-*s  %s\n", maxLeft, r.left, r.right)
			} else {
				fmt.Fprintln(w, r.left)
			}
		}
	}

	if len(schema.positionalFields) > 0 {
		fmt.Fprintln(w, "\nArguments:")
		var posRows []row
		maxLeft := 0
		for _, meta := range schema.positionalFields {
			left := fmt.Sprintf("  %-s <%s>", meta.flagName, formatTypeLabel(meta.fieldType))
			if len(left) > maxLeft {
				maxLeft = len(left)
			}
			posRows = append(posRows, row{
				left:  left,
				right: p.formatFieldDetails(meta),
			})
		}
		for _, r := range posRows {
			if r.right != "" {
				fmt.Fprintf(w, "%-*s  %s\n", maxLeft, r.left, r.right)
			} else {
				fmt.Fprintln(w, r.left)
			}
		}
	}

	fmt.Fprintln(w, "\nFlags:")
	var rows []row
	maxLeft := 0

	addRow := func(left, right string) {
		if len(left) > maxLeft {
			maxLeft = len(left)
		}
		rows = append(rows, row{left: left, right: right})
	}

	for _, meta := range schema.fields {
		if meta.isPositional {
			continue
		}
		var leftBuf strings.Builder
		leftBuf.WriteString("  ")
		if meta.short != "" && p.shortPrefix != "" {
			leftBuf.WriteString(p.shortPrefix)
			leftBuf.WriteString(meta.short)
			leftBuf.WriteString(", ")
		} else {
			leftBuf.WriteString("    ")
		}
		leftBuf.WriteString(p.prefix)
		leftBuf.WriteString(meta.flagName)
		if !meta.isBoolean() {
			leftBuf.WriteString(" <")
			leftBuf.WriteString(formatTypeLabel(meta.fieldType))
			leftBuf.WriteString(">")
		}

		addRow(leftBuf.String(), p.formatFieldDetails(meta))
	}

	if subcmdName == "" && p.configFlag != "" {
		if _, exists := schema.byFlag[strings.ToLower(p.configFlag)]; !exists {
			left := fmt.Sprintf("      %s%s <path>", p.prefix, p.configFlag)
			addRow(left, "Path to configuration file (JSON, YAML, TOML, or .env)")
		}
	}

	if subcmdName == "" && p.completionFlag != "" {
		if _, exists := schema.byFlag[strings.ToLower(p.completionFlag)]; !exists {
			left := fmt.Sprintf("      %s%s <shell>", p.prefix, p.completionFlag)
			addRow(left, "Generate shell completion script (bash, zsh, fish, powershell)")
		}
	}

	if p.helpEnabled {
		shortPart := "    "
		if p.shortPrefix != "" {
			if _, taken := schema.byShort["h"]; !taken {
				shortPart = p.shortPrefix + "h, "
			}
		}
		left := fmt.Sprintf("  %s%shelp", shortPart, p.prefix)
		addRow(left, "Show this help message")
	}

	for _, r := range rows {
		if r.right != "" {
			fmt.Fprintf(w, "%-*s  %s\n", maxLeft, r.left, r.right)
		} else {
			fmt.Fprintln(w, r.left)
		}
	}

	return nil
}

func (p *Parser) formatFieldDetails(meta *fieldMeta) string {
	var parts []string
	if meta.usage != "" {
		parts = append(parts, meta.usage)
	}
	if meta.required {
		parts = append(parts, "(required)")
	}
	if len(meta.oneOf) > 0 {
		parts = append(parts, fmt.Sprintf("[choices: %s]", strings.Join(meta.oneOf, "|")))
	}
	if meta.hasDefault && meta.defaultValue != "" {
		parts = append(parts, fmt.Sprintf("[default: %s]", meta.defaultValue))
	}
	envName := p.displayEnvName(meta)
	if envName != "" {
		parts = append(parts, fmt.Sprintf("[env: %s]", envName))
	}
	return strings.Join(parts, " ")
}

func (p *Parser) displayEnvName(meta *fieldMeta) string {
	if meta.env != "" {
		if p.envPrefix != "" && !strings.HasPrefix(strings.ToUpper(meta.env), strings.ToUpper(p.envPrefix)+"_") {
			return strings.ToUpper(p.envPrefix) + "_" + meta.env
		}
		return meta.env
	}
	if p.autoEnv {
		if p.envPrefix != "" {
			return strings.ToUpper(p.envPrefix) + "_" + meta.autoEnvName
		}
		return meta.autoEnvName
	}
	return ""
}

func formatTypeLabel(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == durationType {
		return "duration"
	}
	if t == timeType {
		return "time"
	}
	if t == urlType {
		return "url"
	}
	if t == regexpType {
		return "regex"
	}
	if implementsTextUnmarshaler(t) {
		return strings.ToLower(t.Name())
	}
	switch t.Kind() {
	case reflect.Slice:
		return formatTypeLabel(t.Elem()) + "[]"
	case reflect.Map:
		return fmt.Sprintf("%s=%s", formatTypeLabel(t.Key()), formatTypeLabel(t.Elem()))
	default:
		return t.Kind().String()
	}
}
