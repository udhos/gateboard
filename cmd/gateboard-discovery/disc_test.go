package main

import (
	"strings"
	"testing"
)

// go test -v -run TestDiscovery ./cmd/gateboard-discovery
func TestDiscovery(t *testing.T) {

	accountID := "123456789012"

	scan := &bogusScanner{
		items: []item{
			{name: "eraseme", id: "id0"},
			{name: "eraseme2", id: "id2"},
			{name: "eraseme2", id: "id2dup"},
			{name: "eraseme3", id: "id3"},
			{name: "eraseme4", id: "id4"},
		},
	}

	save := &bogusSaver{}

	debug := true
	dryRun := false

	credStr := `
- role_arn: "" # empty role_arn means use current credentials
  region: us-east-1
  role_external_id: ""
  # if section 'only' is provided, only these gateways will be accepted
  only:
    eraseme: # accept gateway named 'eraseme'
      rename: gw1 # rename gateway to 'gw1' before saving into server
    eraseme2:
      rename: gw2
    eraseme3: {} # do not rename
    eraseme5: {}
`

	creds, errCred := loadCredentialsFromReader(strings.NewReader(credStr))
	if errCred != nil {
		t.Errorf("error loading credentials: %v", errCred)
	}

	for _, c := range creds {
		findGateways(c, scan, save, accountID, debug, dryRun)
	}

	for i, g := range save.items {
		t.Logf("saved %d: name=%s id=%s", i, g.name, g.id)
	}

	if len(save.items) != 2 {
		t.Errorf("expecting 2 saved items, got %d", len(save.items))
	}

	tab := map[string]string{}
	for _, i := range save.items {
		tab[i.name] = i.id
	}

	{
		const k = "123456789012:us-east-1:gw1"
		const v = "id0"
		if tab[k] != v {
			t.Errorf("unexpected 1st item: %s => %s (expected: %s)", k, tab[k], v)
		}
	}

	{
		const k = "123456789012:us-east-1:eraseme3"
		const v = "id3"
		if tab[k] != v {
			t.Errorf("unexpected 2nd item: %v => %v (expected: %s)", k, tab[k], v)
		}
	}
}

type bogusScanner struct {
	items []item
}

func (s *bogusScanner) list() []item {
	return s.items
}

type bogusSaver struct {
	items []item
}

func (s *bogusSaver) save(name, id string, debug bool) {
	s.items = append(s.items, item{name: name, id: id})
}
