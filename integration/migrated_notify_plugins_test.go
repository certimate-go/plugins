package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	notificationSubject = "[Certimate] Notification Testing"
	notificationMessage = "Welcome to use Certimate!"
)

type expectedNotifyRequest struct {
	method, path, authorization string
	body                        map[string]any
}

type migratedNotifyFixture struct {
	name, credential, host string
	config                 map[string]any
	delivery               expectedNotifyRequest
	verify                 func(*testing.T, *http.Request, map[string]any)
}

func TestMigratedNotificationPlugins(t *testing.T) {
	webhookCases := []migratedNotifyFixture{
		{
			name: "dingtalkbot", credential: "webhookUrl",
			config: map[string]any{"secret": "fixture-secret", "customPayload": `{"text":{"content":"${SUBJECT} / ${CERTIMATE_NOTIFIER_MESSAGE}"}}`},
			delivery: expectedNotifyRequest{http.MethodPost, "/dingtalkbot", "", map[string]any{
				"text": map[string]any{"content": notificationSubject + " / " + notificationMessage},
			}},
			verify: func(t *testing.T, r *http.Request, _ map[string]any) {
				ts, signature := r.URL.Query().Get("timestamp"), r.URL.Query().Get("sign")
				h := hmac.New(sha256.New, []byte("fixture-secret"))
				_, _ = fmt.Fprintf(h, "%s\nfixture-secret", ts)
				if ts == "" || signature != base64.StdEncoding.EncodeToString(h.Sum(nil)) || r.URL.Query().Get("access_token") != "fixture" {
					t.Error("DingTalk webhook did not preserve the token or sign the request")
				}
			},
		},
		{
			name: "larkbot", credential: "webhookUrl", config: map[string]any{"secret": "fixture-secret"},
			delivery: expectedNotifyRequest{http.MethodPost, "/larkbot", "", map[string]any{
				"msg_type": "text", "content": map[string]any{"text": notificationSubject + "\n\n" + notificationMessage},
			}},
			verify: func(t *testing.T, _ *http.Request, body map[string]any) {
				ts, _ := body["timestamp"].(string)
				h := hmac.New(sha256.New, []byte(ts+"\nfixture-secret"))
				if ts == "" || body["sign"] != base64.StdEncoding.EncodeToString(h.Sum(nil)) {
					t.Error("Lark webhook did not sign the request")
				}
				delete(body, "timestamp")
				delete(body, "sign")
			},
		},
		{
			name: "wecombot", credential: "webhookUrl", config: map[string]any{},
			delivery: expectedNotifyRequest{http.MethodPost, "/wecombot", "", map[string]any{
				"msgtype": "text", "text": map[string]any{"content": notificationSubject + "\n\n" + notificationMessage},
			}},
		},
		{
			name: "matrix", credential: "serverUrl", config: map[string]any{"accessToken": "matrix-token", "roomId": "!legacy:fixture", "userId": "@user:fixture"},
			delivery: expectedNotifyRequest{
				http.MethodPut, "/_matrix/client/v3/rooms/!legacy:fixture/send/m.room.message/", "Bearer matrix-token",
				map[string]any{"msgtype": "m.text", "body": notificationSubject + "\n\n" + notificationMessage},
			},
		},
		{
			name: "mattermost", credential: "serverUrl", config: map[string]any{"username": "fixture-user", "password": "fixture-password", "channelId": "legacy-channel"},
			delivery: expectedNotifyRequest{http.MethodPost, "/api/v4/posts", "Bearer mattermost-token", map[string]any{
				"channel_id": "legacy-channel",
				"props":      map[string]any{"attachments": []any{map[string]any{"title": notificationSubject, "text": notificationMessage}}},
			}},
		},
	}

	serveMigratedNotificationFixtures(t, webhookCases)
}

func TestMigratedCloudNotificationPlugins(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the HTTPS fixture uses SSL_CERT_FILE for subprocess trust on Linux")
	}

	text := notificationSubject + "\n" + notificationMessage
	serveMigratedNotificationFixtures(t, []migratedNotifyFixture{
		{
			name: "discordbot", host: "discord.com", config: map[string]any{"botToken": "discord-token", "channelId": "legacy-channel"},
			delivery: expectedNotifyRequest{http.MethodPost, "/api/v9/channels/legacy-channel/messages", "Bot discord-token", map[string]any{"content": text}},
		},
		{
			name: "slackbot", host: "slack.com", config: map[string]any{"botToken": "slack-token", "channelId": "legacy-channel"},
			delivery: expectedNotifyRequest{http.MethodPost, "/api/chat.postMessage", "Bearer slack-token", map[string]any{
				"channel": "legacy-channel", "text": text, "token": "slack-token",
			}},
		},
		{
			name: "telegrambot", host: "api.telegram.org", config: map[string]any{"botToken": "telegram-token", "chatId": -123456789},
			delivery: expectedNotifyRequest{http.MethodPost, "/bottelegram-token/sendMessage", "", map[string]any{
				"chat_id": "-123456789", "text": text,
			}},
		},
	})
}

