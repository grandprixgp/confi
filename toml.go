package confi

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// parseTOML parses a TOML or INI stream into a nested map[string]interface{}.
func parseTOML(r io.Reader) (map[string]interface{}, error) {
	root := make(map[string]interface{})
	currentSection := root

	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, strings.TrimRight(scanner.Text(), "\r"))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for idx := 0; idx < len(lines); idx++ {
		lineNum := idx + 1
		line := strings.TrimSpace(stripTOMLComment(lines[idx]))
		if line == "" {
			continue
		}

		// Section header: [section] or [section.sub]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			secName := strings.TrimSpace(line[1 : len(line)-1])
			secName = strings.TrimPrefix(secName, "[")
			secName = strings.TrimSuffix(secName, "]")
			if secName == "" {
				return nil, fmt.Errorf("toml line %d: empty section header", lineNum)
			}
			currentSection = ensureNestedTOMLMap(root, strings.Split(secName, "."))
			continue
		}

		eqIdx := findTOMLEquals(line)
		if eqIdx < 0 {
			return nil, fmt.Errorf("toml line %d: expected key = value, got %q", lineNum, line)
		}

		rawKey := strings.TrimSpace(line[:eqIdx])
		rawVal := strings.TrimSpace(line[eqIdx+1:])

		// Handle multiline triple-quoted string """..."""
		if strings.HasPrefix(rawVal, `"""`) {
			content := rawVal[3:]
			if endIdx := strings.Index(content, `"""`); endIdx >= 0 {
				rawVal = `"` + escapeSimpleQuotes(content[:endIdx]) + `"`
			} else {
				var ml []string
				if content != "" {
					ml = append(ml, content)
				}
				closed := false
				for idx+1 < len(lines) {
					idx++
					nextRaw := lines[idx]
					if endPos := strings.Index(nextRaw, `"""`); endPos >= 0 {
						ml = append(ml, nextRaw[:endPos])
						closed = true
						break
					}
					ml = append(ml, nextRaw)
				}
				if !closed {
					return nil, fmt.Errorf("toml line %d: unterminated multiline string", lineNum)
				}
				joined := strings.Join(ml, "\n")
				setTOMLKey(currentSection, rawKey, joined)
				continue
			}
		}

		// Handle multiline array [...]
		if strings.HasPrefix(rawVal, "[") && !isBalancedBrackets(rawVal) {
			var acc strings.Builder
			acc.WriteString(rawVal)
			for idx+1 < len(lines) && !isBalancedBrackets(acc.String()) {
				idx++
				nextPart := strings.TrimSpace(stripTOMLComment(lines[idx]))
				if nextPart != "" {
					acc.WriteString(" ")
					acc.WriteString(nextPart)
				}
			}
			rawVal = acc.String()
		}

		val, err := parseTOMLValue(rawVal, lineNum)
		if err != nil {
			return nil, err
		}
		setTOMLKey(currentSection, rawKey, val)
	}

	return root, nil
}

func stripTOMLComment(s string) string {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && inDouble {
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if (ch == '#' || ch == ';') && !inSingle && !inDouble {
			return s[:i]
		}
	}
	return s
}

func findTOMLEquals(s string) int {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if !inSingle && !inDouble && (ch == '=' || ch == ':') {
			return i
		}
	}
	return -1
}

func isBalancedBrackets(s string) bool {
	inSingle := false
	inDouble := false
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && inDouble {
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if !inSingle && !inDouble {
			if ch == '[' {
				depth++
			} else if ch == ']' {
				depth--
			}
		}
	}
	return depth <= 0
}

func ensureNestedTOMLMap(root map[string]interface{}, parts []string) map[string]interface{} {
	curr := root
	for _, rawPart := range parts {
		part := unquoteYAMLScalar(strings.TrimSpace(rawPart))
		if part == "" {
			continue
		}
		if existing, ok := curr[part]; ok {
			if sub, ok := existing.(map[string]interface{}); ok {
				curr = sub
				continue
			}
		}
		sub := make(map[string]interface{})
		curr[part] = sub
		curr = sub
	}
	return curr
}

func setTOMLKey(section map[string]interface{}, rawKey string, val interface{}) {
	// Support dotted keys like server.host = "localhost" if not quoted
	if strings.Contains(rawKey, ".") && !isQuotedYAML(rawKey) {
		parts := strings.Split(rawKey, ".")
		target := ensureNestedTOMLMap(section, parts[:len(parts)-1])
		leaf := unquoteYAMLScalar(strings.TrimSpace(parts[len(parts)-1]))
		target[leaf] = val
		return
	}
	key := unquoteYAMLScalar(rawKey)
	section[key] = val
}

func parseTOMLValue(s string, lineNum int) (interface{}, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}

	// Array: [a, b, c]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []interface{}{}, nil
		}
		parts := splitFlowItems(inner)
		var out []interface{}
		for _, p := range parts {
			if p == "" {
				continue
			}
			v, err := parseTOMLValue(p, lineNum)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}

	// Inline table: { k1 = v1, k2 = v2 }
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		m := make(map[string]interface{})
		if inner == "" {
			return m, nil
		}
		parts := splitFlowItems(inner)
		for _, p := range parts {
			if p == "" {
				continue
			}
			eqIdx := findTOMLEquals(p)
			if eqIdx < 0 {
				return nil, fmt.Errorf("toml line %d: invalid inline table entry %q", lineNum, p)
			}
			k := unquoteYAMLScalar(strings.TrimSpace(p[:eqIdx]))
			v, err := parseTOMLValue(strings.TrimSpace(p[eqIdx+1:]), lineNum)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		return m, nil
	}

	return unquoteYAMLScalar(s), nil
}

func escapeSimpleQuotes(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
