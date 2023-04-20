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
)

const version = "0.1.0"

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

	creds, errCreds := loadCredentials(config.accountsFile)
	if errCreds != nil {
		log.Fatalf("loading credentials: %v", errCreds)
	}

	var save saver
	switch config.save {
	case "server":
		save = newSaverServer(config.gateboardServerURL)
	case "webhook":
		save = newSaverWebhook(config.webhookURL, config.webhookToken, config.webhookMethod)
	case "sqs":
		save = newSaverSQS(config.queueURL, config.queueRoleARN, config.queueRoleExternalID, me)
	case "sns":
		save = newSaverSNS(config.topicARN, config.topicRoleARN, config.topicRoleExternalID, me)
	case "lambda":
		save = newSaverLambda(config.lambdaARN, config.lambdaRoleARN, config.lambdaRoleExternalID, me)
	default:
		log.Fatalf("ERROR: unexpected value for SAVE='%s', valid values: server, webhook, sqs, sns, lambda", config.save)
	}

	sessionName := me

	//
	// loop forever if interval greater than 0,
	// run only once otherwise
	//
	for {
		begin := time.Now()

		//
		// scan all accounts
		//
		for i, c := range creds {
			log.Printf("---------- main account %d/%d", i+1, len(creds))

			scan, accountID := newScannerAWS(c.Region, c.RoleArn, c.RoleExternalID, sessionName)

			if accountID == "" {
				log.Printf("ERROR missing accountId=[%s] %d/%d: region=%s role_arn=%s",
					accountID, i+1, len(creds), c.Region, c.RoleArn)
				continue
			}

			findGateways(c, scan, save, accountID, config.debug, config.dryRun, config.saveRetry, config.saveRetryInterval)
		}

		log.Printf("total scan time: %v", time.Since(begin))

		if config.interval == 0 {
			log.Printf("interval is %v, exiting after single run", config.interval)
			break
		}

		log.Printf("sleeping for %v", config.interval)
		time.Sleep(config.interval)
	}
}

func findGateways(cred credential, scan scanner, save saver, accountID string, debug, dryRun bool, retry int, retryInterval time.Duration) {

	const me = "findGateways"

	log.Printf("%s: region=%s role=%s", me, cred.Region, cred.RoleArn)

	items := scan.list()

	type gateway struct {
		count int
		id    string
	}

	tableDedup := map[string]gateway{}

	for _, i := range items {
		gw, found := tableDedup[i.name]
		if !found {
			gw = gateway{id: i.id}
		}
		gw.count++
		tableDedup[i.name] = gw
	}

	var unique []item

	for k, g := range tableDedup {
		if g.count != 1 {
			log.Printf("%s: region=%s role=%s accountId=%s IGNORING dup gateway=%s count=%d",
				me, cred.Region, cred.RoleArn, accountID, k, g.count)
			continue
		}
		unique = append(unique, item{name: k, id: g.id})
	}

	log.Printf("%s: region=%s role=%s accountId=%s gateways_unique: %d",
		me, cred.Region, cred.RoleArn, accountID, len(unique))

	var saved int

	for _, i := range unique {

		gatewayName := i.name
		gatewayID := i.id
		rename := gatewayName

		if len(cred.Only) != 0 {
			//
			// filter is defined
			//

			if gw, found := cred.Only[gatewayName]; found {
				if gw.Rename != "" {
					rename = gw.Rename
				}
			} else {
				if debug {
					log.Printf("%s: region=%s role=%s accountId=%s skipping filtered gateway=%s id=%s",
						me, cred.Region, cred.RoleArn, accountID, gatewayName, gatewayID)
				}
				continue
			}
		}

		full := accountID + ":" + cred.Region + ":" + rename

		log.Printf("%s: region=%s role=%s accountId=%s name=%s rename=%s full=%s ID=%s dry=%t",
			me, cred.Region, cred.RoleArn, accountID, gatewayName, rename, full, gatewayID, dryRun)

		if dryRun {
			continue
		}

		for attempt := 1; attempt <= retry; attempt++ {

			errSave := save.save(full, i.id, debug)
			if errSave == nil {
				saved++
				break
			}

			log.Printf("%s: save attempt=%d/%d region=%s role=%s accountId=%s name=%s rename=%s full=%s ID=%s error: %v",
				me, attempt, retry, cred.Region, cred.RoleArn, accountID, gatewayName, rename, full, gatewayID, errSave)

			if attempt < retry {
				log.Printf("%s: save attempt=%d/%d region=%s role=%s accountId=%s name=%s rename=%s full=%s ID=%s sleeping %v",
					me, attempt, retry, cred.Region, cred.RoleArn, accountID, gatewayName, rename, full, gatewayID, retryInterval)
				time.Sleep(retryInterval)
			}
		}

	}

	log.Printf("%s: region=%s role=%s accountId=%s gateways_saved: %d (dry=%t)",
		me, cred.Region, cred.RoleArn, accountID, saved, dryRun)
}
