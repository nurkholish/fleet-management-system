package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"fleet-management-system/internal/domain"
)

type Config struct {
	App struct {
		Env      string `mapstructure:"env"`
		LogLevel string `mapstructure:"log_level"`
	} `mapstructure:"app"`

	HTTP struct {
		Port         string        `mapstructure:"port"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout"`
		IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	} `mapstructure:"http"`

	Postgres struct {
		Host            string        `mapstructure:"host"`
		Port            int           `mapstructure:"port"`
		User            string        `mapstructure:"user"`
		Password        string        `mapstructure:"password"`
		DBName          string        `mapstructure:"dbname"`
		SSLMode         string        `mapstructure:"sslmode"`
		MaxOpenConns    int32         `mapstructure:"max_open_conns"`
		MinConns        int32         `mapstructure:"min_conns"`
		ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	} `mapstructure:"postgres"`

	MQTT struct {
		Broker         string        `mapstructure:"broker"`
		ClientID       string        `mapstructure:"client_id"`
		Topic          string        `mapstructure:"topic"`
		QoS            byte          `mapstructure:"qos"`
		KeepAlive      time.Duration `mapstructure:"keep_alive"`
		ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
		WorkerPool     int           `mapstructure:"worker_pool"`
		JobBuffer      int           `mapstructure:"job_buffer"`
		CleanSession   bool          `mapstructure:"clean_session"`
	} `mapstructure:"mqtt"`

	RabbitMQ struct {
		URL          string `mapstructure:"url"`
		Exchange     string `mapstructure:"exchange"`
		ExchangeType string `mapstructure:"exchange_type"`
		Queue        string `mapstructure:"queue"`
		RoutingKey   string `mapstructure:"routing_key"`
		DLXExchange  string `mapstructure:"dlx_exchange"`
		DLQQueue     string `mapstructure:"dlq_queue"`
	} `mapstructure:"rabbitmq"`

	Geofence struct {
		Items []domain.Geofence `mapstructure:"items"`
	} `mapstructure:"geofence"`

	History struct {
		MaxRangeSeconds int `mapstructure:"max_range_seconds"`
		DefaultLimit    int `mapstructure:"default_limit"`
		MaxLimit        int `mapstructure:"max_limit"`
	} `mapstructure:"history"`

	Publisher struct {
		VehicleID   string              `mapstructure:"vehicle_id"`
		Interval    time.Duration       `mapstructure:"interval"`
		ReachMeters float64             `mapstructure:"reach_meters"`
		Waypoints   []PublisherWaypoint `mapstructure:"waypoints"`
	} `mapstructure:"publisher"`
}

type PublisherWaypoint struct {
	Name      string  `mapstructure:"name"`
	Latitude  float64 `mapstructure:"latitude"`
	Longitude float64 `mapstructure:"longitude"`
}

// Load reads config from YAML and overlays env vars with prefix TJ.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("TJ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	applyDefaults(&c)
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) PostgresDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.Postgres.User, c.Postgres.Password),
		Host:   net.JoinHostPort(c.Postgres.Host, strconv.Itoa(c.Postgres.Port)),
		Path:   "/" + c.Postgres.DBName,
	}
	q := u.Query()
	q.Set("sslmode", c.Postgres.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Config) PublisherRoute() []PublisherWaypoint {
	if len(c.Publisher.Waypoints) >= 2 {
		return c.Publisher.Waypoints
	}
	out := make([]PublisherWaypoint, 0, len(c.Geofence.Items))
	for _, f := range c.Geofence.Items {
		out = append(out, PublisherWaypoint{
			Name:      f.ID,
			Latitude:  f.Latitude,
			Longitude: f.Longitude,
		})
	}
	return out
}

func (c *Config) validate() error {
	if c.Postgres.Host == "" || c.Postgres.User == "" || c.Postgres.DBName == "" {
		return fmt.Errorf("postgres: host/user/dbname required")
	}
	if c.MQTT.Broker == "" || c.MQTT.Topic == "" {
		return fmt.Errorf("mqtt: broker/topic required")
	}
	if c.RabbitMQ.URL == "" {
		return fmt.Errorf("rabbitmq: url required")
	}
	if len(c.Geofence.Items) == 0 {
		return fmt.Errorf("geofence: at least one item required")
	}
	return nil
}

func applyDefaults(c *Config) {
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.HTTP.Port == "" {
		c.HTTP.Port = "8080"
	}
	if c.HTTP.ReadTimeout == 0 {
		c.HTTP.ReadTimeout = 10 * time.Second
	}
	if c.HTTP.WriteTimeout == 0 {
		c.HTTP.WriteTimeout = 10 * time.Second
	}
	if c.HTTP.IdleTimeout == 0 {
		c.HTTP.IdleTimeout = 60 * time.Second
	}
	if c.Postgres.MaxOpenConns == 0 {
		c.Postgres.MaxOpenConns = 25
	}
	if c.Postgres.MinConns == 0 {
		c.Postgres.MinConns = 5
	}
	if c.Postgres.ConnMaxLifetime == 0 {
		c.Postgres.ConnMaxLifetime = 30 * time.Minute
	}
	if c.MQTT.QoS == 0 {
		c.MQTT.QoS = 1
	}
	if c.MQTT.KeepAlive == 0 {
		c.MQTT.KeepAlive = 30 * time.Second
	}
	if c.MQTT.ConnectTimeout == 0 {
		c.MQTT.ConnectTimeout = 10 * time.Second
	}
	if c.MQTT.WorkerPool == 0 {
		c.MQTT.WorkerPool = 4
	}
	if c.MQTT.JobBuffer == 0 {
		c.MQTT.JobBuffer = 1000
	}
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "fleet-management-system-backend"
	}
	if c.MQTT.Topic == "" {
		c.MQTT.Topic = "/fleet/vehicle/+/location"
	}
	if c.RabbitMQ.Exchange == "" {
		c.RabbitMQ.Exchange = "fleet.events"
	}
	if c.RabbitMQ.ExchangeType == "" {
		c.RabbitMQ.ExchangeType = "topic"
	}
	if c.RabbitMQ.Queue == "" {
		c.RabbitMQ.Queue = "geofence_alerts"
	}
	if c.RabbitMQ.RoutingKey == "" {
		c.RabbitMQ.RoutingKey = "geofence.entry"
	}
	if c.RabbitMQ.DLXExchange == "" {
		c.RabbitMQ.DLXExchange = c.RabbitMQ.Exchange + ".dlx"
	}
	if c.RabbitMQ.DLQQueue == "" {
		c.RabbitMQ.DLQQueue = c.RabbitMQ.Queue + ".dlq"
	}
	if c.History.MaxRangeSeconds == 0 {
		c.History.MaxRangeSeconds = 7 * 24 * 3600
	}
	if c.History.DefaultLimit == 0 {
		c.History.DefaultLimit = 1000
	}
	if c.History.MaxLimit == 0 {
		c.History.MaxLimit = 10000
	}
	if c.Publisher.VehicleID == "" {
		c.Publisher.VehicleID = "B1234XYZ"
	}
	if c.Publisher.Interval == 0 {
		c.Publisher.Interval = 2 * time.Second
	}
	if c.Publisher.ReachMeters == 0 {
		c.Publisher.ReachMeters = 30
	}
}
