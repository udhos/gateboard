// Package awsconfig creates aws.Config.
package awsconfig

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AwsConfig creates one aws.Config
func AwsConfig(region, roleArn, roleSessionName string) aws.Config {
	const me = "AwsConfig"

	cfg, errConfig := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region))
	if errConfig != nil {
		log.Fatalf("%s: load config: %v", me, errConfig)
	}

	if roleArn != "" {
		//
		// AssumeRole
		//
		log.Printf("%s: AssumeRole: arn: %s", me, roleArn)
		clientSts := sts.NewFromConfig(cfg)
		cfg2, errConfig2 := config.LoadDefaultConfig(
			context.TODO(), config.WithRegion(region),
			config.WithCredentialsProvider(aws.NewCredentialsCache(
				stscreds.NewAssumeRoleProvider(
					clientSts,
					roleArn,
					func(o *stscreds.AssumeRoleOptions) {
						o.RoleSessionName = roleSessionName
					},
				)),
			),
		)
		if errConfig2 != nil {
			log.Fatalf("%s: AssumeRole %s: error: %v", me, roleArn, errConfig2)
		}
		cfg = cfg2
	}

	{
		// show caller identity
		clientSts := sts.NewFromConfig(cfg)
		input := sts.GetCallerIdentityInput{}
		respSts, errSts := clientSts.GetCallerIdentity(context.TODO(), &input)
		if errSts != nil {
			log.Printf("%s: GetCallerIdentity: error: %v", me, errSts)
		} else {
			log.Printf("%s: GetCallerIdentity: Account=%s ARN=%s UserId=%s", me, *respSts.Account, *respSts.Arn, *respSts.UserId)
		}
	}

	return cfg
}