func serveMigratedNotificationFixtures(t *testing.T, providers []migratedNotifyFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var names, hosts []string
	for _, provider := range providers {
		names = append(names, provider.name)
		if provider.host != "" {
			hosts = append(hosts, provider.host)
		}
	}
	calls := make(chan string, 1)
	var rejectDelivery atomic.Bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/matrix/client":
			if r.Header.Get("Authorization") != "" {
				t.Error("Matrix discovery received the access token")
			}
			http.NotFound(w, r)
			return
		case "/_matrix/client/versions":
			_, _ = w.Write([]byte(`{"versions":["v1.1"]}`))
			return
		case "/api/v4/users/login":
			want := expectedNotifyRequest{http.MethodPost, r.URL.Path, "", map[string]any{"login_id": "fixture-user", "password": "fixture-password"}}
			assertNotifyRequest(t, r, want, nil)
			w.Header().Set("Token", "mattermost-token")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		for _, provider := range providers {
			if r.URL.Path != provider.delivery.path && !(provider.name == "matrix" && strings.HasPrefix(r.URL.Path, provider.delivery.path)) {
				continue
			}
			want := provider.delivery
			want.path = r.URL.Path
			assertNotifyRequest(t, r, want, provider.verify)
			select {
			case calls <- provider.name:
			default:
				t.Errorf("duplicate request for %s", provider.name)
			}
			if rejectDelivery.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if provider.name == "matrix" {
				_, _ = w.Write([]byte(`{"event_id":"$event"}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		t.Errorf("unexpected notification request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}

	upstream := httptest.NewServer(http.HandlerFunc(handler))
	defer upstream.Close()
	if len(hosts) > 0 {
		newB4CloudProxy(t, hosts, http.HandlerFunc(handler))
	}
	api := buildAndStartNotificationCore(t, ctx, names...)

	var catalog struct {
		Data []struct {
			ProviderType, Capability string
			OwnsAccess               bool
		}
	}
	if err := api.request(ctx, http.MethodGet, "/api/plugin-catalog", nil, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			loaded := false
			for _, entry := range catalog.Data {
				loaded = loaded || (entry.ProviderType == provider.name && entry.Capability == "notify" && entry.OwnsAccess)
			}
			if !loaded {
				t.Fatalf("migrated plugin missing from notification catalog: %s", provider.name)
			}
			for _, category := range []string{"access", "notify"} {
				var schema struct {
					Data struct{ Provider, Category string }
				}
				if err := api.request(ctx, http.MethodGet, "/api/provider-schemas/"+provider.name+"?category="+category, nil, &schema); err != nil {
					t.Fatal(err)
				}
				if schema.Data.Provider != provider.name || schema.Data.Category != category {
					t.Fatalf("unexpected %s schema: %+v", category, schema)
				}
			}

			switch provider.credential {
			case "webhookUrl":
				provider.config["webhookUrl"] = upstream.URL + provider.delivery.path + "?access_token=fixture"
			case "serverUrl":
				provider.config["serverUrl"] = upstream.URL
			}
			var access struct{ ID string }
			record := map[string]any{"name": provider.name, "provider": provider.name, "reserve": "notif", "config": provider.config}
			if err := api.request(ctx, http.MethodPost, "/api/collections/access/records", record, &access); err != nil {
				t.Fatal(err)
			}
			for _, success := range []bool{true, false, true} {
				rejectDelivery.Store(!success)
				pushMigratedNotification(t, ctx, api, provider.name, access.ID, success)
				assertNotifyDelivery(t, calls, provider.name)
			}
		})
	}
}

func assertNotifyRequest(t *testing.T, r *http.Request, want expectedNotifyRequest, verify func(*testing.T, *http.Request, map[string]any)) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("invalid notification request: %v", err)
		return
	}
	if verify != nil {
		verify(t, r, body)
	}
	if r.Method != want.method || r.URL.Path != want.path || r.Header.Get("Authorization") != want.authorization ||
		!reflect.DeepEqual(body, want.body) {
		t.Errorf("unexpected notification request %s %s: %#v", r.Method, r.URL.Path, body)
	}
}

func assertNotifyDelivery(t *testing.T, calls <-chan string, name string) {
	t.Helper()
	select {
	case delivered := <-calls:
		if delivered != name {
			t.Fatalf("expected notification via %s, got %s", name, delivered)
		}
	default:
		t.Fatalf("plugin %s did not send a request", name)
	}
}

func pushMigratedNotification(t *testing.T, ctx context.Context, api *b1WorkflowAPI, provider, accessID string, success bool) {
	t.Helper()
	var result struct {
		Code int
		Msg  string
	}
	body := map[string]string{"provider": provider, "accessId": accessID}
	if err := api.request(ctx, http.MethodPost, "/api/notifications/test", body, &result); err != nil {
		t.Fatal(err)
	}
	if (result.Code == 0) != success || (!success && !strings.Contains(result.Msg, "401")) {
		t.Fatalf("notification response = %+v, expected success=%t", result, success)
	}
}
