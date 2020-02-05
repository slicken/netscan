# netscan

A simple but fast concurrent IP and PORT range scanner written in Go


```
Usage: main <IP> [<port>] [option1] [option2]..

 <IP>                    ip4,ip6 or host names allowed
 <port>                  (default 1:65536)
                         Range scanning allowed on IP4 and port
                         Example: 192.168.0.1:192.168.255.255 1000:65535

Options:
 -w, --threads           (default: 100)
 -t, --timeout duration  (dafault: 3s)
                         Example: 300ms, 0.5s, 5s
```
