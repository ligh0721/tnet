#!/bin/sh

nohup ./tnet agent :53129 localhost:53128 1>tnet53128.log 
2>&1 &
