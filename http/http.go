package common

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	defaultRetries = 3
	defaultBackoff = 2
	defaultTimeout = time.Second * 30
)

type TimeSleep struct{}

func (*TimeSleep) Sleep(d time.Duration) { time.Sleep(d) }

type Sleeper interface {
	Sleep(d time.Duration)
}

type sleep struct {
	Sleeper
}

func (s *sleep) Sleep(d time.Duration) {
	if s.Sleeper == nil {
		time.Sleep(d)
	} else {
		s.Sleeper.Sleep(d)
	}
}

type HTTPRetry struct {
	Retries    int
	Backoff    int
	Timeout    time.Duration
	HTTPClient *http.Client
	sleep      sleep
}

func NewHTTPRetry(options ...func(*HTTPRetry)) *HTTPRetry {
	instance := &HTTPRetry{
		HTTPClient: &http.Client{
			Timeout: defaultTimeout,
		},
		Retries: defaultRetries,
		Backoff: defaultBackoff,
	}
	for _, o := range options {
		o(instance)
	}
	return instance
}

func WithRetries(retries int) func(httpRetry *HTTPRetry) {
	return func(h *HTTPRetry) {
		h.Retries = retries
	}
}

func WithBackoff(backoff int) func(httpRetry *HTTPRetry) {
	return func(h *HTTPRetry) {
		h.Backoff = backoff
	}
}

func WithTimeout(timeout time.Duration) func(httpRetry *HTTPRetry) {
	return func(h *HTTPRetry) {
		h.HTTPClient = &http.Client{
			Timeout: timeout,
		}
	}
}

func (h *HTTPRetry) Do(req *http.Request) (*http.Response, error) {
	var bod []byte
	if req.Body != nil {
		var err error
		bod, err = io.ReadAll(req.Body)
		if err != nil {
			log.Warn().Fields(map[string]interface{}{"req": req}).Msg("Unable to read body from request")
		} else {
			req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(bod))
		}
	}

	for currentTries := 0; currentTries < h.Retries; currentTries++ {
		log.Trace().Fields(map[string]interface{}{"Current tries": currentTries, "URL": req.URL.String()}).Msg("Http request")

		resp, err := h.HTTPClient.Do(req)
		if err != nil || resp.StatusCode >= 500 {
			log.Warn().Fields(map[string]interface{}{"err": err, "retryCount": currentTries, "responseStatusCode": resp.StatusCode, "responseStatus": resp.Status}).Msg("Http Request Error")
			if len(bod) > 0 {
				req.Body = io.NopCloser(bytes.NewReader(bod))
			}
			h.sleep.Sleep(time.Duration(currentTries*h.Backoff) * time.Second)
			continue
		}

		return resp, nil
	}
	dat, err := httputil.DumpRequest(req, true)
	if err != nil {
		log.Info().Fields(map[string]interface{}{"req": string(dat)}).Msg("Max retry limit for request")
	} else {
		log.Info().Fields(map[string]interface{}{"req": req}).Msg("Max retry limit for request. Also failed to print the request")
	}
	return nil, errors.New("http request failed")
}
