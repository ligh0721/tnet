#!/bin/sh
nohup ./tnet tun :56200 tun0 tutils -m 1400 -a 192.168.100.2 32 -d 8.8.8.8 -r 0.0.0.0 0 1>tnet56200.log 2>&1 &
