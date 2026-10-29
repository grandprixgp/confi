package confi

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var supportedTimeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	time.RFC1123,
	time.RFC822,
}

func implementsTextUnmarshaler(t reflect.Type) bool {
	return t.Implements(textUnmarshalerType) || reflect.PtrTo(t).Implements(textUnmarshalerType)
}

// assignFromString parses a single string (such as from an environment variable or default tag)
// and assigns it to fv.
func assignFromString(fv reflect.Value, raw string, sep string, appendMode bool) error {
	t := fv.Type()
	if implementsTextUnmarshaler(t) {
		return assignScalar(fv, raw)
	}
	if t.Kind() == reflect.Ptr && (t.Elem().Kind() == reflect.Slice || t.Elem().Kind() == reflect.Map) && !implementsTextUnmarshaler(t.Elem()) {
		if fv.IsNil() {
			fv.Set(reflect.New(t.Elem()))
		}
		return assignFromString(fv.Elem(), raw, sep, appendMode)
	}
	if t.Kind() == reflect.Slice {
		if strings.TrimSpace(raw) == "" {
			if !appendMode {
				fv.Set(reflect.MakeSlice(t, 0, 0))
			}
			return nil
		}
		parts := splitBySeparator(raw, sep)
		return assignSliceElements(fv, parts, appendMode)
	}
	if t.Kind() == reflect.Map {
		if strings.TrimSpace(raw) == "" {
			if !appendMode {
				fv.Set(reflect.MakeMap(t))
			}
			return nil
		}
		parts := splitBySeparator(raw, sep)
		return assignMapEntries(fv, parts, appendMode)
	}
	return assignScalar(fv, raw)
}

// assignFromStrings parses one or more CLI argument tokens and assigns them to fv.
func assignFromStrings(fv reflect.Value, raws []string, sep string, appendMode bool) error {
	t := fv.Type()
	if implementsTextUnmarshaler(t) {
		if len(raws) == 0 {
			return fmt.Errorf("missing value for %s", t.String())
		}
		return assignScalar(fv, raws[len(raws)-1])
	}
	if t.Kind() == reflect.Ptr && (t.Elem().Kind() == reflect.Slice || t.Elem().Kind() == reflect.Map) && !implementsTextUnmarshaler(t.Elem()) {
		if fv.IsNil() {
			fv.Set(reflect.New(t.Elem()))
		}
		return assignFromStrings(fv.Elem(), raws, sep, appendMode)
	}

	switch t.Kind() {
	case reflect.Slice:
		var items []string
		for _, r := range raws {
			if sep != "" && strings.Contains(r, sep) {
				items = append(items, splitBySeparator(r, sep)...)
				continue
			}
			items = append(items, r)
		}
		return assignSliceElements(fv, items, appendMode)

	case reflect.Map:
		var entries []string
		for _, r := range raws {
			if sep != "" && strings.Contains(r, sep) {
				entries = append(entries, splitBySeparator(r, sep)...)
			} else {
				entries = append(entries, r)
			}
		}
		return assignMapEntries(fv, entries, appendMode)

	case reflect.String:
		if len(raws) == 0 {
			fv.SetString("")
			return nil
		}
		fv.SetString(strings.Join(raws, " "))
		return nil

	default:
		if len(raws) == 0 {
			if isBoolValue(fv) {
				return assignScalar(fv, "true")
			}
			return fmt.Errorf("missing value for %s", t.String())
		}
		// If multiple values were given to a scalar, use the last one (or join if pointer to string).
		if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.String {
			if fv.IsNil() {
				fv.Set(reflect.New(t.Elem()))
			}
			fv.Elem().SetString(strings.Join(raws, " "))
			return nil
		}
		return assignScalar(fv, raws[len(raws)-1])
	}
}

func splitBySeparator(raw string, sep string) []string {
	if sep == "" {
		sep = ","
	}
	rawParts := strings.Split(raw, sep)
	out := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func assignSliceElements(sliceVal reflect.Value, items []string, appendMode bool) error {
	elemType := sliceVal.Type().Elem()
	result := sliceVal
	if !appendMode || result.IsNil() {
		result = reflect.MakeSlice(sliceVal.Type(), 0, len(items))
	}
	for _, item := range items {
		elemPtr := reflect.New(elemType)
		if err := assignScalar(elemPtr.Elem(), item); err != nil {
			return err
		}
		result = reflect.Append(result, elemPtr.Elem())
	}
	sliceVal.Set(result)
	return nil
}

func assignMapEntries(mapVal reflect.Value, entries []string, appendMode bool) error {
	mapType := mapVal.Type()
	keyType := mapType.Key()
	valType := mapType.Elem()

	targetMap := mapVal
	if !appendMode || targetMap.IsNil() {
		targetMap = reflect.MakeMap(mapType)
	}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		var kStr, vStr string
		eqIdx := strings.Index(entry, "=")
		colonIdx := strings.Index(entry, ":")
		if eqIdx > 0 && (colonIdx < 0 || eqIdx < colonIdx) {
			kStr = strings.TrimSpace(entry[:eqIdx])
			vStr = strings.TrimSpace(entry[eqIdx+1:])
		} else if colonIdx > 0 {
			kStr = strings.TrimSpace(entry[:colonIdx])
			vStr = strings.TrimSpace(entry[colonIdx+1:])
		} else {
			return fmt.Errorf("invalid map entry %q (expected key=value or key:value)", entry)
		}

		kPtr := reflect.New(keyType)
		if err := assignScalar(kPtr.Elem(), kStr); err != nil {
			return fmt.Errorf("invalid map key %q: %v", kStr, err)
		}
		vPtr := reflect.New(valType)
		if err := assignScalar(vPtr.Elem(), vStr); err != nil {
			return fmt.Errorf("invalid map value %q for key %q: %v", vStr, kStr, err)
		}
		targetMap.SetMapIndex(kPtr.Elem(), vPtr.Elem())
	}

	mapVal.Set(targetMap)
	return nil
}

