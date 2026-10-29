package confi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

const redactedMask = "********"

// Dump returns a formatted key-value string representation of target with secret fields masked.
func Dump(target interface{}) (string, error) {
	schema, rootVal, err := extractSchema(target)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	for _, meta := range schema.fields {
		fv, ok := meta.fieldValue(rootVal, false)
		valStr := formatRedactedValue(meta, fv, ok)
		fmt.Fprintf(&buf, "%s = %s\n", meta.flagName, valStr)
	}
	return buf.String(), nil
}

// Sprint returns a single-line or multi-line string of target's fields with secret values masked.
func Sprint(target interface{}) string {
	s, err := Dump(target)
	if err != nil {
		return fmt.Sprintf("<invalid config: %v>", err)
	}
	return strings.TrimSpace(s)
}

// RedactMap returns a nested map representation of target with secret fields replaced by "********".
func RedactMap(target interface{}) (map[string]interface{}, error) {
	schema, rootVal, err := extractSchema(target)
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{})
	for _, meta := range schema.fields {
		fv, ok := meta.fieldValue(rootVal, false)
		var val interface{}
		if meta.secret {
			if ok && !isZeroValue(fv) {
				val = redactedMask
			} else {
				val = ""
			}
		} else if ok && fv.IsValid() {
			if fv.Kind() == reflect.Ptr {
				if !fv.IsNil() {
					val = fv.Elem().Interface()
				}
			} else {
				val = fv.Interface()
			}
		}
		curr := out
		for i := 0; i < len(meta.configPath)-1; i++ {
			seg := meta.configPath[i]
			next, exists := curr[seg]
			if !exists {
				sub := make(map[string]interface{})
				curr[seg] = sub
				curr = sub
			} else if sub, ok := next.(map[string]interface{}); ok {
				curr = sub
			}
		}
		curr[meta.configPath[len(meta.configPath)-1]] = val
	}
	return out, nil
}

func formatRedactedValue(meta *fieldMeta, fv reflect.Value, valid bool) string {
	if meta.secret {
		if valid && !isZeroValue(fv) {
			return redactedMask
		}
		return ""
	}
	if !valid || !fv.IsValid() {
		return ""
	}
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return "<nil>"
		}
		fv = fv.Elem()
	}
	return fmt.Sprintf("%v", fv.Interface())
}

