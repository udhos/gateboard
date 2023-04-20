package gateboard

import (
	"log"
	"os"

	"github.com/udhos/boilerplate/envconfig"
	"github.com/udhos/boilerplate/secret"
)

// NewEnv creates a env context for retrieving parameters.
func NewEnv(sessionName string) *envconfig.Env {
	roleArn := os.Getenv("SECRET_ROLE_ARN")

	log.Printf("SECRET_ROLE_ARN='%s'", roleArn)

	secretOptions := secret.Options{
		RoleSessionName: sessionName,
		RoleArn:         roleArn,
	}
	secret := secret.New(secretOptions)
	envOptions := envconfig.Options{
		Secret: secret,
	}
	env := envconfig.New(envOptions)
	return env
}
