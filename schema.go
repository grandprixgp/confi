package confi

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	durationType        = reflect.TypeOf(time.Duration(0))
	timeType            = reflect.TypeOf(time.Time{})
	urlType             = reflect.TypeOf(url.URL{})
	regexpType          = reflect.TypeOf(regexp.Regexp{})
)

// fieldMeta holds metadata extracted from a struct field and its tags.
type fieldMeta struct {
	fieldName    string
	indexPath    []int
	fieldType    reflect.Type
	flagName     string
	configPath   []string
	aliases      []string
	short        string
	env          string
	autoEnvName  string
	defaultValue string
	hasDefault   bool
	required     bool
	usage        string
	sep          string
	isConfigFile bool
	isPositional bool
}

type structSchema struct {
	fields           []*fieldMeta
	byFlag           map[string]*fieldMeta
	byShort          map[string]*fieldMeta
	positionalFields []*fieldMeta
	configFileField  *fieldMeta
}

func extractSchema(target interface{}) (*structSchema, reflect.Value, error) {
	if target == nil {
		return nil, reflect.Value{}, &ParseError{Message: "target cannot be nil; expected a pointer to a struct"}
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, reflect.Value{}, &ParseError{Message: "target must be a non-nil pointer to a struct"}
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, reflect.Value{}, &ParseError{Message: fmt.Sprintf("target must point to a struct, got pointer to %s", elem.Kind())}
	}

	schema := &structSchema{
		byFlag:  make(map[string]*fieldMeta),
		byShort: make(map[string]*fieldMeta),
	}

	if err := walkStructType(elem.Type(), nil, nil, nil, schema); err != nil {
		return nil, reflect.Value{}, err
	}

	return schema, elem, nil
}

func walkStructType(t reflect.Type, indexPrefix []int, namePrefix []string, envPrefix []string, schema *structSchema) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		// Skip unexported fields
		if sf.PkgPath != "" {
			continue
		}

		confiTag := sf.Tag.Get("confi")
		if confiTag == "-" {
			continue
		}

		opts := parseConfiTag(confiTag)
		if opts.ignore {
			continue
		}

		indexPath := append(append([]int(nil), indexPrefix...), i)

		// Determine if this is a nested struct (and not a leaf struct like time.Time or TextUnmarshaler)
		ft := sf.Type
		derefType := ft
		if derefType.Kind() == reflect.Ptr {
			derefType = derefType.Elem()
		}

		if isNestedStruct(derefType) {
			squash := sf.Anonymous || opts.squash
			nextNamePrefix := namePrefix
			nextEnvPrefix := envPrefix
			if !squash {
				segName := opts.name
				if segName == "" {
					segName = cleanTagName(sf.Tag.Get("json"))
				}
				if segName == "" {
					segName = cleanTagName(sf.Tag.Get("yaml"))
				}
				if segName == "" {
					segName = toKebabCase(sf.Name)
				}
				envSeg := opts.env
				if envSeg == "" {
					envSeg = toSnakeUpper(sf.Name)
				}
				nextNamePrefix = append(append([]string(nil), namePrefix...), segName)
				nextEnvPrefix = append(append([]string(nil), envPrefix...), envSeg)
			}
			if err := walkStructType(derefType, indexPath, nextNamePrefix, nextEnvPrefix, schema); err != nil {
				return err
			}
			continue
		}

		meta := buildFieldMeta(sf, indexPath, namePrefix, envPrefix, opts)
		schema.fields = append(schema.fields, meta)

		if meta.isPositional {
			schema.positionalFields = append(schema.positionalFields, meta)
			continue
		}

		if meta.isConfigFile && schema.configFileField == nil {
			schema.configFileField = meta
		}

		for _, alias := range meta.aliases {
			lower := strings.ToLower(alias)
			if _, exists := schema.byFlag[lower]; !exists {
				schema.byFlag[lower] = meta
			}
		}

		if meta.short != "" {
			if existing, exists := schema.byShort[meta.short]; exists && existing != meta {
				return &ParseError{
					Field:   meta.fieldName,
					Message: fmt.Sprintf("duplicate short flag %q on fields %s and %s", meta.short, existing.fieldName, meta.fieldName),
				}
			}
			schema.byShort[meta.short] = meta
		}
	}
	return nil
}

func isNestedStruct(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	if t == timeType || t == urlType || t == regexpType {
		return false
	}
	if t.Implements(textUnmarshalerType) || reflect.PtrTo(t).Implements(textUnmarshalerType) {
		return false
	}
	return true
}

