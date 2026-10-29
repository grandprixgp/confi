package confi

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

var (
	schemaCache    sync.Map // map[reflect.Type]*structSchema
	regexCacheLock sync.RWMutex
	regexCache     = make(map[string]*regexp.Regexp)
)

func compileRegex(pattern string) (*regexp.Regexp, error) {
	regexCacheLock.RLock()
	re, ok := regexCache[pattern]
	regexCacheLock.RUnlock()
	if ok {
		return re, nil
	}
	regexCacheLock.Lock()
	defer regexCacheLock.Unlock()
	if re, ok = regexCache[pattern]; ok {
		return re, nil
	}
	var err error
	re, err = regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache[pattern] = re
	return re, nil
}

// fieldMeta holds metadata extracted from a struct field and its tags.
type fieldMeta struct {
	fieldName       string
	indexPath       []int
	fieldType       reflect.Type
	flagName        string
	configPath      []string
	aliases         []string
	short           string
	env             string
	autoEnvName     string
	defaultValue    string
	hasDefault      bool
	required        bool
	usage           string
	sep             string
	isConfigFile    bool
	isPositional    bool
	secret          bool
	oneOf           []string
	hasMin          bool
	minVal          float64
	minStr          string
	hasMax          bool
	maxVal          float64
	maxStr          string
	pattern         string
	compiledPattern *regexp.Regexp
}

type subcommandMeta struct {
	name      string
	fieldName string
	aliases   []string
	usage     string
	indexPath []int
	fieldType reflect.Type
	schema    *structSchema
}

type structSchema struct {
	fields           []*fieldMeta
	byFlag           map[string]*fieldMeta
	byShort          map[string]*fieldMeta
	positionalFields []*fieldMeta
	configFileField  *fieldMeta
	subcommands      map[string]*subcommandMeta
	subcommandList   []*subcommandMeta
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

	schema, err := buildSchemaForType(elem.Type())
	if err != nil {
		return nil, reflect.Value{}, err
	}
	return schema, elem, nil
}

func buildSchemaForType(t reflect.Type) (*structSchema, error) {
	if cached, ok := schemaCache.Load(t); ok {
		return cached.(*structSchema), nil
	}
	schema := &structSchema{
		byFlag:      make(map[string]*fieldMeta),
		byShort:     make(map[string]*fieldMeta),
		subcommands: make(map[string]*subcommandMeta),
	}
	visited := make(map[reflect.Type]bool)
	if err := walkStructType(t, nil, nil, nil, schema, visited); err != nil {
		return nil, err
	}
	schemaCache.Store(t, schema)
	return schema, nil
}

func walkStructType(t reflect.Type, indexPrefix []int, namePrefix []string, envPrefix []string, schema *structSchema, visited map[reflect.Type]bool) error {
	if visited[t] {
		return nil
	}
	visited[t] = true
	defer delete(visited, t)
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

		ft := sf.Type
		derefType := ft
		if derefType.Kind() == reflect.Ptr {
			derefType = derefType.Elem()
		}

		// Check if this struct field is a subcommand
		isSubcmd := opts.isSubcommand
		if !isSubcmd {
			subTag := strings.TrimSpace(sf.Tag.Get("subcommand"))
			if strings.EqualFold(subTag, "true") || subTag == "1" {
				isSubcmd = true
			} else if cmdTag := strings.TrimSpace(sf.Tag.Get("cmd")); cmdTag != "" {
				isSubcmd = true
				if opts.name == "" {
					opts.name = cmdTag
				}
			}
		}

		if isSubcmd && derefType.Kind() == reflect.Struct {
			subSchema, err := buildSchemaForType(derefType)
			if err != nil {
				return err
			}
			cmdName := opts.name
			if cmdName == "" {
				cmdName = toKebabCase(sf.Name)
			}
			usage := opts.usage
			if usage == "" {
				for _, key := range []string{"usage", "help", "desc", "description"} {
					if v := sf.Tag.Get(key); v != "" {
						usage = v
						break
					}
				}
			}
			var cmdAliases []string
			cmdAliases = append(cmdAliases, strings.ToLower(cmdName))
			if s := opts.short; s != "" {
				cmdAliases = append(cmdAliases, strings.ToLower(s))
			}
			if a := sf.Tag.Get("alias"); a != "" {
				for _, p := range strings.Split(a, ",") {
					if trimmed := strings.TrimSpace(p); trimmed != "" {
						cmdAliases = append(cmdAliases, strings.ToLower(trimmed))
					}
				}
			}
			subMeta := &subcommandMeta{
				name:      cmdName,
				fieldName: sf.Name,
				aliases:   cmdAliases,
				usage:     usage,
				indexPath: indexPath,
				fieldType: ft,
				schema:    subSchema,
			}
			schema.subcommandList = append(schema.subcommandList, subMeta)
			for _, al := range cmdAliases {
				schema.subcommands[al] = subMeta
			}
			continue
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
					segName = cleanTagName(sf.Tag.Get("toml"))
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
			if err := walkStructType(derefType, indexPath, nextNamePrefix, nextEnvPrefix, schema, visited); err != nil {
				return err
			}
			continue
		}

		meta, err := buildFieldMeta(sf, indexPath, namePrefix, envPrefix, opts)
		if err != nil {
			return err
		}
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
			if existing, exists := schema.byFlag[lower]; exists && existing != meta {
				return &ParseError{
					Field:   meta.fieldName,
					Message: fmt.Sprintf("duplicate flag %q on fields %s and %s", alias, existing.fieldName, meta.fieldName),
				}
			}
			schema.byFlag[lower] = meta
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
	isSubcommand bool
	secret       bool
	oneOf        string
	minStr       string
	maxStr       string
	pattern      string
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
			case "subcommand", "cmd":
				opts.isSubcommand = strings.EqualFold(val, "true") || val == "1"
			case "secret", "redact":
				opts.secret = strings.EqualFold(val, "true") || val == "1"
			case "oneof", "enum":
				opts.oneOf = val
			case "min":
				opts.minStr = val
			case "max":
				opts.maxStr = val
			case "pattern", "regex":
				opts.pattern = val
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
			if idx == 0 && opts.name == "" && lower == "config" {
				opts.name = part
			}
		case "positional", "pos":
			opts.isPositional = true
		case "subcommand", "cmd":
			opts.isSubcommand = true
		case "secret", "redact", "sensitive":
			opts.secret = true
		default:
			if idx == 0 && opts.name == "" {
				opts.name = part
			}
		}
	}
	return opts
}

