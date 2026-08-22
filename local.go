package main

import (
	"errors"
	"net"
	"time"
)

var probeTargets = []string{
	"8.8.8.8:80", "[2001:4860:4860::8888]:80",
	"223.5.5.5:53", "[2606:4700:4700::1111]:53",
	"114.114.114.114:53", "[240c::6666]:53",
}

func localIPs() (net.IP, net.IP, error) {
	var v4, v6 net.IP
	for _, target := range probeTargets {
		conn, err := net.DialTimeout("udp", target, time.Second)
		if err != nil {
			continue
		}
		addr, ok := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if !ok || addr.IP == nil {
			continue
		}
		if addr.IP.To4() != nil {
			if v4 == nil {
				v4 = addr.IP
			}
		} else if v6 == nil {
			v6 = addr.IP
		}
		if v4 != nil && v6 != nil {
			break
		}
	}
	if v4 == nil && v6 == nil {
		return nil, nil, errors.New("无法探测本机 IP，请检查网络连接")
	}
	return v4, v6, nil
}

func ifaceNameFor(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPNet:
				if v.IP.Equal(ip) {
					return ifc.Name
				}
			case *net.IPAddr:
				if v.IP.Equal(ip) {
					return ifc.Name
				}
			}
		}
	}
	return ""
}
