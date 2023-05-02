#!/bin/bash

echo RUN mongo: docker run --rm --name mongo-main -p 27017:27017 mongo
echo RUN redis: docker run --rm --name redis-main -p 6379:6379 redis
echo
echo hit ENTER to continue...
echo 

read i

export TEST_REPO_MONGO=true ;# enable mongodb tests
export TEST_REPO_DYNAMO=true ;# enable dynamodb tests
export TEST_REPO_REDIS=true ;# enable redis tests
export TEST_REPO_S3=pub_bucket_name_here ;# enable S3 tests

go test -count=1 -run TestRepository ./cmd/gateboard
