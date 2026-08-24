package main

import (
	"errors"
	"fmt"
	"net"
	"time"
)

const probeTimeout = time.Second

var probeTargets4 = []string{"8.8.8.8:80", "223.5.5.5:53", "114.114.114.114:53"}

var probeTargets6 = []string{"[2400:3200::1]:53", "[2606:4700:4700::1111]:53", "[240c::6666]:53"}

func familyOf(network string) string {
	if network == "tcp6" {
		return "IPv6"
	}
	return "IPv4"
}

func targetsFor(network string) []string {
	if network == "tcp6" {
		return probeTargets6
	}
	return probeTargets4
}

func firstDial(network string, targets []string) net.Conn {
	for _, target := range targets {
		conn, err := net.DialTimeout(network, target, probeTimeout)
		if err == nil {
			return conn
		}
	}
	return nil
}

func networkAvailable(network string) bool {
	conn := firstDial(network, targetsFor(network))
	if conn == nil {
		return false
	}
	conn.Close()
	return true
}

func localIP(network string) (net.IP, string, error) {
	proto := "udp4"
	if network == "tcp6" {
		proto = "udp6"
	}
	errNoLocal := errors.New("无法探测本机 " + familyOf(network) + " 地址，请检查网络连接")
	conn := firstDial(proto, targetsFor(network))
	if conn == nil {
		return nil, "", errNoLocal
	}
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	conn.Close()
	if !ok || addr.IP == nil {
		return nil, "", errNoLocal
	}
	ip := addr.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		return ip, "", fmt.Errorf("已获取本机 IP %s，但查询网卡名失败: %v", ip, err)
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var cand net.IP
			switch v := a.(type) {
			case *net.IPNet:
				cand = v.IP
			case *net.IPAddr:
				cand = v.IP
			}
			if cand != nil && cand.Equal(ip) {
				return ip, ifc.Name, nil
			}
		}
	}
	return ip, "", nil
}
