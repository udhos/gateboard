/*
This is the main package for the example client.
*/
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/udhos/gateboard/env"
	"github.com/udhos/gateboard/gateboard"
	"gopkg.in/yaml.v3"
)

const version = "0.0.0"

const tryAgain = http.StatusServiceUnavailable
const internalError = http.StatusInternalServerError

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

	client := gateboard.NewClient(gateboard.ClientOptions{
		ServerURL:   "http://localhost:8080/gateway",
		FallbackURL: "http://localhost:8181/gateway",
		Debug:       env.Bool("DEBUG", true),
	})

	log.Printf("reading gateway name from stdin...")
	for {
		reader := bufio.NewReader(os.Stdin)
		txt, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("stdin: %v", err)
			break
		}
		gatewayName := strings.TrimSpace(txt)
		if gatewayName == "" {
			log.Printf("ignoring empty gateway name")
			continue
		}
		status, body := invokeBackend(client, gatewayName)
		log.Printf("RESULT for incomingCall: gateway_name=%s status=%d body:%s",
			gatewayName, status, body)
		fmt.Println("------------------------------")
	}
}

// invokeBackend implements Recommended Usage from
// https://pkg.go.dev/github.com/udhos/gateboard@main/gateboard#hdr-Recommended_Usage
func invokeBackend(client *gateboard.Client, gatewayName string) (int, string) {
	const me = "invokeBackend"

	gatewayID := client.GatewayID(gatewayName)
	if gatewayID == "" {
		log.Printf("%s: GatewayID: gateway_name=%s starting Refresh() async update",
			me, gatewayName)
		return tryAgain, "missing gateway_id"
	}

	log.Printf("%s: mockAwsApiGatewayCall: gateway_name=%s gateway_id=%s",
		me, gatewayName, gatewayID)

	status, body := mockAwsAPIGatewayCall(gatewayID)
	if status == 403 {
		log.Printf("%s: mockAwsApiGatewayCall: gateway_name=%s gateway_id=%s status=%d starting Refresh() async update",
			me, gatewayName, gatewayID, status)
		client.Refresh(gatewayName) // async update
		return tryAgain, "refreshing gateway_id"
	}

	return status, body
}

func mockAwsAPIGatewayCall(gatewayID string) (int, string) {
	const me = "mockAwsAPIGatewayCall"
	filename := "samples/http_mock.yaml"
	data, errFile := os.ReadFile(filename)
	if errFile != nil {
		log.Printf("%s: %s: file error: %v", me, filename, errFile)
		return internalError, "bad file"
	}

	type response struct {
		Code int    `yaml:"code"`
		Body string `yaml:"body"`
	}

	table := map[string]response{}

	errYaml := yaml.Unmarshal(data, &table)
	if errYaml != nil {
		log.Printf("%s: %s: yaml error: %v", me, filename, errYaml)
		return internalError, "bad file yaml"
	}

	//log.Printf("%s: loaded %s: %s", me, filename, string(data))

	r, found := table[gatewayID]
	if found {
		return r.Code, r.Body
	}

	log.Printf("%s: %s: id not found: %s", me, filename, gatewayID)
	return internalError, "missing gateway id from file"
}
