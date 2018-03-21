package statsd

/*

Copyright (c) 2017 Andrey Smirnov

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

import (
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Client implements statsd client
type Client struct {
	options ClientOptions

	bufPool chan []byte
	buf     []byte
	bufSize int
	bufLock sync.Mutex

	sendQueue chan []byte

	shutdown   chan struct{}
	shutdownWg sync.WaitGroup

	lostPacketsPeriod, lostPacketsOverall int64
}

// NewClient creates new statsd client and starts background processing
//
// Client connects to statsd server at addr ("host:port")
//
// Client settings could be controlled via functions of type Option
func NewClient(addr string, options ...Option) *Client {
	c := &Client{
		options: ClientOptions{
			Addr:              addr,
			MetricPrefix:      DefaultMetricPrefix,
			MaxPacketSize:     DefaultMaxPacketSize,
			FlushInterval:     DefaultFlushInterval,
			ReconnectInterval: DefaultReconnectInterval,
			ReportInterval:    DefaultReportInterval,
			RetryTimeout:      DefaultRetryTimeout,
			Logger:            log.New(os.Stderr, DefaultLogPrefix, log.LstdFlags),
			BufPoolCapacity:   DefaultBufPoolCapacity,
			SendQueueCapacity: DefaultSendQueueCapacity,
			SendLoopCount:     DefaultSendLoopCount,
		},

		shutdown: make(chan struct{}),
	}

	// 1024 is room for overflow metric
	c.bufSize = c.options.MaxPacketSize + 1024

	for _, option := range options {
		option(&c.options)
	}

	c.buf = make([]byte, 0, c.bufSize)
	c.bufPool = make(chan []byte, c.options.BufPoolCapacity)
	c.sendQueue = make(chan []byte, c.options.SendQueueCapacity)

	go c.flushLoop()

	for i := 0; i < c.options.SendLoopCount; i++ {
		c.shutdownWg.Add(1)
		go c.sendLoop()
	}

	if c.options.ReportInterval > 0 {
		c.shutdownWg.Add(1)
		go c.reportLoop()
	}

	return c
}

// Close stops the client
func (c *Client) Close() error {
	close(c.shutdown)
	c.shutdownWg.Wait()

	return nil
}

// GetLostPackets returns number of packets lost during client lifecycle
func (c *Client) GetLostPackets() int64 {
	return atomic.LoadInt64(&c.lostPacketsOverall)
}

// Incr increments a counter metric
//
// Often used to note a particular event
func (c *Client) Incr(stat string, count int64) {
	if 0 != count {
		c.bufLock.Lock()
		lastLen := len(c.buf)

		c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
		c.buf = append(c.buf, []byte(stat)...)
		c.buf = append(c.buf, ':')
		c.buf = strconv.AppendInt(c.buf, count, 10)
		c.buf = append(c.buf, []byte("|c\n")...)

		c.checkBuf(lastLen)
		c.bufLock.Unlock()
	}
}

// Decr decrements a counter metri
//
// Often used to note a particular event
func (c *Client) Decr(stat string, count int64) {
	c.Incr(stat, -count)
}

// Timing tracks a duration event, the time delta must be given in milliseconds
func (c *Client) Timing(stat string, delta int64) {
	c.bufLock.Lock()
	lastLen := len(c.buf)

	c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
	c.buf = append(c.buf, []byte(stat)...)
	c.buf = append(c.buf, ':')
	c.buf = strconv.AppendInt(c.buf, delta, 10)
	c.buf = append(c.buf, []byte("|ms\n")...)

	c.checkBuf(lastLen)
	c.bufLock.Unlock()
}

// PrecisionTiming track a duration event, the time delta has to be a duration
func (c *Client) PrecisionTiming(stat string, delta time.Duration) {
	c.bufLock.Lock()
	lastLen := len(c.buf)

	c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
	c.buf = append(c.buf, []byte(stat)...)
	c.buf = append(c.buf, ':')
	c.buf = strconv.AppendFloat(c.buf, float64(delta)/float64(time.Millisecond), 'f', -1, 64)
	c.buf = append(c.buf, []byte("|ms\n")...)

	c.checkBuf(lastLen)
	c.bufLock.Unlock()
}

func (c *Client) igauge(stat string, sign []byte, value int64) {
	c.bufLock.Lock()
	lastLen := len(c.buf)

	c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
	c.buf = append(c.buf, []byte(stat)...)
	c.buf = append(c.buf, ':')
	c.buf = append(c.buf, sign...)
	c.buf = strconv.AppendInt(c.buf, value, 10)
	c.buf = append(c.buf, []byte("|g\n")...)

	c.checkBuf(lastLen)
	c.bufLock.Unlock()
}

// Gauge sets or updates constant value for the interval
//
// Gauges are a constant data type. They are not subject to averaging,
// and they don’t change unless you change them. That is, once you set a gauge value,
// it will be a flat line on the graph until you change it again. If you specify
// delta to be true, that specifies that the gauge should be updated, not set. Due to the
// underlying protocol, you can't explicitly set a gauge to a negative number without
// first setting it to zero.
func (c *Client) Gauge(stat string, value int64) {
	if value < 0 {
		c.igauge(stat, nil, 0)
	}

	c.igauge(stat, nil, value)
}

// GaugeDelta sends a change for a gauge
func (c *Client) GaugeDelta(stat string, value int64) {
	// Gauge Deltas are always sent with a leading '+' or '-'. The '-' takes care of itself but the '+' must added by hand
	if value < 0 {
		c.igauge(stat, nil, value)
	} else {
		c.igauge(stat, []byte{'+'}, value)
	}
}

func (c *Client) fgauge(stat string, sign []byte, value float64) {
	c.bufLock.Lock()
	lastLen := len(c.buf)

	c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
	c.buf = append(c.buf, []byte(stat)...)
	c.buf = append(c.buf, ':')
	c.buf = append(c.buf, sign...)
	c.buf = strconv.AppendFloat(c.buf, value, 'f', -1, 64)
	c.buf = append(c.buf, []byte("|g\n")...)

	c.checkBuf(lastLen)
	c.bufLock.Unlock()
}

// FGauge sends a floating point value for a gauge
func (c *Client) FGauge(stat string, value float64) {
	if value < 0 {
		c.igauge(stat, nil, 0)
	}

	c.fgauge(stat, nil, value)
}

// FGaugeDelta sends a floating point change for a gauge
func (c *Client) FGaugeDelta(stat string, value float64) {
	if value < 0 {
		c.fgauge(stat, nil, value)
	} else {
		c.fgauge(stat, []byte{'+'}, value)
	}
}

// SetAdd adds unique element to a set
func (c *Client) SetAdd(stat string, value string) {
	c.bufLock.Lock()
	lastLen := len(c.buf)

	c.buf = append(c.buf, []byte(c.options.MetricPrefix)...)
	c.buf = append(c.buf, []byte(stat)...)
	c.buf = append(c.buf, ':')
	c.buf = append(c.buf, []byte(value)...)
	c.buf = append(c.buf, []byte("|s\n")...)

	c.checkBuf(lastLen)
	c.bufLock.Unlock()
}
