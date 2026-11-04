package router

import (
	"net"
	"net/http"
)

// limitKey identifies who a request counts against. An IPv6 client controls a whole /64, so that's what counts as one visitor.
func limitKey(r *http.Request) string { return limitKeyFor(clientIP(r)) }

func limitKeyFor(host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String()
}
