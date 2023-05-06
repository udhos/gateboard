package gateboard

import (
	"github.com/udhos/boilerplate/envconfig"
)

// NewEnv creates a env context for retrieving parameters.
func NewEnv(sessionName string) *envconfig.Env {
	return envconfig.NewSimple(sessionName)
}
