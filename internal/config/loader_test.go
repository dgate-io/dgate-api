package config_test

import (
	"os"
	"testing"

	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestConfig_EnvVarRegex(t *testing.T) {
	re := config.EnvVarRegex
	inputs := []string{
		"${var_name1}",
		"${var_name2:-default}",
		"${var_name3:-}",
		"${var_name4:default}",
		"${var_name5:}",
	}
	results := make(map[string]string)
	for _, input := range inputs {
		matches := re.FindAllStringSubmatch(input, -1)
		for _, match := range matches {
			results[input] = match[1] + "//" + match[3]
		}
	}
	assert.Equal(t, results, map[string]string{
		"${var_name1}":          "var_name1//",
		"${var_name2:-default}": "var_name2//default",
		"${var_name3:-}":        "var_name3//",
	})
}

func TestConfig_CommandRegex(t *testing.T) {
	re := config.CommandRegex
	inputs := []string{
		"$(cmd1)",
		"$(cmd2 arg1 arg2)",
		"$(cmd3 \"arg1\" 'arg2')",
	}
	results := make(map[string]string)
	for _, input := range inputs {
		matches := re.FindAllStringSubmatch(input, -1)
		for _, match := range matches {
			results[input] = match[1]
		}
	}
	assert.Equal(t, results, map[string]string{
		"$(cmd1)":                 "cmd1",
		"$(cmd2 arg1 arg2)":       "cmd2 arg1 arg2",
		"$(cmd3 \"arg1\" 'arg2')": "cmd3 \"arg1\" 'arg2'",
	})
}

func TestConfig_LoaderVariables(t *testing.T) {
	os.Setenv("ENV1", "test1")
	os.Setenv("ENV2", "test2")
	os.Setenv("ENV3", "test3")
	os.Setenv("ENV4", "")
	os.Setenv("ENV5", "test5")
	os.Setenv("ADMIN_PORT", "8080")
	conf, err := config.LoadConfig("testdata/env.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{
		"test1",
		"$ENV2",
		"test3",
		"test4",
		"testing",
		"test test5",
	}, conf.Tags)
	assert.Equal(t, "v1", conf.Version)
	assert.Equal(t, true, conf.TestServerConfig.EnableH2C)
	assert.Equal(t, true, conf.TestServerConfig.EnableHTTP2)
	assert.Equal(t, false, conf.TestServerConfig.EnableEnvVars)
	assert.Equal(t, 80, conf.ProxyConfig.Port)
	assert.Equal(t, 8080, conf.AdminConfig.Port)
	assert.Equal(t, 1, len(conf.Storage.Config))
	assert.Equal(t, "test1-test2-testing",
		conf.Storage.Config["testing"])
}
