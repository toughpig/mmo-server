package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 保存应用配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Game     GameConfig     `mapstructure:"game"`
	Log      LogConfig      `mapstructure:"log"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	MaxConnections      int    `mapstructure:"max_connections"`
	ReadTimeoutMs       int    `mapstructure:"read_timeout_ms"`
	WriteTimeoutMs      int    `mapstructure:"write_timeout_ms"`
	HeartbeatIntervalSec int    `mapstructure:"heartbeat_interval_sec"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Driver            string `mapstructure:"driver"`
	Host              string `mapstructure:"host"`
	Port              int    `mapstructure:"port"`
	User              string `mapstructure:"user"`
	Password          string `mapstructure:"password"`
	DBName            string `mapstructure:"dbname"`
	MaxOpenConns      int    `mapstructure:"max_open_conns"`
	MaxIdleConns      int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeSec int    `mapstructure:"conn_max_lifetime_sec"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Password      string `mapstructure:"password"`
	DB            int    `mapstructure:"db"`
	PoolSize      int    `mapstructure:"pool_size"`
	MinIdleConns  int    `mapstructure:"min_idle_conns"`
}

// GameConfig 游戏配置
type GameConfig struct {
	WorldSizeX           int `mapstructure:"world_size_x"`
	WorldSizeY           int `mapstructure:"world_size_y"`
	PositionSyncIntervalMs int `mapstructure:"position_sync_interval_ms"`
	MaxPlayersPerZone    int `mapstructure:"max_players_per_zone"`
	ZoneSize             int `mapstructure:"zone_size"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level       string `mapstructure:"level"`
	File        string `mapstructure:"file"`
	MaxSizeMB   int    `mapstructure:"max_size_mb"`
	MaxBackups  int    `mapstructure:"max_backups"`
	MaxAgeDays  int    `mapstructure:"max_age_days"`
	Compress    bool   `mapstructure:"compress"`
}

// Load 加载配置
func Load(configPath string) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	
	// 首先尝试从指定路径加载
	if configPath != "" {
		viper.AddConfigPath(configPath)
	}
	
	// 然后尝试从默认路径加载
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")
	
	// 读取环境变量
	viper.AutomaticEnv()
	
	// 读取配置
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// 确保日志目录存在
	if config.Log.File != "" {
		logDir := filepath.Dir(config.Log.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	
	return &config, nil
}

// GetDSN 获取数据库连接字符串
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		c.Host, c.Port, c.User, c.Password, c.DBName)
}

// GetRedisAddr 获取Redis地址
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
} 