package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/udhos/boilerplate/secret"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"gopkg.in/yaml.v3"
)

type repoConfig struct {
	Kind     string          `json:"kind"               yaml:"kind"` // mem | mongo | redis | dynamodb | s3
	Name     string          `json:"name"               yaml:"name"`
	Mongo    *mongoConfig    `json:"mongo,omitempty"    yaml:"mongo,omitempty"`
	DynamoDB *dynamoDBConfig `json:"dynamodb,omitempty" yaml:"dynamodb,omitempty"`
	Redis    *redisConfig    `json:"redis,omitempty"    yaml:"redis,omitempty"`
	S3       *s3Config       `json:"s3,omitempty"       yaml:"s3,omitempty"`
	Mem      memConfig       `json:"mem,omitempty"      yaml:"mem,omitempty"`
}

type mongoConfig struct {
	URI        string `json:"uri"         yaml:"uri"`
	Database   string `json:"database"    yaml:"database"`
	Collection string `json:"collection"  yaml:"collection"`
	Username   string `json:"username"    yaml:"username"`
	Password   string `json:"password"    yaml:"password"`
	TLSCaFile  string `json:"tls_ca_file" yaml:"tls_ca_file"`
	MinPool    uint64 `json:"min_pool"    yaml:"min_pool"`
}

type dynamoDBConfig struct {
	Table        string `json:"table"         yaml:"table"`
	Region       string `json:"region"        yaml:"region"`
	RoleArn      string `json:"role_arn"      yaml:"role_arn"`
	ManualCreate bool   `json:"manual_create" yaml:"manual_create"`
}

type redisConfig struct {
	Addr     string `json:"addr"     yaml:"addr"`
	Password string `json:"password" yaml:"password"`
	Key      string `json:"key"      yaml:"key"`
}

type s3Config struct {
	BucketName           string `json:"bucket_name"            yaml:"bucket_name"`
	BucketRegion         string `json:"bucket_region"          yaml:"bucket_region"`
	Prefix               string `json:"prefix"                 yaml:"prefix"`
	RoleArn              string `json:"role_arn"               yaml:"role_arn"`
	ManualCreate         bool   `json:"manual_create"          yaml:"manual_create"`
	ServerSideEncryption string `json:"server_side_encryption" yaml:"server_side_encryption"`
}

type memConfig struct {
	Broken bool          `json:"broken" yaml:"broken"`
	Delay  time.Duration `json:"delay"  yaml:"delay"`
}

func loadRepoConf(input string) ([]repoConfig, error) {

	const me = "loadRepoConf"

	reader, errOpen := os.Open(input)
	if errOpen != nil {
		return nil, fmt.Errorf("%s: open file: %s: %v", me, input, errOpen)
	}

	buf, errRead := io.ReadAll(reader)
	if errRead != nil {
		return nil, fmt.Errorf("%s: read file: %s: %v", me, input, errRead)
	}

	var conf []repoConfig

	errYaml := yaml.Unmarshal(buf, &conf)
	if errYaml != nil {
		return conf, fmt.Errorf("%s: parse yaml: %s: %v", me, input, errYaml)
	}

	return conf, nil
}

func createRepo(sessionName, secretRoleArn string, config repoConfig, debug bool) repository {

	const me = "createRepo"

	sec := secret.New(secret.Options{
		RoleSessionName: sessionName,
		RoleArn:         secretRoleArn,
	})

	kind := config.Kind
	metricRepoName := kind + ":" + config.Name

	switch kind {
	case "mongo":
		repo, errMongo := newRepoMongo(repoMongoOptions{
			metricRepoName: metricRepoName,
			debug:          debug,
			URI:            config.Mongo.URI,
			database:       config.Mongo.Database,
			collection:     config.Mongo.Collection,
			username:       config.Mongo.Username,
			password:       sec.Retrieve(config.Mongo.Password),
			tlsCAFile:      config.Mongo.TLSCaFile,
			minPool:        config.Mongo.MinPool,
			timeout:        time.Second * 10,
		})
		if errMongo != nil {
			zlog.Fatalf("%s: repo mongo: %v", me, errMongo)
		}
		return repo
	case "dynamodb":
		repo, errDynamo := newRepoDynamo(repoDynamoOptions{
			metricRepoName: metricRepoName,
			debug:          debug,
			table:          config.DynamoDB.Table,
			region:         config.DynamoDB.Region,
			roleArn:        config.DynamoDB.RoleArn,
			manualCreate:   config.DynamoDB.ManualCreate,
			sessionName:    sessionName,
		})
		if errDynamo != nil {
			zlog.Fatalf("%s: repo dynamodb: %v", me, errDynamo)
		}
		return repo
	case "redis":
		repo, errRedis := newRepoRedis(repoRedisOptions{
			metricRepoName: metricRepoName,
			debug:          debug,
			addr:           config.Redis.Addr,
			password:       sec.Retrieve(config.Redis.Password),
			key:            config.Redis.Key,
		})
		if errRedis != nil {
			zlog.Fatalf("%s: repo redis: %v", me, errRedis)
		}
		return repo
	case "mem":
		return newRepoMem(repoMemOptions{
			metricRepoName: metricRepoName,
			broken:         config.Mem.Broken,
			delay:          config.Mem.Delay,
		})
	case "s3":
		repo, errS3 := newRepoS3(repoS3Options{
			metricRepoName:       metricRepoName,
			debug:                debug,
			bucket:               config.S3.BucketName,
			region:               config.S3.BucketRegion,
			prefix:               config.S3.Prefix,
			roleArn:              config.S3.RoleArn,
			manualCreate:         config.S3.ManualCreate,
			serverSideEncryption: config.S3.ServerSideEncryption,
			sessionName:          sessionName,
		})
		if errS3 != nil {
			zlog.Fatalf("%s: repo s3: %v", me, errS3)
		}
		return repo
	}

	zlog.Fatalf("%s: unsupported repo type: %s (supported types: mongo, dynamodb, mem, redis, s3)", me, kind)

	return nil
}
