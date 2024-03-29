package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Scanner ...
type Scanner struct {
	host    string
	ip      net.IP
	timeout time.Duration
}

func usage(msg string, exit bool) {
	if msg != "" {
		fmt.Println(msg)
	} else {
		_, main := filepath.Split(os.Args[0])
		fmt.Printf(`
Usage:
  ./%s <IP>[:<IP>] [<port>[:<port>]] [Options]

* <IP>   [:<IP>]          IP4, IP6 or host name allowed
  <port> [:<port>]        (default 1:65536)
                          Range scanning allowed on IP4 and port
                          Example: 192.168.0.1:192.168.1.255 10:10000

Options:
  -w, --threads           (default 100)
  -t, --timeout duration  (dafault 3s)
                          Example: 300ms, 0.5s, 5

`, main)

	}
	if exit {
		os.Exit(0)
	}
}

var (
	threads            = 100
	timeout, _         = time.ParseDuration("3s")
	ip_start, ip_end   string
	portStart, portEnd int
	m                  = &sync.Mutex{}
)

func main() {
	if len(os.Args) < 2 {
		usage("", true)
	}

	var err error
	if strings.Contains(os.Args[1], ":") {
		ipRange := strings.Split(os.Args[1], ":")
		ip_start = ipRange[0]
		ip_end = ipRange[1]
	} else {
		ip_start = os.Args[1]
		ip_end = ip_start
	}

	if len(os.Args) > 2 {
		if strings.Contains(os.Args[2], ":") {
			portRange := strings.Split(os.Args[2], ":")
			portStart, _ = strconv.Atoi(portRange[0])
			portEnd, _ = strconv.Atoi(portRange[1])
		} else {
			portStart, _ = strconv.Atoi(os.Args[2])
			portEnd = portStart
		}
	}
	if portStart < 1 {
		portStart = 1
	}
	if portEnd < 1 || portEnd > 65536 {
		portEnd = 65536
	}
	if portEnd < portStart {
		usage("Port End must be greater than Port Start", true)
	}
	if portStart < 1 || portStart > 65536 || portEnd < 1 || portEnd > 65536 {
		usage("Port range must be between 1 and 65536", true)
	}

	for i, arg := range os.Args[1:] {
		if arg == "-t" || arg == "--timeout" {
			timeout, err = time.ParseDuration(os.Args[i+2])
			if err != nil {
				usage("Could not get timeout.  Use: -t or --timeout <duration>  Example: 300ms, 0.5s, 5s\n", true)
			}
		}
		if arg == "-w" || arg == "--threads" {
			threads, err = strconv.Atoi(os.Args[i+2])
			if err != nil {
				usage("Could not get threads.  Use: -w or --threads <num>  number of threads", true)
			}
		}
	}

	// ---

	handleInterrupt()

	ip_string := createIP4Table(ip_start, ip_end)
	// use semphore channels to limit running threads
	sem := make(chan int, threads)
	t := time.Now()

	for _, ip := range ip_string {
		scan := New(ip)
		scan.Start(portStart, portEnd, sem)
	}

	// wait for empty semphore, then we know we are done
	for len(sem) != 0 {
		time.Sleep(10 * time.Millisecond)
	}

	close(sem)
	fmt.Println("completed in", time.Since(t))
}

// New Scanner
func New(host string) *Scanner {
	return &Scanner{
		ip:      net.ParseIP(host),
		host:    host,
		timeout: timeout,
	}
}

// Start scanning ...
func (h *Scanner) Start(portStart int, portEnds int, sem chan int) {
	for port := portStart; port <= portEnds; port++ {
		// +1 thread
		sem <- 1
		// make it concurrent
		go func(p int) {
			if h.connect(p) {
				m.Lock()
				fmt.Printf("%9d %10v %45s\n", p, h.ip.String(), mapPortDescriptions[p])
				m.Unlock()
			}
			// free thread
			<-sem
		}(port)
	}
}