type tagOptions struct {
	name         string
	short        string
	env          string
	defaultValue string
	hasDefault   bool
	required     bool
	usage        string
	sep          string
	squash       bool
	isConfig     bool
	isPositional bool
	ignore       bool
}

func parseConfiTag(tag string) tagOptions {
	var opts tagOptions
	if tag == "" {
		return opts
	}
	if tag == "-" {
		opts.ignore = true
		return opts
	}

	parts := splitTagParts(tag)
	for idx, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}

		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			val := unquoteTagVal(strings.TrimSpace(kv[1]))
			switch key {
			case "name", "flag":
				opts.name = val
			case "short":
				opts.short = val
			case "env":
				opts.env = val
			case "default":
				opts.defaultValue = val
				opts.hasDefault = true
			case "usage", "help", "desc", "description":
				opts.usage = val
			case "sep", "delimiter":
				opts.sep = val
			case "required":
				opts.required = strings.EqualFold(val, "true") || val == "1"
			case "config", "config_file":
				opts.isConfig = strings.EqualFold(val, "true") || val == "1"
			case "positional", "pos":
				opts.isPositional = strings.EqualFold(val, "true") || val == "1"
			}
			continue
		}

		lower := strings.ToLower(part)
		switch lower {
		case "required":
			opts.required = true
		case "squash", "inline":
			opts.squash = true
		case "config", "config_file", "is_config":
			opts.isConfig = true
		case "positional", "pos":
			opts.isPositional = true
		default:
			if idx == 0 && opts.name == "" {
				opts.name = part
			}
		}
	}
	return opts
}

// splitTagParts splits a comma-separated tag string while respecting single-quoted substrings.
func splitTagParts(s string) []string {
	var parts []string
	var buf strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' {
			inQuote = !inQuote
			buf.WriteByte(ch)
			continue
		}
		if ch == ',' && !inQuote {
			parts = append(parts, buf.String())
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	parts = append(parts, buf.String())
	return parts
}

