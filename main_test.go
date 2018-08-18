package main

import (
	"fmt"
	"testing"
)

func TestNextIP(t *testing.T) {
	var ips = [4]string{
		"0.0.0.254",
		"0.0.255.254",
		"0.255.255.254",
		"255.255.255.254",
	}

	for _, ip := range ips {
		for i := 0; i < 2; i++ {
			ip = nextIP4(ip)
			fmt.Println(ip)
		}
	}
}

func TestCreateIP4Table(t *testing.T) {
	fmt.Println(createIP4Table("127.0.255.250", "127.1.0.5"))
}
