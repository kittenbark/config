package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func Get[T any](cache *Cache, name string) (*T, error) {
	cfg, updated, err := cache.verboseGet(name)
	if err != nil {
		return nil, err
	}

	if cfg.Value != nil && !updated {
		return cfg.Value.(*T), nil
	}
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cfg.Value != nil && !updated {
		return cfg.Value.(*T), nil
	}

	var result T
	if err = json.Unmarshal(cfg.Raw, &result); err != nil {
		return nil, err
	}
	cfg.Value = &result
	return &result, nil
}

func NewCache(directory string) *Cache {
	return &Cache{
		directory:   directory,
		syncTimeout: time.Minute,
		configs:     make(map[string]*configValue),
	}
}

type Cache struct {
	directory   string
	lock        sync.RWMutex
	configs     map[string]*configValue
	syncTimeout time.Duration
}

func (cache *Cache) SyncTimeout(duration time.Duration) *Cache {
	cache.syncTimeout = duration
	return cache
}

func (cache *Cache) Get(name string) ([]byte, error) {
	cfg, _, err := cache.verboseGet(name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return cfg.Raw, err
}

func (cache *Cache) Handler() http.Handler {
	type RequestSchema struct {
		Config string `json:"config"`
	}
	type ResponseError struct {
		Error string `json:"error"`
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		configName := req.URL.Query().Get("config")
		if configName == "" {
			var body RequestSchema
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				data, _ := json.Marshal(ResponseError{Error: err.Error()})
				_, _ = rw.Write(data)
				return
			}
		}

		result, err := cache.Get(configName)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			data, _ := json.Marshal(ResponseError{Error: err.Error()})
			_, _ = rw.Write(data)
			return
		}

		rw.WriteHeader(http.StatusOK)
		data, _ := json.Marshal(result)
		_, _ = rw.Write(data)
		return
	})
}

func (cache *Cache) verboseGet(name string) (cfg *configValue, updated bool, err error) {
	cache.lock.RLock()

	config, ok := cache.configs[name]
	if ok && time.Since(config.LastUpdate) < cache.syncTimeout {
		defer cache.lock.RUnlock()
		return config, false, nil
	}
	var lastUpdate time.Time
	if ok {
		lastUpdate = config.LastUpdate
	}

	path := filepath.Join(cache.directory, fmt.Sprintf("%s.json", strings.TrimSpace(name)))
	stat, err := os.Stat(path)
	if err != nil {
		defer cache.lock.RUnlock()
		return nil, false, err
	}
	if !stat.ModTime().After(lastUpdate) {
		defer cache.lock.RUnlock()
		return config, false, nil
	}

	// This is not a race, right? Double-checking and so on.
	cache.lock.RUnlock()
	cache.lock.Lock()
	defer cache.lock.Unlock()
	config, ok = cache.configs[name]
	if ok && !stat.ModTime().After(config.LastUpdate) {
		return config, false, nil
	}

	loaded := time.Now()
	data, err := os.ReadFile(path)
	if err != nil {
		defer cache.lock.RUnlock()
		return nil, false, err
	}
	config = &configValue{
		LastUpdate: loaded,
		Raw:        data,
	}
	cache.configs[name] = config
	return config, true, nil
}

type Stats struct {
	Directory string
	Configs   []string
}

func (cache *Cache) Stats() Stats {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	result := Stats{
		Directory: cache.directory,
	}
	for configName := range cache.configs {
		result.Configs = append(result.Configs, configName)
	}
	return result
}

type configValue struct {
	LastUpdate time.Time
	Value      any
	Raw        []byte
}

func as[T any](config *configValue) (*T, error) {
	result, ok := config.Value.(*T)
	if !ok {
		return nil, fmt.Errorf("config: unexpected type %T", config.Value)
	}
	return result, nil
}

func reloadConfig[T any](cache *Cache, name string, path string) (*T, error) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	syncedAt := time.Now()
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	var cfg T
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}
	cache.configs[name] = &configValue{
		LastUpdate: syncedAt,
		Value:      &cfg,
	}
	return &cfg, nil
}
