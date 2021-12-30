// Package traefik_modsecurity_plugin a modsecurity plugin.
package traefik_modsecurity_plugin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

// Net client is a custom client to timeout after 2 seconds if the service is not ready
var httpClient = &http.Client{
	Timeout: time.Second * 2,
}

// Config the plugin configuration.
type Config struct {
	ModSecurityUrl string `json:"modSecurityUrl,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// Modsecurity a Modsecurity plugin.
type Modsecurity struct {
	next           http.Handler
	modSecurityUrl string
	name           string
}

// New created a new Modsecurity plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.ModSecurityUrl) == 0 {
		return nil, fmt.Errorf("modSecurityUrl cannot be empty")
	}

	return &Modsecurity{
		modSecurityUrl: config.ModSecurityUrl,
		next:           next,
		name:           name,
	}, nil
}

func (a *Modsecurity) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// Webscoket not supported
	if isWebsocket(req) {
		a.next.ServeHTTP(rw, req)
		return
	}

	// we need to buffer the body if we want to read it here and send it
	// in the request.
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// you can reassign the body if you need to parse it as multipart
	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	// create a new url from the raw RequestURI sent by the client
	url := fmt.Sprintf("%s%s", a.modSecurityUrl, req.RequestURI)

	proxyReq, err := http.NewRequest(req.Method, url, bytes.NewReader(body))

	// We may want to filter some headers, otherwise we could just use a shallow copy
	// proxyReq.Header = req.Header
	proxyReq.Header = make(http.Header)
	for h, val := range req.Header {
		proxyReq.Header[h] = val
	}

	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// copy headers
		for k, vv := range resp.Header {
			for _, v := range vv {
				rw.Header().Set(k, v)
			}
		}
		// copy status
		rw.WriteHeader(resp.StatusCode)
		// copy body
		io.Copy(rw, resp.Body)
		return
	}

	a.next.ServeHTTP(rw, req)
}

func isWebsocket(req *http.Request) bool {
	for _, header := range req.Header["Upgrade"] {
		if header == "websocket" {
			return true
		}
	}
	return false
}
