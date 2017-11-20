#!/bin/sh
go get -u git.tutils.com/tutils/tnet/tnet
go generate git.tutils.com/tutils/tnet/tcenter
go install git.tutils.com/tutils/tnet/tnet

