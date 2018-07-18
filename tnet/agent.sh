#!/bin/sh

nohup ./tnet agent :53129 localhost:53128 &>tnet_agent_53129.log &
