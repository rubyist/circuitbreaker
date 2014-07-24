package circuitbreaker

import (
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPClient is a wrapper around http.Client that provides circuit breaker capabilities.
type HTTPClient struct {
	Timeout        time.Duration
	Threshold      int64
	Client         *http.Client
	BreakerOpen    func(error)
	BreakerClosed  func()
	defaultBreaker *CircuitBreaker
	breakers       map[string]*CircuitBreaker
	breakerLock    sync.Mutex
	hostBased      bool
}

// NewCircuitBreakerClient provides a circuit breaker wrapper around http.Client.
// It wraps all of the regular http.Client functions. Specifying 0 for timeout will
// give a breaker that does not check for time outs.
func NewCircuitBreakerClient(timeout time.Duration, threshold int64, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{}
	}

	breaker := NewTimeoutCircuitBreaker(timeout, threshold)
	brclient := &HTTPClient{
		Timeout:        timeout,
		Threshold:      threshold,
		Client:         client,
		defaultBreaker: breaker,
		hostBased:      false}
	breaker.BreakerOpen = brclient.runBreakerOpen
	breaker.BreakerClosed = brclient.runBreakerClosed
	return brclient
}

func NewHostBasedCircuitBreakerClient(timeout time.Duration, threshold int64, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{}
	}

	return &HTTPClient{
		Timeout:   timeout,
		Threshold: threshold,
		Client:    client,
		breakers:  make(map[string]*CircuitBreaker, 0),
		hostBased: true}
}

// Do wraps http.Client Do()
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	cb, err := c.lookupCircuitBreaker(req.URL.String())
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	cb.Call(func() error {
		resp, err = c.Client.Do(req)
		return err
	})
	return resp, err
}

// Get wraps http.Client Get()
func (c *HTTPClient) Get(url string) (*http.Response, error) {
	cb, err := c.lookupCircuitBreaker(url)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	err = cb.Call(func() error {
		aresp, err := c.Client.Get(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Head wraps http.Client Head()
func (c *HTTPClient) Head(url string) (*http.Response, error) {
	cb, err := c.lookupCircuitBreaker(url)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	err = cb.Call(func() error {
		aresp, err := c.Client.Head(url)
		resp = aresp
		return err
	})
	return resp, err
}

// Post wraps http.Client Post()
func (c *HTTPClient) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	cb, err := c.lookupCircuitBreaker(url)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	err = cb.Call(func() error {
		aresp, err := c.Client.Post(url, bodyType, body)
		resp = aresp
		return err
	})
	return resp, err
}

// PostForm wraps http.Client PostForm()
func (c *HTTPClient) PostForm(url string, data url.Values) (*http.Response, error) {
	cb, err := c.lookupCircuitBreaker(url)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	err = cb.Call(func() error {
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

func (c *HTTPClient) lookupCircuitBreaker(rawURL string) (*CircuitBreaker, error) {
	if !c.hostBased {
		return c.defaultBreaker, nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	host := parsedURL.Host

	c.breakerLock.Lock()
	defer c.breakerLock.Unlock()
	cb, ok := c.breakers[host]
	if !ok {
		cb = NewTimeoutCircuitBreaker(c.Timeout, c.Threshold)
		cb.BreakerOpen = c.runBreakerOpen
		cb.BreakerClosed = c.runBreakerClosed
		c.breakers[host] = cb
	}
	return cb, nil
}
