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
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setupListener(t *testing.T) (*net.UDPConn, chan []byte) {
	inSocket, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP: net.IPv4(127, 0, 0, 1),
	})
	if err != nil {
		t.Error(err)
	}

	received := make(chan []byte)

	go func() {
		for {
			buf := make([]byte, 1500)

			n, err := inSocket.Read(buf)
			if err != nil {
				return
			}

			received <- buf[0:n]
		}

	}()

	return inSocket, received
}

func TestCommands(t *testing.T) {
	inSocket, received := setupListener(t)

	client := NewClient(inSocket.LocalAddr().String(),
		MetricPrefix("foo."),
		MaxPacketSize(1400),
		ReconnectInterval(10*time.Second))

	compareOutput := func(actions func(), expected []string) func(*testing.T) {
		return func(t *testing.T) {
			actions()

			for _, exp := range expected {
				buf := <-received

				if string(buf) != exp {
					t.Errorf("unexpected part received: %#v != %#v", string(buf), exp)
				}
			}
		}
	}

	t.Run("Incr", compareOutput(
		func() { client.Incr("req.count", 30) },
		[]string{"foo.req.count:30|c"}))

	t.Run("Decr", compareOutput(
		func() { client.Decr("req.count", 30) },
		[]string{"foo.req.count:-30|c"}))

	t.Run("Timing", compareOutput(
		func() { client.Timing("req.duration", 100) },
		[]string{"foo.req.duration:100|ms"}))

	t.Run("PrecisionTiming", compareOutput(
		func() { client.PrecisionTiming("req.duration", 157356*time.Microsecond) },
		[]string{"foo.req.duration:157.356|ms"}))

	t.Run("Gauge", compareOutput(
		func() { client.Gauge("req.clients", 33); client.Gauge("req.clients", -533) },
		[]string{"foo.req.clients:33|g\nfoo.req.clients:0|g\nfoo.req.clients:-533|g"}))

	t.Run("GaugeDelta", compareOutput(
		func() { client.GaugeDelta("req.clients", 33); client.GaugeDelta("req.clients", -533) },
		[]string{"foo.req.clients:+33|g\nfoo.req.clients:-533|g"}))

	t.Run("FGauge", compareOutput(
		func() { client.FGauge("req.clients", 33.5); client.FGauge("req.clients", -533.3) },
		[]string{"foo.req.clients:33.5|g\nfoo.req.clients:0|g\nfoo.req.clients:-533.3|g"}))

	t.Run("FGaugeDelta", compareOutput(
		func() { client.FGaugeDelta("req.clients", 33.5); client.FGaugeDelta("req.clients", -533.3) },
		[]string{"foo.req.clients:+33.5|g\nfoo.req.clients:-533.3|g"}))

	t.Run("SetAdd", compareOutput(
		func() { client.SetAdd("req.user", "bob") },
		[]string{"foo.req.user:bob|s"}))

	t.Run("FlushedIncr", compareOutput(
		func() {
			client.Incr("req.count", 30)
			client.Incr("req.count", 20)
			time.Sleep(150 * time.Millisecond)
			client.Incr("req.count", 10)
		},
		[]string{"foo.req.count:30|c\nfoo.req.count:20|c", "foo.req.count:10|c"}))

	t.Run("SplitIncr", compareOutput(
		func() {
			for i := 0; i < 100; i++ {
				client.Incr("req.count", 30)
			}
		},
		[]string{
			"foo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c",
			"foo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c",
		}))

	_ = client.Close()
	_ = inSocket.Close()
	close(received)
}

func TestConcurrent(t *testing.T) {
	inSocket, received := setupListener(t)

	client := NewClient(inSocket.LocalAddr().String(), MetricPrefix("foo."), SendLoopCount(3))

	var totalSent, totalReceived int64

	var wg1, wg2 sync.WaitGroup

	wg1.Add(1)

	go func() {
		for buf := range received {
			for _, part := range strings.Split(string(buf), "\n") {
				i1 := strings.Index(part, ":")
				i2 := strings.Index(part, "|")

				if i1 == -1 || i2 == -1 {
					t.Logf("unaparseable part: %#v", part)
					continue
				}

				count, err := strconv.ParseInt(part[i1+1:i2], 10, 64)
				if err != nil {
					t.Log(err)
					continue
				}

				atomic.AddInt64(&totalReceived, count)
			}
		}

		wg1.Done()
	}()

	workers := 32
	count := 4096

	for i := 0; i < workers; i++ {
		wg2.Add(1)

		go func(i int) {
			for j := 0; j < count; j++ {
				// to simulate real load, sleep a bit in between the stats calls
				time.Sleep(time.Duration(rand.ExpFloat64() * float64(time.Microsecond)))

				increment := i + j
				client.Incr("some.counter", int64(increment))

				atomic.AddInt64(&totalSent, int64(increment))
			}

			wg2.Done()
		}(i)
	}

	wg2.Wait()

	if client.GetLostPackets() > 0 {
		t.Errorf("some packets were lost during the test, results are not valid: %d", client.GetLostPackets())
	}

	_ = client.Close()

	time.Sleep(time.Second)

	_ = inSocket.Close()
	close(received)

	wg1.Wait()

	if atomic.LoadInt64(&totalSent) != atomic.LoadInt64(&totalReceived) {
		t.Errorf("sent != received: %v != %v", totalSent, totalReceived)
	}
}

func BenchmarkComplexDelivery(b *testing.B) {
	inSocket, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP: net.IPv4(127, 0, 0, 1),
	})
	if err != nil {
		b.Error(err)
	}

	go func() {
		buf := make([]byte, 1500)
		for {
			_, err := inSocket.Read(buf)
			if err != nil {
				return
			}
		}

	}()

	client := NewClient(inSocket.LocalAddr().String(), MetricPrefix("foo."))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.Incr("number.requests", 33)
		client.Timing("another.value", 157)
		client.PrecisionTiming("response.time.for.some.api", 150*time.Millisecond)
		client.PrecisionTiming("response.time.for.some.api.case1", 150*time.Millisecond)
	}

	_ = client.Close()
	_ = inSocket.Close()
}
