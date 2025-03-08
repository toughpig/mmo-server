package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config 网关配置
type Config struct {
	ID                     string             `json:"id"`
	Region                 string             `json:"region"`
	WebSocket              WebSocketConfig    `json:"websocket"`
	SecureWebSocket        SecureWSConfig     `json:"secure_websocket"`
	QUIC                   QUICConfig         `json:"quic"`
	Router                 RouterConfig       `json:"router"`
	LoadBalancer           LoadBalancerConfig `json:"load_balancer"`
	SessionCleanupInterval int                `json:"session_cleanup_interval"`
	MaxSessionInactiveTime int                `json:"max_session_inactive_time"`
}

// WebSocketConfig WebSocket服务器配置
type WebSocketConfig struct {
	Enabled      bool   `json:"enabled"`
	Address      string `json:"address"`
	ReadTimeout  int    `json:"read_timeout"`
	WriteTimeout int    `json:"write_timeout"`
	IdleTimeout  int    `json:"idle_timeout"`
	BufferSize   int    `json:"buffer_size"`
}

// SecureWSConfig 安全WebSocket服务器配置
type SecureWSConfig struct {
	Enabled      bool   `json:"enabled"`
	Address      string `json:"address"`
	ReadTimeout  int    `json:"read_timeout"`
	WriteTimeout int    `json:"write_timeout"`
	IdleTimeout  int    `json:"idle_timeout"`
	BufferSize   int    `json:"buffer_size"`
	CertFile     string `json:"cert_file"`
	KeyFile      string `json:"key_file"`
}

// QUICConfig QUIC服务器配置
type QUICConfig struct {
	Enabled     bool   `json:"enabled"`
	Address     string `json:"address"`
	CertFile    string `json:"cert_file"`
	KeyFile     string `json:"key_file"`
	IdleTimeout int    `json:"idle_timeout"`
	MaxStreams  int    `json:"max_streams"`
	BufferSize  int    `json:"buffer_size"`
}

// RouterConfig 路由器配置
type RouterConfig struct {
	Timeout    int            `json:"timeout"`
	ServiceMap map[int]string `json:"service_map"`
}

// LoadBalancerConfig 负载均衡器配置
type LoadBalancerConfig struct {
	Type string `json:"type"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ID:     "gateway-1",
		Region: "default",
		WebSocket: WebSocketConfig{
			Enabled:      true,
			Address:      ":8080",
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  120,
			BufferSize:   4096,
		},
		SecureWebSocket: SecureWSConfig{
			Enabled:      false,
			Address:      ":8443",
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  120,
			BufferSize:   4096,
			CertFile:     "cert.pem",
			KeyFile:      "key.pem",
		},
		QUIC: QUICConfig{
			Enabled:     false,
			Address:     ":8443",
			CertFile:    "cert.pem",
			KeyFile:     "key.pem",
			IdleTimeout: 120,
			MaxStreams:  100,
			BufferSize:  4096,
		},
		Router: RouterConfig{
			Timeout: 5000,
			ServiceMap: map[int]string{
				1: "gateway",
				2: "chat",
				3: "game",
				4: "auth",
			},
		},
		LoadBalancer: LoadBalancerConfig{
			Type: "round_robin",
		},
		SessionCleanupInterval: 60,
		MaxSessionInactiveTime: 300,
	}
}

// LoadGatewayConfig 加载网关配置
func LoadGatewayConfig(configPath string) (*Config, error) {
	viper.SetConfigName("gateway")
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
		return nil, fmt.Errorf("failed to read gateway config: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gateway config: %w", err)
	}

	// 确保日志目录存在
	if config.WebSocket.Address != "" {
		logDir := filepath.Dir(config.WebSocket.Address)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	return &config, nil
}

// GetKeyRotationDuration 获取密钥轮换间隔
func (wsc *WebSocketConfig) GetKeyRotationDuration() time.Duration {
	if wsc.IdleTimeout <= 0 {
		return 24 * time.Hour // 默认24小时
	}
	return time.Duration(wsc.IdleTimeout) * time.Minute
}

// GetKeyRotationDuration 获取密钥轮换间隔
func (qsc *QUICConfig) GetKeyRotationDuration() time.Duration {
	if qsc.IdleTimeout <= 0 {
		return 24 * time.Hour // 默认24小时
	}
	return time.Duration(qsc.IdleTimeout) * time.Minute
}
