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

import "sync/atomic"

// checkBuf checks current buffer for overflow, and flushes buffer up to lastLen bytes on overflow
//
// overflow part is preserved in flushBuf
func (c *Client) checkBuf(lastLen int) {
	if len(c.buf) > c.options.MaxPacketSize {
		c.flushBuf(lastLen)
	}
}

// flushBuf sends buffer to the queue and initializes new buffer
func (c *Client) flushBuf(length int) {
	sendBuf := c.buf[0:length]
	tail := c.buf[length:len(c.buf)]

	// get new buffer
	select {
	case c.buf = <-c.bufPool:
		c.buf = c.buf[0:0]
	default:
		c.buf = make([]byte, 0, c.bufSize)
	}

	// copy tail to the new buffer
	c.buf = append(c.buf, tail...)

	// flush current buffer
	select {
	case c.sendQueue <- sendBuf:
	default:
		// flush failed, we lost some data
		atomic.AddInt64(&c.lostPacketsPeriod, 1)
		atomic.AddInt64(&c.lostPacketsOverall, 1)
	}

}
