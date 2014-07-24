package circuitbreaker

import (
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPClient is a wrapper around http.Client that provides circuit breaker capabilities.
type HTTPClient struct {
	Client        *http.Client
	BreakerOpen   func(error)
	BreakerClosed func()
	cb            *CircuitBreaker
}

// NewCircuitBreakerClient provides a circuit breaker wrapper around http.Client.
// It wraps all of the regular http.Client functions. Specifying 0 for timeout will
// give a breaker that does not check for time outs.
func NewCircuitBreakerClient(timeout time.Duration, threshold int64, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{}
	}

	breaker := NewTimeoutCircuitBreaker(timeout, threshold)
	brclient := &HTTPClient{Client: client, cb: breaker}
	breaker.BreakerOpen = brclient.runBreakerOpen
	breaker.BreakerClosed = brclient.runBreakerClosed
	return brclient
}

// Do wraps http.Client Do()
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	c.cb.Call(func() error {
		resp, err = c.Client.Do(req)
		return err
	})
	return resp, err
}

// Get wraps http.Client Get()
func (c *HTTPClient) Get(url string) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Get(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Head wraps http.Client Head()
func (c *HTTPClient) Head(url string) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Head(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Post wraps http.Client Post()
func (c *HTTPClient) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Post(url, bodyType, body)
		resp = aresp
		return err
	})
	return resp, err
}

// PostForm wraps http.Client PostForm()
func (c *HTTPClient) PostForm(url string, data url.Values) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.PostForm(url, data)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HTTPClient) runBreakerOpen(cb *CircuitBreaker, err error) {
	if c.BreakerOpen != nil {
		c.BreakerOpen(err)
	}
}

func (c *HTTPClient) runBreakerClosed(cb *CircuitBreaker) {
	if c.BreakerClosed != nil {
		c.BreakerClosed()
	}
}
