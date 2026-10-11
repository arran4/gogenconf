package testutil

import (
	"testing"

	"golang.org/x/tools/txtar"
)

func TestTreeRejectsInvalidPaths(t *testing.T) {
	_, err := Tree(&txtar.Archive{Files: []txtar.File{{Name: "input/../escape", Data: []byte("x")}}}, "input")
	if err == nil {
		t.Fatal("accepted an invalid projected path")
	}
}

func TestTreeSeparatesInputAndExpected(t *testing.T) {
	ar := &txtar.Archive{Files: []txtar.File{{Name: "input/a.txt", Data: []byte("in")}, {Name: "expected/a.txt", Data: []byte("out")}}}
	in, err := Tree(ar, "input")
	if err != nil || string(in["a.txt"].Data) != "in" {
		t.Fatalf("input = %v, %v", in, err)
	}
	out, err := Tree(ar, "expected")
	if err != nil || string(out["a.txt"].Data) != "out" {
		t.Fatalf("expected = %v, %v", out, err)
	}
}
