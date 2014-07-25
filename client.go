package circuitbreaker

import (
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPClient is a wrapper around http.Client that provides circuit breaker capabilities.
//
// By default, the client will use its defaultBreaker. A BreakerLookup function may be
// provided to allow different breakers to be used based on the circumstance. See the
// implementation of NewHostBasedHTTPClient for an example of this.
type HTTPClient struct {
	Client         *http.Client
	BreakerTripped func()
	BreakerReset   func()
	BreakerLookup  func(*HTTPClient, interface{}) *TimeoutBreaker
	defaultBreaker *TimeoutBreaker
	breakers       map[interface{}]*TimeoutBreaker
	breakerLock    sync.Mutex
}

// NewCircuitBreakerClient provides a circuit breaker wrapper around http.Client.
// It wraps all of the regular http.Client functions. Specifying 0 for timeout will
// give a breaker that does not check for time outs.
func NewHTTPClient(timeout time.Duration, threshold int64, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{}
	}

	breaker := NewTimeoutBreaker(timeout, threshold)
	breakers := make(map[interface{}]*TimeoutBreaker, 0)
	brclient := &HTTPClient{Client: client, defaultBreaker: breaker, breakers: breakers}
	breaker.BreakerTripped = brclient.runBreakerTripped
	breaker.BreakerReset = brclient.runBreakerReset
	return brclient
}

// NewHostBasedHTTPClient provides a circuit breaker wrapper around http.Client. This
// client will use one circuit breaker per host parsed from the request URL. This allows
// you to use a single HTTPClient for multiple hosts with one host's breaker not affecting
// the other hosts.
func NewHostBasedHTTPClient(timeout time.Duration, threshold int64, client *http.Client) *HTTPClient {
	brclient := NewHTTPClient(timeout, threshold, client)

	brclient.BreakerLookup = func(c *HTTPClient, val interface{}) *TimeoutBreaker {
		rawURL := val.(string)
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return c.defaultBreaker
		}
		host := parsedURL.Host

		c.breakerLock.Lock()
		defer c.breakerLock.Unlock()
		cb, ok := c.breakers[host]
		if !ok {
			cb = NewTimeoutBreaker(timeout, threshold)
			cb.BreakerTripped = brclient.runBreakerTripped
			cb.BreakerReset = brclient.runBreakerReset
			c.breakers[host] = cb
		}
		return cb
	}

	return brclient
}

// Do wraps http.Client Do()
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	breaker := c.breakerLookup(req.URL.String())
	breaker.Call(func() error {
		resp, err = c.Client.Do(req)
		return err
	})
	return resp, err
}

// Get wraps http.Client Get()
func (c *HTTPClient) Get(url string) (*http.Response, error) {
	var resp *http.Response
	breaker := c.breakerLookup(url)
	err := breaker.Call(func() error {
		aresp, err := c.Client.Get(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Head wraps http.Client Head()
func (c *HTTPClient) Head(url string) (*http.Response, error) {
	var resp *http.Response
	breaker := c.breakerLookup(url)
	err := breaker.Call(func() error {
		aresp, err := c.Client.Head(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Post wraps http.Client Post()
func (c *HTTPClient) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	var resp *http.Response
	breaker := c.breakerLookup(url)
	err := breaker.Call(func() error {
		aresp, err := c.Client.Post(url, bodyType, body)
		resp = aresp
		return err
	})
	return resp, err
}

// PostForm wraps http.Client PostForm()
func (c *HTTPClient) PostForm(url string, data url.Values) (*http.Response, error) {
	var resp *http.Response
	breaker := c.breakerLookup(url)
	err := breaker.Call(func() error {
		aresp, err := c.Client.PostForm(url, data)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HTTPClient) breakerLookup(val interface{}) *TimeoutBreaker {
	if c.BreakerLookup != nil {
		return c.BreakerLookup(c, val)
	}
	return c.defaultBreaker
}

func (c *HTTPClient) runBreakerTripped() {
	if c.BreakerTripped != nil {
		c.BreakerTripped()
	}
}

func (c *HTTPClient) runBreakerReset() {
	if c.BreakerReset != nil {
		c.BreakerReset()
	}
}
