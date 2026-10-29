package confi

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxYAMLDepth = 100

type yamlLine struct {
	lineNum int
	indent  int
	content string
	rawText string
}

// parseYAML parses a YAML stream into a nested map[string]interface{}.
func parseYAML(r io.Reader) (map[string]interface{}, error) {
	scanner := bufio.NewScanner(r)
	var rawLines []string
	for scanner.Scan() {
		rawLines = append(rawLines, strings.TrimRight(scanner.Text(), "\r"))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var lines []yamlLine
	for idx, raw := range rawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "---" || trimmed == "..." {
			if len(lines) > 0 {
				return nil, fmt.Errorf("yaml line %d: multi-document streams are not supported in config loader", idx+1)
			}
			continue
		}
		indent := countIndent(raw)
		leadBytes := leadWhitespaceBytes(raw)
		content := stripYAMLComment(raw[leadBytes:])
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		lines = append(lines, yamlLine{
			lineNum: idx + 1,
			indent:  indent,
			content: content,
			rawText: raw,
		})
	}

	if len(lines) == 0 {
		return make(map[string]interface{}), nil
	}

	if strings.HasPrefix(lines[0].content, "- ") || lines[0].content == "-" || (strings.HasPrefix(lines[0].content, "[") && strings.HasSuffix(lines[0].content, "]")) {
		return nil, fmt.Errorf("yaml line %d: root document must be a mapping, got sequence", lines[0].lineNum)
	}

	res, _, err := parseYAMLMapping(lines, 0, lines[0].indent, rawLines, 0)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func leadWhitespaceBytes(s string) int {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func cutIndent(s string, indentCols int) string {
	cols := 0
	i := 0
	for i < len(s) && cols < indentCols {
		if s[i] == ' ' {
			cols++
			i++
		} else if s[i] == '\t' {
			cols += 2
			i++
		} else {
			break
		}
	}
	return s[i:]
}

func countIndent(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			n++
		} else if s[i] == '\t' {
			n += 2
		} else {
			break
		}
	}
	return n
}

func stripYAMLComment(s string) string {
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
		if ch == '#' && !inSingle && !inDouble {
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return s[:i]
			}
		}
	}
	return s
}

