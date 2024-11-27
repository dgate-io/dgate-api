package commands

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dgate-io/dgate-api/internal/config"
	ms "github.com/mitchellh/mapstructure"
)

func createMapFromArgs[N any](
	args []string,
	// tags []string,
	required ...string,
) (*N, error) {
	m := make(map[string]any)

	// parse file string
	for i, arg := range args {
		if !strings.Contains(arg, "@=") {
			continue
		}
		pair := strings.SplitN(arg, "@=", 2)
		if len(pair) != 2 || pair[0] == "" {
			return nil, fmt.Errorf("invalid key-value pair: %s", arg)
		}
		var v any
		if pair[1] != "" {
			file, err := os.ReadFile(pair[1])
			if err != nil {
				return nil, fmt.Errorf("error reading file: %s", err.Error())
			}
			v = base64.StdEncoding.EncodeToString(file)
		} else {
			v = ""
		}
		m[pair[0]] = v
		args[i] = ""
	}

	// parse json strings
	for i, arg := range args {
		if !strings.Contains(arg, ":=") {
			continue
		}
		pair := strings.SplitN(arg, ":=", 2)
		if len(pair) != 2 || pair[0] == "" {
			return nil, fmt.Errorf("invalid key-value pair: %s", arg)
		}
		var v any
		if pair[1] != "" {
			err := json.Unmarshal([]byte(pair[1]), &v)
			if err != nil {
				if _, ok := err.(*json.SyntaxError); ok {
					err = fmt.Errorf("error parsing values: invalid json for key '%s'", pair[0])
					return nil, err
				}
				return nil, fmt.Errorf("invalid json value - key:'%s' value:'%s'", pair[0], pair[1])
			}
		} else {
			v = ""
		}
		m[pair[0]] = v
		args[i] = ""
	}

	// parse raw strings
	for _, arg := range args {
		if !strings.Contains(arg, "=") {
			continue
		}
		pair := strings.SplitN(arg, "=", 2)
		if len(pair) != 2 || pair[0] == "" {
			return nil, fmt.Errorf("invalid key-value pair: %s", arg)
		}
		m[pair[0]] = pair[1]
	}

	var missingKeys []string
	for _, requiredKey := range required {
		if _, ok := m[requiredKey]; !ok {
			missingKeys = append(missingKeys, requiredKey)
		}
	}
	if len(missingKeys) > 0 {
		return nil, errors.New("missing required keys: " +
			strings.Join(required, ", "))
	}

	var resource N
	var metadata ms.Metadata
	if decoder, err := ms.NewDecoder(&ms.DecoderConfig{
		TagName:  "json",
		Result:   &resource,
		Metadata: &metadata,
		DecodeHook: ms.ComposeDecodeHookFunc(
			ms.StringToTimeDurationHookFunc(),
			ms.StringToSliceHookFunc(","),
			config.StringToBoolHookFunc(),
			config.StringToIntHookFunc(),
		),
	}); err != nil {
		return nil, err
	} else if err = decoder.Decode(m); err != nil {
		return nil, err
	}
	// add '--strict' flag to make this error a warning
	if len(metadata.Unused) > 0 {
		return nil, fmt.Errorf("input error: unused keys found - %s", strings.Join(metadata.Unused, ", "))
	}
	return &resource, nil
}

func jsonPrettyPrint(item any) error {
	b, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