// splitTagParts splits a comma-separated tag string while respecting single- and double-quoted substrings.
func splitTagParts(s string) []string {
	var parts []string
	var buf strings.Builder
	var activeQuote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '"' || s[i+1] == '\\') {
			buf.WriteByte(ch)
			buf.WriteByte(s[i+1])
			i++
			continue
		}
		if activeQuote != 0 {
			if ch == activeQuote {
				activeQuote = 0
			}
			buf.WriteByte(ch)
			continue
		}
		if ch == '\'' || ch == '"' {
			activeQuote = ch
			buf.WriteByte(ch)
			continue
		}
		if ch == ',' {
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
		quote := s[0]
		inner := s[1 : len(s)-1]
		if quote == '"' && strings.Contains(inner, `\"`) {
			inner = strings.ReplaceAll(inner, `\"`, `"`)
		} else if quote == '\'' && strings.Contains(inner, `\'`) {
			inner = strings.ReplaceAll(inner, `\'`, `'`)
		}
		return inner
	}
	return s
}

func buildFieldMeta(sf reflect.StructField, indexPath []int, namePrefix []string, envPrefix []string, opts tagOptions) (*fieldMeta, error) {
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

	configPath := append(append([]string(nil), namePrefix...), baseName)
	flagName := strings.Join(configPath, ".")

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
		for _, tagKey := range []string{"json", "yaml", "toml", "ini"} {
			if tName := cleanTagName(sf.Tag.Get(tagKey)); tName != "" {
				addAlias(tName)
			}
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

	short := opts.short
	if short == "" {
		short = strings.TrimSpace(sf.Tag.Get("short"))
	}
	short = strings.TrimLeft(short, "-")

	env := opts.env
	if env == "" {
		env = strings.TrimSpace(sf.Tag.Get("env"))
	}
	autoEnvParts := append(append([]string(nil), envPrefix...), toSnakeUpper(sf.Name))
	autoEnvName := strings.Join(autoEnvParts, "_")

	defaultVal := opts.defaultValue
	hasDefault := opts.hasDefault
	if !hasDefault {
		if v, ok := sf.Tag.Lookup("default"); ok {
			defaultVal = v
			hasDefault = true
		}
	}

	required := opts.required
	if !required {
		reqTag := strings.TrimSpace(sf.Tag.Get("required"))
		if strings.EqualFold(reqTag, "true") || reqTag == "1" {
			required = true
		}
	}

	usage := opts.usage
	if usage == "" {
		for _, key := range []string{"usage", "help", "desc", "description"} {
			if v := sf.Tag.Get(key); v != "" {
				usage = v
				break
			}
		}
	}

	sep := opts.sep
	if sep == "" {
		sep = sf.Tag.Get("sep")
	}
	if sep == "" {
		sep = ","
	}

	isConfig := opts.isConfig
	if !isConfig {
		cfgTag := strings.TrimSpace(sf.Tag.Get("config"))
		if strings.EqualFold(cfgTag, "true") || cfgTag == "1" {
			isConfig = true
		}
	}

	isPositional := opts.isPositional
	if !isPositional {
		posTag := strings.TrimSpace(sf.Tag.Get("positional"))
		if strings.EqualFold(posTag, "true") || posTag == "1" {
			isPositional = true
		}
	}

	secret := opts.secret
	if !secret {
		secTag := strings.TrimSpace(sf.Tag.Get("secret"))
		if strings.EqualFold(secTag, "true") || secTag == "1" {
			secret = true
		}
	}

	fullFieldName := sf.Name
	if len(namePrefix) > 0 {
		fullFieldName = strings.Join(namePrefix, ".") + "." + sf.Name
	}

	// Parse validation rules: oneof / enum, min, max, pattern / regex
	oneOfRaw := opts.oneOf
	if oneOfRaw == "" {
		oneOfRaw = sf.Tag.Get("oneof")
	}
	if oneOfRaw == "" {
		oneOfRaw = sf.Tag.Get("enum")
	}
	var oneOf []string
	if oneOfRaw != "" {
		delim := "|"
		if !strings.Contains(oneOfRaw, "|") && strings.Contains(oneOfRaw, ",") {
			delim = ","
		}
		for _, item := range strings.Split(oneOfRaw, delim) {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				oneOf = append(oneOf, trimmed)
			}
		}
	}

	derefFieldType := sf.Type
	if derefFieldType.Kind() == reflect.Ptr {
		derefFieldType = derefFieldType.Elem()
	}

	minStr := opts.minStr
	if minStr == "" {
		minStr = strings.TrimSpace(sf.Tag.Get("min"))
	}
	var hasMin bool
	var minVal float64
	if minStr != "" {
		var v float64
		var err error
		if derefFieldType == durationType {
			var d time.Duration
			d, err = time.ParseDuration(minStr)
			v = float64(d)
		} else {
			v, err = strconv.ParseFloat(minStr, 64)
		}
		if err != nil {
			return nil, &ParseError{Field: fullFieldName, Message: fmt.Sprintf("invalid min tag value %q: %v", minStr, err)}
		}
		hasMin = true
		minVal = v
	}

	maxStr := opts.maxStr
	if maxStr == "" {
		maxStr = strings.TrimSpace(sf.Tag.Get("max"))
	}
	var hasMax bool
	var maxVal float64
	if maxStr != "" {
		var v float64
		var err error
		if derefFieldType == durationType {
			var d time.Duration
			d, err = time.ParseDuration(maxStr)
			v = float64(d)
		} else {
			v, err = strconv.ParseFloat(maxStr, 64)
		}
		if err != nil {
			return nil, &ParseError{Field: fullFieldName, Message: fmt.Sprintf("invalid max tag value %q: %v", maxStr, err)}
		}
		hasMax = true
		maxVal = v
	}

	pattern := opts.pattern
	if pattern == "" {
		pattern = sf.Tag.Get("pattern")
	}
	if pattern == "" {
		pattern = sf.Tag.Get("regex")
	}
	var compiledPattern *regexp.Regexp
	if pattern != "" {
		re, err := compileRegex(pattern)
		if err != nil {
			return nil, &ParseError{Field: fullFieldName, Message: fmt.Sprintf("invalid regex pattern %q: %v", pattern, err)}
		}
		compiledPattern = re
	}

	return &fieldMeta{
		fieldName:       fullFieldName,
		indexPath:       indexPath,
		fieldType:       sf.Type,
		flagName:        flagName,
		configPath:      configPath,
		aliases:         aliases,
		short:           short,
		env:             env,
		autoEnvName:     autoEnvName,
		defaultValue:    defaultVal,
		hasDefault:      hasDefault,
		required:        required,
		usage:           usage,
		sep:             sep,
		isConfigFile:    isConfig,
		isPositional:    isPositional,
		secret:          secret,
		oneOf:           oneOf,
		hasMin:          hasMin,
		minVal:          minVal,
		minStr:          minStr,
		hasMax:          hasMax,
		maxVal:          maxVal,
		maxStr:          maxStr,
		pattern:         pattern,
		compiledPattern: compiledPattern,
	}, nil
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
	return resolveIndexPath(rootStruct, m.indexPath, allocate)
}

func resolveIndexPath(rootStruct reflect.Value, indexPath []int, allocate bool) (reflect.Value, bool) {
	curr := rootStruct
	for i, idx := range indexPath {
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
		if i < len(indexPath)-1 && curr.Kind() == reflect.Ptr {
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
