package config

import (
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	PublicHost     string   `mapstructure:"public_host"`
	StationRoot    string   `mapstructure:"station_root"`
	Addr           string   `mapstructure:"addr"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	Mode           string   `mapstructure:"mode"`
}

// DBConfig 数据库配置
type DBConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
	// 连接池（仅 PostgreSQL 生效；SQLite 固定单连接）
	MaxOpenConns       int `mapstructure:"max_open_conns"`
	MaxIdleConns       int `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeMin int `mapstructure:"conn_max_lifetime_min"`
}

// JWTConfig 鉴权配置
type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
}

// PayConfig 支付站点级配置（网关密钥在数据库 gateways 表）
type PayConfig struct {
	ReturnURL  string `mapstructure:"return_url"`
	NotifyBase string `mapstructure:"notify_base"`
}

// AdminConfig 默认管理员（首次启动播种）
type AdminConfig struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// MailConfig SMTP 邮件配置（支付成功卡密通知，Host 为空则不发送）
type MailConfig struct {
	Host string `mapstructure:"host"`
	Port string `mapstructure:"port"`
	User string `mapstructure:"user"`
	Pass string `mapstructure:"pass"`
	From string `mapstructure:"from"`
}

// UploadConfig 文件上传配置
type UploadConfig struct {
	Dir     string `mapstructure:"dir"`
	MaxSize int64  `mapstructure:"max_size"` // 字节
}

// Config 全局配置
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	DB     DBConfig     `mapstructure:"db"`
	JWT    JWTConfig    `mapstructure:"jwt"`
	Pay    PayConfig    `mapstructure:"pay"`
	Admin  AdminConfig  `mapstructure:"admin"`
	Mail   MailConfig   `mapstructure:"mail"`
	Upload UploadConfig `mapstructure:"upload"`
}

// Load 加载配置：优先级 环境变量 > config.yaml > 默认值
func Load() *Config {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	v.SetDefault("server.public_host", "")
	v.SetDefault("server.station_root", "")
	_ = v.BindEnv("server.public_host", "SERVER_PUBLIC_HOST")
	_ = v.BindEnv("server.station_root", "SERVER_STATION_ROOT")
	// 默认值
	v.SetDefault("server.addr", ":8080")
	v.SetDefault("server.trusted_proxies", []string{})
	_ = v.BindEnv("server.trusted_proxies", "SERVER_TRUSTED_PROXIES")
	v.SetDefault("server.mode", "debug")
	v.SetDefault("db.driver", "sqlite")
	v.SetDefault("db.dsn", "data/wybo.db")
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 10)
	v.SetDefault("db.conn_max_lifetime_min", 30)
	v.SetDefault("jwt.secret", "change-me")
	v.SetDefault("jwt.expire_hours", 72)
	v.SetDefault("pay.return_url", "http://localhost:5173/orders")
	v.SetDefault("pay.notify_base", "http://localhost:8080/api/v1/pay/notify")
	v.SetDefault("admin.username", "admin")
	v.SetDefault("admin.password", "admin123")
	v.SetDefault("mail.host", "")
	v.SetDefault("mail.port", "465")
	v.SetDefault("mail.user", "")
	v.SetDefault("mail.pass", "")
	v.SetDefault("mail.from", "")
	v.SetDefault("upload.dir", "uploads")
	v.SetDefault("upload.max_size", int64(5*1024*1024))

	// 环境变量绑定
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("db.driver", "DB_DRIVER")
	_ = v.BindEnv("db.dsn", "DB_DSN")
	_ = v.BindEnv("jwt.secret", "JWT_SECRET")
	_ = v.BindEnv("server.addr", "SERVER_ADDR")
	_ = v.BindEnv("admin.username", "ADMIN_USERNAME")
	_ = v.BindEnv("admin.password", "ADMIN_PASSWORD")
	_ = v.BindEnv("mail.host", "SMTP_HOST")
	_ = v.BindEnv("mail.port", "SMTP_PORT")
	_ = v.BindEnv("mail.user", "SMTP_USER")
	_ = v.BindEnv("mail.pass", "SMTP_PASS")
	_ = v.BindEnv("mail.from", "SMTP_FROM")
	_ = v.BindEnv("upload.dir", "UPLOAD_DIR")

	// 配置文件不存在时忽略，回退默认值/环境变量
	_ = v.ReadInConfig()

	cfg := &Config{}
	_ = v.Unmarshal(cfg)
	return cfg
}