func unquoteTagVal(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

func buildFieldMeta(sf reflect.StructField, indexPath []int, namePrefix []string, envPrefix []string, opts tagOptions) *fieldMeta {
	// Resolve primary base name
	baseName := opts.name
	if baseName == "" {
		if v := sf.Tag.Get("flag"); v != "" {
			baseName = cleanTagName(v)
		} else if v := sf.Tag.Get("name"); v != "" {
			baseName = cleanTagName(v)
		}
	}
	kebabField := toKebabCase(sf.Name)
	snakeField := toSnakeCase(sf.Name)
	if baseName == "" {
		baseName = kebabField
	}

	// Build full flag name and config path
	configPath := append(append([]string(nil), namePrefix...), baseName)
	flagName := strings.Join(configPath, ".")

	// Collect aliases for flexible matching
	aliasSet := make(map[string]struct{})
	var aliases []string
	addAlias := func(a string) {
		a = strings.TrimSpace(a)
		if a == "" {
			return
		}
		lower := strings.ToLower(a)
		if _, ok := aliasSet[lower]; !ok {
			aliasSet[lower] = struct{}{}
			aliases = append(aliases, lower)
		}
	}

	addAlias(flagName)
	addAlias(strings.Join(configPath, "-"))
	addAlias(strings.Join(configPath, "_"))
	if len(namePrefix) == 0 {
		addAlias(baseName)
		addAlias(kebabField)
		addAlias(snakeField)
		addAlias(strings.ToLower(sf.Name))
		if jName := cleanTagName(sf.Tag.Get("json")); jName != "" {
			addAlias(jName)
		}
		if yName := cleanTagName(sf.Tag.Get("yaml")); yName != "" {
			addAlias(yName)
		}
	} else {
		prefixDot := strings.Join(namePrefix, ".") + "."
		prefixDash := strings.Join(namePrefix, "-") + "-"
		prefixUnder := strings.Join(namePrefix, "_") + "_"
		for _, leaf := range []string{baseName, kebabField, snakeField, strings.ToLower(sf.Name)} {
			addAlias(prefixDot + leaf)
			addAlias(prefixDash + leaf)
			addAlias(prefixUnder + leaf)
		}
	}

	// Short flag
	short := opts.short
	if short == "" {
		short = strings.TrimSpace(sf.Tag.Get("short"))
	}
	short = strings.TrimLeft(short, "-")

	// Env variable
	env := opts.env
	if env == "" {
		env = strings.TrimSpace(sf.Tag.Get("env"))
	}
	autoEnvParts := append(append([]string(nil), envPrefix...), toSnakeUpper(sf.Name))
	autoEnvName := strings.Join(autoEnvParts, "_")

	// Default value
	defaultVal := opts.defaultValue
	hasDefault := opts.hasDefault
	if !hasDefault {
		if v, ok := sf.Tag.Lookup("default"); ok {
			defaultVal = v
			hasDefault = true
		}
	}

	// Required
	required := opts.required
	if !required {
		reqTag := strings.TrimSpace(sf.Tag.Get("required"))
		if strings.EqualFold(reqTag, "true") || reqTag == "1" {
			required = true
		}
	}

	// Usage / description
	usage := opts.usage
	if usage == "" {
		for _, key := range []string{"usage", "help", "desc", "description"} {
			if v := sf.Tag.Get(key); v != "" {
				usage = v
				break
			}
		}
	}

	// Separator for slices/maps
	sep := opts.sep
	if sep == "" {
		sep = sf.Tag.Get("sep")
	}
	if sep == "" {
		sep = ","
	}

	// Config file marker
	isConfig := opts.isConfig
	if !isConfig {
		cfgTag := strings.TrimSpace(sf.Tag.Get("config"))
		if strings.EqualFold(cfgTag, "true") || cfgTag == "1" {
			isConfig = true
		}
	}

	// Positional marker
	isPositional := opts.isPositional
	if !isPositional {
		posTag := strings.TrimSpace(sf.Tag.Get("positional"))
		if strings.EqualFold(posTag, "true") || posTag == "1" {
			isPositional = true
		}
	}

	fullFieldName := sf.Name
	if len(namePrefix) > 0 {
		fullFieldName = strings.Join(namePrefix, ".") + "." + sf.Name
	}

	return &fieldMeta{
		fieldName:    fullFieldName,
		indexPath:    indexPath,
		fieldType:    sf.Type,
		flagName:     flagName,
		configPath:   configPath,
		aliases:      aliases,
		short:        short,
		env:          env,
		autoEnvName:  autoEnvName,
		defaultValue: defaultVal,
		hasDefault:   hasDefault,
		required:     required,
		usage:        usage,
		sep:          sep,
		isConfigFile: isConfig,
		isPositional: isPositional,
	}
}

func cleanTagName(tagVal string) string {
	if tagVal == "" || tagVal == "-" {
		return ""
	}
	parts := strings.Split(tagVal, ",")
	name := strings.TrimSpace(parts[0])
	if name == "-" {
		return ""
	}
	return name
}

// fieldValue resolves a field's reflect.Value within rootStruct, allocating intermediate nil struct pointers if allocate is true.
func (m *fieldMeta) fieldValue(rootStruct reflect.Value, allocate bool) (reflect.Value, bool) {
	curr := rootStruct
	for i, idx := range m.indexPath {
		if curr.Kind() == reflect.Ptr {
			if curr.IsNil() {
				if !allocate {
					return reflect.Value{}, false
				}
				curr.Set(reflect.New(curr.Type().Elem()))
			}
			curr = curr.Elem()
		}
		curr = curr.Field(idx)
		if i < len(m.indexPath)-1 && curr.Kind() == reflect.Ptr {
			if curr.IsNil() {
				if !allocate {
					return reflect.Value{}, false
				}
				curr.Set(reflect.New(curr.Type().Elem()))
			}
			curr = curr.Elem()
		}
	}
	return curr, true
}

func (m *fieldMeta) isBoolean() bool {
	t := m.fieldType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Bool
}

// toKebabCase converts CamelCase (e.g. "MaxHTTPConnections") to kebab-case ("max-http-connections").
func toKebabCase(s string) string {
	return strings.ReplaceAll(toDelimited(s, '-'), "_", "-")
}

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
	return strings.ReplaceAll(toDelimited(s, '_'), "-", "_")
}

// toSnakeUpper converts CamelCase to UPPER_SNAKE_CASE.
func toSnakeUpper(s string) string {
	return strings.ToUpper(toSnakeCase(s))
}

func toDelimited(s string, delim rune) string {
	runes := []rune(s)
	var out []rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextIsLower) {
					if prev != '_' && prev != '-' {
						out = append(out, delim)
					}
				}
			}
			out = append(out, unicode.ToLower(r))
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
