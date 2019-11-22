package confi

import (
	"os"
	"strings"
	"reflect"
	"strconv"
	"fmt"
)

// CoreSettings ...
type CoreSettings struct {
	name string
	arguments []string
	prefix string
}

// NewCoreSettings ...
func NewCoreSettings(name string, prefix string) *CoreSettings {
	return &CoreSettings{name: name, arguments: os.Args[1:], prefix: prefix}
}

// CoreRawArgument stores raw argument information
// for example "--argument value1 value2" would be stored
// 		name: argument
//		values: [value1, value2]
type CoreRawArgument struct {
	name string
	values []string
}

// CoreArgument stores user supplied struct
// as empty interface, struct is filled with arguments
// which are processed during parsing and filled with
// values from a: cmd line or b: configuration file
type CoreArgument struct {
	arguments interface{}
}

// NewCoreArgument ...
func NewCoreArgument(arguments interface{}) *CoreArgument {
	return &CoreArgument{arguments: arguments}
}

// Initialize accepts a CoreAgument containing
// a structure filled with arguments to listen for
// and calls the CoreArgument's process function
// to proceed with parsing and populating the structure
func Initialize(args CoreArgument, configuration CoreSettings) {
	rawArguments := parse(configuration)
	if len(rawArguments) > 0 {
		args.process(rawArguments)
	}
}

// process iterates through the CoreRawArguments provided to it
// and uses each CoreRawArgument to check the fields
// of it's CoreArgument and populate them with values
// Notes:
//		structValue
//			the struct passed as empty interface to the CoreArgument
//		structField
//			each field of the struct matching the CoreRawArgument
//		structFieldType
//			the underlying type, for example if the structField is a slice 
//			of strings then the underlying type would be a slice of strings
//		structFieldKind
//			the underlying kind, for example if the structField is a slice 
//			of strings then the underlying kind would be a slice
//		structFieldElementType
//			the underlying type of the element of the composite type i.e
//			(map, slice, interface, struct, array, function, channel ...)
//			for example if the structField is a slice of strings then
//			the structfieldElementType would be string
//		structFieldElementKind
//			the underlying kind of the element of the composite type i.e
//			(map, slice, interface, struct, array, function, channel ...)
//			for example if the structField is a slice of slices of strings
//			then the structfieldElementType would be slice
func (arg *CoreArgument) process(arguments []CoreRawArgument) {
	structValue := reflect.ValueOf(arg.arguments).Elem()

	for _, argument := range arguments {
		structField := structValue.FieldByName(strings.Title(argument.name))

		if structField.IsValid() {
			structFieldKind := structField.Kind()
			structFieldType := structField.Type()
			switch structFieldKind {
			case reflect.String:
				if len(argument.values) > 1 {
					var finalValue string
					for _, value := range argument.values {
						finalValue += fmt.Sprintf("%s ", value)
					}
					structField.Set(reflect.ValueOf(finalValue))
				} else {
					structField.Set(reflect.ValueOf(argument.values[0]))
				}
			case reflect.Slice:
				structFieldElementType := structFieldType.Elem()
				structFieldElementKind := structFieldElementType.Kind()
				for _, value := range argument.values {
					switch structFieldElementKind {
					case reflect.String:
						structField.Set(reflect.Append(structField, reflect.ValueOf(value)))
					case reflect.Int8:
						newElementValueConv, err := strconv.ParseInt(value, 0, 8)
						newElementValue := int8(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Int16:
						newElementValueConv, err := strconv.ParseInt(value, 0, 16)
						newElementValue := int(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Int32:
						newElementValueConv, err := strconv.ParseInt(value, 0, 32)
						newElementValue := int32(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Int:
						newElementValueConv, err := strconv.ParseInt(value, 0, 32)
						newElementValue := int(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Int64:
						newElementValueConv, err := strconv.ParseInt(value, 0, 64)
						newElementValue := int64(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Uint8:
						newElementValueConv, err := strconv.ParseUint(value, 0, 8)
						newElementValue := uint8(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Uint16:
						newElementValueConv, err := strconv.ParseUint(value, 0, 16)
						newElementValue := uint16(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Uint32:
						newElementValueConv, err := strconv.ParseUint(value, 0, 32)
						newElementValue := uint32(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Uint:
						newElementValueConv, err := strconv.ParseUint(value, 0, 32)
						newElementValue := uint(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Uint64:
						newElementValueConv, err := strconv.ParseUint(value, 0, 64)
						newElementValue := uint64(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Float32:
						newElementValueConv, err := strconv.ParseFloat(value, 32)
						newElementValue := float32(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Float64:
						newElementValueConv, err := strconv.ParseFloat(value, 64)
						newElementValue := float64(newElementValueConv)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					case reflect.Bool:
						newElementValue, err := strconv.ParseBool(value)
						if err == nil {
							structField.Set(reflect.Append(structField, reflect.ValueOf(newElementValue)))
						}
					}
				}
			case reflect.Int8:
				newValueConv, err := strconv.ParseInt(argument.values[0], 10, 8)
				newValue := int8(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Int16:
				newValueConv, err := strconv.ParseInt(argument.values[0], 10, 16)
				newValue := int16(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Int32:
				newValueConv, err := strconv.ParseInt(argument.values[0], 10, 32)
				newValue := int32(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Int:
				newValueConv, err := strconv.ParseInt(argument.values[0], 10, 32)
				newValue := int(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Int64:
				newValueConv, err := strconv.ParseInt(argument.values[0], 10, 64)
				newValue := int64(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Uint8:
				newValueConv, err := strconv.ParseUint(argument.values[0], 10, 8)
				newValue := uint8(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Uint16:
				newValueConv, err := strconv.ParseUint(argument.values[0], 10, 16)
				newValue := uint16(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Uint32:
				newValueConv, err := strconv.ParseUint(argument.values[0], 10, 32)
				newValue := uint32(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Uint:
				newValueConv, err := strconv.ParseUint(argument.values[0], 10, 32)
				newValue := uint(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			case reflect.Uint64:
				newValueConv, err := strconv.ParseUint(argument.values[0], 10, 64)
				newValue := uint64(newValueConv)
				if err == nil {
					structField.Set(reflect.ValueOf(newValue))
				}
			}
		}
	}
}