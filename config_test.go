package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kittenbark/config"
	"github.com/kittenbark/config/config_web"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
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

func TestConfig_Parallel(t *testing.T) {
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

	wg := &sync.WaitGroup{}
	value := atomic.Int64{}
	updates := atomic.Int64{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for ctx.Err() == nil {
		for j := 0; j < 10; j++ {
			wg.Go(func() {
				val := value.Add(1)
				err := config.UpdateContext(ctx, cache, "config_name", ConfigNameT{
					Integer: int(val),
					Float:   0.5,
					String:  "hello",
					Boolean: true,
					Object:  ObjectT{Key: "value"},
				})
				if err == nil {
					updates.Add(1)
				}
				if err != nil && !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("error while updating config_name: %v", err)
				}
			})
		}
		wg.Go(func() {
			_, err := config.GetContext[ConfigNameT](ctx, cache, "config_name")
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("error while getting 'config_name' %v", err)
				return
			}
		})
	}
	wg.Wait()

	println("updates: ", updates.Load())
}

func TestConfig_Parallel_HTTP(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	if err := os.CopyFS(dir, os.DirFS("./testdata")); err != nil {
		t.Fatalf("error while copying dir: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*6)
	defer cancel()

	timeout := time.Millisecond
	cache := config.NewCache(dir).
		SyncTimeout(timeout)
	mux := http.NewServeMux()
	mux.HandleFunc(config_web.DefaultWebUrlGet, func(w http.ResponseWriter, r *http.Request) {
		if err := config_web.HandlerGetVerbose(cache)(ctx, w, r); err != nil {
			slog.Error("error while getting 'config_name'", "err", err)
		}
	})
	mux.HandleFunc(config_web.DefaultWebUrlUpdate, func(w http.ResponseWriter, r *http.Request) {
		if err := config_web.HandlerUpdateVerbose(cache)(ctx, w, r); err != nil {
			slog.Error("error while updating 'config_name'", "err", err)
		}
	})
	server := &http.Server{Handler: mux}
	time.AfterFunc(time.Second*5, func() { _ = server.Shutdown(context.Background()) })
	go func() { _ = http.ListenAndServe(":8080", mux) }()
	time.Sleep(time.Second * 2)

	wg := &sync.WaitGroup{}
	value := atomic.Int64{}
	updates := atomic.Int64{}
	client := &config_web.Client{Host: "http://127.0.0.1:8080"}
	for ctx.Err() == nil {
		for j := 0; j < 10; j++ {
			wg.Go(func() {
				val := value.Add(1)
				err := config_web.UpdateContext(ctx, client, "config_name", ConfigNameT{
					Integer: int(val),
					Float:   0.5,
					String:  "hello",
					Boolean: true,
					Object:  ObjectT{Key: "value"},
				})
				if err == nil {
					updates.Add(1)
				}
				if err != nil && !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("error while updating config_name: %v", err)
				}
			})
		}
		wg.Go(func() {
			_, err := config_web.GetContext[ConfigNameT](ctx, client, "config_name")
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("error while getting 'config_name' %v", err)
				return
			}
		})
		time.Sleep(time.Millisecond * 10)
	}
	wg.Wait()

	println("updates: ", updates.Load())
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
