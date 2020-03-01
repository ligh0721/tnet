package main

import "git.tutils.com/tutils/tnet/tqa"

func main() {
	qas := tqa.NewQaServer()
	qas.HttpAddr = ":58000"
	qas.StaticRoot = "qaroot/static/"
	qas.ExpiredAnswered = 3600e9
	qas.ExpiredUnanswered = 36 * 3600e9
	qas.Start()
}
