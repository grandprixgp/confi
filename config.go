package confi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

func (p *Parser) loadConfigSource(src configSource, schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	var data []byte
	var err error

	if src.reader != nil {
		data, err = ioutil.ReadAll(src.reader)
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("failed to read config stream: %v", err)}
		}
	} else if src.path != "" {
		data, err = ioutil.ReadFile(src.path)
		if err != nil {
			if os.IsNotExist(err) && src.optional {
				return nil
			}
			return &ParseError{Message: fmt.Sprintf("failed to read config file %q: %v", src.path, err)}
		}
	} else {
		return nil
	}

	format := detectFormat(src.path, data, src.format)
	switch format {
	case FormatEnv:
		envMap, err := parseDotEnv(bytes.NewReader(data), p.lookupEnv)
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("invalid .env config: %v", err)}
		}
		return p.applyDotEnvMap(envMap, schema, rootVal, sources)

	case FormatJSON:
		rawMap, err := parseJSONConfig(data)
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("invalid JSON config: %v", err)}
		}
		return p.applyStructuredMap(rawMap, schema, rootVal, sources)

	case FormatYAML:
		rawMap, err := parseYAML(bytes.NewReader(data))
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("invalid YAML config: %v", err)}
		}
		return p.applyStructuredMap(rawMap, schema, rootVal, sources)

	default:
		return &ParseError{Message: fmt.Sprintf("unsupported config format: %v", format)}
	}
}

func detectFormat(path string, data []byte, hint Format) Format {
	if hint != FormatAuto {
		return hint
	}
	if path != "" {
		base := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".json":
			return FormatJSON
		case ".yaml", ".yml":
			return FormatYAML
		case ".env":
			return FormatEnv
		}
		if strings.HasPrefix(base, ".env.") || base == ".env" {
			return FormatEnv
		}
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return FormatJSON
	}

	// Heuristic: check if first non-comment line uses KEY=VALUE rather than YAML key: value
	scanner := strings.Split(string(trimmed), "\n")
	for _, line := range scanner {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") && !strings.Contains(line, ": ") && !strings.HasSuffix(line, ":") {
			return FormatEnv
		}
		break
	}
	return FormatYAML
}

func parseJSONConfig(data []byte) (map[string]interface{}, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return make(map[string]interface{}), nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var out map[string]interface{}
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Parser) applyDotEnvMap(envMap map[string]string, schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	consumed := make(map[string]bool)

	for _, meta := range schema.fields {
		candidates := p.envCandidatesForField(meta, true)
		for _, alias := range meta.aliases {
			candidates = append(candidates, alias, strings.ToUpper(strings.ReplaceAll(alias, "-", "_")), strings.ToUpper(strings.ReplaceAll(alias, ".", "_")))
		}

		var matchedKey string
		var matchedVal string
		found := false
		for _, cand := range candidates {
			if v, ok := lookupMapCaseInsensitive(envMap, cand); ok {
				matchedKey = cand
				matchedVal = v
				found = true
				break
			}
		}
		if !found {
			continue
		}

		for k := range envMap {
			if strings.EqualFold(k, matchedKey) {
				consumed[k] = true
			}
		}

		fv, _ := meta.fieldValue(rootVal, true)
		if err := assignFromString(fv, matchedVal, meta.sep, false); err != nil {
			return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("config value error for %s: %v", meta.fieldName, err)}
		}
		sources[meta] = sourceConfig
	}

	if p.strict {
		for k := range envMap {
			if !consumed[k] {
				return &ParseError{Field: k, Message: fmt.Sprintf("unknown config key %q", k)}
			}
		}
	}
	return nil
}

func lookupMapCaseInsensitive(m map[string]string, target string) (string, bool) {
	if v, ok := m[target]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, target) {
			return v, true
		}
	}
	return "", false
}

