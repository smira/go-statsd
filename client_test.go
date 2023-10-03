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

	received := make(chan []byte, 1024)

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

func TestWrongAddress(t *testing.T) {
	client := NewClient("BOOM:BOOM")
	if err := client.Close(); err != nil {
		t.Errorf("error from close: %v", err)
	}
}

func TestCommands(t *testing.T) {
	inSocket, received := setupListener(t)

	client := NewClient(inSocket.LocalAddr().String(),
		MetricPrefix("foo."),
		MaxPacketSize(1400),
		ReconnectInterval(10*time.Second))
	clientTagged := NewClient(inSocket.LocalAddr().String(),
		TagStyle(TagFormatDatadog),
		DefaultTags(StringTag("host", "example.com"), Int64Tag("weight", 38)))

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

	t.Run("IncrTaggedInflux", compareOutput(
		func() { client.Incr("req.count", 30, StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"foo.req.count,app=service,port=80:30|c"}))

	t.Run("IncrTaggedDatadog", compareOutput(
		func() { clientTagged.Incr("req.count", 30, StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"req.count:30|c|#host:example.com,weight:38,app:service,port:80"}))

	t.Run("Decr", compareOutput(
		func() { client.Decr("req.count", 30) },
		[]string{"foo.req.count:-30|c"}))

	t.Run("FIncr", compareOutput(
		func() { client.FIncr("req.count", 0.3) },
		[]string{"foo.req.count:0.3|c"}))

	t.Run("FDecr", compareOutput(
		func() { client.FDecr("req.count", 0.3) },
		[]string{"foo.req.count:-0.3|c"}))

	t.Run("Timing", compareOutput(
		func() { client.Timing("req.duration", 100) },
		[]string{"foo.req.duration:100|ms"}))

	t.Run("TimingTaggedInflux", compareOutput(
		func() { client.Timing("req.duration", 100, StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"foo.req.duration,app=service,port=80:100|ms"}))

	t.Run("TimingTaggedDatadog", compareOutput(
		func() { clientTagged.Timing("req.duration", 100, StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"req.duration:100|ms|#host:example.com,weight:38,app:service,port:80"}))

	t.Run("PrecisionTiming", compareOutput(
		func() { client.PrecisionTiming("req.duration", 157356*time.Microsecond) },
		[]string{"foo.req.duration:157.356|ms"}))

	t.Run("PrecisionTimingTaggedInflux", compareOutput(
		func() {
			client.PrecisionTiming("req.duration", 157356*time.Microsecond, StringTag("app", "service"), IntTag("port", 80))
		},
		[]string{"foo.req.duration,app=service,port=80:157.356|ms"}))

	t.Run("PrecisionTimingTaggedDatadog", compareOutput(
		func() {
			clientTagged.PrecisionTiming("req.duration", 157356*time.Microsecond, StringTag("app", "service"), IntTag("port", 80))
		},
		[]string{"req.duration:157.356|ms|#host:example.com,weight:38,app:service,port:80"}))

	t.Run("Gauge", compareOutput(
		func() { client.Gauge("req.clients", 33); client.Gauge("req.clients", -533) },
		[]string{"foo.req.clients:33|g\nfoo.req.clients:0|g\nfoo.req.clients:-533|g"}))

	t.Run("GaugeTaggedInflux", compareOutput(
		func() {
			client.Gauge("req.clients", 33, StringTag("app", "service"), IntTag("port", 80))
			client.Gauge("req.clients", -533, StringTag("app", "service"), IntTag("port", 80))
		},
		[]string{"foo.req.clients,app=service,port=80:33|g\nfoo.req.clients,app=service,port=80:0|g\nfoo.req.clients,app=service,port=80:-533|g"}))

	t.Run("GaugeDelta", compareOutput(
		func() { client.GaugeDelta("req.clients", 33); client.GaugeDelta("req.clients", -533) },
		[]string{"foo.req.clients:+33|g\nfoo.req.clients:-533|g"}))

	t.Run("GaugeDeltaTaggedDatadog", compareOutput(
		func() { clientTagged.GaugeDelta("req.clients", 33); clientTagged.GaugeDelta("req.clients", -533) },
		[]string{"req.clients:+33|g|#host:example.com,weight:38\nreq.clients:-533|g|#host:example.com,weight:38"}))

	t.Run("FGauge", compareOutput(
		func() { client.FGauge("req.clients", 33.5); client.FGauge("req.clients", -533.3) },
		[]string{"foo.req.clients:33.5|g\nfoo.req.clients:0|g\nfoo.req.clients:-533.3|g"}))

	t.Run("FGaugeTaggedInflux", compareOutput(
		func() {
			client.FGauge("req.clients", 33.5, StringTag("app", "service"), IntTag("port", 80))
			client.FGauge("req.clients", -533.3, StringTag("app", "service"), IntTag("port", 80))
		},
		[]string{"foo.req.clients,app=service,port=80:33.5|g\nfoo.req.clients,app=service,port=80:0|g\nfoo.req.clients,app=service,port=80:-533.3|g"}))

	t.Run("FGaugeDelta", compareOutput(
		func() { client.FGaugeDelta("req.clients", 33.5); client.FGaugeDelta("req.clients", -533.3) },
		[]string{"foo.req.clients:+33.5|g\nfoo.req.clients:-533.3|g"}))

	t.Run("FGaugeDeltaTaggedDatadog", compareOutput(
		func() { clientTagged.FGaugeDelta("req.clients", 33.5); clientTagged.FGaugeDelta("req.clients", -533.3) },
		[]string{"req.clients:+33.5|g|#host:example.com,weight:38\nreq.clients:-533.3|g|#host:example.com,weight:38"}))

	t.Run("SetAdd", compareOutput(
		func() { client.SetAdd("req.user", "bob") },
		[]string{"foo.req.user:bob|s"}))

	t.Run("SetAddTaggedInflux", compareOutput(
		func() { client.SetAdd("req.user", "bob", StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"foo.req.user,app=service,port=80:bob|s"}))

	t.Run("SetAddTaggedDatadog", compareOutput(
		func() { clientTagged.SetAdd("req.user", "bob", StringTag("app", "service"), IntTag("port", 80)) },
		[]string{"req.user:bob|s|#host:example.com,weight:38,app:service,port:80"}))

	t.Run("FlushedIncr", compareOutput(
		func() {
			client.Incr("req.count", 40)
			client.Incr("req.count", 20)
			time.Sleep(150 * time.Millisecond)
			client.Incr("req.count", 10)
		},
		[]string{"foo.req.count:40|c\nfoo.req.count:20|c", "foo.req.count:10|c"}))

	t.Run("SplitIncr", compareOutput(
		func() {
			for i := 0; i < 100; i++ {
				client.Incr("req.count", 30)
			}
		},
		[]string{
			"foo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c",
			"foo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c\nfoo.req.count:30|c",
		}))

	_ = client.Close()
	_ = inSocket.Close()
	close(received)
}

func TestClones(t *testing.T) {
	inSocket, received := setupListener(t)

	client := NewClient(inSocket.LocalAddr().String(),
		MetricPrefix("foo."),
		MaxPacketSize(1400),
		ReconnectInterval(10*time.Second))
	client2 := client.CloneWithPrefix("bar.")
	client3 := client2.CloneWithPrefixExtension("blah.")

	compareOutput := func(actions func(), expected []string) func(*testing.T) {
		return func(t *testing.T) {
			actions()

			for _, exp := range expected {
				var buf []byte
				select {
				case buf = <-received:
				case <-time.After(time.Second):
					t.Errorf("timeout waiting for %v", exp)
					return
				}

				if string(buf) != exp {
					t.Errorf("unexpected part received: %#v != %#v", string(buf), exp)
				}
			}
		}
	}

	t.Run("Original", compareOutput(
		func() { client.Incr("req.count", 30) },
		[]string{"foo.req.count:30|c"}))

	t.Run("CloneWithPrefix", compareOutput(
		func() { client2.Incr("req.count", 30) },
		[]string{"bar.req.count:30|c"}))

	t.Run("CloneWithPrefixExtension", compareOutput(
		func() { client3.Incr("req.count", 30) },
		[]string{"bar.blah.req.count:30|c"}))

	_ = client.Close()
	_ = client2.Close()
	_ = client3.Close()
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
					t.Logf("non-parsable part: %#v", part)
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

	workers := 16
	count := 1024

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

	// wait for 30 seconds for all the packets to be received
	for i := 0; i < 30; i++ {
		if atomic.LoadInt64(&totalSent) == atomic.LoadInt64(&totalReceived) {
			break
		}

		time.Sleep(time.Second)
	}

	_ = inSocket.Close()
	close(received)

	wg1.Wait()

	if atomic.LoadInt64(&totalSent) != atomic.LoadInt64(&totalReceived) {
		t.Errorf("sent != received: %v != %v", totalSent, totalReceived)
	}
}

func BenchmarkSimple(b *testing.B) {
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

	c := NewClient(inSocket.LocalAddr().String(), MetricPrefix("metricPrefix"), MaxPacketSize(1432),
		FlushInterval(100*time.Millisecond), SendLoopCount(2))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Incr("foo.bar.counter", 1)
		c.Gauge("foo.bar.gauge", 42)
		c.PrecisionTiming("foo.bar.timing", 153*time.Millisecond)
	}
	_ = c.Close()
	_ = inSocket.Close()
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

func BenchmarkTagged(b *testing.B) {
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

	client := NewClient(inSocket.LocalAddr().String(), MetricPrefix("metricPrefix"), MaxPacketSize(1432),
		FlushInterval(100*time.Millisecond), SendLoopCount(2), DefaultTags(StringTag("host", "foo")),
		SendQueueCapacity(10), BufPoolCapacity(40))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.Incr("foo.bar.counter", 1, StringTag("route", "api.one"), IntTag("status", 200))
		client.Timing("another.value", 157, StringTag("service", "db"))
		client.PrecisionTiming("response.time.for.some.api", 150*time.Millisecond, IntTag("status", 404))
		client.PrecisionTiming("response.time.for.some.api.case1", 150*time.Millisecond, StringTag("service", "db"), IntTag("status", 200))
	}
	_ = client.Close()
	_ = inSocket.Close()
}
