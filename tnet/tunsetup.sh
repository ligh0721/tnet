#!/bin/sh
ip tuntap add dev tun0 mode tun
ifconfig tun0 192.168.100.1 dstaddr 192.168.100.2 up
iptables -t nat -A POSTROUTING -s 192.168.100.0/24 -o ens3 -j MASQUERADE
