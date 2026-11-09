package ir

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSmokeDump(t *testing.T) {
	b, err := json.MarshalIndent(sampleView(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))
}