func parseYAMLMapping(lines []yamlLine, startIdx int, baseIndent int, rawLines []string, depth int) (map[string]interface{}, int, error) {
	if depth > maxYAMLDepth {
		lineNum := 1
		if startIdx < len(lines) {
			lineNum = lines[startIdx].lineNum
		}
		return nil, startIdx, fmt.Errorf("yaml line %d: maximum nesting depth (%d) exceeded", lineNum, maxYAMLDepth)
	}
	result := make(map[string]interface{})
	i := startIdx

	for i < len(lines) {
		ln := lines[i]
		if ln.indent < baseIndent {
			break
		}
		if ln.indent > baseIndent {
			return nil, i, fmt.Errorf("yaml line %d: unexpected indentation", ln.lineNum)
		}
		if strings.HasPrefix(ln.content, "- ") || ln.content == "-" {
			break
		}

		key, valPart, ok := splitYAMLKeyValue(ln.content)
		if !ok {
			return nil, i, fmt.Errorf("yaml line %d: expected key: value, got %q", ln.lineNum, ln.content)
		}
		key = unquoteYAMLScalar(key)
		valPart = strings.TrimSpace(valPart)

		// Multiline block scalar: | or >
		if valPart == "|" || valPart == "|-" || valPart == ">" || valPart == ">-" {
			folded := strings.HasPrefix(valPart, ">")
			chomp := strings.HasSuffix(valPart, "-")
			blockStr, nextI := readYAMLBlockScalar(lines, i, ln.lineNum, ln.indent, rawLines, folded, chomp)
			result[key] = blockStr
			i = nextI
			continue
		}

		// Empty value after colon -> nested mapping or sequence on subsequent lines
		if valPart == "" {
			i++
			if i >= len(lines) {
				result[key] = ""
				continue
			}
			nextLn := lines[i]
			// Allow sequence at same indent or deeper indent
			if (nextLn.indent > ln.indent) || (nextLn.indent == ln.indent && (strings.HasPrefix(nextLn.content, "- ") || nextLn.content == "-")) {
				if strings.HasPrefix(nextLn.content, "- ") || nextLn.content == "-" {
					seq, nextI, err := parseYAMLSequence(lines, i, nextLn.indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					result[key] = seq
					i = nextI
				} else {
					subMap, nextI, err := parseYAMLMapping(lines, i, nextLn.indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					result[key] = subMap
					i = nextI
				}
			} else {
				result[key] = ""
			}
			continue
		}

		val, err := parseYAMLInlineValue(valPart, ln.lineNum, depth+1)
		if err != nil {
			return nil, i, err
		}
		result[key] = val
		i++
	}

	return result, i, nil
}

func parseYAMLSequence(lines []yamlLine, startIdx int, seqIndent int, rawLines []string, depth int) ([]interface{}, int, error) {
	if depth > maxYAMLDepth {
		lineNum := 1
		if startIdx < len(lines) {
			lineNum = lines[startIdx].lineNum
		}
		return nil, startIdx, fmt.Errorf("yaml line %d: maximum nesting depth (%d) exceeded", lineNum, maxYAMLDepth)
	}
	var seq []interface{}
	i := startIdx

	for i < len(lines) {
		ln := lines[i]
		if ln.indent < seqIndent {
			break
		}
		if ln.indent != seqIndent || (!strings.HasPrefix(ln.content, "- ") && ln.content != "-") {
			break
		}

		itemText := ""
		if len(ln.content) > 1 {
			itemText = strings.TrimSpace(ln.content[1:])
		}

		if itemText == "" {
			i++
			if i < len(lines) && lines[i].indent > seqIndent {
				if strings.HasPrefix(lines[i].content, "- ") || lines[i].content == "-" {
					nestedSeq, nextI, err := parseYAMLSequence(lines, i, lines[i].indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					seq = append(seq, nestedSeq)
					i = nextI
					continue
				}
				subMap, nextI, err := parseYAMLMapping(lines, i, lines[i].indent, rawLines, depth+1)
				if err != nil {
					return nil, nextI, err
				}
				seq = append(seq, subMap)
				i = nextI
				continue
			}
			seq = append(seq, "")
			continue
		}

		// Check if itemText is an inline key: value starting a mapping item in the sequence
		if k, vPart, ok := splitYAMLKeyValue(itemText); ok && !isQuotedYAML(itemText) {
			itemMap := make(map[string]interface{})
			k = unquoteYAMLScalar(k)
			vPart = strings.TrimSpace(vPart)
			i++
			if vPart != "" {
				parsedV, err := parseYAMLInlineValue(vPart, ln.lineNum, depth+1)
				if err != nil {
					return nil, i, err
				}
				itemMap[k] = parsedV
			} else if i < len(lines) && lines[i].indent > seqIndent {
				if strings.HasPrefix(lines[i].content, "- ") || lines[i].content == "-" {
					nestedSeq, nextI, err := parseYAMLSequence(lines, i, lines[i].indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					itemMap[k] = nestedSeq
					i = nextI
				} else {
					nestedMap, nextI, err := parseYAMLMapping(lines, i, lines[i].indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					itemMap[k] = nestedMap
					i = nextI
				}
			} else {
				itemMap[k] = ""
			}
			if i < len(lines) && lines[i].indent > seqIndent {
				if !strings.HasPrefix(lines[i].content, "- ") && lines[i].content != "-" {
					moreMap, nextI, err := parseYAMLMapping(lines, i, lines[i].indent, rawLines, depth+1)
					if err != nil {
						return nil, nextI, err
					}
					for mk, mv := range moreMap {
						itemMap[mk] = mv
					}
					i = nextI
				}
			}
			seq = append(seq, itemMap)
			continue
		}

		val, err := parseYAMLInlineValue(itemText, ln.lineNum, depth+1)
		if err != nil {
			return nil, i, err
		}
		seq = append(seq, val)
		i++
	}

	return seq, i, nil
}

func readYAMLBlockScalar(lines []yamlLine, currentIdx int, startLineNum int, keyIndent int, rawLines []string, folded bool, chomp bool) (string, int) {
	var collected []string
	blockIndent := -1

	// Determine the line range in rawLines before the next yamlLine with indent <= keyIndent
	nextIdx := currentIdx + 1
	endRawLine := len(rawLines)
	for nextIdx < len(lines) {
		if lines[nextIdx].indent <= keyIndent {
			endRawLine = lines[nextIdx].lineNum - 1
			break
		}
		nextIdx++
	}

	for rIdx := startLineNum; rIdx < endRawLine; rIdx++ {
		raw := rawLines[rIdx]
		if strings.TrimSpace(raw) == "" {
			collected = append(collected, "")
			continue
		}
		ind := countIndent(raw)
		if ind <= keyIndent {
			break
		}
		if blockIndent < 0 {
			blockIndent = ind
		}
		cut := blockIndent
		if ind < cut {
			cut = ind
		}
		collected = append(collected, cutIndent(raw, cut))
	}

	// Trim trailing empty lines
	for len(collected) > 0 && collected[len(collected)-1] == "" {
		collected = collected[:len(collected)-1]
	}

	var out string
	if folded {
		out = strings.Join(collected, " ")
	} else {
		out = strings.Join(collected, "\n")
		if !chomp && len(collected) > 0 {
			out += "\n"
		}
	}
	return out, nextIdx
}

func splitYAMLKeyValue(s string) (string, string, bool) {
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
		if ch == ':' && !inSingle && !inDouble {
			if i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t' {
				return strings.TrimSpace(s[:i]), s[i+1:], true
			}
		}
	}
	return "", "", false
}

func parseYAMLInlineValue(s string, lineNum int, depth int) (interface{}, error) {
	if depth > maxYAMLDepth {
		return nil, fmt.Errorf("yaml line %d: maximum nesting depth (%d) exceeded", lineNum, maxYAMLDepth)
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "~" || strings.EqualFold(s, "null") {
		return nil, nil
	}

	// Flow sequence: [a, b, c]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []interface{}{}, nil
		}
		parts := splitFlowItems(inner)
		items := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			v, err := parseYAMLInlineValue(p, lineNum, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return items, nil
	}

	// Flow mapping: {k1: v1, k2: v2}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		m := make(map[string]interface{})
		if inner == "" {
			return m, nil
		}
		parts := splitFlowItems(inner)
		for _, p := range parts {
			k, vPart, ok := splitYAMLKeyValue(p)
			if !ok {
				return nil, fmt.Errorf("yaml line %d: invalid flow mapping entry %q", lineNum, p)
			}
			v, err := parseYAMLInlineValue(vPart, lineNum, depth+1)
			if err != nil {
				return nil, err
			}
			m[unquoteYAMLScalar(k)] = v
		}
		return m, nil
	}

	return unquoteYAMLScalar(s), nil
}

func splitFlowItems(s string) []string {
	var parts []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	depth := 0

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
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if !inSingle && !inDouble {
			switch ch {
			case '[', '{':
				depth++
			case ']', '}':
				if depth > 0 {
					depth--
				}
			case ',':
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(buf.String()))
					buf.Reset()
					continue
				}
			}
		}
		buf.WriteByte(ch)
	}
	if buf.Len() > 0 {
		parts = append(parts, strings.TrimSpace(buf.String()))
	}
	return parts
}

