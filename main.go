package main

import (
	"flag"
	"os"
	"time"
)

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Second, "单个公网 IP 服务请求超时时间")
	flag.Parse()

	os.Exit(run(*jsonOut, *timeout))
}
