package main

type scanner interface {
	list() []item
}

type item struct {
	name string
	id   string
}
