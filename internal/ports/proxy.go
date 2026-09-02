package ports

import (
	"net"
	"strings"
)

// ParseDockerProxy extracts the forwarding target from a docker-proxy command
// line such as:
//
//	/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080 -container-ip 172.17.0.2 -container-port 80
//
// It returns "172.17.0.2:80" and true, or "" and false when the command line is
// not a docker-proxy invocation.
func ParseDockerProxy(cmdline string) (string, bool) {
	fields := strings.Fields(cmdline)
	if len(fields) == 0 || !strings.Contains(fields[0], "docker-proxy") {
		return "", false
	}
	var ip, port string
	for i := 0; i+1 < len(fields); i++ {
		switch strings.TrimLeft(fields[i], "-") {
		case "container-ip":
			ip = fields[i+1]
		case "container-port":
			port = fields[i+1]
		}
	}
	if ip == "" || port == "" {
		return "", false
	}
	return net.JoinHostPort(ip, port), true
}
