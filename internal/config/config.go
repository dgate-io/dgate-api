package config

import (
	"time"

	"github.com/dgate-io/dgate-api/pkg/spec"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	DGateConfig struct {
		Version  string `koanf:"version"`
		LogLevel string `koanf:"log_level,string"`
		LogJson  bool   `koanf:"log_json"`
		LogColor bool   `koanf:"log_color"`

		NodeId           string                 `koanf:"node_id"`
		Logging          *LoggingConfig         `koanf:"Logger"`
		Storage          DGateStorageConfig     `koanf:"storage"`
		ProxyConfig      DGateProxyConfig       `koanf:"proxy"`
		AdminConfig      *DGateAdminConfig      `koanf:"admin"`
		TestServerConfig *DGateTestServerConfig `koanf:"test_server"`

		DisableMetrics          bool     `koanf:"disable_metrics"`
		DisableDefaultNamespace bool     `koanf:"disable_default_namespace"`
		Debug                   bool     `koanf:"debug"`
		Tags                    []string `koanf:"tags"`
	}

	LoggingConfig struct {
		ZapConfig *zap.Config `koanf:",squash"`
	}

	DGateProxyConfig struct {
		Host                     string                   `koanf:"host"`
		Port                     int                      `koanf:"port"`
		TLS                      *DGateTLSConfig          `koanf:"tls"`
		EnableH2C                bool                     `koanf:"enable_h2c"`
		EnableHTTP2              bool                     `koanf:"enable_http2"`
		ConsoleLogLevel          string                   `koanf:"console_log_level"`
		RedirectHttpsDomains     []string                 `koanf:"redirect_https"`
		AllowedDomains           []string                 `koanf:"allowed_domains"`
		GlobalHeaders            map[string]string        `koanf:"global_headers"`
		Transport                DGateHttpTransportConfig `koanf:"client_transport"`
		DisableXForwardedHeaders bool                     `koanf:"disable_x_forwarded_headers"`
		StrictMode               bool                     `koanf:"strict_mode"`
		XForwardedForDepth       int                      `koanf:"x_forwarded_for_depth"`
		AllowList                []string                 `koanf:"allow_list"`

		// WARN: debug use only
		InitResources *DGateResources `koanf:"init_resources"`
	}

	DGateTestServerConfig struct {
		Host          string            `koanf:"host"`
		Port          int               `koanf:"port"`
		EnableH2C     bool              `koanf:"enable_h2c"`
		EnableHTTP2   bool              `koanf:"enable_http2"`
		EnableEnvVars bool              `koanf:"enable_env_vars"`
		GlobalHeaders map[string]string `koanf:"global_headers"`
	}

	DGateNativeModulesConfig struct {
		Name string `koanf:"name"`
		Path string `koanf:"path"`
	}

	DGateAdminConfig struct {
		Host               string                  `koanf:"host"`
		Port               int                     `koanf:"port"`
		AllowList          []string                `koanf:"allow_list"`
		XForwardedForDepth int                     `koanf:"x_forwarded_for_depth"`
		WatchOnly          bool                    `koanf:"watch_only"`
		Replication        *DGateReplicationConfig `koanf:"replication,omitempty"`
		TLS                *DGateTLSConfig         `koanf:"tls"`
		AuthMethod         AuthMethodType          `koanf:"auth_method"`
		BasicAuth          *DGateBasicAuthConfig   `koanf:"basic_auth"`
		KeyAuth            *DGateKeyAuthConfig     `koanf:"key_auth"`
		JWTAuth            *DGateJWTAuthConfig     `koanf:"jwt_auth"`
	}

	DGateReplicationConfig struct {
		RaftID           string      `koanf:"id"`
		SharedKey        string      `koanf:"shared_key"`
		BootstrapCluster bool        `koanf:"bootstrap_cluster"`
		DiscoveryDomain  string      `koanf:"discovery_domain"`
		ClusterAddrs     []string    `koanf:"cluster_address"`
		AdvertAddr       string      `koanf:"advert_address"`
		AdvertScheme     string      `koanf:"advert_scheme"`
		RaftConfig       *RaftConfig `koanf:"raft_config"`
	}

	RaftConfig struct {
		HeartbeatTimeout   time.Duration `koanf:"heartbeat_timeout"`
		ElectionTimeout    time.Duration `koanf:"election_timeout"`
		CommitTimeout      time.Duration `koanf:"commit_timeout"`
		SnapshotInterval   time.Duration `koanf:"snapshot_interval"`
		SnapshotThreshold  int           `koanf:"snapshot_threshold"`
		MaxAppendEntries   int           `koanf:"max_append_entries"`
		TrailingLogs       int           `koanf:"trailing_logs"`
		LeaderLeaseTimeout time.Duration `koanf:"leader_lease_timeout"`
	}

	DGateDashboardConfig struct {
		Enable bool `koanf:"enable"`
	}

	AuthMethodType string

	DGateBasicAuthConfig struct {
		Users []DGateUserCredentials `koanf:"users"`
	}

	DGateUserCredentials struct {
		Username string `koanf:"username"`
		Password string `koanf:"password"`
	}

	DGateKeyAuthConfig struct {
		QueryParamName string   `koanf:"query_param_name"`
		HeaderName     string   `koanf:"header_name"`
		Keys           []string `koanf:"keys"`
	}

	DGateJWTAuthConfig struct {
		// HeaderName is the name of the header to extract the JWT token from
		HeaderName      string         `koanf:"header_name"`
		Algorithm       string         `koanf:"algorithm"`
		SignatureConfig map[string]any `koanf:",remain"`
	}

	AsymmetricSignatureConfig struct {
		// ES256, ES384, ES512
		// PS256, PS384, PS512
		// RS256, RS384, RS512
		Algorithm     string `koanf:"algorithm"`
		PublicKey     string `koanf:"public_key"`
		PublicKeyFile string `koanf:"public_key_file"`
	}

	SymmetricSignatureConfig struct {
		// HS256, HS384, HS512
		Algorithm string `koanf:"algorithm"`
		Key       string `koanf:"key"`
	}

	DGateTLSConfig struct {
		Port     int    `koanf:"port"`
		CertFile string `koanf:"cert_file"`
		KeyFile  string `koanf:"key_file"`
	}

	DGateHttpTransportConfig struct {
		DNSServer              string        `koanf:"dns_server"`
		DNSTimeout             time.Duration `koanf:"dns_timeout"`
		DNSPreferGo            bool          `koanf:"dns_prefer_go"`
		MaxIdleConns           int           `koanf:"max_idle_conns"`
		MaxIdleConnsPerHost    int           `koanf:"max_idle_conns_per_host"`
		MaxConnsPerHost        int           `koanf:"max_conns_per_host"`
		IdleConnTimeout        time.Duration `koanf:"idle_conn_timeout"`
		ForceAttemptHttp2      bool          `koanf:"force_attempt_http2"`
		DisableCompression     bool          `koanf:"disable_compression"`
		TLSHandshakeTimeout    time.Duration `koanf:"tls_handshake_timeout"`
		ExpectContinueTimeout  time.Duration `koanf:"expect_continue_timeout"`
		MaxResponseHeaderBytes int64         `koanf:"max_response_header_bytes"`
		WriteBufferSize        int           `koanf:"write_buffer_size"`
		ReadBufferSize         int           `koanf:"read_buffer_size"`
		MaxBodyBytes           int           `koanf:"max_body_bytes"`
		DisableKeepAlives      bool          `koanf:"disable_keep_alives"`
		KeepAlive              time.Duration `koanf:"keep_alive"`
		ResponseHeaderTimeout  time.Duration `koanf:"response_header_timeout"`
		DialTimeout            time.Duration `koanf:"dial_timeout"`
		DisablePrivateIPs      bool          `koanf:"disable_private_ips"`
	}

	DGateStorageConfig struct {
		StorageType StorageType    `koanf:"type"`
		Config      map[string]any `koanf:",remain"`
	}

	DGateFileConfig struct {
		Dir string `koanf:"dir"`
	}

	DGateResources struct {
		SkipValidation bool              `koanf:"skip_validation"`
		Namespaces     []spec.Namespace  `koanf:"namespaces"`
		Services       []spec.Service    `koanf:"services"`
		Routes         []spec.Route      `koanf:"routes"`
		Modules        []ModuleSpec      `koanf:"modules"`
		Domains        []DomainSpec      `koanf:"domains"`
		Collections    []spec.Collection `koanf:"collections"`
		Documents      []spec.Document   `koanf:"documents"`
		Secrets        []spec.Secret     `koanf:"secrets"`
	}

	DomainSpec struct {
		spec.Domain `koanf:",squash"`
		CertFile    string `koanf:"cert_file"`
		KeyFile     string `koanf:"key_file"`
	}

	ModuleSpec struct {
		spec.Module `koanf:",squash"`
		PayloadFile string `koanf:"payload_file"`
	}
)

