package config_web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kittenbark/config"
	"io"
	"net/http"
)

func HandlerGetVerbose(cache *config.Cache) func(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	type RequestSchema struct {
		Config string `json:"config"`
	}
	type ResponseError struct {
		Error string `json:"error"`
	}

	return func(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
		configName := req.URL.Query().Get("config")
		if configName == "" {
			var body RequestSchema
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				data, _ := json.Marshal(ResponseError{Error: err.Error()})
				_, respErr := rw.Write(data)
				return fmt.Errorf("config_web: get, error parsing config name %v", errors.Join(err, respErr))
			}
		}
		if configName == "" {
			return fmt.Errorf("config_web: get, request config name not found")
		}

		resultData, err := cache.Get(configName)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			data, _ := json.Marshal(ResponseError{Error: err.Error()})
			_, respErr := rw.Write(data)
			return fmt.Errorf("config_web: get, error finding config %v", errors.Join(err, respErr))
		}

		rw.WriteHeader(http.StatusOK)
		if _, respErr := rw.Write(resultData); respErr != nil {
			return fmt.Errorf("config_web: get, error making response %v", respErr)
		}
		return nil
	}
}

func HandlerUpdateVerbose(cache *config.Cache) func(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	type RequestSchema struct {
		Config string `json:"config"`
	}
	type ResponseError struct {
		Error string `json:"error"`
	}

	return func(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
		configName := req.URL.Query().Get("config")
		if configName == "" {
			var body RequestSchema
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				data, _ := json.Marshal(ResponseError{Error: err.Error()})
				_, respErr := rw.Write(data)
				return fmt.Errorf("config_web: update, error parsing config name %v", errors.Join(err, respErr))
			}
		}

		//body := io.LimitReader(req.Body, 10<<20 /*10 MB*/) //todo(kit): uncomment
		body := req.Body
		bodyData, err := io.ReadAll(body)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			data, _ := json.Marshal(ResponseError{Error: err.Error()})
			_, respErr := rw.Write(data)
			return fmt.Errorf("config_web: update, error reading body %v", errors.Join(err, respErr))
		}

		if !json.Valid(bodyData) {
			rw.WriteHeader(http.StatusBadRequest)
			data, _ := json.Marshal(ResponseError{Error: "config sent is invalid as json"})
			_, respErr := rw.Write(data)
			return fmt.Errorf("config_web: update, error parsing body %v", errors.Join(err, respErr))
		}

		if err := cache.Update(configName, bodyData); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			data, _ := json.Marshal(ResponseError{Error: err.Error()})
			_, respErr := rw.Write(data)
			return fmt.Errorf("config_web: update, error updating config %v", errors.Join(err, respErr))
		}

		rw.WriteHeader(http.StatusOK)
		return nil
	}
}

func HandlerGet(cache *config.Cache) func(w http.ResponseWriter, r *http.Request) {
	handler := HandlerGetVerbose(cache)
	return func(w http.ResponseWriter, r *http.Request) {
		_ = handler(context.Background(), w, r)
	}
}

func HandlerUpdate(cache *config.Cache) func(w http.ResponseWriter, r *http.Request) {
	handler := HandlerUpdateVerbose(cache)
	return func(w http.ResponseWriter, r *http.Request) {
		_ = handler(context.Background(), w, r)
	}
}
