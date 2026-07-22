package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/worryzyy/upstream-hub/internal/storage"
)

func TestBarkSend(t *testing.T) {
	type requestBody struct {
		DeviceKey string `json:"device_key"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Group     string `json:"group"`
	}
	type receivedRequest struct {
		Method      string
		Path        string
		ContentType string
		Body        requestBody
		DecodeErr   error
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		decodeErr := json.NewDecoder(r.Body).Decode(&body)
		received <- receivedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
			DecodeErr:   decodeErr,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "success"})
	}))
	defer server.Close()

	raw, err := json.Marshal(barkConfig{
		URL:   server.URL + "/test-device-key/",
		Group: "upstream-hub",
	})
	if err != nil {
		t.Fatal(err)
	}
	notifier, err := newBark(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if notifier.Type() != storage.NotifyBark {
		t.Fatalf("unexpected notifier type: %s", notifier.Type())
	}

	err = notifier.Send(context.Background(), Message{Subject: "测试标题", Body: "测试正文"})
	if err != nil {
		t.Fatal(err)
	}
	got := <-received
	if got.DecodeErr != nil {
		t.Fatal(got.DecodeErr)
	}
	if got.Method != http.MethodPost || got.Path != "/push" {
		t.Fatalf("unexpected request: %s %s", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.ContentType, "application/json") {
		t.Fatalf("unexpected content type: %s", got.ContentType)
	}
	if got.Body.DeviceKey != "test-device-key" || got.Body.Title != "测试标题" || got.Body.Body != "测试正文" || got.Body.Group != "upstream-hub" {
		t.Fatalf("unexpected request body: %+v", got.Body)
	}
}

func TestNewBarkRejectsInvalidURL(t *testing.T) {
	tests := []string{
		`{"url":""}`,
		`{"url":"ftp://api.day.app/device-key"}`,
		`{"url":"https://api.day.app/"}`,
		`{"url":"https://api.day.app/device-key#fragment"}`,
	}
	for _, raw := range tests {
		if _, err := newBark(raw); err == nil {
			t.Errorf("newBark(%s) expected an error", raw)
		}
	}
}

func TestBarkSendRejectsApplicationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 400, "message": "invalid device key"})
	}))
	defer server.Close()

	raw, err := json.Marshal(barkConfig{URL: server.URL + "/secret-device-key"})
	if err != nil {
		t.Fatal(err)
	}
	notifier, err := newBark(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	err = notifier.Send(context.Background(), Message{Body: "test"})
	if err == nil || !strings.Contains(err.Error(), "invalid device key") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "secret-device-key") {
		t.Fatalf("error exposed device key: %v", err)
	}
}
