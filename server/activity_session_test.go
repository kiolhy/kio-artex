package server

import "testing"

func TestParseActivitySessionAll(t *testing.T) {
	filter, ok := parseActivitySession("all")
	if !ok {
		t.Fatal("all should be a valid activity session")
	}
	if filter.Main || filter.Worker != "" || filter.NodeID != nil || filter.MainSeg != nil {
		t.Fatalf("all should select the whole task, got %+v", filter)
	}
}

func TestParseActivitySessionDefaultRemainsMain(t *testing.T) {
	filter, ok := parseActivitySession("")
	if !ok || !filter.Main || filter.MainSeg != nil {
		t.Fatalf("empty session should remain current main segment, got %+v ok=%v", filter, ok)
	}
}
