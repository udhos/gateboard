package gateboard

import "testing"

type pickTestCase struct {
	name       string
	list       string
	random     int
	expectedID string
}

var pickTestTable = []pickTestCase{
	{"one", "id1", 1, "id1"},
	{"two 1", "id1:1,id2:2", 1, "id1"},
	{"two 2", "id1:1,id2:2", 2, "id2"},
	{"two 3", "id1:1,id2:2", 3, "id2"},
	{"three 1", "id1:2,id2:3,id3:5", 1, "id1"},
	{"three 2", "id1:2,id2:3,id3:5", 2, "id1"},
	{"three 3", "id1:2,id2:3,id3:5", 3, "id2"},
	{"three 4", "id1:2,id2:3,id3:5", 4, "id2"},
	{"three 5", "id1:2,id2:3,id3:5", 5, "id2"},
	{"three 6", "id1:2,id2:3,id3:5", 6, "id3"},
	{"three 7", "id1:2,id2:3,id3:5", 7, "id3"},
	{"three 8", "id1:2,id2:3,id3:5", 8, "id3"},
	{"three 9", "id1:2,id2:3,id3:5", 9, "id3"},
	{"three 10", "id1:2,id2:3,id3:5", 10, "id3"},
}

// go test -run TestPick ./cmd/gateboard
func TestPick(t *testing.T) {

	for _, data := range pickTestTable {
		list, err := newIDList(data.list)
		if err != nil {
			t.Errorf("%s: list='%s' unexpected error: %v",
				data.name, list, err)
			continue
		}

		pick := pickRandom(list, data.random)

		if pick != data.expectedID {
			t.Errorf("%s: list='%s' random=%d expecting '%s', but got '%s'",
				data.name, data.list, data.random, data.expectedID, pick)
			continue
		}
	}
}
