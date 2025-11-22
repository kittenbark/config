package config_web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	DefaultWebUrlGet    = "/v1/config/get"
	DefaultWebUrlUpdate = "/v1/config/update"
)

func Get[T any](client *Client, name string, reqMod ...func(r *http.Request)) (*T, error) {
	return GetContext[T](context.Background(), client, name, reqMod...)
}

func Update[T any](client *Client, name string, val *T, reqMod ...func(r *http.Request)) error {
	return UpdateContext(context.Background(), client, name, val, reqMod...)
}

func GetContext[T any](ctx context.Context, client *Client, name string, reqMod ...func(r *http.Request)) (*T, error) {
	data, err := client.GetContext(ctx, name, reqMod...)
	if err != nil {
		return nil, err
	}
	var val T
	if err := json.Unmarshal(data, &val); err != nil {
		slog.Error("config_web: unmarshal config name %v: %v", "err", err, "data", string(data))
		return nil, err
	}
	return &val, nil
}

func UpdateContext[T any](ctx context.Context, client *Client, name string, val T, reqMod ...func(r *http.Request)) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return client.UpdateContext(ctx, name, data, reqMod...)
}

type Client struct {
	Host      string
	UrlGet    string // default: DefaultWebUrlGet
	UrlUpdate string // default: DefaultWebUrlUpdate
	Client    *http.Client

	lock        sync.RWMutex
	initialized bool
}

func (client *Client) GetContext(ctx context.Context, name string, reqMod ...func(r *http.Request)) ([]byte, error) {
	client.init()

	endpoint, err := url.Parse(client.Host)
	if err != nil {
		return nil, fmt.Errorf("config_web: get, request url build error %w", err)
	}
	endpoint = endpoint.JoinPath(client.UrlGet)
	q := endpoint.Query()
	q.Add("config", name)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("config_web: get, request build error %w", err)
	}
	for _, mod := range reqMod {
		mod(req)
	}
	resp, err := client.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config_web: get, request error %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config_web: get, request status code %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("config_web: get, read response body error %w", err)
	}
	return body, nil
}

func (client *Client) UpdateContext(ctx context.Context, name string, data []byte, reqMod ...func(r *http.Request)) error {
	client.init()

	endpoint, err := url.Parse(client.Host)
	if err != nil {
		return fmt.Errorf("config_web: update, request url build error %w", err)
	}
	endpoint = endpoint.JoinPath(client.UrlUpdate)
	q := endpoint.Query()
	q.Add("config", name)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("config_web: update, request error %w", err)
	}
	for _, mod := range reqMod {
		mod(req)
	}
	resp, err := client.Client.Do(req)
	if err != nil {
		return fmt.Errorf("config_web: update, request error %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("config_web: update, request status code %s", resp.Status)
	}
	return nil
}

func (client *Client) init() *Client {
	client.lock.RLock()
	if client.initialized {
		return client
	}
	client.lock.RUnlock()
	client.lock.Lock()
	defer client.lock.Unlock()
	if client.initialized {
		return client
	}

	client.Host = strings.TrimSuffix(client.Host, "/")
	client.initialized = true
	client.UrlGet = DefaultWebUrlGet
	client.UrlUpdate = DefaultWebUrlUpdate
	client.Client = http.DefaultClient
	return client
}
