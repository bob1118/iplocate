package main

import (
	"flag"
	"os"
	"time"
)

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Second, "每轮公网查询及每次 DNS 探测/归属查询的预算（非全程序总时限）")
	flag.Parse()

	os.Exit(run(*jsonOut, *timeout))
}