// GenerateSample generates a documented sample configuration file for target in the given format.
func GenerateSample(target interface{}, format Format, opts ...Option) (string, error) {
	var buf bytes.Buffer
	if err := WriteSample(&buf, target, format, opts...); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// WriteSample writes a documented sample configuration file for target in the given format to w.
func WriteSample(w io.Writer, target interface{}, format Format, opts ...Option) error {
	p := NewParser("", opts...)
	schema, rootVal, err := extractSchema(target)
	if err != nil {
		return err
	}

	switch format {
	case FormatYAML, FormatAuto:
		return p.writeSampleYAML(w, schema, rootVal)
	case FormatJSON:
		return p.writeSampleJSON(w, schema, rootVal)
	case FormatTOML, FormatINI:
		return p.writeSampleTOML(w, schema, rootVal)
	case FormatEnv:
		return p.writeSampleEnv(w, schema, rootVal)
	default:
		return &ParseError{Message: fmt.Sprintf("unsupported sample format: %v", format)}
	}
}

func (p *Parser) writeSampleYAML(w io.Writer, schema *structSchema, rootVal reflect.Value) error {
	if p.description != "" {
		fmt.Fprintf(w, "# %s\n\n", p.description)
	}
	var prevPrefix string
	for idx, meta := range schema.fields {
		if meta.isPositional || meta.isConfigFile {
			continue
		}
		prefix := ""
		if len(meta.configPath) > 1 {
			prefix = strings.Join(meta.configPath[:len(meta.configPath)-1], ".")
		}
		if idx > 0 && prefix != prevPrefix {
			fmt.Fprintln(w)
		}
		indent := ""
		if prefix != "" && prefix != prevPrefix {
			var prevParts []string
			if prevPrefix != "" {
				prevParts = strings.Split(prevPrefix, ".")
			}
			currParts := meta.configPath[:len(meta.configPath)-1]
			common := 0
			for common < len(prevParts) && common < len(currParts) && prevParts[common] == currParts[common] {
				common++
			}
			for i := common; i < len(currParts); i++ {
				fmt.Fprintf(w, "%s%s:\n", strings.Repeat("  ", i), currParts[i])
			}
		}
		prevPrefix = prefix
		if len(meta.configPath) > 1 {
			indent = strings.Repeat("  ", len(meta.configPath)-1)
		}

		if comment := p.sampleComment(meta); comment != "" {
			fmt.Fprintf(w, "%s# %s\n", indent, comment)
		}
		leafKey := meta.configPath[len(meta.configPath)-1]
		sampleVal := sampleValueYAML(meta, rootVal)
		fmt.Fprintf(w, "%s%s: %s\n", indent, leafKey, sampleVal)
	}
	return nil
}

func (p *Parser) writeSampleTOML(w io.Writer, schema *structSchema, rootVal reflect.Value) error {
	if p.description != "" {
		fmt.Fprintf(w, "# %s\n\n", p.description)
	}
	// First write top-level fields, then section fields
	var topFields []*fieldMeta
	sectionOrder := []string{}
	sectionMap := make(map[string][]*fieldMeta)

	for _, meta := range schema.fields {
		if meta.isPositional || meta.isConfigFile {
			continue
		}
		if len(meta.configPath) == 1 {
			topFields = append(topFields, meta)
		} else {
			sec := strings.Join(meta.configPath[:len(meta.configPath)-1], ".")
			if _, exists := sectionMap[sec]; !exists {
				sectionOrder = append(sectionOrder, sec)
			}
			sectionMap[sec] = append(sectionMap[sec], meta)
		}
	}

	for _, meta := range topFields {
		if comment := p.sampleComment(meta); comment != "" {
			fmt.Fprintf(w, "# %s\n", comment)
		}
		fmt.Fprintf(w, "%s = %s\n", meta.configPath[0], sampleValueTOML(meta, rootVal))
	}

	for _, sec := range sectionOrder {
		fmt.Fprintf(w, "\n[%s]\n", sec)
		for _, meta := range sectionMap[sec] {
			if comment := p.sampleComment(meta); comment != "" {
				fmt.Fprintf(w, "# %s\n", comment)
			}
			leaf := meta.configPath[len(meta.configPath)-1]
			fmt.Fprintf(w, "%s = %s\n", leaf, sampleValueTOML(meta, rootVal))
		}
	}
	return nil
}

func (p *Parser) writeSampleEnv(w io.Writer, schema *structSchema, rootVal reflect.Value) error {
	if p.description != "" {
		fmt.Fprintf(w, "# %s\n\n", p.description)
	}
	for _, meta := range schema.fields {
		if meta.isPositional || meta.isConfigFile {
			continue
		}
		if comment := p.sampleComment(meta); comment != "" {
			fmt.Fprintf(w, "# %s\n", comment)
		}
		envKey := p.displayEnvName(meta)
		if envKey == "" {
			if p.envPrefix != "" {
				envKey = strings.ToUpper(p.envPrefix) + "_" + meta.autoEnvName
			} else {
				envKey = meta.autoEnvName
			}
		}
		val := meta.defaultValue
		if val == "" {
			if fv, ok := meta.fieldValue(rootVal, false); ok && !isZeroValue(fv) && !meta.secret {
				val = fmt.Sprintf("%v", fv.Interface())
			}
		}
		fmt.Fprintf(w, "%s=%s\n", envKey, val)
	}
	return nil
}

func (p *Parser) writeSampleJSON(w io.Writer, schema *structSchema, rootVal reflect.Value) error {
	root := make(map[string]interface{})
	for _, meta := range schema.fields {
		if meta.isPositional || meta.isConfigFile {
			continue
		}
		curr := root
		for i := 0; i < len(meta.configPath)-1; i++ {
			seg := meta.configPath[i]
			if existing, ok := curr[seg]; ok {
				if sub, ok := existing.(map[string]interface{}); ok {
					curr = sub
					continue
				}
			}
			sub := make(map[string]interface{})
			curr[seg] = sub
			curr = sub
		}
		leaf := meta.configPath[len(meta.configPath)-1]
		curr[leaf] = sampleValueObject(meta, rootVal)
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func (p *Parser) sampleComment(meta *fieldMeta) string {
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
	if env := p.displayEnvName(meta); env != "" {
		parts = append(parts, fmt.Sprintf("[env: %s]", env))
	}
	return strings.Join(parts, " ")
}

func sampleValueYAML(meta *fieldMeta, rootVal reflect.Value) string {
	if meta.secret {
		return `""`
	}
	if meta.hasDefault && meta.defaultValue != "" {
		t := meta.fieldType
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() == reflect.Slice && !implementsTextUnmarshaler(t) {
			items := splitBySeparator(meta.defaultValue, meta.sep)
			return "[" + strings.Join(items, ", ") + "]"
		}
		if t.Kind() == reflect.String || t == durationType {
			return fmt.Sprintf("%q", meta.defaultValue)
		}
		return meta.defaultValue
	}
	if rootVal.IsValid() {
		if fv, ok := meta.fieldValue(rootVal, false); ok && fv.IsValid() && !isZeroValue(fv) && !meta.secret {
			return formatRuntimeValueYAML(fv)
		}
	}
	t := meta.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if t == durationType {
			return `"0s"`
		}
		return "0"
	case reflect.Float32, reflect.Float64:
		return "0.0"
	case reflect.Slice:
		if implementsTextUnmarshaler(t) {
			return `""`
		}
		return "[]"
	case reflect.Map:
		return "{}"
	default:
		return `""`
	}
}

func sampleValueTOML(meta *fieldMeta, rootVal reflect.Value) string {
	if meta.secret {
		return `""`
	}
	t := meta.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if meta.hasDefault && meta.defaultValue != "" {
		if t.Kind() == reflect.Slice && !implementsTextUnmarshaler(t) {
			items := splitBySeparator(meta.defaultValue, meta.sep)
			elemType := t.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.String || elemType == durationType {
				quoted := make([]string, len(items))
				for i, it := range items {
					quoted[i] = fmt.Sprintf("%q", it)
				}
				return "[" + strings.Join(quoted, ", ") + "]"
			}
			return "[" + strings.Join(items, ", ") + "]"
		}
		if t.Kind() == reflect.String || t == durationType {
			return fmt.Sprintf("%q", meta.defaultValue)
		}
		return meta.defaultValue
	}
	if rootVal.IsValid() {
		if fv, ok := meta.fieldValue(rootVal, false); ok && fv.IsValid() && !isZeroValue(fv) && !meta.secret {
			return formatRuntimeValueTOML(fv)
		}
	}
	return sampleValueYAML(meta, rootVal)
}

func sampleValueObject(meta *fieldMeta, rootVal reflect.Value) interface{} {
	if meta.secret {
		return ""
	}
	t := meta.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if meta.hasDefault && meta.defaultValue != "" {
		ptr := reflect.New(t)
		if err := assignFromString(ptr.Elem(), meta.defaultValue, meta.sep, false); err == nil {
			if t == durationType {
				return meta.defaultValue
			}
			return ptr.Elem().Interface()
		}
		return meta.defaultValue
	}
	if rootVal.IsValid() {
		if fv, ok := meta.fieldValue(rootVal, false); ok && fv.IsValid() && !isZeroValue(fv) && !meta.secret {
			return formatRuntimeValueObject(fv)
		}
	}
	switch t.Kind() {
	case reflect.Bool:
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if t == durationType {
			return "0s"
		}
		return 0
	case reflect.Float32, reflect.Float64:
		return 0.0
	case reflect.Slice:
		if implementsTextUnmarshaler(t) {
			return ""
		}
		return []interface{}{}
	case reflect.Map:
		return map[string]interface{}{}
	default:
		return ""
	}
}

func formatRuntimeValueYAML(fv reflect.Value) string {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return `""`
		}
		fv = fv.Elem()
	}
	t := fv.Type()
	if t == durationType {
		return fmt.Sprintf("%q", fv.Interface().(time.Duration).String())
	}
	if t == timeType {
		return fmt.Sprintf("%q", fv.Interface().(time.Time).Format(time.RFC3339))
	}
	if implementsTextUnmarshaler(t) {
		if s, ok := fv.Interface().(fmt.Stringer); ok {
			return fmt.Sprintf("%q", s.String())
		}
		return fmt.Sprintf("%q", fmt.Sprintf("%v", fv.Interface()))
	}
	switch fv.Kind() {
	case reflect.String:
		return fmt.Sprintf("%q", fv.String())
	case reflect.Bool:
		if fv.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprintf("%d", fv.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", fv.Float())
	case reflect.Slice:
		var items []string
		for i := 0; i < fv.Len(); i++ {
			elem := fv.Index(i)
			if elem.Kind() == reflect.Ptr && !elem.IsNil() {
				elem = elem.Elem()
			}
			if elem.Type() == durationType {
				items = append(items, fmt.Sprintf("%q", elem.Interface().(time.Duration).String()))
			} else if elem.Kind() == reflect.String {
				items = append(items, fmt.Sprintf("%q", elem.String()))
			} else {
				items = append(items, fmt.Sprintf("%v", elem.Interface()))
			}
		}
		return "[" + strings.Join(items, ", ") + "]"
	case reflect.Map:
		if fv.Len() == 0 {
			return "{}"
		}
		var parts []string
		for _, k := range fv.MapKeys() {
			val := fv.MapIndex(k)
			parts = append(parts, fmt.Sprintf("%v: %s", k.Interface(), formatRuntimeValueYAML(val)))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", fv.Interface())
	}
}

func formatRuntimeValueTOML(fv reflect.Value) string {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return `""`
		}
		fv = fv.Elem()
	}
	t := fv.Type()
	if t == durationType {
		return fmt.Sprintf("%q", fv.Interface().(time.Duration).String())
	}
	if t == timeType {
		return fmt.Sprintf("%q", fv.Interface().(time.Time).Format(time.RFC3339))
	}
	if implementsTextUnmarshaler(t) {
		if s, ok := fv.Interface().(fmt.Stringer); ok {
			return fmt.Sprintf("%q", s.String())
		}
		return fmt.Sprintf("%q", fmt.Sprintf("%v", fv.Interface()))
	}
	switch fv.Kind() {
	case reflect.String:
		return fmt.Sprintf("%q", fv.String())
	case reflect.Bool:
		if fv.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprintf("%d", fv.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", fv.Float())
	case reflect.Slice:
		var items []string
		for i := 0; i < fv.Len(); i++ {
			elem := fv.Index(i)
			if elem.Kind() == reflect.Ptr && !elem.IsNil() {
				elem = elem.Elem()
			}
			if elem.Type() == durationType {
				items = append(items, fmt.Sprintf("%q", elem.Interface().(time.Duration).String()))
			} else if elem.Kind() == reflect.String {
				items = append(items, fmt.Sprintf("%q", elem.String()))
			} else {
				items = append(items, fmt.Sprintf("%v", elem.Interface()))
			}
		}
		return "[" + strings.Join(items, ", ") + "]"
	case reflect.Map:
		if fv.Len() == 0 {
			return "{}"
		}
		var parts []string
		for _, k := range fv.MapKeys() {
			val := fv.MapIndex(k)
			parts = append(parts, fmt.Sprintf("%v = %s", k.Interface(), formatRuntimeValueTOML(val)))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		return fmt.Sprintf("%v", fv.Interface())
	}
}

func formatRuntimeValueObject(fv reflect.Value) interface{} {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}
	t := fv.Type()
	if t == durationType {
		return fv.Interface().(time.Duration).String()
	}
	if t == timeType {
		return fv.Interface().(time.Time).Format(time.RFC3339)
	}
	if implementsTextUnmarshaler(t) {
		if s, ok := fv.Interface().(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprintf("%v", fv.Interface())
	}
	switch fv.Kind() {
	case reflect.Slice:
		var items []interface{}
		for i := 0; i < fv.Len(); i++ {
			elem := fv.Index(i)
			items = append(items, formatRuntimeValueObject(elem))
		}
		return items
	case reflect.Map:
		out := make(map[string]interface{})
		for _, k := range fv.MapKeys() {
			out[fmt.Sprintf("%v", k.Interface())] = formatRuntimeValueObject(fv.MapIndex(k))
		}
		return out
	default:
		return fv.Interface()
	}
}
