package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/goccy/go-json"
	"gopkg.in/yaml.v3"

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

	input := &s3.CreateBucketInput{
		Bucket: aws.String(r.options.bucket),
	}

	// cant specify us-east-1
	if r.options.region != "us-east-1" {
		input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(r.options.region),
		}
	}

	_, errCreate := r.s3Client.CreateBucket(context.TODO(), input)

	if errCreate != nil {
		log.Printf("%s: error: %v", me, errCreate)
		return
	}

	if r.options.debug {
		log.Printf("%s: bucket created: %s", me, r.options.bucket)
	}
}

func (r *repoS3) dropDatabase() error {
	const me = "repoS3.dropDatabase"

	keys, errList := r.listKeys()
	if errList != nil {
		return errList
	}

	var objectIds []s3types.ObjectIdentifier
	for _, key := range keys {
		objectIds = append(objectIds, s3types.ObjectIdentifier{Key: aws.String(key)})
	}
	input := s3.DeleteObjectsInput{
		Bucket: aws.String(r.options.bucket),
		Delete: &s3types.Delete{Objects: objectIds},
	}

	_, err := r.s3Client.DeleteObjects(context.TODO(), &input)

	return err
}

func (r *repoS3) dump() (repoDump, error) {
	const me = "repoS3.dump"

	list := repoDump{}

	keys, errList := r.listKeys()
	if errList != nil {
		return list, errList
	}

	for _, key := range keys {

		gatewayName := strings.TrimPrefix(key, r.options.prefix)
		if gatewayName == "/" {
			continue // skip zero-size folder
		}

		body, errGet := r.get(gatewayName)
		if errGet != nil {
			return list, errGet
		}

		info := map[string]interface{}{
			"gateway_name": body.GatewayName,
			"gateway_id":   body.GatewayID,
			"changes":      body.Changes,
			"last_update":  body.LastUpdate,
			"token":        body.Token,
		}

		list = append(list, info)
	}

	return list, nil
}

func (r *repoS3) listKeys() ([]string, error) {
	var maxKeys int32 = 1000

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(r.options.bucket),
		Prefix: aws.String(r.options.prefix),
	}

	// Create the Paginator for the ListObjectsV2 operation.
	p := s3.NewListObjectsV2Paginator(r.s3Client, input, func(o *s3.ListObjectsV2PaginatorOptions) {
		o.Limit = maxKeys
	})

	var list []string

	for p.HasMorePages() {
		// Next Page takes a new context for each page retrieval. This is where
		// you could add timeouts or deadlines.
		page, errPage := p.NextPage(context.TODO())
		if errPage != nil {
			return list, errPage
		}

		for _, o := range page.Contents {
			key := *o.Key
			list = append(list, key)
		}
	}

	return list, nil
}

func (r *repoS3) get(gatewayName string) (gateboard.BodyGetReply, error) {
	const me = "repoS3.get"

	var body gateboard.BodyGetReply

	if strings.TrimSpace(gatewayName) == "" {
		return body, fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}

	key := r.s3key(gatewayName)

	input := &s3.GetObjectInput{
		Bucket: aws.String(r.options.bucket),
		Key:    aws.String(key),
	}

	result, errS3 := r.s3Client.GetObject(context.TODO(), input)
	if errS3 != nil {

		// not found error?
		var errAPI smithy.APIError
		if errors.As(errS3, &errAPI) {
			switch errAPI.(type) {
			case *s3types.NoSuchBucket, *s3types.NoSuchKey, *s3types.NotFound:
				return body, errRepositoryGatewayNotFound
			}
		}

		return body, errS3
	}

	buf, errRead := io.ReadAll(result.Body)
	if errRead != nil {
		return body, errRead
	}

	// We put as JSON and get as YAML
	errYaml := yaml.Unmarshal(buf, &body)

	return body, errYaml
}

func (r *repoS3) put(gatewayName, gatewayID string) error {
	const me = "repoS3.put"

	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}
	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	//
	// get previous object since we need to increase the changes counter
	//

	body, errGet := r.get(gatewayName)
	switch errGet {
	case nil:
	case errRepositoryGatewayNotFound:
		body.GatewayName = gatewayName
	default:
		return errGet
	}

	//
	// update and save item
	//

	body.GatewayID = gatewayID
	body.LastUpdate = time.Now()
	body.Changes++

	return r.s3put(gatewayName, body)
}

func (r *repoS3) s3key(gatewayName string) string {
	return path.Join(r.options.prefix, gatewayName)
}

func (r *repoS3) s3put(gatewayName string, body gateboard.BodyGetReply) error {

	// We put as JSON and get as YAML
	buf, errMarshal := json.Marshal(body)
	if errMarshal != nil {
		return errMarshal
	}

	key := r.s3key(gatewayName)

	input := &s3.PutObjectInput{
		Bucket: aws.String(r.options.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewBuffer(buf),
	}

	_, errS3 := r.s3Client.PutObject(context.TODO(), input)

	return errS3
}

func (r *repoS3) putToken(gatewayName, token string) error {
	const me = "repoS3.putToken"

	//
	// get previous object since we need to update the token field
	//

	body, errGet := r.get(gatewayName)
	switch errGet {
	case nil:
	case errRepositoryGatewayNotFound:
		body.GatewayName = gatewayName
	default:
		return errGet
	}

	//
	// update and save item
	//

	body.Token = token

	return r.s3put(gatewayName, body)
}
