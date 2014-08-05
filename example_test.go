package circuitbreaker

import (
	"fmt"
	"io/ioutil"
	"log"
	"time"
)

func ExampleThresholdBreaker() {
	// This example sets up a ThresholdBreaker that will trip if remoteCall returns
	// an error 10 times in a row. The error returned by Call() will be the error
	// returned by remoteCall, unless the breaker has been tripped, in which case
	// it will return ErrBreakerOpen.
	breaker := NewThresholdBreaker(10)
	err := breaker.Call(remoteCall)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleThresholdBreaker_manual() {
	// This example demonstrates the manual use of a ThresholdBreaker. The breaker
	// will trip when Fail is called 10 times in a row.
	breaker := NewThresholdBreaker(10)
	if breaker.Ready() {
		err := remoteCall
		if err != nil {
			breaker.Fail()
			log.Fatal(err)
		} else {
			breaker.Reset()
		}
	}
}

func ExampleTimeoutBreaker() {
	// This example sets up a TimeoutBreaker that will trip if remoteCall returns
	// an error OR takes longer than one second 10 times in a row. The error returned
	// by Call() will be the error returned by remoteCall with two exceptions: if
	// remoteCall takes longer than one second the return value will be ErrBreakerTimeout,
	// if the breaker has been tripped the return value will be ErrBreakerOpen.
	breaker := NewTimeoutBreaker(time.Second, 10)
	err := breaker.Call(remoteCall)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleFrequencyBreaker() {
	// This example sets up a FrequencyBreaker that will trip if remoteCall returns
	// an error 10 times in a row within a period of 2 minutes.
	breaker := NewFrequencyBreaker(time.Minute*2, 10)
	err := breaker.Call(remoteCall)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleHTTPClient() {
	// This example sets up an HTTP client wrapped in a TimeoutBreaker. The breaker
	// will trip with the same behavior as TimeoutBreaker.
	client := NewHTTPClient(time.Second*5, 10, nil)

	resp, err := client.Get("http://example.com/resource.json")
	if err != nil {
		log.Fatal(err)
	}
	resource, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", resource)
}

func ExampleCircuitBreaker_events() {
	// This example demonstrates the BreakerTripped and BreakerReset callbacks. These are
	// available on all breaker types.
	breaker := NewThresholdBreaker(1)
	events := breaker.Subscribe()

	go func() {
		for {
			e := <-events
			switch e {
			case BreakerTripped:
				log.Println("breaker tripped")
			case BreakerReset:
				log.Println("breaker reset")
			case BreakerFail:
				log.Println("breaker fail")
			case BreakerReady:
				log.Println("breaker ready")
			}
		}
	}()

	breaker.Fail()
	breaker.Reset()
}

func remoteCall() error {
	// Expensive remote call
	return nil
}
