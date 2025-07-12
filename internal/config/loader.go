package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dgate-io/dgate-api/pkg/util/iplist"
	"github.com/hashicorp/raft"
	kjson "github.com/knadh/koanf/parsers/json"
	ktoml "github.com/knadh/koanf/parsers/toml"
	kyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/mapstructure"
)

var (
	EnvVarRegex  = regexp.MustCompile(`\${(?P<var_name>[a-zA-Z0-9_]{1,})(:-(?P<default>.*?)?)?}`)
	CommandRegex = regexp.MustCompile(`\$\((?P<cmd>.*?)\)`)
)

func LoadConfig(dgateConfigPath string) (*DGateConfig, error) {
	ctx := context.Background()
	var dgateConfigData string
	if dgateConfigPath == "" {
		dgateConfigPath = os.Getenv("DG_CONFIG_PATH")
		dgateConfigData = os.Getenv("DG_CONFIG_DATA")
	}

	configDataType := os.Getenv("DG_CONFIG_TYPE")
	if configDataType == "" && dgateConfigPath != "" {
		fileExt := strings.ToLower(path.Ext(dgateConfigPath))
		if fileExt == "" {
			return nil, errors.New("no config file extension: set env DG_CONFIG_TYPE to json, toml or yaml")
		}
		configDataType = fileExt[1:]
	}

	var k = koanf.New(".")
	var parser koanf.Parser

	dgateConfig := &DGateConfig{}
	if dgateConfigPath != "" {
		parser, err := determineParser(configDataType)
		if err != nil {
			return nil, err
		}
		err = k.Load(file.Provider(dgateConfigPath), parser)
		if err != nil {
			return nil, fmt.Errorf("error loading '%s' with %s parser: %v", dgateConfigPath, configDataType, err)
		}
	} else if dgateConfigData != "" {
		configFileData, err := base64.StdEncoding.DecodeString(
			strings.TrimSpace(dgateConfigData))
		if err != nil {
			return nil, err
		}
		k.Load(rawbytes.Provider(configFileData), parser)
	} else {
		defaultConfigExts := []string{
			"yml", "yaml", "json", "toml",
		}

		var err error
		for _, ext := range defaultConfigExts {
			parser, err = determineParser(ext)
			if err != nil {
				return nil, err
			}
			err := k.Load(file.Provider("./config.dgate."+ext), parser)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf(
				"no config file: ./config.dgate.%s",
				defaultConfigExts,
			)
		}
	}

	panicVars := []string{}
	if !util.EnvVarCheckBool("DG_DISABLE_SHELL_PARSER") {
		data := k.All()
		shell := "/bin/sh"
		if shellEnv := os.Getenv("SHELL"); shellEnv != "" {
			shell = shellEnv
		}
		resolveConfigStringPattern(data, CommandRegex, func(value string, results map[string]string) (string, error) {
			cmdResult, err := exec.CommandContext(
				ctx, shell, "-c", results["cmd"]).Output()
			if err != nil {
				panicVars = append(panicVars, results["cmd"])
				return "", err
			}
			return strings.TrimSpace(string(cmdResult)), nil
		}, func(results map[string]string, err error) {
			panic("error on command - `" + results["cmd"] + "`: " + err.Error())
		})
		k.Load(confmap.Provider(data, "."), nil)
	}

	if !util.EnvVarCheckBool("DG_DISABLE_ENV_PARSER") {
		data := k.All()
		resolveConfigStringPattern(data, EnvVarRegex, func(value string, results map[string]string) (string, error) {
			if envVar := os.Getenv(results["var_name"]); envVar != "" {
				return envVar, nil
			} else if strings.Contains(value, results["var_name"]+":-") {
				return results["default"], nil
			}
			return "", nil
		}, func(results map[string]string, err error) {
			panicVars = append(panicVars, results["var_name"])
		})

		if len(panicVars) > 0 {
			panic("required env vars not set: " + strings.Join(panicVars, ", "))
		}
		k.Load(confmap.Provider(data, "."), nil)
	}

	// validate configuration
	var err error
	kDefault(k, "log_level", "info")
	err = kRequireAll(k, "version")
	if err != nil {
		return nil, err
	}
	nodeId := os.Getenv("DGATE_NODE_ID")
	if nodeId == "" {
		nodeId = os.Getenv("HOST")
	}
	kDefault(k, "node_id", nodeId)

	err = kRequireAll(k, "storage", "storage.type")
	if err != nil {
		return nil, err
	}
	if k.Get("storage.type") == "file" {
		err = kRequireAll(k, "storage.dir")
		if err != nil {
			return nil, errors.New("if storage.type is file, " + err.Error())
		}
	}

	kDefault(k, "proxy.port", 80)
	kDefault(k, "proxy.enable_h2c", false)
	kDefault(k, "proxy.enable_http2", false)
	kDefault(k, "proxy.console_log_level", k.Get("log_level"))

	if k.Exists("proxy.allow_list") {
		var ips []string = k.Get("proxy.allow_list").([]string)
		ipList := iplist.NewIPList()
		err = ipList.AddAll(ips)
		if err != nil {
			return nil, errors.New("proxy.allow_list error: " + err.Error())
		}
	}

	if k.Get("proxy.enable_h2c") == true &&
		k.Get("proxy.enable_http2") == false {
		return nil, errors.New("proxy: enable_h2c is true but enable_http2 is false")
	}

	err = kRequireIfExists(k, "proxy.tls", "proxy.tls.port")
	if err != nil {
		return nil, err
	}

	// kDefault(k, "proxy.transport.max_idle_conns", 100)
	// kDefault(k, "proxy.transport.force_attempt_http2", true)
	// kDefault(k, "proxy.transport.idle_conn_timeout", "90s")
	// kDefault(k, "proxy.transport.tls_handshake_timeout", "10s")
	// kDefault(k, "proxy.transport.expect_continue_timeout", "1s")
	if k.Exists("test_server") {
		kDefault(k, "test_server.enable_h2c", true)
		kDefault(k, "test_server.enable_http2", true)
		if k.Get("test_server.enable_h2c") == true &&
			k.Get("test_server.enable_http2") == false {
			panic("test_server: enable_h2c is true but enable_http2 is false")
		}
	}
	if k.Exists("admin") {
		kDefault(k, "admin.host", "127.0.0.1")
		kDefault(k, "admin.x_forwarded_for_depth", -1)
		err = kRequireAll(k, "admin.port")
		if err != nil {
			return nil, err
		}
		err = kRequireIfExists(k, "admin.tls", "admin.tls.port")
		if err != nil {
			return nil, err
		}

		if k.Exists("admin.replication") {
			kDefault(k, "admin.replication.raft_id", k.Get("node_id"))
			err = kRequireAll(k, "admin.host")
			if err != nil {
				return nil, err
			}
			err = kRequireAll(k, "admin.replication.bootstrap_cluster")
			if err != nil {
				return nil, err
			}
			defaultScheme := "http"
			if k.Exists("admin.tls") {
				defaultScheme = "https"
				address := fmt.Sprintf("%s:%s", k.Get("admin.host"), k.Get("admin.tls.port"))
				kDefault(k, "admin.replication.advert_address", address)
			} else {
				address := fmt.Sprintf("%s:%s", k.Get("admin.host"), k.Get("admin.port"))
				kDefault(k, "admin.replication.advert_address", address)
			}
			kDefault(k, "admin.replication.advert_scheme", defaultScheme)
		}

		if k.Exists("admin.auth_method") {
			switch authMethod := k.Get("admin.auth_method"); authMethod {
			case "basic":
				err = kRequireAll(k, "admin.basic_auth", "admin.basic_auth.users")
			case "key":
				err = kRequireAll(k, "admin.key_auth.key")
			case "jwt":
				err = kRequireAny(k, "admin.jwt_auth.secret", "admin.jwt_auth.secret_file")
				if err == nil {
					err = kRequireAny(k, "admin.jwt_auth.header_name")
				}
			case "none", "", nil:
			default:
				return nil, fmt.Errorf("admin: invalid auth_method: %v", authMethod)
			}
			if err != nil {
				return nil, err
			}
		}
	}

	err = k.UnmarshalWithConf("", dgateConfig, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			Result: dgateConfig,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				StringToIntHookFunc(), StringToBoolHookFunc(),
			),
		},
	})
	if err != nil {
		return nil, err
	}
	return dgateConfig, nil
}

