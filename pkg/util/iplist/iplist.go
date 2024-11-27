package iplist

import (
	"bytes"
	"fmt"
	"net"

	"github.com/dgate-io/dgate-api/pkg/util/linkedlist"
)

type IPList struct {
	v4s *linkedlist.LinkedList[*net.IPNet]
	v6s *linkedlist.LinkedList[*net.IPNet]
}

func NewIPList() *IPList {
	return &IPList{
		v4s: linkedlist.New[*net.IPNet](),
		v6s: linkedlist.New[*net.IPNet](),
	}
}

func (l *IPList) AddCIDRString(cidr string) error {
	_, ipn, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	if ipn.IP.To4() != nil {
		l.insertIPv4(ipn)
		return nil
	} else if ipn.IP.To16() != nil {
		l.insertIPv6(ipn)
		return nil
	}
	return fmt.Errorf("invalid ip address: %s", cidr)
}

func (l *IPList) AddIPString(ipstr string) error {
	ip := net.ParseIP(ipstr)
	if ip == nil {
		return fmt.Errorf("invalid ip address: %s", ipstr)
	}
	maskBits := 32
	if ip.To4() != nil {
		mask := net.CIDRMask(maskBits, maskBits)
		ipn := &net.IPNet{IP: ip, Mask: mask}
		l.insertIPv4(ipn)
		return nil
	} else if ip.To16() != nil {
		maskBits := 128
		mask := net.CIDRMask(maskBits, maskBits)
		ipn := &net.IPNet{IP: ip, Mask: mask}
		l.insertIPv6(ipn)
		return nil
	}
	return fmt.Errorf("invalid ip address: %s", ipstr)
}

func (l *IPList) Len() int {
	return l.v4s.Len() + l.v6s.Len()
}

func (l *IPList) Contains(ipstr string) (bool, error) {
	if l.Len() == 0 {
		return false, nil
	}
	// parse ip
	ip := net.ParseIP(ipstr)
	if ip == nil {
		return false, fmt.Errorf("invalid ip address: %s", ipstr)
	}
	if ip.To4() != nil {
		return l.containsIPv4(ip), nil
	} else if ip.To16() != nil {
		return l.containsIPv6(ip), nil
	}
	return false, nil
}

func (l *IPList) containsIPv4(ip net.IP) bool {
	for n := l.v4s.Head; n != nil; n = n.Next {
		ipn := n.Value
		if ipn.Contains(ip) {
			return true
		}
	}
	return false
}

func (l *IPList) containsIPv6(ip net.IP) bool {
	for n := l.v6s.Head; n != nil; n = n.Next {
		ipn := n.Value
		if ipn.Contains(ip) {
			return true
		}
	}
	return false
}

func (l *IPList) insertIPv6(ipmask *net.IPNet) {
	if l.v6s.Len() == 0 {
		l.v6s.Insert(ipmask)
		return
	}
	for n := l.v6s.Head; n.Next != nil; n = n.Next {
		ipn := n.Value
		if compareIPs(ipmask.IP, ipn.IP) < 0 {
			l.v6s.InsertBefore(n, ipmask)
			return
		}
	}
	l.v6s.Insert(ipmask)
}

func (l *IPList) insertIPv4(ipmask *net.IPNet) {
	if l.v4s.Len() == 0 {
		l.v4s.Insert(ipmask)
		return
	}
	for n := l.v4s.Head; n.Next != nil; n = n.Next {
		ipn := n.Value
		if compareIPs(ipmask.IP, ipn.IP) < 0 {
			l.v4s.InsertBefore(n, ipmask)
			return
		}
	}
	l.v4s.Insert(ipmask)
}

func (l *IPList) String() string {
	var buf bytes.Buffer
	buf.WriteString("IPv4: ")
	buf.WriteString(l.v4s.String())
	buf.WriteString("\n")
	buf.WriteString("IPv6: ")
	buf.WriteString(l.v6s.String())
	buf.WriteString("\n")
	return buf.String()
}

func compareIPs(a, b net.IP) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return bytes.Compare(a, b)
}