const (
	AuthMethodNone      AuthMethodType = "none"
	AuthMethodBasicAuth AuthMethodType = "basic"
	AuthMethodKeyAuth   AuthMethodType = "key"
	AuthMethodJWTAuth   AuthMethodType = "jwt"
)

func (conf *DGateConfig) GetLogger() (*zap.Logger, error) {
	level, err := zap.ParseAtomicLevel(conf.LogLevel)
	if err != nil {
		return nil, err
	}
	if conf.Logging == nil {
		conf.Logging = &LoggingConfig{}
	}
	if conf.Logging.ZapConfig == nil {
		config := zap.NewProductionConfig()
		config.Level = level
		config.DisableCaller = true
		config.DisableStacktrace = true
		config.Development = conf.Debug
		config.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
		config.OutputPaths = []string{"stdout"}
		config.Sampling = nil

		if config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder; conf.LogColor {
			config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}

		if config.Encoding = "console"; conf.LogJson {
			config.InitialFields = map[string]interface{}{
				"version":     conf.Version,
				"server_tags": conf.Tags,
				"node_id":     conf.NodeId,
			}
			config.Encoding = "json"
		}

		conf.Logging.ZapConfig = &config
	}

	if logger, err := conf.Logging.ZapConfig.Build(); err != nil {
		return nil, err
	} else {
		zap.ReplaceGlobals(logger)
		return logger, nil
	}
}
