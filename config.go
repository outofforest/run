package run

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/pflag"

	"github.com/outofforest/logger"
)

// ReadConfig reads config from file and CLI flags.
// First, config file is read and default values are taken from there.
// Then, those values (or respective env variables) are used as defaults for CLI flags.
// Finally, config fields are set based on the flags.
func ReadConfig(appName string, args []string, config any) error {
	if config == nil {
		return errors.New("config must be provided")
	}

	t := reflect.TypeOf(config)
	if t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		return errors.New("config must be a pointer to struct")
	}

	var configFile string
	configEnvName := strings.ToUpper(appName) + "_CONFIG_FILE"

	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	flags.ParseErrorsAllowlist.UnknownFlags = true
	flags.StringVar(&configFile, "config", defaultString(configEnvName, ""),
		"")
	// Dummy flag to turn off printing usage of this flag set
	flags.BoolP("help", "h", false, "")
	_ = flags.Parse(args)

	if err := readConfigFromFile(appName, configFile, config); err != nil {
		return err
	}

	flags = pflag.NewFlagSet(appName, pflag.ExitOnError)
	logger.AddFlags(logger.DefaultConfig, flags)

	flags.String("config", defaultString(configEnvName, ""),
		fmt.Sprintf("File to read configuration from (env: %s)", configEnvName))

	v := reflect.ValueOf(config).Elem()
	for i := range v.NumField() {
		field := v.Type().FieldByIndex([]int{i})
		fieldValue := v.FieldByIndex([]int{i})

		description := field.Tag.Get("description")
		if description == "" {
			return errors.Errorf("field %s has no description", field.Name)
		}
		flagName := fieldNameToFlagName(field.Name)
		envName := strings.ToUpper(appName + "_" + strings.ReplaceAll(flagName, "-", "_"))
		description += fmt.Sprintf(" (env: %s)", envName)

		switch field.Type.Kind() {
		case reflect.Bool:
			flags.BoolVar(fieldValue.Addr().Interface().(*bool), flagName,
				defaultBool(envName, fieldValue.Interface().(bool)), description)
		case reflect.Int:
			flags.IntVar(fieldValue.Addr().Interface().(*int), flagName,
				defaultInt(envName, fieldValue.Interface().(int)), description)
		case reflect.Slice:
			elem := field.Type.Elem()
			switch elem.Kind() {
			case reflect.String:
				flags.StringSliceVar(fieldValue.Addr().Interface().(*[]string), flagName,
					defaultStringSlice(envName, fieldValue.Interface().([]string)), description)
			default:
				return errors.Errorf("field type %s is not supported for field %s", field.Type, field.Name)
			}
		case reflect.String:
			flags.StringVar(fieldValue.Addr().Interface().(*string), flagName,
				defaultString(envName, fieldValue.Interface().(string)), description)
		default:
			return errors.Errorf("field type %s is not supported for field %s", field.Type, field.Name)
		}
	}

	_ = flags.Parse(args)
	return nil
}

func readConfigFromFile(appName string, configFile string, config any) error {
	if configFile == "" {
		return nil
	}

	f, err := os.Open(configFile)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	globalConfig := map[string]json.RawMessage{}

	if err := json.NewDecoder(f).Decode(&globalConfig); err != nil {
		return errors.WithStack(err)
	}

	appConfig, exists := globalConfig[appName]
	if !exists {
		return nil
	}

	return errors.WithStack(json.Unmarshal(appConfig, &config))
}

func fieldNameToFlagName(fieldName string) string {
	var flagName strings.Builder
	wasPrevUpper := true

	lastI := len([]rune(fieldName)) - 1
	for i, r := range fieldName {
		if unicode.IsUpper(r) {
			if !wasPrevUpper || (i > 0 && i < lastI && unicode.IsLower(rune(fieldName[i+1]))) {
				flagName.WriteRune('-')
			}
			wasPrevUpper = true
		} else {
			wasPrevUpper = false
		}
		flagName.WriteRune(unicode.ToLower(r))
	}

	return flagName.String()
}

func defaultString(envName string, defaultValue string) string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

func defaultBool(envName string, defaultValue bool) bool {
	envValue := os.Getenv(envName)
	switch {
	case strings.ToLower(envValue) == "true" || envValue == "1":
		return true
	case strings.ToLower(envValue) == "false" || envValue == "0":
		return false
	default:
		return defaultValue
	}
}

func defaultStringSlice(envName string, defaultValue []string) []string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return strings.Split(envValue, ",")
	}
	return defaultValue
}

func defaultInt(envName string, defaultValue int) int {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return lo.Must(strconv.Atoi(envValue))
	}
	return defaultValue
}
