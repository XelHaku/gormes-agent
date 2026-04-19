# Golang Configuration Best Practices for Gormes

Research date: 2026-02-09

## Recommended Libraries

| Library | Best For | Config Sources |
|---------|----------|----------------|
| **Viper** | Complex apps, multiple formats | Files, env, flags, remote |
| **envconfig** | 12-factor, env-var focused | Env vars only |
| **koanf** | Modern alternative to Viper | Files, env, flags, remote |

**Recommendation**: Viper is the most battle-tested and widely used. However, `koanf` is gaining popularity as a simpler, more performant alternative.

## Precedence Order (highest to lowest)

1. Environment variables
2. Config files (YAML/TOML/JSON)
3. Remote (Consul, etcd)
4. Defaults

## Key Best Practices

### 1. Use Struct-Based Config with Unmarshaling

```go
package config

import (
	"fmt"
	"strings"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Features FeaturesConfig `mapstructure:"features"`
}

type ServerConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Timeout     int    `mapstructure:"timeout"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	MaxConnections int  `mapstructure:"max_connections"`
}

type FeaturesConfig struct {
	EnableCache   bool `mapstructure:"enable_cache"`
	EnableMetrics bool `mapstructure:"enable_metrics"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	
	// Set defaults BEFORE reading config
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.timeout", 30)
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.max_connections", 50)
	v.SetDefault("features.enable_cache", false)
	v.SetDefault("features.enable_metrics", false)
	
	// Set config file name and paths
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/myapp")
		v.AddConfigPath(".")
	}
	
	// Enable environment variables
	v.SetEnvPrefix("HERMES")  // e.g., HERMES_SERVER_PORT
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	
	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		// It's OK if no config file exists when using env vars
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}
	
	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into config struct: %w", err)
	}
	
	return &cfg, nil
}
```

### 2. Always Set Defaults Before Reading Config

```go
v.SetDefault("server.port", 8080)
v.SetDefault("server.host", "localhost")
```

This ensures the app can start without a config file when environment variables are used.

### 3. Use Env Var Prefixes to Avoid Conflicts

```go
v.SetEnvPrefix("HERMES")  // HERMES_SERVER_PORT → server.port
v.AutomaticEnv()
```

### 4. Separate Secrets from Config Files

**DO NOT commit secrets to config.yaml:**

```yaml
# config.yaml (safe to commit)
database:
  host: "localhost"
  port: 5432
  username: "myapp"
  # password: Use environment variable HERMES_DATABASE_PASSWORD
```

**Set secrets via environment:**

```bash
export HERMES_DATABASE_PASSWORD="secret_value"
```

### 5. Validate After Loading

```go
func (c *Config) Validate() error {
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1024-65535)", c.Server.Port)
	}
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	return nil
}
```

### 6. Support Multiple Config Formats

```go
v.SetConfigType("yaml")  // Or auto-detect from file extension
// Viper supports YAML, JSON, TOML, HCL, envfile, and Java properties
```

### 7. Optional: Hot-Reload Config

```go
v.WatchConfig()
v.OnConfigChange(func(e fsnotify.Event) {
	fmt.Println("Config file changed:", e.Name)
	// Reload or signal the application
})
```

## Configuration Sources

| Source | Use Case |
|--------|----------|
| **Environment Variables** | Containerized deployments, 12-factor apps, secrets |
| **YAML Config Files** | Complex nested configs, default values |
| **Command-Line Flags** | Runtime overrides, development |
| **Remote (Consul/etcd)** | Dynamic config in distributed systems |

## Environment Variable Naming

Use the pattern: `<PREFIX>_<KEY_PATH>`

Examples:
- `HERMES_SERVER_PORT` → `server.port`
- `HERMES_DATABASE_HOST` → `database.host`
- `HERMES_FEATURES_ENABLE_CACHE` → `features.enable_cache`

## For Gormes Specifically

Since Hermes-Agent has multi-profile support:

1. **Profile-based config paths**: `./config/profiles/<name>/config.yaml`
2. **Environment variable overrides per profile**: `HERMES_PROFILE=<name>`
3. **Consider `koanf` for cleaner profile isolation** (simpler API, no global state)

### Profile Isolation Pattern

```go
func LoadConfigForProfile(profile string) (*Config, error) {
	var configPath string
	if profile != "" && profile != "default" {
		configPath = fmt.Sprintf("./config/profiles/%s/config.yaml", profile)
	} else {
		configPath = "./config/config.yaml"
	}
	return LoadConfig(configPath)
}
```

## References

- [BackendBytes: Go Configuration Mastery with Viper](https://backendbytes.com/articles/go-configuration-viper-patterns/) (2026-02)
- [OneUptime: How to Manage Configuration in Go with Viper](https://oneuptime.com/blog/post/2026-01-07-go-viper-configuration/view) (2026-01)
- [Codezup: Effective Configuration Management](https://codezup.com/effective-configuration-management-in-go-applications/) (2025-03)
- [Mike Christensen: Configuring Go Applications with Viper](https://christensen.codes/posts/configuring-go-applications/)
