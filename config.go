package confi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

const maxConfigSize = 10 << 20 // 10MB default cap

func (p *Parser) loadConfigSource(src configSource, schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	var data []byte
	var err error

	if src.reader != nil {
		limitR := io.LimitReader(src.reader, maxConfigSize+1)
		data, err = ioutil.ReadAll(limitR)
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("failed to read config stream: %v", err)}
		}
		if int64(len(data)) > maxConfigSize {
			return &ParseError{Message: fmt.Sprintf("config stream exceeds maximum limit of %d bytes", maxConfigSize)}
		}
	} else if src.path != "" {
		st, statErr := os.Stat(src.path)
		if statErr != nil {
			if os.IsNotExist(statErr) && src.optional {
				return nil
			}
			return &ParseError{Message: fmt.Sprintf("failed to read config file %q: %v", src.path, statErr)}
		}
		if st.Size() > maxConfigSize {
			return &ParseError{Message: fmt.Sprintf("config file %q exceeds maximum limit of %d bytes", src.path, maxConfigSize)}
		}
		data, err = ioutil.ReadFile(src.path)
		if err != nil {
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

	case FormatTOML, FormatINI:
		rawMap, err := parseTOML(bytes.NewReader(data))
		if err != nil {
			return &ParseError{Message: fmt.Sprintf("invalid TOML/INI config: %v", err)}
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
		case ".toml":
			return FormatTOML
		case ".ini", ".cfg", ".conf":
			return FormatINI
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

	// Heuristic: check first non-comment line for [section] (TOML/INI) or KEY=VALUE (.env)
	scanner := strings.Split(string(trimmed), "\n")
	for _, line := range scanner {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return FormatTOML
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

	for _, meta := range schema.fields {
		val, _, found := lookupFieldInStructuredMap(rawMap, meta)
		if !found || val == nil {
			continue
		}

		fv, _ := meta.fieldValue(rootVal, true)
		if err := assignInterfaceValue(fv, val, meta.sep); err != nil {
			return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("config value error for %s: %v", meta.fieldName, err)}
		}
		sources[meta] = sourceConfig
	}

	if p.strict {
		if err := validateStrictStructuredMap(rawMap, nil, schema); err != nil {
			return err
		}
	}
	return nil
}

func isMapField(m *fieldMeta) bool {
	if m == nil {
		return false
	}
	t := m.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Map
}

func validateStrictStructuredMap(curr map[string]interface{}, pathPrefix []string, schema *structSchema) error {
	for k, val := range curr {
		fullPath := append(append([]string(nil), pathPrefix...), k)
		fullPathStr := strings.Join(fullPath, ".")

		matchedLeaf := false
		isMap := false
		for _, meta := range schema.fields {
			if len(pathPrefix) == 0 {
				for _, alias := range meta.aliases {
					if strings.EqualFold(alias, k) || normalizeKey(alias) == normalizeKey(k) {
						matchedLeaf = true
						isMap = isMapField(meta)
						break
					}
				}
			}
			if !matchedLeaf && slicesMatchKey(meta.configPath, fullPath) {
				matchedLeaf = true
				isMap = isMapField(meta)
				break
			}
			if matchedLeaf {
				break
			}
		}

		if matchedLeaf {
			if isMap {
				continue
			}
			if subMap, ok := val.(map[string]interface{}); ok {
				if err := validateStrictStructuredMap(subMap, fullPath, schema); err != nil {
					return err
				}
			}
			continue
		}

		isPrefix := false
		for _, meta := range schema.fields {
			if len(meta.configPath) > len(fullPath) && slicesMatchKey(meta.configPath[:len(fullPath)], fullPath) {
				isPrefix = true
				break
			}
		}

		if isPrefix {
			if subMap, ok := val.(map[string]interface{}); ok {
				if err := validateStrictStructuredMap(subMap, fullPath, schema); err != nil {
					return err
				}
				continue
			}
		}

		return &ParseError{Field: fullPathStr, Message: fmt.Sprintf("unknown config key %q", fullPathStr)}
	}
	return nil
}

func slicesMatchKey(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) && normalizeKey(a[i]) != normalizeKey(b[i]) {
			return false
		}
	}
	return true
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
		if t.Kind() == reflect.Struct {
			return assignStructFromMap(fv, v, sep)
		}
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

func assignStructFromMap(fv reflect.Value, m map[string]interface{}, sep string) error {
	t := fv.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}
		fieldVal := fv.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		var candidates []string
		for _, tagKey := range []string{"confi", "toml", "yaml", "json"} {
			if tagVal := sf.Tag.Get(tagKey); tagVal != "" && tagVal != "-" {
				parts := strings.Split(tagVal, ",")
				name := strings.TrimSpace(parts[0])
				if name != "" && name != "-" {
					candidates = append(candidates, name)
				}
			}
		}
		candidates = append(candidates, sf.Name)
		candidates = append(candidates, toKebabCase(sf.Name))
		candidates = append(candidates, toSnakeCase(sf.Name))

		if sf.Anonymous {
			matchedExplicit := false
			for _, cand := range candidates {
				if v, _, ok := findKeyInMap(m, cand); ok {
					matchedExplicit = true
					if err := assignInterfaceValue(fieldVal, v, sep); err != nil {
						return fmt.Errorf("field %s: %v", sf.Name, err)
					}
					break
				}
			}
			if matchedExplicit {
				continue
			}
			if sf.Type.Kind() == reflect.Struct {
				if err := assignStructFromMap(fieldVal, m, sep); err != nil {
					return err
				}
				continue
			}
		}

		var foundVal interface{}
		var found bool
		for _, cand := range candidates {
			if v, _, ok := findKeyInMap(m, cand); ok {
				foundVal = v
				found = true
				break
			}
		}

		if found {
			if err := assignInterfaceValue(fieldVal, foundVal, sep); err != nil {
				return fmt.Errorf("field %s: %v", sf.Name, err)
			}
		}
	}
	return nil
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
