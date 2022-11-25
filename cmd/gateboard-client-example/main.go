package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/udhos/gateboard/gateboard"
)

const version = "0.0.0"

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
}

func main() {

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	{
		v := getVersion(me)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	client := gateboard.NewClient(gateboard.ClientOptions{ServerURL: "http://localhost:8080/gateway"})

	for _, name := range flag.Args() {
		id, err := client.GatewayID(name)
		if err != nil {
			log.Printf("gateway_name=%s error: %v", name, err)
			continue
		}
		log.Printf("gateway_name=%s gateway_id=%s", name, id)
	}
}
