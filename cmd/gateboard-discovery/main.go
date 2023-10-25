/*
This is the main package for gateboard-discovery service.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/otelconfig/oteltrace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const version = "1.7.1"

func main() {

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	config := newConfig(me)

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tr, cancel, errTracer := oteltrace.TraceStart(options)

		if errTracer != nil {
			log.Fatalf("tracer: %v", errTracer)
		}

		defer cancel()

		tracer = tr
	}

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
		save = newSaverSQS(config.queueURL, config.queueRoleARN, config.queueRoleExternalID, me, newSqsClient)
	case "sns":
		save = newSaverSNS(config.topicARN, config.topicRoleARN, config.topicRoleExternalID, me, newSnsClient)
	case "lambda":
		save = newSaverLambda(config.lambdaARN, config.lambdaRoleARN, config.lambdaRoleExternalID, me, newLambdaClient)
	default:
		log.Fatalf("ERROR: unexpected value for SAVE='%s', valid values: server, webhook, sqs, sns, lambda", config.save)
	}

	//
	// loop forever if interval greater than 0,
	// run only once otherwise
	//
	for {
		scanOnce(me, tracer, creds, config, save)

		if config.interval == 0 {
			log.Printf("interval is %v, exiting after single run", config.interval)
			break
		}

		log.Printf("sleeping for %v", config.interval)
		time.Sleep(config.interval)
	}
}

func scanOnce(sessionName string, tracer trace.Tracer, creds []credential, config appConfig, save saver) {

	const me = "scanOnce"

	ctx, span := newSpan(context.TODO(), me, tracer)
	if span != nil {
		defer span.End()
	}

	begin := time.Now()

	//
	// scan all accounts
	//
	for i, c := range creds {
		log.Printf("---------- main account %d/%d", i+1, len(creds))

		scan, accountID := newScannerAWS(ctx, tracer, c.Region, c.RoleArn, c.RoleExternalID, sessionName)

		if accountID == "" {
			log.Printf("ERROR missing accountId=[%s] %d/%d: region=%s role_arn=%s",
				accountID, i+1, len(creds), c.Region, c.RoleArn)
			continue
		}

		findGateways(ctx, tracer, c, scan, save, accountID, config.debug, config.dryRun, config.saveRetry, config.saveRetryInterval)
	}

	log.Printf("total scan time: %v", time.Since(begin))
}

func dedup(items []item, region, roleArn, accountID string) []item {

	const me = "dedup"

	type dedupGateway struct {
		count int
		id    string
	}

	tableDedup := map[string]dedupGateway{}

	for _, i := range items {
		gw, found := tableDedup[i.name]
		if !found {
			gw = dedupGateway{id: i.id}
		}
		gw.count++
		tableDedup[i.name] = gw
	}

	var unique []item

	for k, g := range tableDedup {
		if g.count != 1 {
			log.Printf("%s: region=%s role=%s accountId=%s ignoring DUP gateway=%s count=%d",
				me, region, roleArn, accountID, k, g.count)
			continue
		}
		unique = append(unique, item{name: k, id: g.id})
	}

	return unique
}

func findGateways(ctx context.Context, tracer trace.Tracer, cred credential, scan scanner, save saver, accountID string, debug, dryRun bool, retry int, retryInterval time.Duration) {

	const me = "findGateways"

	ctxNew, span := newSpan(ctx, me, tracer)
	if span != nil {
		span.SetAttributes(attribute.String("region", cred.Region), attribute.String("roleArn", cred.RoleArn))
		defer span.End()
	}

	log.Printf("%s: region=%s role=%s", me, cred.Region, cred.RoleArn)

	items := scan.list(ctxNew, tracer)

	unique := dedup(items, cred.Region, cred.RoleArn, accountID)

	log.Printf("%s: region=%s role=%s accountId=%s gateways_unique: %d",
		me, cred.Region, cred.RoleArn, accountID, len(unique))

	var saved int

	for _, i := range unique {

		gatewayName := i.name
		gatewayID := i.id
		rename := gatewayName
		writeToken := cred.DefaultToken

		if len(cred.Only) != 0 {
			//
			// filter is defined
			//

			if gw, found := cred.Only[gatewayName]; found {
				if gw.Rename != "" {
					rename = gw.Rename
				}
				if gw.Token != "" {
					writeToken = gw.Token
				}
			} else {
				if debug {
					log.Printf("%s: region=%s role=%s accountId=%s skipping FILTERED OUT gateway=%s id=%s",
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

			errSave := callSave(ctxNew, tracer, save, full, gatewayID, writeToken, debug)
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

func callSave(ctx context.Context, tracer trace.Tracer, save saver, name, id, writeToken string, debug bool) error {

	const me = "callSave"

	ctxNew, span := newSpan(ctx, me, tracer)
	if span != nil {
		span.SetAttributes(attribute.String("gateway_name", name), attribute.String("gateway_id", id))
		defer span.End()
	}

	return save.save(ctxNew, tracer, name, id, writeToken, debug)
}
