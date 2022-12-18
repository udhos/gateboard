package gateboard

import (
	"fmt"
	"strconv"
	"strings"
)

type idList struct {
	list []idEntry
	sum  int
}

type idEntry struct {
	id     string
	weight int
	sum    int
}

func newIDEntry(s string) (idEntry, error) {
	id, weight, hasSep := strings.Cut(s, ":")
	id = strings.TrimSpace(id)
	if id == "" {
		return idEntry{}, fmt.Errorf("invalid blank id")
	}
	if !hasSep {
		return idEntry{id: id, weight: 1}, nil
	}
	weight = strings.TrimSpace(weight)
	w, errConv := strconv.Atoi(weight)
	if errConv != nil {
		return idEntry{}, errConv
	}
	if w < 1 {
		return idEntry{}, fmt.Errorf("invalid weight: %d", w)
	}
	return idEntry{id: id, weight: w}, nil
}

func (e idEntry) String() string {
	if e.weight == 1 {
		return e.id
	}
	return fmt.Sprintf("%s:%d", e.id, e.weight)
}

func newIDList(s string) (idList, error) {
	list := strings.Split(s, ",")
	var result idList
	for _, i := range list {
		e, err := newIDEntry(i)
		if err != nil {
			return result, err
		}
		result.list = append(result.list, e)
		result.sum += e.weight
	}
	return result, nil
}

func (l idList) String() string {
	list := make([]string, 0, len(l.list))
	for _, e := range l.list {
		list = append(list, e.String())
	}
	return strings.Join(list, ",")
}
