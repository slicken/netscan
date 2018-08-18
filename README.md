# netScan
# a simple and fast ip and port range scanner written in Go
#

Usage: netScan <IP> [<port>] [option1] [option2]..

 <IP>                                   ip4,ip6 or host names allowed
 <port>                                 (default 1:65536)
                                        Range scanning allowed on IP4 and port
                                        Example: 192.0.0.1:192.2.255 80:90

Options:
 -w, --threads                          (default: 100)
 -t, --timeout duration                 (dafault: 2s)
 -t,                                    Example: 300ms, 0.5s, 5s