func (p *Parser) applyStructuredMap(rawMap map[string]interface{}, schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	if len(rawMap) == 0 {
		return nil
	}

	consumedTopKeys := make(map[string]bool)

	for _, meta := range schema.fields {
		val, topKey, found := lookupFieldInStructuredMap(rawMap, meta)
		if !found || val == nil {
			continue
		}
		consumedTopKeys[topKey] = true

		fv, _ := meta.fieldValue(rootVal, true)
		if err := assignInterfaceValue(fv, val, meta.sep); err != nil {
			return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("config value error for %s: %v", meta.fieldName, err)}
		}
		sources[meta] = sourceConfig
	}

	if p.strict {
		for k := range rawMap {
			if !consumedTopKeys[k] {
				return &ParseError{Field: k, Message: fmt.Sprintf("unknown config key %q", k)}
			}
		}
	}
	return nil
}

func lookupFieldInStructuredMap(rawMap map[string]interface{}, meta *fieldMeta) (interface{}, string, bool) {
	// 1. Check direct alias match at top level (handles flat keys and top-level fields)
	for _, alias := range meta.aliases {
		if v, actualKey, ok := findKeyInMap(rawMap, alias); ok {
			return v, actualKey, true
		}
	}

	// 2. Traverse nested path for nested struct fields
	if len(meta.configPath) > 1 {
		curr := rawMap
		var topKey string
		for i := 0; i < len(meta.configPath)-1; i++ {
			seg := meta.configPath[i]
			nextVal, actualKey, ok := findKeyInMap(curr, seg)
			if !ok {
				return nil, "", false
			}
			if i == 0 {
				topKey = actualKey
			}
			subMap, ok := nextVal.(map[string]interface{})
			if !ok {
				return nil, "", false
			}
			curr = subMap
		}

		leafSeg := meta.configPath[len(meta.configPath)-1]
		leafCandidates := []string{
			leafSeg,
			strings.ReplaceAll(leafSeg, "-", "_"),
			strings.ReplaceAll(leafSeg, "_", ""),
		}
		for _, cand := range leafCandidates {
			if v, _, ok := findKeyInMap(curr, cand); ok {
				return v, topKey, true
			}
		}
	}

	return nil, "", false
}

func findKeyInMap(m map[string]interface{}, target string) (interface{}, string, bool) {
	if v, ok := m[target]; ok {
		return v, target, true
	}
	normTarget := normalizeKey(target)
	for k, v := range m {
		if strings.EqualFold(k, target) || normalizeKey(k) == normTarget {
			return v, k, true
		}
	}
	return nil, "", false
}

func normalizeKey(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func assignInterfaceValue(fv reflect.Value, val interface{}, sep string) error {
	if val == nil {
		return nil
	}

	t := fv.Type()
	if t.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(t.Elem()))
		}
		return assignInterfaceValue(fv.Elem(), val, sep)
	}

	switch v := val.(type) {
	case []interface{}:
		if t.Kind() != reflect.Slice {
			return fmt.Errorf("cannot assign list to %s", t.String())
		}
		elemType := t.Elem()
		slice := reflect.MakeSlice(t, 0, len(v))
		for _, item := range v {
			elemPtr := reflect.New(elemType)
			if err := assignInterfaceValue(elemPtr.Elem(), item, sep); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		fv.Set(slice)
		return nil

	case map[string]interface{}:
		if t.Kind() != reflect.Map {
			return fmt.Errorf("cannot assign object/map to %s", t.String())
		}
		keyType := t.Key()
		valType := t.Elem()
		outMap := reflect.MakeMap(t)
		for mk, mv := range v {
			kPtr := reflect.New(keyType)
			if err := assignScalar(kPtr.Elem(), mk); err != nil {
				return fmt.Errorf("invalid map key %q: %v", mk, err)
			}
			vPtr := reflect.New(valType)
			if err := assignInterfaceValue(vPtr.Elem(), mv, sep); err != nil {
				return fmt.Errorf("invalid map value for key %q: %v", mk, err)
			}
			outMap.SetMapIndex(kPtr.Elem(), vPtr.Elem())
		}
		fv.Set(outMap)
		return nil

	default:
		strVal, err := scalarInterfaceToString(val)
		if err != nil {
			return err
		}
		return assignFromString(fv, strVal, sep, false)
	}
}

func scalarInterfaceToString(val interface{}) (string, error) {
	switch v := val.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
