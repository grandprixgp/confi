package confi

import (
	"fmt"
	"reflect"
	"strings"
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

		if meta.required && isZero {
			if !(meta.isBoolean() && isSet) {
				missing = append(missing, meta.flagName)
				continue
			}
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
				elemStr := fmt.Sprintf("%v", fv.Index(i).Interface())
				if !matchesOneOf(elemStr, meta.oneOf) {
					return &ParseError{
						Field:   meta.fieldName,
						Message: fmt.Sprintf("value %q must be one of [%s]", elemStr, strings.Join(meta.oneOf, ", ")),
					}
				}
			}
		} else {
			valStr := fmt.Sprintf("%v", fv.Interface())
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
			num := float64(fv.Int())
			if meta.hasMin && num < meta.minVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v is less than minimum %v", fv.Int(), meta.minVal)}
			}
			if meta.hasMax && num > meta.maxVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v exceeds maximum %v", fv.Int(), meta.maxVal)}
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			num := float64(fv.Uint())
			if meta.hasMin && num < meta.minVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v is less than minimum %v", fv.Uint(), meta.minVal)}
			}
			if meta.hasMax && num > meta.maxVal {
				return &ParseError{Field: meta.fieldName, Message: fmt.Sprintf("value %v exceeds maximum %v", fv.Uint(), meta.maxVal)}
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

func invokeCustomValidators(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	// Recurse into struct fields first so leaf/nested structs validate before parent
	if v.Kind() == reflect.Struct {
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			sf := t.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			fv := v.Field(i)
			if fv.Kind() == reflect.Struct || (fv.Kind() == reflect.Ptr && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct) {
				if err := invokeCustomValidators(fv); err != nil {
					return err
				}
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
	}

	return nil
}
