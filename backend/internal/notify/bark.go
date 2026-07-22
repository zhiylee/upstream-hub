package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/worryzyy/upstream-hub/internal/storage"
)

func init() {
	Register(storage.NotifyBark, func(raw string) (Notifier, error) { return newBark(raw) })
}

type barkConfig struct {
	URL   string `json:"url"`
	Group string `json:"group,omitempty"`
}

type bark struct {
	cfg       barkConfig
	endpoint  string
	deviceKey string
	http      *resty.Client
}

func newBark(raw string) (*bark, error) {
	var cfg barkConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Group = strings.TrimSpace(cfg.Group)
	if cfg.URL == "" {
		return nil, errors.New("bark url is required")
	}

	pushURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid bark url: %w", err)
	}
	if (pushURL.Scheme != "http" && pushURL.Scheme != "https") || pushURL.Host == "" {
		return nil, errors.New("bark url must be an absolute http or https URL")
	}
	if pushURL.Fragment != "" {
		return nil, errors.New("bark url must not contain a fragment")
	}

	escapedPath := strings.TrimRight(pushURL.EscapedPath(), "/")
	lastSlash := strings.LastIndex(escapedPath, "/")
	if lastSlash < 0 || lastSlash == len(escapedPath)-1 {
		return nil, errors.New("bark url must include a device key")
	}
	deviceKey, err := url.PathUnescape(escapedPath[lastSlash+1:])
	if err != nil || strings.TrimSpace(deviceKey) == "" {
		return nil, errors.New("bark url contains an invalid device key")
	}

	baseEscapedPath := strings.TrimRight(escapedPath[:lastSlash], "/")
	basePath, err := url.PathUnescape(baseEscapedPath)
	if err != nil {
		return nil, errors.New("bark url contains an invalid path")
	}
	pushURL.Path = basePath + "/push"
	pushURL.RawPath = baseEscapedPath + "/push"
	if pushURL.RawPath == pushURL.Path {
		pushURL.RawPath = ""
	}

	return &bark{
		cfg:       cfg,
		endpoint:  pushURL.String(),
		deviceKey: deviceKey,
		http:      resty.New().SetTimeout(10 * time.Second),
	}, nil
}

func (b *bark) Type() storage.NotificationChannelType { return storage.NotifyBark }

func (b *bark) Send(ctx context.Context, msg Message) error {
	body := map[string]any{
		"device_key": b.deviceKey,
		"title":      msg.Subject,
		"body":       msg.Body,
	}
	if b.cfg.Group != "" {
		body["group"] = b.cfg.Group
	}

	resp, err := b.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json; charset=utf-8").
		SetBody(body).
		Post(b.endpoint)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return errors.New("bark returned " + resp.Status())
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err == nil && result.Code != 0 && result.Code != 200 {
		if result.Message != "" {
			return fmt.Errorf("bark returned code %d: %s", result.Code, result.Message)
		}
		return fmt.Errorf("bark returned code %d", result.Code)
	}
	return nil
}
