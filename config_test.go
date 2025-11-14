package config_test

import (
	"encoding/json"
	"github.com/kittenbark/config"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

type ObjectT struct {
	Key string `json:"key"`
}

type ConfigNameT struct {
	Integer int     `json:"integer"`
	Float   float64 `json:"float"`
	String  string  `json:"string"`
	Boolean bool    `json:"boolean"`
	Object  ObjectT `json:"object"`
}

var (
	expectedConfigNameValue = &ConfigNameT{
		Integer: 1,
		Float:   0.5,
		String:  "hello",
		Boolean: true,
		Object: ObjectT{
			Key: "value",
		},
	}
)

func TestConfig_Basic(t *testing.T) {
	t.Parallel()

	cache := config.NewCache("./testdata")

	cfg, err := config.Get[ConfigNameT](cache, "config_name")
	if err != nil {
		t.Fatalf("error while getting 'config_name' %v", err)
	}
	if !reflect.DeepEqual(expectedConfigNameValue, cfg) {
		t.Fatalf("expected: %v, actual: %v", expectedConfigNameValue, cfg)
	}

	if cfg, err = config.Get[ConfigNameT](cache, "config_name_not_found"); err == nil {
		t.Fatalf("cfg.Get[ConfigNameT] expected to get an error")
	}

	kvConfig, err := config.Get[map[string]string](cache, "config_key_value")
	if err != nil {
		t.Fatalf("error while getting 'config_name' %v", err)
	}
	expectedKvConfig := map[string]string{"key": "value", "key_2": "value_2"}
	if !maps.Equal(expectedKvConfig, *kvConfig) {
		t.Fatalf("expected: %v, actual: %v", expectedKvConfig, *kvConfig)
	}
}

func TestConfig_Sync(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	if err := os.CopyFS(dir, os.DirFS("./testdata")); err != nil {
		t.Fatalf("error while copying dir: %v", err)
	}

	timeout := time.Millisecond
	cache := config.NewCache(dir).
		SyncTimeout(timeout)
	configPath := filepath.Join(dir, "config_name.json")

	cfg := *expectedConfigNameValue
	wg := sync.WaitGroup{}
	expectedCounter := cfg.Integer
	lastLoaded := 0
	sameLoadedCount := 0
	iterations := 10_000
	for range iterations {
		cfg.Integer += 1
		if err := os.WriteFile(configPath, must(json.Marshal(cfg)), 0644); err != nil {
			t.Fatalf("error while writing config: %v", err)
		}
		time.AfterFunc(timeout, func() { expectedCounter += 1 })

		loaded, err := config.Get[ConfigNameT](cache, "config_name")
		if err != nil {
			t.Fatalf("error while getting 'config_name' %v", err)
		}
		if loaded.Integer+5 < expectedCounter {
			t.Fatalf("config_name not updated")
		}

		if loaded.Integer == lastLoaded {
			sameLoadedCount++
		} else {
			lastLoaded = loaded.Integer
		}
	}
	wg.Wait()

	if sameLoadedCount == 0 {
		t.Fatalf("config_name syncing every time?")
	}
	if sameLoadedCount+1 == iterations {
		t.Fatalf("config_name never updated?")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
