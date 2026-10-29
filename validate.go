package confi

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Validator can be implemented by a configuration struct (or any nested struct/field)
// to perform custom post-parse validation.
type Validator interface {
	Validate() error
}

// Runner can be implemented by a subcommand struct (or root struct) to execute command logic
// automatically when WithAutoRun(true) is enabled or when Parser.RunCommand is called.
type Runner interface {
	Run() error
}

var (
	validatorType = reflect.TypeOf((*Validator)(nil)).Elem()
	runnerType    = reflect.TypeOf((*Runner)(nil)).Elem()
)

func validateSchemaFields(schema *structSchema, rootVal reflect.Value, sources map[*fieldMeta]valueSource) error {
	var missing []string

	for _, meta := range schema.fields {
		fv, ok := meta.fieldValue(rootVal, false)
		isSet := sources[meta] != sourceNone
		isZero := !ok || isZeroValue(fv)

		// A required field is missing if it was not explicitly set and remains zero-valued.
		// If it was explicitly set (e.g. --port 0, --rate 0.0, --verbose=false), it is valid.
		if meta.required && !isSet && isZero {
			missing = append(missing, meta.flagName)
			continue
		}

		// Skip constraint checks on unset zero-valued optional fields
		if isZero && !isSet {
			continue
		}

		if err := validateFieldConstraints(meta, fv); err != nil {
			return err
		}
	}

	if len(missing) > 0 {
		return &ValidationError{MissingFields: missing}
	}

	return nil
}

func validateFieldConstraints(meta *fieldMeta, fv reflect.Value) error {
	if !fv.IsValid() {
		return nil
	}
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}

	// 1. Validate oneOf / enum
	if len(meta.oneOf) > 0 {
		if fv.Kind() == reflect.Slice && !implementsTextUnmarshaler(fv.Type()) {
			for i := 0; i < fv.Len(); i++ {
				elemStr := formatValueForOneOf(fv.Index(i))
				if !matchesOneOf(elemStr, meta.oneOf) {
					return &ParseError{
						Field:   meta.fieldName,
						Message: fmt.Sprintf("value %q must be one of [%s]", elemStr, strings.Join(meta.oneOf, ", ")),
					}
				}
			}
		} else {
			valStr := formatValueForOneOf(fv)
			if !matchesOneOf(valStr, meta.oneOf) {
				return &ParseError{
					Field:   meta.fieldName,
					Message: fmt.Sprintf("value %q must be one of [%s]", valStr, strings.Join(meta.oneOf, ", ")),
				}
			}
		}
	}

	// 2. Validate min / max
	if meta.hasMin || meta.hasMax {
		switch fv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if fv.Type() == durationType {
				dur := time.Duration(fv.Int())
				if meta.hasMin && float64(dur) < meta.minVal {
					return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("duration %v is less than minimum %v", dur, time.Duration(meta.minVal))}
				}
				if meta.hasMax && float64(dur) > meta.maxVal {
					return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("duration %v exceeds maximum %v", dur, time.Duration(meta.maxVal))}
				}
			} else {
				val := fv.Int()
				if meta.hasMin {
					minI, err := strconv.ParseInt(meta.minStr, 10, 64)
					if err != nil {
						minI = int64(meta.minVal)
					}
					if val < minI {
						return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v is less than minimum %v", val, minI)}
					}
				}
				if meta.hasMax {
					maxI, err := strconv.ParseInt(meta.maxStr, 10, 64)
					if err != nil {
						maxI = int64(meta.maxVal)
					}
					if val > maxI {
						return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v exceeds maximum %v", val, maxI)}
					}
				}
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val := fv.Uint()
			if meta.hasMin {
				minU, err := strconv.ParseUint(meta.minStr, 10, 64)
				if err != nil {
					minU = uint64(meta.minVal)
				}
				if val < minU {
					return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v is less than minimum %v", val, minU)}
				}
			}
			if meta.hasMax {
				maxU, err := strconv.ParseUint(meta.maxStr, 10, 64)
				if err != nil {
					maxU = uint64(meta.maxVal)
				}
				if val > maxU {
					return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v exceeds maximum %v", val, maxU)}
				}
			}
		case reflect.Float32, reflect.Float64:
			num := fv.Float()
			if meta.hasMin && num < meta.minVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v is less than minimum %v", num, meta.minVal)}
			}
			if meta.hasMax && num > meta.maxVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v exceeds maximum %v", num, meta.maxVal)}
			}
		case reflect.String:
			length := float64(len(fv.String()))
			if meta.hasMin && length < meta.minVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("length %d is less than minimum %v", len(fv.String()), meta.minVal)}
			}
			if meta.hasMax && length > meta.maxVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("length %d exceeds maximum %v", len(fv.String()), meta.maxVal)}
			}
		case reflect.Slice, reflect.Map:
			length := float64(fv.Len())
			if meta.hasMin && length < meta.minVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("count %d is less than minimum %v", fv.Len(), meta.minVal)}
			}
			if meta.hasMax && length > meta.maxVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("count %d exceeds maximum %v", fv.Len(), meta.maxVal)}
			}
		}
	}

	// 3. Validate regex pattern
	if meta.compiledPattern != nil {
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
			for i := 0; i < fv.Len(); i++ {
				s := fv.Index(i).String()
				if !meta.compiledPattern.MatchString(s) {
					return &ParseError{
						Field:   meta.fieldName,
						Message: fmt.Sprintf("value %q does not match pattern %q", s, meta.pattern),
					}
				}
			}
		} else if fv.Kind() == reflect.String {
			s := fv.String()
			if !meta.compiledPattern.MatchString(s) {
				return &ParseError{
					Field:   meta.fieldName,
					Message: fmt.Sprintf("value %q does not match pattern %q", s, meta.pattern),
				}
			}
		}
	}

	return nil
}

func matchesOneOf(val string, choices []string) bool {
	for _, c := range choices {
		if val == c {
			return true
		}
	}
	return false
}

func formatValueForOneOf(v reflect.Value) string {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == durationType {
			return time.Duration(v.Int()).String()
		}
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	default:
		if v.CanInterface() {
			return fmt.Sprintf("%v", v.Interface())
		}
		return ""
	}
}

func invokeCustomValidators(v reflect.Value) error {
	visited := make(map[uintptr]bool)
	return invokeCustomValidatorsHelper(v, visited)
}

func invokeCustomValidatorsHelper(v reflect.Value, visited map[uintptr]bool) error {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		ptr := v.Pointer()
		if visited[ptr] {
			return nil
		}
		visited[ptr] = true
		v = v.Elem()
	}

	// Recurse into child elements first so leaf/nested elements validate before parent
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			sf := t.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			fv := v.Field(i)
			if err := invokeCustomValidatorsHelper(fv, visited); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := invokeCustomValidatorsHelper(v.Index(i), visited); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if err := invokeCustomValidatorsHelper(v.MapIndex(key), visited); err != nil {
				return err
			}
		}
	}

	if v.CanAddr() && v.Addr().Type().Implements(validatorType) {
		if err := v.Addr().Interface().(Validator).Validate(); err != nil {
			return err
		}
	} else if v.Type().Implements(validatorType) {
		if err := v.Interface().(Validator).Validate(); err != nil {
			return err
		}
	} else if reflect.PtrTo(v.Type()).Implements(validatorType) {
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		if err := vp.Interface().(Validator).Validate(); err != nil {
			return err
		}
	}

	return nil
}