// connect ...
func (h *Scanner) connect(port int) bool {
	addr := fmt.Sprintf("%s:%d", h.host, port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", tcpAddr.String(), h.timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// createIP4Table slice
func createIP4Table(ip_start, ip_end string) []string {
	if ip_start == ip_end {
		return []string{ip_start}
	}

	var table []string
	var ip = ip_start
	for {
		table = append(table, ip)
		ip = nextIP4(ip)

		if ip == "" || !isIP4Range(ip, ip_start, ip_end) {
			return table
		}
	}
}

// nextIP4 ...
func nextIP4(ip_string string) string {
	ip := strings.Split(ip_string, ".")

	for i := len(ip) - 1; i >= 0; i-- {
		v, err := strconv.Atoi(ip[i])
		if err != nil {
			panic(err)
		}
		if v >= 255 {
			ip[i] = "0"
			continue
		}
		v++
		ip[i] = strconv.Itoa(v)
		return strings.Join(ip, ".")
	}
	return ""
}

// ip4Range checks if ip4 is withing a range
func isIP4Range(ip_string, ip_start, ip_end string) bool {
	ip := net.ParseIP(ip_string)
	if ip.To4() == nil {
		fmt.Printf("%v is not an IPv4 address\n", ip_string)
		os.Exit(1)
	}
	start := net.ParseIP(ip_start)
	if start.To4() == nil {
		fmt.Printf("ip_start: %v is not an IPv4 address\n", ip_start)
		os.Exit(1)
	}
	end := net.ParseIP(ip_end)
	if end.To4() == nil {
		fmt.Printf("ip_end: %v is not an IPv4 address\n", ip_end)
		os.Exit(1)
	}
	if bytes.Compare(ip, start) >= 0 && bytes.Compare(ip, end) <= 0 {
		return true
	}
	return false
}

// handleInterrupt
func handleInterrupt() {
	cancel := make(chan os.Signal, 1)
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	go func() {
		fmt.Printf("/n%s recived.\n", <-cancel)
		os.Exit(1)
	}()
}

var mapPortDescriptions = map[int]string{
	/*
		Table C-1
		lists the Well Known Ports as defined by IANA
		and is used by Red Hat Enterprise Linux as default
		communication ports for various services, including FTP, SSH, and Samba.
	*/
	1:   "(tcpmux/TCP) port service multiplexer",
	7:   "(echo/Echo) service",
	9:   "(discard/Null) service for connection testing",
	11:  "(systat	System) Status service for listing connected ports",
	13:  "(daytime) Sends date and time to requesting host",
	17:  "(qotd) Sends quote of the day to connected host",
	18:  "(msp) Message Send Protocol",
	19:  "(chargen) Character Generation service; sends endless stream of characters",
	20:  "(ftp-data) FTP data port",
	21:  "(ftp) File Transfer Protocol (FTP) port; sometimes used by File Service Protocol (FSP)",
	22:  "(ssh) Secure Shell (SSH) service",
	23:  "(telnet) The Telnet service",
	25:  "(smtp) Simple Mail Transfer Protocol (SMTP)",
	37:  "(time) Time Protocol",
	39:  "(rlp) Resource Location Protocol",
	42:  "(nameserver) Internet Name Service",
	43:  "(nicname) WHOIS directory service",
	49:  "(tacacs) Terminal Access Controller Access Control System for TCP/IP based authentication and access",
	50:  "(re-mail-ck) Remote Mail Checking Protocol",
	53:  "(domain) domain name services (such as BIND)",
	63:  "(whois++) WHOIS++, extended WHOIS services",
	67:  "(bootps) Bootstrap Protocol (BOOTP) services; also used by Dynamic Host Configuration Protocol (DHCP) services",
	68:  "(bootpc) Bootstrap (BOOTP) client; also used by Dynamic Host Control Protocol (DHCP) clients",
	69:  "(tftp) Trivial File Transfer Protocol (TFTP)",
	70:  "(gopher) Gopher Internet document search and retrieval",
	71:  "(netrjs-1) Remote Job Service",
	72:  "(netrjs-2) Remote Job Service",
	73:  "(netrjs-4) Remote Job Service",
	79:  "(finger) Finger service for user contact information",
	80:  "(http) HyperText Transfer Protocol (HTTP) for World Wide Web (WWW) services",
	88:  "(kerberos) Kerberos network authentication system",
	95:  "(supdup) Telnet protocol extension",
	101: "(hostname) Hostname services on SRI-NIC machines",
	102: "(tcp) iso-tsap ISO Development Environment (ISODE) network applications",
	105: "(csnet-ns) Mailbox nameserver; also used by CSO nameserver",
	107: "(rtelnet) Remote Telnet",
	109: "(pop2) Post Office Protocol version 2",
	110: "(pop3) Post Office Protocol version 3",
	111: "(sunrpc) Remote Procedure Call (RPC) Protocol for remote command execution, used by Network Filesystem (NFS)",
	113: "(auth) Authentication and Ident protocols",
	115: "(sftp) Secure File Transfer Protocol (SFTP) services",
	117: "(uucp-path	Unix-to-Unix) Copy Protocol (UUCP) Path services",
	119: "(nntp) Network News Transfer Protocol (NNTP) for the USENET discussion system",
	123: "(ntp) Network Time Protocol (NTP)",
	137: "(netbios-ns) NETBIOS Name Service used in Red Hat Enterprise Linux by Samba",
	138: "(netbios-dgm) NETBIOS Datagram Service used in Red Hat Enterprise Linux by Samba",
	139: "(netbios-ssn) NETBIOS Session Service used in Red Hat Enterprise Linux by Samba",
	143: "(imap) Internet Message Access Protocol (IMAP)",
	161: "(snmp) Simple Network Management Protocol (SNMP)",
	162: "(snmptrap) Traps for SNMP",
	163: "(cmip-man) Common Management Information Protocol (CMIP)",
	164: "(cmip-agent) Common Management Information Protocol (CMIP)",
	174: "(mailq) AILQ email transport queue",
	177: "(xdmcp) X Display Manager Control Protocol (XDMCP)",
	178: "(nextstep) NeXTStep window server",
	179: "(bgp) Border Gateway Protocol",
	191: "(prospero) Prospero distributed filesystem services",
	194: "(irc) Internet Relay Chat (IRC)",
	199: "(smux) SNMP UNIX Multiplexer",
	201: "(at-rtmp) AppleTalk routing",
	202: "(at-nbp) AppleTalk name binding",
	204: "(at-echo) AppleTalk echo",
	206: "(at-zis) AppleTalk zone information",
	209: "(qmtp) Quick Mail Transfer Protocol (QMTP)",
	210: "(z39.50) NISO Z39.50 database",
	213: "(ipx) Internetwork Packet Exchange (IPX), a datagram protocol commonly used in Novell Netware environments",
	220: "(imap3) Internet Message Access Protocol version 3",
	245: "(link) LINK / 3-DNS iQuery service",
	347: "(fatserv) FATMEN file and tape management server",
	363: "(rsvp_tunnel) RSVP Tunnel",
	369: "(rpc2portmap) Coda file system portmapper",
	370: "(codaauth2) Coda file system authentication services",
	372: "(ulistproc) UNIX LISTSERV",
	389: "(ldap) Lightweight Directory Access Protocol (LDAP)",
	427: "(svrloc) Service Location Protocol (SLP)",
	434: "(mobileip-agent) Mobile Internet Protocol (IP) agent",
	435: "(mobilip-mn) Mobile Internet Protocol (IP) manager",
	443: "(https) Secure Hypertext Transfer Protocol (HTTP)",
	444: "(snpp) Simple Network Paging Protocol",
	445: "(microsoft-ds) Server Message Block (SMB) over TCP/IP",
	464: "(kpasswd) Kerberos password and key changing services",
	468: "(photuris) Photuris session key management protocol",
	487: "(saft) Simple Asynchronous File Transfer (SAFT) protocol",
	488: "(gss-http) Generic Security Services (GSS) for HTTP",
	496: "(pim-rp-disc) Rendezvous Point Discovery (RP-DISC) for Protocol Independent Multicast (PIM) services",
	500: "(isakmp) Internet Security Association and Key Management Protocol (ISAKMP)",
	535: "(iiop) Internet Inter-Orb Protocol (IIOP)",
	538: "(gdomap) GNUstep Distributed Objects Mapper (GDOMAP)",
	546: "(dhcpv6-client) Dynamic Host Configuration Protocol (DHCP) version 6 client",
	547: "(dhcpv6-server) Dynamic Host Configuration Protocol (DHCP) version 6 Service",
	554: "rtsp Real Time Stream Control Protocol (RTSP)",
	563: "(nntps) Network News Transport Protocol over Secure Sockets Layer (NNTPS)",
	565: "(whoami) whoami user ID listing",
	587: "(submission) Mail Message Submission Agent (MSA)",
	610: "(npmp-local) Network Peripheral Management Protocol (NPMP) local / Distributed Queueing System (DQS)",
	611: "(npmp-gui) Network Peripheral Management Protocol (NPMP) GUI / Distributed Queueing System (DQS)",
	612: "(hmmp-ind) HyperMedia Management Protocol (HMMP) Indication / DQS",
	631: "(ipp) Internet Printing Protocol (IPP)",
	636: "(ldaps) Lightweight Directory Access Protocol over Secure Sockets Layer (LDAPS)",
	674: "(acap) Application Configuration Access Protocol (ACAP)",
	694: "(ha-cluster) Heartbeat services for High-Availability Clusters",
	749: "(kerberos-adm) Kerberos version 5 (v5) 'kadmin' database administration",
	750: "(kerberos-iv) Kerberos version 4 (v4) services",
	765: "(webster) Network Dictionary",
	767: "(phonebook) Network Phonebook",
	873: "(rsync) rsync file transfer services",
	992: "(telnets) Telnet over Secure Sockets Layer (TelnetS)",
	993: "(imaps) Internet Message Access Protocol over Secure Sockets Layer (IMAPS)",
	994: "(ircs) Internet Relay Chat over Secure Sockets Layer (IRCS)",
	995: "(pop3s) Post Office Protocol version 3 over Secure Sockets Layer (POP3S)",
	/*
	   Table C-2
	   lists UNIX-specific ports and cover services
	   ranging from email to authentication and more.
	   Names enclosed in brackets (for example, [service])
	   are either daemon names for the service or common alias(es).
	*/
	512: "(exec) Authentication for remote process execution\nbiff [comsat] Asynchrous mail client (biff) and service (comsat)",
	513: "(login) Remote Login (rlogin)\nwho [whod] whod user logging daemon",
	514: "(shell) [cmd] Remote shell (rshell) and remote copy (rcp) with no logging\nsyslog UNIX system logging service",
	515: "(printer) [spooler] Line printer (lpr) spooler",
	517: "(talk) Talk remote calling service and client",
	518: "(ntalk) Network talk (ntalk) remote calling service and client",
	519: "(utime) [unixtime] UNIX time (utime) protocol",
	520: "(efs) Extended Filename Server (EFS)\nrouter [route, routed]	Routing Information Protocol (RIP)",
	521: "(ripng) Routing Information Protocol for Internet Protocol version 6 (IPv6)",
	525: "(timed) [timeserver] Time daemon (timed)",
	526: "(tempo) [newdate] Tempo",
	530: "(courier) [rpc] Courier Remote Procedure Call (RPC) protocol",
	531: "(conference) [chat] Internet Relay Chat",
	532: "(netnews) Netnews newsgroup service",
	533: "(netwall) Netwall for emergency broadcasts",
	540: "(uucp) [uucpd] UNIX-to-UNIX copy services",
	543: "(klogin) Kerberos version 5 (v5) remote login",
	544: "(kshell) Kerberos version 5 (v5) remote shell",
	548: "(afpovertcp) Appletalk Filing Protocol (AFP) over Transmission Control Protocol (TCP)",
	556: "(remotefs) [rfs_server, rfs] Brunhoff's Remote Filesystem (RFS)",
	/*
	   Table C-3
	   lists ports submitted by the network and software community
	   to the IANA for formal registration into the port number list.
	*/
	1080:  "(socks) SOCKS network application proxy services",
	1236:  "(bvcontrol) [rmtcfg] Remote configuration server for Gracilis Packeten network switches[a]",
	1300:  "(h323hostcallsc) H.323 telecommunication Host Call Secure",
	1433:  "(ms-sql-s) Microsoft SQL Server",
	1434:  "(ms-sql-m) Microsoft SQL Monitor",
	1494:  "(ica) Citrix ICA Client",
	1512:  "(wins) Microsoft Windows Internet Name Server",
	1524:  "(ingreslock) Ingres Database Management System (DBMS) lock services",
	1525:  "(prospero-np) Prospero non-privileged",
	1645:  "(datametrics) [old-radius] Datametrics / old radius entry",
	1646:  "(sa-msg-port) [oldradacct] sa-msg-port / old radacct entry",
	1649:  "(kermit) Kermit file transfer and management service",
	1701:  "(l2tp) [l2f] Layer 2 Tunneling Protocol (LT2P) / Layer 2 Forwarding (L2F)",
	1718:  "(h323gatedisc) H.323 telecommunication Gatekeeper Discovery",
	1719:  "(h323gatestat) H.323 telecommunication Gatekeeper Status",
	1720:  "(h323hostcall) H.323 telecommunication Host Call setup",
	1758:  "(tftp-mcast) Trivial FTP Multicast",
	1759:  "(mtftp) Multicast Trivial FTP (MTFTP)",
	1789:  "(hello) Hello router communication protocol",
	1812:  "(radius) Radius dial-up authentication and accounting services",
	1813:  "(radius-acct) Radius Accounting",
	1911:  "(mtp) Starlight Networks Multimedia Transport Protocol (MTP)",
	1985:  "(hsrp) Cisco Hot Standby Router Protocol",
	1986:  "(licensedaemon) Cisco License Management Daemon",
	1997:  "(gdp-port) Cisco Gateway Discovery Protocol (GDP)",
	2049:  "(nfs) [nfsd] Network File System (NFS)",
	2102:  "(zephyr-srv) Zephyr distributed messaging Server",
	2103:  "(zephyr-clt) Zephyr client",
	2104:  "(zephyr-hm) Zephyr host manager",
	2401:  "(cvspserver) Concurrent Versions System (CVS) client/server operations",
	2430:  "(venus) Venus cache manager for Coda file system (codacon port)\nvenus Venus cache manager for Coda file system (callback/wbc interface)",
	2431:  "(venus-se) Venus Transmission Control Protocol (TCP) side effects\nvenus-se Venus User Datagram Protocol (UDP) side effects",
	2432:  "(codasrv) Coda file system server port",
	2433:  "(codasrv-se) Coda file system TCP side effects\ncodasrv-se Coda file system UDP SFTP side effect",
	2600:  "(hpstgmgr) [zebrasrv] Zebra routing[b]",
	2601:  "(discp-client) [zebra] discp client; Zebra integrated shell",
	2602:  "(discp-server) [ripd] discp server; Routing Information Protocol daemon (ripd)",
	2603:  "(servicemeter) [ripngd] Service Meter; RIP daemon for IPv6",
	2604:  "(nsc-ccs) [ospfd] NSC CCS; Open Shortest Path First daemon (ospfd)",
	2605:  "(nsc-posa) NSC POSA; Border Gateway Protocol daemon (bgpd)",
	2606:  "(netmon) [ospf6d] Dell Netmon; OSPF for IPv6 daemon (ospf6d)",
	2809:  "(corbaloc) Common Object Request Broker Architecture (CORBA) naming service locator",
	3130:  "(icpv2) Internet Cache Protocol version 2 (v2); used by Squid proxy caching server",
	3306:  "(mysql) MySQL database service",
	3346:  "(trnsprntproxy) Transparent proxy",
	4011:  "(pxe) Pre-execution Environment (PXE) service",
	4321:  "(rwhois) Remote Whois (rwhois) service",
	4444:  "(krb524) Kerberos version 5 (v5) to version 4 (v4) ticket translator",
	5002:  "(rfe) Radio Free Ethernet (RFE) audio broadcasting system",
	5308:  "(cfengine) Configuration engine (Cfengine)",
	5999:  "(cvsup) [CVSup] CVSup file transfer and update tool",
	6000:  "(x11) [X] X Window System services",
	7000:  "(afs3-fileserver) Andrew File System (AFS) file server",
	7001:  "(afs3-callback) AFS port for callbacks to cache manager",
	7002:  "(afs3-prserver) AFS user and group database",
	7003:  "(afs3-vlserver) AFS volume location database",
	7004:  "(afs3-kaserver) AFS Kerberos authentication service",
	7005:  "(afs3-volser) AFS volume management server",
	7006:  "(afs3-errors) AFS error interpretation service",
	7007:  "(afs3-bos) AFS basic overseer process",
	7008:  "(afs3-update) AFS server-to-server updater",
	7009:  "(afs3-rmtsys) AFS remote cache manager service",
	9876:  "(sd) Session Director for IP multicast conferencing",
	10080: "(amanda) Advanced Maryland Automatic Network Disk Archiver (Amanda) backup services",
	11371: "(pgpkeyserver) Pretty Good Privacy (PGP) / GNU Privacy Guard (GPG) public keyserver",
	11720: "(h323callsigalt) H.323 Call Signal Alternate",
	13720: "(bprd) Veritas NetBackup Request Daemon (bprd)",
	13721: "(bpdbm) Veritas NetBackup Database Manager (bpdbm)",
	13722: "(bpjava-msvc) Veritas NetBackup Java / Microsoft Visual C++ (MSVC) protocol",
	13724: "(vnetd) Veritas network utility",
	13782: "(bpcd) Veritas NetBackup",
	13783: "(vopied) Veritas VOPIE authentication daemon",
	22273: "(wnn6) [wnn4] Kana/Kanji conversion system[c]",
	26000: "(quake) Quake (and related) multi-player game servers",
	26208: "(wnn6-ds) Wnn6 Kana/Kanji server",
	33434: "(traceroute) Traceroute network tracking tool",
	/*
	   Table C-5
	   is a listing of ports related to the Kerberos network
	   authentication protocol.
	   Where noted, v5 refers to the Kerberos version 5 protocol.
	   Note that these ports are not registered with the IANA.
	*/
	751:  "(kerberos_master) Kerberos authentication",
	752:  "(passwd_server) Kerberos Password (kpasswd) server",
	754:  "(krb5_prop) Kerberos v5 slave propagation",
	760:  "(krbupdate) [kreg] Kerberos registration",
	1109: "(kpop) Kerberos Post Office Protocol (KPOP)",
	2053: "(knetd) Kerberos de-multiplexor",
	2105: "(eklogin) Kerberos v5 encrypted remote login (rlogin)",
	/*
	   Table C-6 is a listing of unregistered ports
	   that are used by services and protocols that may be installed
	   on your Red Hat Enterprise Linux system, or that is necessary
	   for communication between Red Hat Enterprise Linux and other operating systems.
	*/
	15:    "(netstat) Network Status (netstat)",
	98:    "(linuxconf) Linuxconf Linux administration tool",
	106:   "(poppassd) Post Office Protocol password change daemon (POPPASSD)",
	465:   "(smtps) Simple Mail Transfer Protocol over Secure Sockets Layer (SMTPS)",
	616:   "(gii) Gated (routing daemon) Interactive Interface",
	808:   "(omirr) [omirrd] Online Mirror (Omirr) file mirroring services",
	871:   "(supfileserv) Software Upgrade Protocol (SUP) server",
	901:   "(swat) Samba Web Administration Tool (SWAT)",
	953:   "(rndc) Berkeley Internet Name Domain version 9 (BIND 9) remote configuration tool",
	1127:  "(supfiledbg) Software Upgrade Protocol (SUP) debugging",
	1178:  "(skkserv) Simple Kana to Kanji (SKK) Japanese input server",
	1313:  "(xtel	French) Minitel text information system",
	1529:  "(support) [prmsd, gnatsd]	GNATS bug tracking system",
	2003:  "(cfinger) GNU finger",
	2150:  "(ninstall) Network Installation Service",
	2988:  "(afbackup) afbackup client-server backup system",
	3128:  "(squid) Squid Web proxy cache",
	3455:  "(prsvp) RSVP port",
	5432:  "(postgres) PostgreSQL database",
	4557:  "(fax) FAX transmission service (old service)",
	4559:  "(hylafax) HylaFAX client-server protocol (new service)",
	5232:  "(sgi-dgl) SGI Distributed Graphics Library",
	5354:  "(noclog) NOCOL network operation center logging daemon (noclogd)",
	5355:  "(hostmon) NOCOL network operation center host monitoring",
	5680:  "(canna) Canna Japanese character input interface",
	6010:  "(x11-ssh-offset) Secure Shell (SSH) X11 forwarding offset",
	6667:  "(ircd) Internet Relay Chat daemon (ircd)",
	7100:  "(xfs) X Font Server (XFS)",
	7666:  "(tircproxy) Tircproxy IRC proxy service",
	8008:  "(http-alt) Hypertext Tranfer Protocol (HTTP) alternate",
	8080:  "(webcache) World Wide Web (WWW) caching service",
	8081:  "(tproxy) Transparent Proxy",
	9100:  "(jetdirect) [laserjet, hplj] Hewlett-Packard (HP) JetDirect network printing service",
	9359:  "(mandelspawn) [mandelbrot]	Parallel mandelbrot spawning program for the X Window System",
	10081: "(kamanda) Amanda backup service over Kerberos",
	10082: "(amandaidx) Amanda index server",
	10083: "(amidxtape) Amanda tape server",
	20011: "(isdnlog) Integrated Services Digital Network (ISDN) logging system",
	20012: "(vboxd) ISDN voice box daemon (vboxd)",
	22305: "(wnn4_Kr) kWnn Korean input system",
	22289: "(wnn4_Cn) cWnn Chinese input system",
	22321: "(wnn4_Tw) tWnn Chinese input system (Taiwan)",
	24554: "(binkp) Binkley TCP/IP Fidonet mailer daemon",
	27374: "(asp) Address Search Protocol",
	60177: "(tfido) Ifmail FidoNet compatible mailer service",
	60179: "(fido) FidoNet electronic mail and news network",
}