func determineParser(configDataType string) (koanf.Parser, error) {
	switch configDataType {
	case "json":
		return kjson.Parser(), nil
	case "toml":
		return ktoml.Parser(), nil
	case "yaml", "yml":
		return kyaml.Parser(), nil
	default:
		return nil, errors.New("unknown config type: " + configDataType)
	}
}

func (config *DGateReplicationConfig) LoadRaftConfig(defaultConfig *raft.Config) *raft.Config {
	rc := defaultConfig
	if defaultConfig == nil {
		rc = raft.DefaultConfig()
	}
	if rc.ProtocolVersion == raft.ProtocolVersionMin {
		rc.ProtocolVersion = raft.ProtocolVersionMax
	}
	if config.RaftConfig != nil {
		if config.RaftConfig.HeartbeatTimeout != 0 {
			rc.HeartbeatTimeout = config.RaftConfig.HeartbeatTimeout
		}
		if config.RaftConfig.ElectionTimeout != 0 {
			rc.ElectionTimeout = config.RaftConfig.ElectionTimeout
		}
		if config.RaftConfig.SnapshotThreshold > 0 {
			rc.SnapshotThreshold = uint64(config.RaftConfig.SnapshotThreshold)
		}
		if config.RaftConfig.SnapshotInterval != 0 {
			rc.SnapshotInterval = config.RaftConfig.SnapshotInterval
		}
		if config.RaftConfig.LeaderLeaseTimeout != 0 {
			rc.LeaderLeaseTimeout = config.RaftConfig.LeaderLeaseTimeout
		}
		if config.RaftConfig.CommitTimeout != 0 {
			rc.CommitTimeout = config.RaftConfig.CommitTimeout
		}
		if config.RaftConfig.MaxAppendEntries != 0 {
			rc.MaxAppendEntries = config.RaftConfig.MaxAppendEntries
		}
		if config.RaftConfig.TrailingLogs > 0 {
			rc.TrailingLogs = uint64(config.RaftConfig.TrailingLogs)
		}
		if config.RaftID != "" {
			rc.LocalID = raft.ServerID(config.RaftID)
		}
	}
	err := raft.ValidateConfig(rc)
	if err != nil {
		panic(err)
	}
	return rc
}
