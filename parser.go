package confi

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

type parsedFlag struct {
	meta   *fieldMeta
	name   string
	values []string
}

type cliParseResult struct {
	flags           []parsedFlag
	positional      []string
	configFilePath  string
	helpRequested   bool
	completionShell string
	subcommand      *subcommandMeta
	subcommandArgs  []string
}

func (p *Parser) parseCLI(args []string, schema *structSchema) (*cliParseResult, error) {
	res := &cliParseResult{}
	prefix := p.prefix
	shortPrefix := p.shortPrefix

	var currentFlag *parsedFlag
	expectingValue := false

	flushCurrent := func() {
		if currentFlag != nil {
			res.flags = append(res.flags, *currentFlag)
			currentFlag = nil
			expectingValue = false
		}
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// End-of-flags delimiter "--"
		if arg == "--" {
			flushCurrent()
			if i+1 < len(args) {
				res.positional = append(res.positional, args[i+1:]...)
			}
			break
		}

		// If current flag is expecting a value (or is a multi-value flag) and arg looks like a negative number (e.g. "-5"),
		// treat it as a value rather than a flag.
		if currentFlag != nil && (expectingValue || isMultiValueField(currentFlag.meta)) && looksLikeNegativeNumber(arg) {
			currentFlag.values = append(currentFlag.values, arg)
			if !isMultiValueField(currentFlag.meta) {
				flushCurrent()
			} else {
				expectingValue = false
			}
			continue
		}

		// If no flag is active and arg looks like a negative number (e.g. "-10"), treat it as a positional argument.
		if currentFlag == nil && looksLikeNegativeNumber(arg) {
			res.positional = append(res.positional, arg)
			continue
		}

		// Check long prefix (default "--")
		if prefix != "" && strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
			flushCurrent()
			withoutPrefix := arg[len(prefix):]

			var rawName, inlineVal string
			hasInlineVal := false
			if eqIdx := strings.IndexByte(withoutPrefix, '='); eqIdx >= 0 {
				rawName = strings.TrimSpace(withoutPrefix[:eqIdx])
				inlineVal = withoutPrefix[eqIdx+1:]
				hasInlineVal = true
			} else {
				fields := strings.Fields(withoutPrefix)
				if len(fields) == 0 {
					continue
				}
				rawName = fields[0]
				if len(fields) > 1 {
					inlineVal = strings.Join(fields[1:], " ")
					hasInlineVal = true
				}
			}

			lowerName := strings.ToLower(rawName)

			// Check help flag
			if p.helpEnabled && (lowerName == "help" || lowerName == "h") {
				if _, exists := schema.byFlag[lowerName]; !exists {
					res.helpRequested = true
					return res, nil
				}
			}

			// Check completion flag (e.g. --completion bash)
			if p.completionFlag != "" && strings.EqualFold(rawName, p.completionFlag) {
				if hasInlineVal && inlineVal != "" {
					res.completionShell = inlineVal
				} else if i+1 < len(args) {
					i++
					res.completionShell = args[i]
				} else {
					return nil, &ParseError{Field: rawName, Message: fmt.Sprintf("flag %s%s requires a shell name (bash, zsh, fish, powershell)", prefix, rawName)}
				}
				return res, nil
			}

			// Check dynamic config flag (e.g., --config path.yaml)
			if p.configFlag != "" && strings.EqualFold(rawName, p.configFlag) {
				if hasInlineVal && inlineVal != "" {
					res.configFilePath = inlineVal
				} else if i+1 < len(args) {
					i++
					res.configFilePath = args[i]
				} else {
					return nil, &ParseError{Field: rawName, Message: fmt.Sprintf("flag %s%s requires a file path", prefix, rawName)}
				}
				continue
			}

			meta, exists := schema.byFlag[lowerName]
			negatedBool := false
			if !exists && strings.HasPrefix(lowerName, "no-") {
				if candidate, ok := schema.byFlag[strings.TrimPrefix(lowerName, "no-")]; ok && candidate.isBoolean() {
					meta = candidate
					exists = true
					negatedBool = true
				}
			}

			if !exists {
				if p.strict {
					return nil, &ParseError{Field: rawName, Message: fmt.Sprintf("unknown flag %s%s", prefix, rawName)}
				}
				currentFlag = &parsedFlag{name: rawName}
				if hasInlineVal && inlineVal != "" {
					currentFlag.values = append(currentFlag.values, inlineVal)
				} else {
					expectingValue = true
				}
				continue
			}

			if negatedBool {
				val := "false"
				if hasInlineVal && inlineVal != "" {
					if b, err := parseBoolFlexible(inlineVal); err == nil {
						if b {
							val = "false"
						} else {
							val = "true"
						}
					}
				}
				res.flags = append(res.flags, parsedFlag{
					meta:   meta,
					name:   rawName,
					values: []string{val},
				})
				continue
			}

			if meta.isBoolean() {
				if hasInlineVal {
					val := inlineVal
					if val == "" {
						val = "true"
					}
					res.flags = append(res.flags, parsedFlag{
						meta:   meta,
						name:   rawName,
						values: []string{val},
					})
					continue
				}
				if i+1 < len(args) && isExplicitBoolLiteral(args[i+1]) {
					i++
					res.flags = append(res.flags, parsedFlag{
						meta:   meta,
						name:   rawName,
						values: []string{args[i]},
					})
					continue
				}
				res.flags = append(res.flags, parsedFlag{
					meta:   meta,
					name:   rawName,
					values: []string{"true"},
				})
				continue
			}

			currentFlag = &parsedFlag{
				meta: meta,
				name: rawName,
			}
			if hasInlineVal {
				if inlineVal != "" || !isMultiValueField(meta) {
					currentFlag.values = append(currentFlag.values, inlineVal)
					if !isMultiValueField(meta) {
						flushCurrent()
					}
				} else {
					expectingValue = true
				}
			} else {
				expectingValue = true
			}
			continue
		}

		// Check short prefix (default "-")
		if shortPrefix != "" && strings.HasPrefix(arg, shortPrefix) && len(arg) > len(shortPrefix) && (prefix == "" || !strings.HasPrefix(arg, prefix)) {
			flushCurrent()
			withoutShort := arg[len(shortPrefix):]

			var rawShort, inlineVal string
			hasInlineVal := false
			if eqIdx := strings.IndexByte(withoutShort, '='); eqIdx >= 0 {
				rawShort = withoutShort[:eqIdx]
				inlineVal = withoutShort[eqIdx+1:]
				hasInlineVal = true
			} else {
				rawShort = withoutShort
			}

			if p.helpEnabled && rawShort == "h" {
				if _, exists := schema.byShort["h"]; !exists {
					res.helpRequested = true
					return res, nil
				}
			}

			if meta, exists := schema.byShort[rawShort]; exists {
				if meta.isBoolean() {
					val := "true"
					if hasInlineVal && inlineVal != "" {
						val = inlineVal
					} else if i+1 < len(args) && isExplicitBoolLiteral(args[i+1]) {
						i++
						val = args[i]
					}
					res.flags = append(res.flags, parsedFlag{
						meta:   meta,
						name:   rawShort,
						values: []string{val},
					})
					continue
				}
				currentFlag = &parsedFlag{
					meta: meta,
					name: rawShort,
				}
				if hasInlineVal {
					if inlineVal != "" || !isMultiValueField(meta) {
						currentFlag.values = append(currentFlag.values, inlineVal)
						if !isMultiValueField(meta) {
							flushCurrent()
						}
					} else {
						expectingValue = true
					}
				} else {
					expectingValue = true
				}
				continue
			}

			// Check bundled single-character short flags, e.g. "-abc"
			if len(rawShort) > 1 && isBundledShorts(rawShort, schema) {
				runes := []rune(rawShort)
				for idx := 0; idx < len(runes); idx++ {
					ch := string(runes[idx])
					meta := schema.byShort[ch]
					if meta.isBoolean() {
						val := "true"
						if idx == len(runes)-1 && hasInlineVal && inlineVal != "" {
							val = inlineVal
						}
						res.flags = append(res.flags, parsedFlag{
							meta:   meta,
							name:   ch,
							values: []string{val},
						})
					} else {
						currentFlag = &parsedFlag{
							meta: meta,
							name: ch,
						}
						if hasInlineVal {
							if inlineVal != "" || !isMultiValueField(meta) {
								currentFlag.values = append(currentFlag.values, inlineVal)
								if !isMultiValueField(meta) {
									flushCurrent()
								}
							} else {
								expectingValue = true
							}
						} else {
							expectingValue = true
						}
					}
				}
				continue
			}

			if p.strict {
				return nil, &ParseError{Field: rawShort, Message: fmt.Sprintf("unknown short flag %s%s", shortPrefix, rawShort)}
			}
			currentFlag = &parsedFlag{name: rawShort}
			if hasInlineVal && inlineVal != "" {
				currentFlag.values = append(currentFlag.values, inlineVal)
			} else {
				expectingValue = true
			}
			continue
		}

		// Check if token is a subcommand (when not actively expecting the first value of a flag)
		if !expectingValue && len(schema.subcommands) > 0 {
			if subMeta, ok := schema.subcommands[strings.ToLower(arg)]; ok {
				flushCurrent()
				res.subcommand = subMeta
				if i+1 < len(args) {
					res.subcommandArgs = args[i+1:]
				}
				break
			}
		}

		// Non-flag token
		if currentFlag != nil {
			currentFlag.values = append(currentFlag.values, arg)
			expectingValue = false
			if currentFlag.meta != nil && !isMultiValueField(currentFlag.meta) {
				if isStringField(currentFlag.meta) && len(schema.positionalFields) == 0 && len(schema.subcommands) == 0 {
					continue
				}
				flushCurrent()
			}
		} else {
			res.positional = append(res.positional, arg)
		}
	}

	flushCurrent()
	return res, nil
}

func isMultiValueField(m *fieldMeta) bool {
	if m == nil {
		return true
	}
	t := m.fieldType
	if implementsTextUnmarshaler(t) {
		return false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if implementsTextUnmarshaler(t) {
			return false
		}
	}
	return t.Kind() == reflect.Slice || t.Kind() == reflect.Map
}

func isStringField(m *fieldMeta) bool {
	if m == nil {
		return false
	}
	t := m.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.String
}

func isExplicitBoolLiteral(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "false":
		return true
	default:
		return false
	}
}

func looksLikeNegativeNumber(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	r := rune(s[1])
	return unicode.IsDigit(r) || (r == '.' && len(s) > 2 && unicode.IsDigit(rune(s[2])))
}

func isBundledShorts(rawShort string, schema *structSchema) bool {
	runes := []rune(rawShort)
	for i := 0; i < len(runes); i++ {
		ch := string(runes[i])
		meta, exists := schema.byShort[ch]
		if !exists {
			return false
		}
		if i < len(runes)-1 && !meta.isBoolean() {
			return false
		}
	}
	return true
}
