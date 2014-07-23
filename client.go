package circuitbreaker

import (
	"io"
	"net/http"
	"net/url"
)

type HttpClient struct {
	Client        *http.Client
	BreakerOpen   func(error)
	BreakerClosed func()
	cb            *CircuitBreaker
}

// NewCircuitBreakerClient provides a circuit breaker wrapper around http.Client.
// It wraps all of the regular http.Client functions. Specifying 0 for timeout will
// give a breaker that does not check for time outs.
func NewCircuitBreakerClient(timeout, threshold int, client *http.Client) *HttpClient {
	if client == nil {
		client = &http.Client{}
	}

	breaker := NewTimeoutCircuitBreaker(timeout, threshold)
	brclient := &HttpClient{Client: client, cb: breaker}
	breaker.BreakerOpen = brclient.runBreakerOpen
	breaker.BreakerClosed = brclient.runBreakerClosed
	return brclient
}

func (c *HttpClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	c.cb.Call(func() error {
		resp, err = c.Client.Do(req)
		return err
	})
	return resp, err
}

func (c *HttpClient) Get(url string) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Get(url)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HttpClient) Head(url string) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Head(url)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HttpClient) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.Post(url, bodyType, body)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HttpClient) PostForm(url string, data url.Values) (*http.Response, error) {
	var resp *http.Response
	err := c.cb.Call(func() error {
		aresp, err := c.Client.PostForm(url, data)
		resp = aresp
		return err
	})
	return resp, err
}

func (c *HttpClient) runBreakerOpen(cb *CircuitBreaker, err error) {
	if c.BreakerOpen != nil {
		c.BreakerOpen(err)
	}
}

func (c *HttpClient) runBreakerClosed(cb *CircuitBreaker) {
	if c.BreakerClosed != nil {
		c.BreakerClosed()
	}
}
