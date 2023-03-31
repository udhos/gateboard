package env

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/udhos/gateboard/awsconfig"
)

// secretsmanager:region:name:json_field
func secretsManagerGet(key, roleArn, roleSessionName string) string {
	const me = "secretsManagerGet"

	//
	// parse key: secretsmanager:region:name:json_field
	//

	fields := strings.SplitN(key, ":", 4)
	if len(fields) < 3 {
		log.Printf("%s: missing fields: %s", me, key)
		return key
	}

	if fields[0] != "secretsmanager" {
		log.Printf("%s: missing prefix: %s", me, key)
		return key
	}

	region := fields[1]
	secretName := fields[2]
	var jsonField string
	if len(fields) > 3 {
		jsonField = fields[3]
	}

	log.Printf("%s: key=%s region=%s json_field=%s role_arn=%s session=%s",
		me, key, region, jsonField, roleArn, roleSessionName)

	//
	// retrieve secret
	//

	secretString, errSecret := retrieve(roleArn, roleSessionName, region, secretName, jsonField)
	if errSecret != nil {
		log.Printf("%s: secret error: %v", me, errSecret)
		return key
	}

	if jsonField == "" {
		// return scalar (non-JSON) secret
		log.Printf("%s: key=%s region=%s json_field=%s role_arn=%s session=%s: value=%s",
			me, key, region, jsonField, roleArn, roleSessionName, secretString)
		return secretString
	}

	//
	// extract field from secret in JSON
	//

	value := map[string]string{}

	errJSON := json.Unmarshal([]byte(secretString), &value)
	if errJSON != nil {
		log.Printf("%s: json error: %v", me, errJSON)
		return secretString
	}

	fieldValue := value[jsonField]

	log.Printf("%s: key=%s region=%s json_field=%s role_arn=%s session=%s: value=%s",
		me, key, region, jsonField, roleArn, roleSessionName, fieldValue)

	return fieldValue
}

//
// We only cache secrets with JSON fields:
//
//     {"uri":"mongodb://127.0.0.2:27017", "database":"bogus"}
//
// In order to fetch multiple fields from a secret with a single (cached)
// query against AWS Secrets Manager:
//
//     export      MONGO_URL=secretsmanager:us-east-1:mongo:uri
//     export MONGO_DATABASE=secretsmanager:us-east-1:mongo:database
//

var cache = map[string]secret{}

type secret struct {
	value   string
	created time.Time
}

func retrieve(roleArn, roleSessionName, region, secretName, field string) (string, error) {
	const (
		me       = "retrieve"
		cacheTTL = time.Minute
	)

	var cacheKey string
	var secretString string

	if field != "" {
		//
		// check cache, only for JSON values
		//
		cacheKey = region + ":" + secretName
		cached, found := cache[cacheKey]
		if found {
			elapsed := time.Since(cached.created)
			if elapsed < cacheTTL {
				secretString = cached.value
				log.Printf("%s: from cache: %s=%s (elapsed=%s TTL=%s)",
					me, cacheKey, secretString, elapsed, cacheTTL)
			} else {
				delete(cache, cacheKey)
			}
		}
	}

	if secretString == "" {
		//
		// load aws config
		//
		cfg := awsconfig.AwsConfig(region, roleArn, roleSessionName)
		sm := secretsmanager.NewFromConfig(cfg)

		//
		// retrieve from secrets manager
		//
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
		}
		result, errSecret := sm.GetSecretValue(context.TODO(), input)
		if errSecret != nil {
			log.Printf("%s: secret error: %v", me, errSecret)
			return "", errSecret
		}
		secretString = *result.SecretString

		log.Printf("%s: from secretsmanager: %s=%s", me, secretName, secretString)

		if field != "" {
			//
			// save to cache
			//
			cache[cacheKey] = secret{
				value:   secretString,
				created: time.Now(),
			}
		}
	}

	return secretString, nil
}
