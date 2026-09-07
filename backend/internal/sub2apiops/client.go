package sub2apiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	adminPrefix    = "/api/v1/admin"
	maxResponseLen = 8 << 20
)

type RemoteError struct {
	StatusCode int
	Message    string
}

func (e *RemoteError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Sub2API 返回 HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("Sub2API: %s", e.Message)
}

type Client struct {
	baseURL  string
	adminKey string
	http     *http.Client
}

func NormalizeSiteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Sub2API 地址不能为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("解析 Sub2API 地址: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("Sub2API 地址仅支持 http 或 https")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("Sub2API 地址缺少主机名")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Sub2API 地址不能包含用户信息、查询参数或片段")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	normalized := parsed.String()
	if len(normalized) > 512 {
		return "", errors.New("Sub2API 地址不能超过 512 个字符")
	}
	return normalized, nil
}

func (c *Client) Test(ctx context.Context) error {
	payload, _, err := c.Do(ctx, http.MethodGet, "/compliance", nil, nil)
	if err != nil {
		return err
	}
	var compliance struct {
		Code int `json:"code"`
		Data struct {
			Required *bool `json:"required"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &compliance); err != nil || compliance.Code != 0 || compliance.Data.Required == nil {
		return errors.New("目标未返回有效的 Sub2API 管理响应")
	}
	if *compliance.Data.Required {
		return errors.New("Sub2API 管理员尚未完成合规确认")
	}

	payload, _, err = c.Do(ctx, http.MethodGet, "/groups/all", nil, nil)
	if err != nil {
		return err
	}
	var protected struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &protected); err != nil || protected.Code != 0 || len(protected.Data) == 0 {
		return errors.New("目标未返回有效的 Sub2API 管理响应")
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(protected.Data, &groups); err != nil {
		return errors.New("目标未返回有效的 Sub2API 管理响应")
	}
	return nil
}

func NewClient(siteURL, adminKey string) (*Client, error) {
	baseURL, err := NormalizeSiteURL(siteURL)
	if err != nil {
		return nil, err
	}
	adminKey = strings.TrimSpace(adminKey)
	if adminKey == "" {
		return nil, errors.New("Admin Key 不能为空")
	}
	if len(adminKey) > 1024 {
		return nil, errors.New("Admin Key 过长")
	}
	return &Client{
		baseURL:  baseURL,
		adminKey: adminKey,
		http: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Do(
	ctx context.Context,
	method string,
	endpoint string,
	query url.Values,
	body []byte,
) ([]byte, int, error) {
	if !strings.HasPrefix(endpoint, "/") || strings.Contains(endpoint, "..") {
		return nil, 0, errors.New("非法 Sub2API 管理接口路径")
	}
	target, err := url.Parse(c.baseURL + adminPrefix + endpoint)
	if err != nil {
		return nil, 0, fmt.Errorf("构造 Sub2API 请求地址: %w", err)
	}
	target.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("构造 Sub2API 请求: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-API-Key", c.adminKey)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 Sub2API: %w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, maxResponseLen+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("读取 Sub2API 响应: %w", err)
	}
	if len(payload) > maxResponseLen {
		return nil, response.StatusCode, errors.New("Sub2API 响应超过 8 MiB 限制")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.StatusCode, &RemoteError{
			StatusCode: response.StatusCode,
			Message:    remoteErrorMessage(payload),
		}
	}
	return payload, response.StatusCode, nil
}

func remoteErrorMessage(payload []byte) string {
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return strings.TrimSpace(string(payload))
	}
	for _, key := range []string{"reason", "message", "error", "code"} {
		switch item := value[key].(type) {
		case string:
			if strings.TrimSpace(item) != "" {
				return item
			}
		case map[string]any:
			if message, ok := item["message"].(string); ok && message != "" {
				return message
			}
		}
	}
	return ""
}
