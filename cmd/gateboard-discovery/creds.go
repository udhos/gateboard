package main

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type credential struct {
	RoleArn        string                 `yaml:"role_arn"`
	RoleExternalID string                 `yaml:"role_external_id"`
	Region         string                 `yaml:"region"`
	Only           map[string]credGateway `yaml:"only"`
	DefaultToken   string                 `yaml:"default_token"` // default write token, if required by server
}

type credGateway struct {
	Rename string `yaml:"rename"`
	Token  string `yaml:"token"` // per-gateway write token, if required by server
}

func loadCredentials(input string) ([]credential, error) {

	reader, errOpen := os.Open(input)
	if errOpen != nil {
		return nil, fmt.Errorf("loadCredentials: read file: %s: %v", input, errOpen)
	}

	defer reader.Close()

	return loadCredentialsFromReader(reader)
}

func loadCredentialsFromReader(reader io.Reader) ([]credential, error) {
	var creds []credential

	buf, errRead := io.ReadAll(reader)
	if errRead != nil {
		return nil, fmt.Errorf("loadCredentialsFromReader: read: %v", errRead)
	}

	errYaml := yaml.Unmarshal(buf, &creds)
	if errYaml != nil {
		return creds, fmt.Errorf("loadCredentialsFromReader: parse yaml: %v", errYaml)
	}

	table := map[string]bool{}

	for i, c := range creds {
		for name, cg := range c.Only {
			rename := cg.Rename
			if rename == "" {
				continue
			}
			if table[rename] {
				return creds, fmt.Errorf("loadCredentialsFromReader: dup rename: item=%d gateway=%s rename=%s",
					i, name, rename)
			}
			table[rename] = true
		}
	}

	return creds, nil
}
