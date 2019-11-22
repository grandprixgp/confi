package configgle

import (
	"strings"
)

// parse takes the raw os.Args and splits them up into
// a slice of CoreRawArguments for future processing
func parse(settings CoreSettings) []CoreRawArgument {

	rawArguments := make(map[string]*CoreRawArgument)

	var previousArgumentName string

	for _, argument := range settings.arguments {
		if strings.HasPrefix(argument, settings.prefix) {
			rawArgument := new(CoreRawArgument)
			
			argumentWithoutPrefix := argument[len(settings.prefix):]
			var values []string
			var length int
			if strings.Contains(argumentWithoutPrefix, "=") {
				values = strings.Split(argumentWithoutPrefix, "=")
				length = len(values)
			} else {
				values = strings.Fields(argumentWithoutPrefix)
				length = len(values)
			}
			switch true {
			case length == 0:
				continue
			case length == 1:
				argumentWithValue := values[0]
				if strings.Contains(values[0], "=") {
					rawArgument.name, rawArgument.values = strings.Split(argumentWithValue, "=")[0], strings.Split(argumentWithValue, "=")[1:]
				} else {
					rawArgument.name = values[0]
				}
			case length > 1:
				rawArgument.name, rawArgument.values = values[0], values[1:]
			}

			previousArgumentName = rawArgument.name
			if _, alreadyExists := rawArguments[rawArgument.name]; alreadyExists {
				for _, value := range rawArgument.values {
					rawArguments[rawArgument.name].values = append(rawArguments[rawArgument.name].values, value)
				}
			} else {
				rawArguments[rawArgument.name] = rawArgument
			}
		} else {
			if len(previousArgumentName) > 0 {
				rawArguments[previousArgumentName].values = append(rawArguments[previousArgumentName].values, argument)
			}
		}
	}

	rawArgumentsReturnValue := make([]CoreRawArgument, 0)
	for _, rawArgument := range rawArguments {
		values := make([]string, 0)
		for _, value := range rawArgument.values {
			if len(value) != 0 {
				values = append(values, value)
			}
		}
		rawArgument.values = values
		rawArgumentsReturnValue = append(rawArgumentsReturnValue, *rawArgument)
	}

	return rawArgumentsReturnValue
}