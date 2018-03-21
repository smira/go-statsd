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
	"net"
	"sync/atomic"
	"time"
)

// flushLoop makes sure metrics are flushed every flushInterval
func (c *Client) flushLoop() {
	var flushC <-chan time.Time

	if c.options.FlushInterval > 0 {
		flushTicker := time.NewTicker(c.options.FlushInterval)
		defer flushTicker.Stop()
		flushC = flushTicker.C
	}

	for {
		select {
		case <-c.shutdown:
			c.bufLock.Lock()
			if len(c.buf) > 0 {
				c.flushBuf(len(c.buf))
			}
			c.bufLock.Unlock()

			close(c.sendQueue)
			return
		case <-flushC:
			c.bufLock.Lock()
			if len(c.buf) > 0 {
				c.flushBuf(len(c.buf))
			}
			c.bufLock.Unlock()
		}
	}
}

// sendLoop handles packet delivery over UDP and periodic reconnects
func (c *Client) sendLoop() {
	var (
		sock       net.Conn
		err        error
		reconnectC <-chan time.Time
	)

	if c.options.ReconnectInterval > 0 {
		reconnectTicker := time.NewTicker(c.options.ReconnectInterval)
		defer reconnectTicker.Stop()
		reconnectC = reconnectTicker.C
	}

RECONNECT:
	// Attempt to connect
	sock, err = net.Dial("udp", c.options.Addr)
	if err != nil {
		c.options.Logger.Printf("[STATSD] Error connecting to server: %s", err)
		goto WAIT
	}

	for {
		select {
		case buf, ok := <-c.sendQueue:
			// Get a buffer from the queue
			if !ok {
				_ = sock.Close()
				c.shutdownWg.Done()
				return
			}

			if len(buf) > 0 {
				// cut off \n in the end
				_, err := sock.Write(buf[0 : len(buf)-1])
				if err != nil {
					c.options.Logger.Printf("[STATSD] Error writing to socket: %s", err)
					_ = sock.Close()
					goto WAIT
				}
			}

			// return buffer to the pool
			select {
			case c.bufPool <- buf:
			default:
				// pool is full, let GC handle the buf
			}
		case <-reconnectC:
			_ = sock.Close()
			goto RECONNECT
		}
	}

WAIT:
	// Wait for a while
	time.Sleep(c.options.RetryTimeout)
	goto RECONNECT
}

// reportLoop reports periodically number of packets lost
func (c *Client) reportLoop() {
	defer c.shutdownWg.Done()

	reportTicker := time.NewTicker(c.options.ReportInterval)
	defer reportTicker.Stop()

	for {
		select {
		case <-c.shutdown:
			return
		case <-reportTicker.C:
			lostPeriod := atomic.SwapInt64(&c.lostPacketsPeriod, 0)
			if lostPeriod > 0 {
				c.options.Logger.Printf("[STATSD] %d packets lost (overflow)", lostPeriod)
			}
		}
	}
}
