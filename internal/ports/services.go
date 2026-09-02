package ports

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// wellKnown maps ports to friendly names. It is deliberately curated for what
// developers actually run rather than mirroring IANA: /etc/services calls 3000
// "ppp" and 5000 "commplex-main", which helps nobody.
var wellKnown = map[uint16]string{
	20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns",
	67: "dhcp", 68: "dhcp", 69: "tftp", 80: "http", 88: "kerberos", 110: "pop3",
	111: "rpcbind", 119: "nntp", 123: "ntp", 135: "msrpc", 137: "netbios-ns",
	138: "netbios-dgm", 139: "netbios-ssn", 143: "imap", 161: "snmp", 162: "snmp-trap",
	179: "bgp", 389: "ldap", 443: "https", 445: "smb", 465: "smtps", 500: "isakmp",
	514: "syslog", 515: "printer", 546: "dhcpv6", 547: "dhcpv6", 548: "afp",
	554: "rtsp", 587: "submission", 631: "ipp", 636: "ldaps", 853: "dns-over-tls",
	873: "rsync", 989: "ftps-data", 990: "ftps", 993: "imaps", 995: "pop3s",
	1080: "socks", 1194: "openvpn", 1433: "mssql", 1521: "oracle", 1701: "l2tp",
	1723: "pptp", 1883: "mqtt", 1900: "ssdp", 2049: "nfs", 2181: "zookeeper",
	2375: "docker", 2376: "docker-tls", 2379: "etcd", 2380: "etcd-peer",
	3000: "dev-server", 3001: "dev-server", 3128: "squid", 3306: "mysql",
	3389: "rdp", 3478: "stun", 4000: "dev-server", 4200: "angular", 4222: "nats",
	4369: "epmd", 4443: "https-alt", 4567: "sinatra", 5000: "dev-server",
	5001: "dev-server", 5060: "sip", 5061: "sips", 5173: "vite", 5222: "xmpp",
	5353: "mdns", 5355: "llmnr", 5432: "postgres", 5555: "adb", 5601: "kibana",
	5672: "amqp", 5900: "vnc", 5984: "couchdb", 6000: "x11", 6379: "redis",
	6443: "kube-api", 6543: "pgbouncer", 6667: "irc", 7000: "dev-server",
	7077: "spark", 7474: "neo4j", 7687: "neo4j-bolt", 8000: "http-dev",
	8008: "http-alt", 8025: "mailhog", 8080: "http-alt", 8081: "http-alt",
	8088: "http-alt", 8123: "home-assistant", 8200: "vault", 8333: "bitcoin",
	8443: "https-alt", 8500: "consul", 8529: "arangodb", 8888: "jupyter",
	9000: "php-fpm", 9001: "supervisor", 9042: "cassandra", 9090: "prometheus",
	9092: "kafka", 9093: "alertmanager", 9100: "node-exporter", 9200: "elasticsearch",
	9300: "elasticsearch", 9418: "git", 9443: "https-alt", 9999: "dev-server",
	10250: "kubelet", 11211: "memcached", 15672: "rabbitmq-ui", 19000: "expo",
	24224: "fluentd", 25565: "minecraft", 27017: "mongodb", 27018: "mongodb",
	28015: "rethinkdb", 32400: "plex", 50051: "grpc", 51820: "wireguard",
}

var (
	etcServicesOnce sync.Once
	etcServices     map[string]string // "80/tcp" -> "http"
)

// ServiceName returns a friendly service name for a port, or "".
// Curated developer-friendly names win; below 1024 the system services file is
// consulted as a fallback because IANA assignments there are reliable.
func ServiceName(port uint16, proto string) string {
	if name, ok := wellKnown[port]; ok {
		return name
	}
	if port >= 1024 {
		return ""
	}
	etcServicesOnce.Do(loadEtcServices)
	return etcServices[strconv.Itoa(int(port))+"/"+strings.ToLower(proto)]
}

func loadEtcServices() {
	etcServices = map[string]string{}
	path := "/etc/services"
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		path = root + `\System32\drivers\etc\services`
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[1])
		if _, exists := etcServices[key]; !exists {
			etcServices[key] = fields[0]
		}
	}
}
