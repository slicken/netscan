# netscan

A simple but fast concurrent IP and PORT range scanner written in Go


```

Usage:
  ./app <IP>[:<IP>] [<port>[:<port>]] [Options]

* <IP>   [:<IP>]          IP4, IP6 or host name allowed
  <port> [:<port>]        (default 1:65536)
                          Range scanning allowed on IP4 and port
                          Example: 192.168.0.1:192.168.1.255 10:10000

Options:
  -w, --threads           (default 100)
  -t, --timeout duration  (dafault 3s)
                          Example: 300ms, 0.5s, 5

```
