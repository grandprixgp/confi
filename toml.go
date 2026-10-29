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
		rawLine := lines[idx]
		trimmedRaw := strings.TrimSpace(rawLine)
		if trimmedRaw == "" {
			continue
		}

		if strings.HasPrefix(trimmedRaw, "#") || (strings.HasPrefix(trimmedRaw, ";") && (len(trimmedRaw) == 1 || trimmedRaw[1] == ' ' || trimmedRaw[1] == '\t')) {
			continue
		}

		// Array of tables: [[table]] or [[table.sub]]
		strippedForHeader := strings.TrimSpace(stripTOMLComment(trimmedRaw))
		if strings.HasPrefix(strippedForHeader, "[[") && strings.HasSuffix(strippedForHeader, "]]") {
			secName := strings.TrimSpace(strippedForHeader[2 : len(strippedForHeader)-2])
			if secName == "" {
				return nil, fmt.Errorf("toml line %d: empty table array header", lineNum)
			}
			parts := splitTOMLDottedKey(secName)
			sec, err := appendTableArrayEntry(root, parts)
			if err != nil {
				return nil, fmt.Errorf("toml line %d: %v", lineNum, err)
			}
			currentSection = sec
			continue
		}

		// Section header: [section] or [section.sub]
		if strings.HasPrefix(strippedForHeader, "[") && strings.HasSuffix(strippedForHeader, "]") {
			secName := strings.TrimSpace(strippedForHeader[1 : len(strippedForHeader)-1])
			if secName == "" {
				return nil, fmt.Errorf("toml line %d: empty section header", lineNum)
			}
			sec, err := ensureNestedTOMLMap(root, splitTOMLDottedKey(secName))
			if err != nil {
				return nil, fmt.Errorf("toml line %d: %v", lineNum, err)
			}
			currentSection = sec
			continue
		}

		eqIdx := findTOMLEquals(rawLine)
		if eqIdx < 0 {
			return nil, fmt.Errorf("toml line %d: expected key = value, got %q", lineNum, trimmedRaw)
		}

		rawKey := strings.TrimSpace(rawLine[:eqIdx])
		rawVal := strings.TrimSpace(rawLine[eqIdx+1:])

		// Handle multiline triple-quoted string: """...""" (basic) or '''...''' (literal)
		if strings.HasPrefix(rawVal, `"""`) || strings.HasPrefix(rawVal, `'''`) {
			delim := rawVal[:3]
			isLiteral := (delim == `'''`)
			content := rawVal[3:]
			if endIdx := findTOMLClosingTripleQuote(content, delim); endIdx >= 0 {
				strVal := content[:endIdx]
				if isLiteral {
					strVal = processTOMLMultilineLiteral(strVal)
				} else {
					strVal = processTOMLMultilineBasic(strVal)
				}
				if err := setTOMLKey(currentSection, rawKey, strVal); err != nil {
					return nil, fmt.Errorf("toml line %d: %v", lineNum, err)
				}
				continue
			} else {
				var ml []string
				ml = append(ml, content)
				closed := false
				for idx+1 < len(lines) {
					idx++
					nextRaw := lines[idx]
					if endPos := findTOMLClosingTripleQuote(nextRaw, delim); endPos >= 0 {
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
				if isLiteral {
					joined = processTOMLMultilineLiteral(joined)
				} else {
					joined = processTOMLMultilineBasic(joined)
				}
				if err := setTOMLKey(currentSection, rawKey, joined); err != nil {
					return nil, fmt.Errorf("toml line %d: %v", lineNum, err)
				}
				continue
			}
		}

		rawVal = strings.TrimSpace(stripTOMLComment(rawVal))

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
			if !isBalancedBrackets(acc.String()) {
				return nil, fmt.Errorf("toml line %d: unterminated multiline array", lineNum)
			}
			rawVal = acc.String()
		}

		val, err := parseTOMLValue(rawVal, lineNum)
		if err != nil {
			return nil, err
		}
		if err := setTOMLKey(currentSection, rawKey, val); err != nil {
			return nil, fmt.Errorf("toml line %d: %v", lineNum, err)
		}
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
		if !inSingle && !inDouble {
			if ch == '#' {
				return s[:i]
			}
			if ch == ';' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
				return s[:i]
			}
		}
	}
	return s
}

