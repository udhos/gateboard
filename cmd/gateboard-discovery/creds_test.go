package main

import "testing"

// go test -v -run TestCreds ./...
func TestCreds(t *testing.T) {
	{
		_, errCreds := loadCredentials("testdata/discovery-accounts.yaml")
		if errCreds != nil {
			t.Errorf("loading credentials: %v", errCreds)
		}
	}

	_, errCreds := loadCredentials("testdata/discovery-accounts-dup.yaml")
	if errCreds == nil {
		t.Errorf("unxpected loading credentials, should have failed")
	}
}
