package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func Get[T any](cache *Cache, name string) (*T, error) {
	return GetContext[T](context.Background(), cache, name)
}

func Update[T any](cache *Cache, name string, value T) error {
	return UpdateContext[T](context.Background(), cache, name, value)
}

func GetContext[T any](ctx context.Context, cache *Cache, name string) (*T, error) {
	cfg, updated, err := cache.verboseGet(ctx, name)
	if err != nil {
		return nil, err
	}

	if cfg.Value != nil && !updated {
		return cfg.Value.(*T), nil
	}
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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

func UpdateContext[T any](ctx context.Context, cache *Cache, name string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cache.UpdateContext(ctx, name, data)
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

func (cache *Cache) GetContext(ctx context.Context, name string) ([]byte, error) {
	cfg, _, err := cache.verboseGet(ctx, name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return cfg.Raw, err
}

func (cache *Cache) UpdateContext(ctx context.Context, name string, data []byte) error {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	path := filepath.Join(cache.directory, fmt.Sprintf("%s.json", name))
	return os.WriteFile(path, data, 0666)
}

func (cache *Cache) Get(name string) ([]byte, error) {
	return cache.GetContext(context.Background(), name)
}

func (cache *Cache) Update(name string, data []byte) error {
	return cache.UpdateContext(context.Background(), name, data)
}

func (cache *Cache) verboseGet(ctx context.Context, name string) (cfg *configValue, updated bool, err error) {
	cache.lock.RLock()

	if ctx.Err() != nil {
		defer cache.lock.RUnlock()
		return nil, false, ctx.Err()
	}

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

	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}

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
