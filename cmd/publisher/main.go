package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"fleet-management-system/pkg/geo"
)

type waypoint struct {
	Name string
	Lat  float64
	Lon  float64
}

type publisherConfig struct {
	Broker    string
	VehicleID string
	Interval  time.Duration
	Topic     string
	Route     []waypoint
	ReachM    float64
}

func main() {
	cfg := loadPublisherConfig()

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID("mock-publisher-" + cfg.VehicleID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetCleanSession(true)

	c := mqtt.NewClient(opts)
	if tok := c.Connect(); !tok.WaitTimeout(10 * time.Second) {
		panic("mqtt connect timeout")
	} else if tok.Error() != nil {
		panic(tok.Error())
	}
	defer c.Disconnect(250)

	fmt.Printf("[publisher] connected to %s\n", cfg.Broker)
	fmt.Printf("[publisher] publishing to %s every %s\n", cfg.Topic, cfg.Interval)
	fmt.Printf("[publisher] route (%d waypoints, reach threshold %.0fm):\n",
		len(cfg.Route), cfg.ReachM)
	for i, wp := range cfg.Route {
		fmt.Printf("  [%d] %-10s (%.4f, %.4f)\n", i, wp.Name, wp.Lat, wp.Lon)
	}

	lat, lon := cfg.Route[0].Lat, cfg.Route[0].Lon
	targetIdx := 1

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			fmt.Println("[publisher] stopping")
			return
		case <-ticker.C:
			target := cfg.Route[targetIdx]
			dist := geo.HaversineMeters(lat, lon, target.Lat, target.Lon)

			if dist < cfg.ReachM {
				next := cfg.Route[(targetIdx+1)%len(cfg.Route)]
				fmt.Printf("[publisher] reached %s (dist=%.1fm) → next=%s\n",
					target.Name, dist, next.Name)
				targetIdx = (targetIdx + 1) % len(cfg.Route)
				target = cfg.Route[targetIdx]
			}

			lat += (target.Lat - lat) * 0.15
			lon += (target.Lon - lon) * 0.15

			lat += (rng.Float64() - 0.5) * 0.00006
			lon += (rng.Float64() - 0.5) * 0.00006

			payload, err := json.Marshal(map[string]any{
				"vehicle_id": cfg.VehicleID,
				"latitude":   lat,
				"longitude":  lon,
				"timestamp":  time.Now().Unix(),
			})
			if err != nil {
				fmt.Printf("[publisher] marshal error: %v\n", err)
				continue
			}

			tok := c.Publish(cfg.Topic, 1, false, payload)
			if !tok.WaitTimeout(3 * time.Second) {
				fmt.Println("[publisher] publish timeout")
				continue
			}
			if err := tok.Error(); err != nil {
				fmt.Printf("[publisher] publish error: %v\n", err)
				continue
			}

			remaining := geo.HaversineMeters(lat, lon, target.Lat, target.Lon)
			fmt.Printf("[publisher] %s → target=%-8s dist=%.0fm pos=(%.5f,%.5f)\n",
				time.Now().Format("15:04:05"),
				target.Name, remaining, lat, lon)
		}
	}
}

func loadPublisherConfig() publisherConfig {
	broker := envOr("TJ_MQTT_BROKER", "tcp://mosquitto:1883")
	vehicleID := envOr("TJ_VEHICLE_ID", "B1234XYZ")
	topic := fmt.Sprintf("/fleet/vehicle/%s/location", vehicleID)

	interval := 2 * time.Second
	if s := os.Getenv("TJ_PUBLISH_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			interval = d
		}
	}

	reachM := 30.0
	if s := os.Getenv("TJ_REACH_METERS"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			reachM = f
		}
	}

	route := defaultRoute()
	if s := os.Getenv("TJ_ROUTE"); s != "" {
		if r, err := parseRoute(s); err == nil && len(r) >= 2 {
			route = r
		} else {
			fmt.Fprintf(os.Stderr,
				"[publisher] invalid TJ_ROUTE %q: %v (using default)\n", s, err)
		}
	}

	return publisherConfig{
		Broker:    broker,
		VehicleID: vehicleID,
		Interval:  interval,
		Topic:     topic,
		Route:     route,
		ReachM:    reachM,
	}
}

func defaultRoute() []waypoint {
	return []waypoint{
		{Name: "blok_m", Lat: -6.2441, Lon: 106.8000},
		{Name: "monas", Lat: -6.1754, Lon: 106.8272},
	}
}

func parseRoute(s string) ([]waypoint, error) {
	parts := strings.Split(s, ";")
	out := make([]waypoint, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fields := strings.Split(p, ":")
		if len(fields) != 3 {
			return nil, fmt.Errorf("entry %d: expected name:lat:lon, got %q", i, p)
		}
		lat, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return nil, fmt.Errorf("entry %d lat: %w", i, err)
		}
		lon, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("entry %d lon: %w", i, err)
		}
		out = append(out, waypoint{Name: fields[0], Lat: lat, Lon: lon})
	}
	return out, nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
