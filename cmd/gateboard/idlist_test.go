package main

import "testing"

const (
	expectError expectResult = iota
	expectSuccess
)

type expectResult int

type idEntryTestCase struct {
	name          string
	value         string
	expectedEntry idEntry
	expectedError expectResult
}

var idEntryTestTable = []idEntryTestCase{
	{"empty", "", idEntry{}, expectError},
	{"only sep", ":", idEntry{}, expectError},
	{"only id", "id", idEntry{id: "id", weight: 1}, expectSuccess},
	{"missing id", ":3", idEntry{}, expectError},
	{"missing weight", "id:", idEntry{}, expectError},
	{"id plus weight one", "id:1", idEntry{id: "id", weight: 1}, expectSuccess},
	{"id plus weight one", "id:2", idEntry{id: "id", weight: 2}, expectSuccess},
	{"id plus weight zero", "id:0", idEntry{}, expectError},
	{"id plus weight negative", "id:-3", idEntry{}, expectError},

	{"with blanks - empty", " ", idEntry{}, expectError},
	{"with blanks - only sep", " : ", idEntry{}, expectError},
	{"with blanks - only id", " id ", idEntry{id: "id", weight: 1}, expectSuccess},
	{"with blanks - missing id", " : 3 ", idEntry{}, expectError},
	{"with blanks - missing weight", " id : ", idEntry{}, expectError},
	{"with blanks - id plus weight one", " id : 1 ", idEntry{id: "id", weight: 1}, expectSuccess},
	{"with blanks - id plus weight one", " id : 2 ", idEntry{id: "id", weight: 2}, expectSuccess},
	{"with blanks - id plus weight zero", " id : 0 ", idEntry{}, expectError},
	{"with blanks - id plus weight negative", " id : -3 ", idEntry{}, expectError},
}

// go test -run TestIdEntry ./cmd/gateboard
func TestIdEntry(t *testing.T) {

	for _, data := range idEntryTestTable {
		entry, err := newIDEntry(data.value)
		if err != nil {
			if data.expectedError == expectSuccess {
				t.Errorf("%s: value='%s' expecting success, but got error: %v",
					data.name, data.value, err)
			}
			continue
		}

		if data.expectedError == expectError {
			t.Errorf("%s: value='%s' expecting error, but got success",
				data.name, data.value)
			continue
		}

		if data.expectedEntry != entry {
			t.Errorf("%s: value='%s' expecting '%v', but got '%v'",
				data.name, data.value, data.expectedEntry, entry)
		}
	}
}

type idEntryTestCaseString struct {
	name           string
	entry          idEntry
	expectedString string
}

var idEntryTestStringTable = []idEntryTestCaseString{
	{"only id", idEntry{id: "id", weight: 1}, "id"},
	{"id plus weight", idEntry{id: "id", weight: 2}, "id:2"},
}

// go test -run TestIdEntryString ./cmd/gateboard
func TestIdEntryString(t *testing.T) {
	for _, data := range idEntryTestStringTable {
		str := data.entry.String()
		if data.expectedString != str {
			t.Errorf("%s: entry=%v expecting '%s', but got '%s'",
				data.name, data.entry, data.expectedString, str)
		}
	}
}
