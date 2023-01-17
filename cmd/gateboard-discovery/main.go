/*
This is the main package for gateboard-discovery service.
*/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
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

	config := newConfig()

	creds := loadCredentials(config.accountsFile)

	for {
		for i, c := range creds {
			log.Printf("---------- main account %d/%d", i+1, len(creds))
			findGateways(c, me, config)
		}
		if config.interval == 0 {
			log.Printf("interval is %v, exiting after single run", config.interval)
			break
		}
		log.Printf("sleeping for %v", config.interval)
		time.Sleep(config.interval)
	}

}

type credential struct {
	RoleArn        string `yaml:"role_arn"`
	RoleExternalID string `yaml:"role_external_id"`
	Region         string `yaml:"region"`
}

func loadCredentials(input string) []credential {
	buf, errRead := os.ReadFile(input)
	if errRead != nil {
		log.Fatalf("loadCredentials: read file: %s: %v", input, errRead)
	}
	var creds []credential
	errYaml := yaml.Unmarshal(buf, &creds)
	if errYaml != nil {
		log.Fatalf("loadCredentials: parse yaml: %s: %v", input, errYaml)
	}
	return creds
}
