package iplist_test

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/iplist"
	"github.com/stretchr/testify/assert"
)

func TestIPList_IPv4_IPv6_CIDR(t *testing.T) {
	ipl := iplist.NewIPList()
	assert.Nil(t, ipl.AddCIDRString("192.168.0.0/16"))
	assert.Nil(t, ipl.AddCIDRString("192.168.0.0/24"))
	assert.Nil(t, ipl.AddCIDRString("10.0.0.0/8"))

	assert.Nil(t, ipl.AddCIDRString("e0d4::/64"))
	assert.Nil(t, ipl.AddCIDRString("e0d3::/128"))

	assert.Equal(t, 5, ipl.Len())

	assert.Nil(t, ipl.AddIPString("255.255.255.255"))
	assert.Nil(t, ipl.AddIPString("0.0.0.0"))
	assert.Nil(t, ipl.AddIPString("e0d5::d1ee:0c22"))
	assert.Nil(t, ipl.AddIPString("e0d5::0c22"))
	assert.Nil(t, ipl.AddIPString("::0c22"))
	assert.Nil(t, ipl.AddIPString("::1"))
	assert.Nil(t, ipl.AddIPString("::"))

	assert.Equal(t, 12, ipl.Len())

	t.Log(ipl.String())

	ipTests := map[string]bool{
		"192.168.0.0":               true,
		"192.168.255.255":           true,
		"10.0.0.0":                  true,
		"10.255.255.255":            true,
		"255.255.255.255":           true,
		"0.0.0.0":                   true,
		"e0d4::":                    true,
		"e0d4::ffff:ffff:ffff:ffff": true,
		"e0d3::":                    true,
		"e0d5::d1ee:0c22":           true,
		"e0d5::0c22":                true,
		"::0c22":                    true,
		"::1":                       true,
		"::":                        true,

		"11.0.0.0":         false,
		"9.255.255.255":    false,
		"255.255.255.254":  false,
		"0.0.0.1":          false,
		"::2":              false,
		"e0d3::1":          false,
		"::612f:efe5:ed85": false,
		"::c341:0997":      false,
		"::0997":           false,
	}
	for ip, exp := range ipTests {
		contains, err := ipl.Contains(ip)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(ip, func(t *testing.T) {
			assert.True(
				t, contains == exp,
				"expected validation for %s to be %v", ip, exp,
			)
		})
	}
}

func BenchmarkIPList(b *testing.B) {
	ipl := iplist.NewIPList()
	err := ipl.AddCIDRString("::/64")
	if err != nil {
		b.Fatal(err)
	}
	err = ipl.AddCIDRString("0.0.0.0/16")
	if err != nil {
		b.Fatal(err)
	}
	err = ipl.AddIPString("127.0.0.1")
	if err != nil {
		b.Fatal(err)
	}
	err = ipl.AddIPString("f60b::1")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Contains", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ipl.Contains("255.255.255.255")
		}
	})
	b.Run("AddIPString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// interger to ip
			ip := make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, uint32(i))

			assert.Nil(b, ipl.AddIPString(ip.String()))
		}
	})
}