func isQuotedYAML(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return false
	}
	q := s[0]
	if q != '"' && q != '\'' {
		return false
	}
	if q == '\'' {
		for i := 1; i < len(s); i++ {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				return i == len(s)-1
			}
		}
		return false
	}
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
			return i == len(s)-1
		}
	}
	return false
}

func unescapeStringEscapes(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case '"':
				buf.WriteByte('"')
			case '\\':
				buf.WriteByte('\\')
			case '/':
				buf.WriteByte('/')
			case '0':
				buf.WriteByte(0)
			case 'u':
				if i+4 < len(s) {
					hexStr := s[i+1 : i+5]
					val, err := strconv.ParseUint(hexStr, 16, 32)
					if err == nil {
						buf.WriteRune(rune(val))
						i += 4
						continue
					}
				}
				buf.WriteByte('u')
			case 'U':
				if i+8 < len(s) {
					hexStr := s[i+1 : i+9]
					val, err := strconv.ParseUint(hexStr, 16, 32)
					if err == nil {
						buf.WriteRune(rune(val))
						i += 8
						continue
					}
				}
				buf.WriteByte('U')
			default:
				buf.WriteByte(s[i])
			}
			continue
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

func unquoteYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return unescapeStringEscapes(s[1 : len(s)-1])
	}
	return s
}
