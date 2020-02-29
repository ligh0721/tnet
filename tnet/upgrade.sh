#!/bin/sh
go get -u tnet/tnet
go generate tnet/tcenter
go install tnet/tnet