func findTOMLEquals(s string) int {
	inSingle := false
	inDouble := false
	colonIdx := -1
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
			if ch == '=' {
				return i
			}
			if ch == ':' && colonIdx < 0 {
				colonIdx = i
			}
		}
	}
	return colonIdx
}

func splitTOMLDottedKey(s string) []string {
	var parts []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && inDouble && i+1 < len(s) {
			buf.WriteByte(ch)
			i++
			buf.WriteByte(s[i])
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(ch)
			continue
		}
		if ch == '.' && !inSingle && !inDouble {
			parts = append(parts, buf.String())
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	parts = append(parts, buf.String())
	return parts
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
				if depth < 0 {
					return false
				}
			}
		}
	}
	return depth == 0
}

func ensureNestedTOMLMap(root map[string]interface{}, parts []string) (map[string]interface{}, error) {
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
			return nil, fmt.Errorf("key %q already defined as non-table value (%T)", part, existing)
		}
		sub := make(map[string]interface{})
		curr[part] = sub
		curr = sub
	}
	return curr, nil
}

func appendTableArrayEntry(root map[string]interface{}, parts []string) (map[string]interface{}, error) {
	curr := root
	for i := 0; i < len(parts)-1; i++ {
		part := unquoteYAMLScalar(strings.TrimSpace(parts[i]))
		if part == "" {
			continue
		}
		if existing, ok := curr[part]; ok {
			if sub, ok := existing.(map[string]interface{}); ok {
				curr = sub
				continue
			}
			return nil, fmt.Errorf("key %q already defined as non-table value (%T)", part, existing)
		}
		sub := make(map[string]interface{})
		curr[part] = sub
		curr = sub
	}

	leaf := unquoteYAMLScalar(strings.TrimSpace(parts[len(parts)-1]))
	newEntry := make(map[string]interface{})
	if existing, ok := curr[leaf]; ok {
		if arr, ok := existing.([]interface{}); ok {
			curr[leaf] = append(arr, newEntry)
			return newEntry, nil
		}
		return nil, fmt.Errorf("key %q already defined as non-table value (%T)", leaf, existing)
	}
	curr[leaf] = []interface{}{newEntry}
	return newEntry, nil
}

func setTOMLKey(section map[string]interface{}, rawKey string, val interface{}) error {
	// Support dotted keys like server.host = "localhost" if not quoted
	parts := splitTOMLDottedKey(rawKey)
	if len(parts) > 1 {
		target, err := ensureNestedTOMLMap(section, parts[:len(parts)-1])
		if err != nil {
			return err
		}
		leaf := unquoteYAMLScalar(strings.TrimSpace(parts[len(parts)-1]))
		target[leaf] = val
		return nil
	}
	key := unquoteYAMLScalar(rawKey)
	section[key] = val
	return nil
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

func processTOMLMultilineLiteral(content string) string {
	if strings.HasPrefix(content, "\r\n") {
		content = content[2:]
	} else if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}
	return content
}

func processTOMLMultilineBasic(content string) string {
	if strings.HasPrefix(content, "\r\n") {
		content = content[2:]
	} else if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}
	var buf strings.Builder
	for i := 0; i < len(content); i++ {
		if content[i] == '\\' && i+1 < len(content) {
			j := i + 1
			for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
				j++
			}
			if j < len(content) && (content[j] == '\n' || content[j] == '\r') {
				if content[j] == '\r' && j+1 < len(content) && content[j+1] == '\n' {
					j += 2
				} else {
					j++
				}
				for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
					j++
				}
				i = j - 1
				continue
			}
		}
		buf.WriteByte(content[i])
	}
	return unescapeStringEscapes(buf.String())
}

func findTOMLClosingTripleQuote(s string, delim string) int {
	q := delim[0]
	idx := 0
	for {
		pos := strings.Index(s[idx:], delim)
		if pos < 0 {
			return -1
		}
		actualPos := idx + pos
		quoteCount := 3
		for actualPos+quoteCount < len(s) && s[actualPos+quoteCount] == q {
			quoteCount++
		}
		if quoteCount == 4 {
			return actualPos + 1
		}
		if quoteCount == 5 {
			return actualPos + 2
		}
		return actualPos + (quoteCount - 3)
	}
}