func assignScalar(fv reflect.Value, raw string) error {
	if !fv.CanSet() {
		return fmt.Errorf("field of type %s cannot be set", fv.Type())
	}

	t := fv.Type()

	// Handle pointers by allocating if nil and unwrapping
	if t.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(t.Elem()))
		}
		// Special case: *url.URL, *regexp.Regexp, or pointer implementing TextUnmarshaler
		return assignScalar(fv.Elem(), raw)
	}

	// Special standard library types
	if t == durationType {
		d, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			// Also allow plain integer nanoseconds or seconds if suffixed, or return duration parse error
			return fmt.Errorf("invalid duration %q: %v", raw, err)
		}
		fv.SetInt(int64(d))
		return nil
	}

	if t == timeType {
		ts, err := parseTimeValue(raw)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(ts))
		return nil
	}

	if t == urlType {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("invalid URL %q: %v", raw, err)
		}
		fv.Set(reflect.ValueOf(*u))
		return nil
	}

	if t == regexpType {
		re, err := regexp.Compile(raw)
		if err != nil {
			return fmt.Errorf("invalid regexp %q: %v", raw, err)
		}
		fv.Set(reflect.ValueOf(*re))
		return nil
	}

	// Check encoding.TextUnmarshaler
	if fv.CanAddr() && fv.Addr().Type().Implements(textUnmarshalerType) {
		unmarshaler := fv.Addr().Interface().(encoding.TextUnmarshaler)
		if err := unmarshaler.UnmarshalText([]byte(raw)); err != nil {
			return fmt.Errorf("cannot unmarshal %q into %s: %v", raw, t.String(), err)
		}
		return nil
	}

	switch t.Kind() {
	case reflect.String:
		fv.SetString(raw)
		return nil

	case reflect.Bool:
		b, err := parseBoolFlexible(raw)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q", raw)
		}
		fv.SetBool(b)
		return nil

	case reflect.Int:
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 0, strconv.IntSize)
		if err != nil {
			return fmt.Errorf("invalid int value %q: %v", raw, err)
		}
		fv.SetInt(v)
		return nil
	case reflect.Int8:
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 0, 8)
		if err != nil {
			return fmt.Errorf("invalid int8 value %q: %v", raw, err)
		}
		fv.SetInt(v)
		return nil
	case reflect.Int16:
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 0, 16)
		if err != nil {
			return fmt.Errorf("invalid int16 value %q: %v", raw, err)
		}
		fv.SetInt(v)
		return nil
	case reflect.Int32:
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 0, 32)
		if err != nil {
			return fmt.Errorf("invalid int32 value %q: %v", raw, err)
		}
		fv.SetInt(v)
		return nil
	case reflect.Int64:
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 0, 64)
		if err != nil {
			return fmt.Errorf("invalid int64 value %q: %v", raw, err)
		}
		fv.SetInt(v)
		return nil

	case reflect.Uint:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, strconv.IntSize)
		if err != nil {
			return fmt.Errorf("invalid uint value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil
	case reflect.Uint8:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 8)
		if err != nil {
			return fmt.Errorf("invalid uint8 value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil
	case reflect.Uint16:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 16)
		if err != nil {
			return fmt.Errorf("invalid uint16 value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil
	case reflect.Uint32:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 32)
		if err != nil {
			return fmt.Errorf("invalid uint32 value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil
	case reflect.Uint64:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 64)
		if err != nil {
			return fmt.Errorf("invalid uint64 value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil
	case reflect.Uintptr:
		v, err := strconv.ParseUint(strings.TrimSpace(raw), 0, strconv.IntSize)
		if err != nil {
			return fmt.Errorf("invalid uintptr value %q: %v", raw, err)
		}
		fv.SetUint(v)
		return nil

	case reflect.Float32:
		v, err := strconv.ParseFloat(strings.TrimSpace(raw), 32)
		if err != nil {
			return fmt.Errorf("invalid float32 value %q: %v", raw, err)
		}
		fv.SetFloat(v)
		return nil
	case reflect.Float64:
		v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return fmt.Errorf("invalid float64 value %q: %v", raw, err)
		}
		fv.SetFloat(v)
		return nil

	default:
		return fmt.Errorf("unsupported field type %s", t.String())
	}
}

func parseBoolFlexible(raw string) (bool, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	default:
		return strconv.ParseBool(s)
	}
}

func parseTimeValue(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	for _, layout := range supportedTimeLayouts {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts, nil
		}
	}
	if unixSec, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(unixSec, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time value %q", raw)
}

func isBoolValue(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Bool
}

// isZeroValue reports whether v is at its zero value.
func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !isZeroValue(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if v.Type() == timeType {
			if v.CanInterface() {
				return v.Interface().(time.Time).IsZero()
			}
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
