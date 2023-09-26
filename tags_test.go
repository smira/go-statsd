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

import "testing"

func TestTags(t *testing.T) {
	compare := func(tag Tag, style *TagFormat, expected string) func(*testing.T) {
		return func(t *testing.T) {
			buf := tag.Append([]byte{}, style)

			if string(buf) != expected {
				t.Errorf("unexpected tag format: %#v != %#v", string(buf), expected)
			}
		}
	}

	t.Run("StringDatadog",
		compare(StringTag("name", "value"), TagFormatDatadog, "name:value"))
	t.Run("StringInflux",
		compare(StringTag("name", "value"), TagFormatInfluxDB, "name=value"))
	t.Run("StringGraphite",
		compare(StringTag("name", "value"), TagFormatInfluxDB, "name=value"))
	t.Run("IntDatadog",
		compare(IntTag("foo", -33), TagFormatDatadog, "foo:-33"))
	t.Run("IntInflux",
		compare(IntTag("foo", -33), TagFormatInfluxDB, "foo=-33"))
	t.Run("IntGraphite",
		compare(IntTag("foo", -33), TagFormatInfluxDB, "foo=-33"))
	t.Run("Int64Datadog",
		compare(Int64Tag("foo", 1024*1024*1024*1024), TagFormatDatadog, "foo:1099511627776"))
	t.Run("Int64Influx",
		compare(Int64Tag("foo", 1024*1024*1024*1024), TagFormatInfluxDB, "foo=1099511627776"))
	t.Run("Int64Graphite",
		compare(Int64Tag("foo", 1024*1024*1024*1024), TagFormatInfluxDB, "foo=1099511627776"))
}

func TestFormatTags(t *testing.T) {
	compare := func(tags []Tag, style *TagFormat, expected string) func(*testing.T) {
		return func(t *testing.T) {
			client := NewClient("127.0.0.1:4444", TagStyle(style), DefaultTags(StringTag("host", "foo")))
			buf := client.formatTags([]byte{}, tags)

			if string(buf) != expected {
				t.Errorf("unexpected tag format: %#v != %#v", string(buf), expected)
			}
		}
	}

	t.Run("Datadog",
		compare([]Tag{StringTag("type", "web"), IntTag("status", 200)}, TagFormatDatadog, "|#host:foo,type:web,status:200"))
	t.Run("Influx",
		compare([]Tag{StringTag("type", "web"), IntTag("status", 200)}, TagFormatInfluxDB, ",host=foo,type=web,status=200"))
	t.Run("Graphite",
		compare([]Tag{StringTag("type", "web"), IntTag("status", 200)}, TagFormatGraphite, ";host=foo;type=web;status=200"))
	t.Run("Okmeter",
		compare([]Tag{StringTag("type", "web"), IntTag("status", 200)}, TagFormatOkmeter, ".host_is_foo.type_is_web.status_is_200"))
}
