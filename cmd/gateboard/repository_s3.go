package main

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/udhos/boilerplate/awsconfig"
	"github.com/udhos/gateboard/gateboard"
)

type repoS3Options struct {
	bucket       string
	region       string
	prefix       string
	roleArn      string
	sessionName  string
	debug        bool
	manualCreate bool
}

type repoS3 struct {
	options  repoS3Options
	s3Client *s3.Client
}

func newRepoS3(opt repoS3Options) (*repoS3, error) {
	const me = "newRepoS3"

	awsConfOptions := awsconfig.Options{
		Region:          opt.region,
		RoleArn:         opt.roleArn,
		RoleSessionName: opt.sessionName,
	}

	cfg, errAwsConfig := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConfig != nil {
		return nil, errAwsConfig
	}

	r := &repoS3{
		options:  opt,
		s3Client: s3.NewFromConfig(cfg.AwsConfig),
	}

	if !r.options.manualCreate {
		r.createBucket()
	}

	return r, nil
}

func (r *repoS3) createBucket() {
	const me = "repoS3.createBucket"

	log.Printf("%s: FIXME WRITEME", me)
}

func (r *repoS3) dropDatabase() error {
	const me = "repoS3.dropDatabase"

	return fmt.Errorf("%s: FIXME WRITEME", me)
}

func (r *repoS3) dump() (repoDump, error) {
	const me = "repoS3.dump"

	list := repoDump{}

	return list, fmt.Errorf("%s: FIXME WRITEME", me)
}

func (r *repoS3) get(gatewayName string) (gateboard.BodyGetReply, error) {
	const me = "repoS3.get"

	var body gateboard.BodyGetReply

	return body, fmt.Errorf("%s: FIXME WRITEME", me)
}

func (r *repoS3) put(gatewayName, gatewayID string) error {
	const me = "repoS3.put"

	return fmt.Errorf("%s: FIXME WRITEME", me)
}

func (r *repoS3) putToken(gatewayName, token string) error {
	const me = "repoS3.putToken"

	return fmt.Errorf("%s: FIXME WRITEME", me)
}
