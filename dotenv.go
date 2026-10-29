package confi

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// parseDotEnv reads a .env stream and returns key-value pairs.
func parseDotEnv(r io.Reader, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "export ") || strings.HasPrefix(lowerLine, "export\t") {
			line = strings.TrimSpace(line[len("export"):])
		}

		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			// Also allow KEY: VALUE in .env if no '=' is present
			colonIdx := strings.IndexByte(line, ':')
			if colonIdx > 0 {
				eqIdx = colonIdx
			} else {
				return nil, fmt.Errorf(".env line %d: missing '=' in %q", lineNum, line)
			}
		}

		key := strings.TrimSpace(line[:eqIdx])
		if key == "" {
			return nil, fmt.Errorf(".env line %d: empty key", lineNum)
		}

		rawVal := strings.TrimSpace(line[eqIdx+1:])

		// If rawVal begins with an opening quote and is unclosed on the current line,
		// continue reading subsequent lines until the closing quote is encountered.
		if isUnclosedDotEnvQuote(rawVal) {
			for scanner.Scan() {
				lineNum++
				rawVal += "\n" + scanner.Text()
				if !isUnclosedDotEnvQuote(rawVal) {
					break
				}
			}
		}

		val, err := parseDotEnvValue(rawVal, result, lookupEnv)
		if err != nil {
			return nil, fmt.Errorf(".env line %d: %v", lineNum, err)
		}
		result[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func isUnclosedDotEnvQuote(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "'") {
		return strings.IndexByte(s[1:], '\'') < 0
	}
	if strings.HasPrefix(s, "\"") {
		escaped := false
		for i := 1; i < len(s); i++ {
			if escaped {
				escaped = false
				continue
			}
			if s[i] == '\\' {
				escaped = true
				continue
			}
			if s[i] == '"' {
				return false
			}
		}
		return true
	}
	return false
}

func parseDotEnvValue(raw string, current map[string]string, lookupEnv func(string) (string, bool)) (string, error) {
	if raw == "" {
		return "", nil
	}

	// Single-quoted value: literal
	if strings.HasPrefix(raw, "'") {
		endIdx := strings.IndexByte(raw[1:], '\'')
		if endIdx < 0 {
			return "", fmt.Errorf("unterminated single-quoted string %s", raw)
		}
		closeIdx := 1 + endIdx
		suffix := strings.TrimSpace(raw[closeIdx+1:])
		if suffix != "" && !strings.HasPrefix(suffix, "#") {
			return "", fmt.Errorf("unexpected trailing characters after quote: %q", suffix)
		}
		return raw[1:closeIdx], nil
	}

	// Double-quoted value: process escapes and variable interpolation
	if strings.HasPrefix(raw, "\"") {
		var buf strings.Builder
		escaped := false
		closed := false
		closeIdx := -1
		for i := 1; i < len(raw); i++ {
			ch := raw[i]
			if escaped {
				switch ch {
				case 'n':
					buf.WriteByte('\n')
				case 'r':
					buf.WriteByte('\r')
				case 't':
					buf.WriteByte('\t')
				case '"':
					buf.WriteByte('"')
				case '\\':
					if i+1 < len(raw) && raw[i+1] == '$' {
						buf.WriteString("\\\\$")
						i++
					} else {
						buf.WriteByte('\\')
					}
				case '$':
					buf.WriteString("\\$")
				default:
					buf.WriteByte(ch)
				}
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				closed = true
				closeIdx = i
				break
			}
			buf.WriteByte(ch)
		}
		if !closed {
			return "", fmt.Errorf("unterminated double-quoted string %s", raw)
		}
		suffix := strings.TrimSpace(raw[closeIdx+1:])
		if suffix != "" && !strings.HasPrefix(suffix, "#") {
			return "", fmt.Errorf("unexpected trailing characters after quote: %q", suffix)
		}
		return expandEnvVars(buf.String(), current, lookupEnv), nil
	}

	// Unquoted value: strip inline comment (# preceded by space/tab)
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && (i == 0 || raw[i-1] == ' ' || raw[i-1] == '\t') {
			raw = strings.TrimSpace(raw[:i])
			break
		}
	}

	return expandEnvVars(raw, current, lookupEnv), nil
}

func expandEnvVars(s string, current map[string]string, lookupEnv func(string) (string, bool)) string {
	lookup := func(k string) string {
		if v, ok := current[k]; ok {
			return v
		}
		if lookupEnv != nil {
			if v, ok := lookupEnv(k); ok {
				return v
			}
		}
		return ""
	}

	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '$' {
			out.WriteByte('$')
			i++
			continue
		}
		if s[i] != '$' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		if s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end >= 0 {
				expr := s[i+2 : i+2+end]
				defVal := ""
				hasDef := false
				if defIdx := strings.Index(expr, ":-"); defIdx >= 0 {
					defVal = expr[defIdx+2:]
					expr = expr[:defIdx]
					hasDef = true
				}
				val := lookup(expr)
				if val == "" && hasDef {
					val = defVal
				}
				out.WriteString(val)
				i += 2 + end
				continue
			}
		}
		// $VAR syntax
		start := i + 1
		end := start
		for end < len(s) && isEnvVarChar(s[end], end == start) {
			end++
		}
		if end > start {
			varName := s[start:end]
			out.WriteString(lookup(varName))
			i = end - 1
			continue
		}
		out.WriteByte('$')
	}
	return out.String()
}

func isEnvVarChar(b byte, first bool) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' {
		return true
	}
	return !first && b >= '0' && b <= '9'
}
