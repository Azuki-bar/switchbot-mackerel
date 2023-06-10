package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/caarlos0/env/v8"
	"github.com/nasa9084/go-switchbot"
	"golang.org/x/exp/slog"
)

type Config struct {
	MackerelAPIKey      string   `env:"MACKEREL_API_KEY,required"`
	MackerelServiceName string   `env:"MACKEREL_SERVICE_NAME,required"`
	SwitchbotOpenToken  string   `env:"SWITCHBOT_OPEN_TOKEN,required"`
	SwitchbotSecretKey  string   `env:"SWITCHBOT_SECRET_KEY,required"`
	SwitchbotDeviceIDs  []string `env:"SWITCHBOT_DEVICE_IDS,required"`
}
type MackerelConfig struct {
	APIKey      string
	ServiceName string
}

type SwitchbotConfig struct {
	OpenToken string
	SecretKey string
	DeviceIDs []string
}

type SwitchbotCollector struct {
	client    *switchbot.Client
	deviceIDs []string
}
type SwitchbotHubMetric struct {
	DeviceID string
	Values   map[string]float64
}

type MackerelMetric struct {
	Name  string  `json:"name"`
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

func NewSwitchbotCollector(openToken, secretKey string, deviceIDs []string) *SwitchbotCollector {
	client := switchbot.New(openToken, secretKey)
	c := SwitchbotCollector{
		client:    client,
		deviceIDs: deviceIDs,
	}
	return &c
}

func (sc *SwitchbotCollector) Collect(ctx context.Context) ([]SwitchbotHubMetric, error) {
	metrics := make([]SwitchbotHubMetric, len(sc.deviceIDs))
	for i, deviceID := range sc.deviceIDs {
		status, err := sc.client.Device().Status(ctx, deviceID)
		if err != nil {
			return nil, err
		}
		metrics[i] = SwitchbotHubMetric{
			DeviceID: deviceID,
			Values: map[string]float64{
				Humidity:    float64(status.Humidity),
				Temperature: status.Temperature,
				Battery:     float64(status.Battery),
			},
		}
	}
	return metrics, nil
}

const (
	mackerelEndPoint = "https://api.mackerelio.com/"
)
const (
	Humidity    = "humidity"
	Temperature = "temperature"
	Battery     = "battery"
)

type MackerelSender struct {
	serviceName string
	apiKey      string
}

func NewMackerelSender(apikey, serviceName string) *MackerelSender {
	return &MackerelSender{
		apiKey:      apikey,
		serviceName: serviceName,
	}
}

func (me *MackerelSender) Send(ctx context.Context, metrics []MackerelMetric) error {
	path, err := url.JoinPath(mackerelEndPoint, "api", "v0", "services", me.serviceName, "tsdb")
	if err != nil {
		return err
	}
	b, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	if err != nil {
		slog.Error("create request failed", err)
		return err
	}
	req.Header.Add("X-Api-Key", me.apiKey)
	req.Header.Add("Content-Type", "application/json")
	req = req.WithContext(ctx)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("mackerel returns non 200 status code", resp.StatusCode)
		return fmt.Errorf("mackerel returns %d", resp.StatusCode)
	}
	return nil
}

var logger *slog.Logger
var config Config

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := env.Parse(&config)
	if err != nil {
		logger.Error("parse env config failed", err)
		os.Exit(1)
	}
}

func main() {
	ctx, cancelCause := context.WithCancelCause(context.Background())
	defer cancelCause(nil)
	collector := NewSwitchbotCollector(config.SwitchbotOpenToken, config.SwitchbotSecretKey, config.SwitchbotDeviceIDs)
	metrics, err := collector.Collect(ctx)
	if err != nil {
		logger.Error("collect metrics failed", err)
		os.Exit(1)
	}
	sender := NewMackerelSender(config.MackerelAPIKey, config.MackerelServiceName)
	mackerelMetics := make([]MackerelMetric, 0)
	timeNow := time.Now().Unix()

	for _, metric := range metrics {
		for _, valType := range []string{Humidity, Temperature, Battery} {
			mackerelMetics = append(
				mackerelMetics,
				MackerelMetric{
					Name:  fmt.Sprintf("switchbot.%s.%s", metric.DeviceID, valType),
					Time:  timeNow,
					Value: metric.Values[valType],
				},
			)
		}

	}

	err = sender.Send(ctx, mackerelMetics)
	if err != nil {
		logger.Error("send metrics failed", err)
		os.Exit(1)
	}
	slog.Info("send metrics success")
}
