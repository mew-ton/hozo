package govalid

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// TestCheckMatchesEmit は、direction "check" が、emit と同じ誤りを返すこと（docs/plugin-protocol.md「check」、
// docs/decisions.md「決めたこと」404）: testdata のすべての入力の、このトランスパイラを並べた単位の View ごとに、
// Check の diagnostics が、Emit の errors（severity は "error"）と、同じ並び。Emit が誤りを返す View では、Check も誤りを返す。
func TestCheckMatchesEmit(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{}
	views, withErrors := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join("testdata", e.Name(), "views.json")); err != nil {
			continue
		}
		for _, view := range loadViews(t, e.Name()) {
			mine := false
			for _, name := range view.Config.Transpilers {
				mine = mine || name == TranspilerName
			}
			if !mine {
				continue
			}
			views++
			emit, emitErr := h.Emit(&plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: TranspilerName, View: view})
			check, checkErr := h.Check(&plugin.CheckRequest{Protocol: ir.Version, Direction: "check", View: view})
			if emitErr != nil || checkErr != nil {
				if (emitErr == nil) != (checkErr == nil) {
					t.Errorf("%s（%s）: Emit の誤り = %v、Check の誤り = %v（どちらも誤りか、どちらも誤りでないはず）", e.Name(), view.Viewer, emitErr, checkErr)
				}
				continue
			}
			want := []plugin.Diagnostic{}
			for _, d := range emit.Errors {
				d.Severity = "error"
				want = append(want, d)
			}
			if len(want) > 0 {
				withErrors++
			}
			if !reflect.DeepEqual(check.Diagnostics, want) {
				t.Errorf("%s（%s）: Check の diagnostics が、Emit の errors と違う\n got: %+v\nwant: %+v", e.Name(), view.Viewer, check.Diagnostics, want)
			}
		}
	}
	if views == 0 || withErrors == 0 {
		t.Fatalf("確かめた View が %d、誤りを返した View が %d（どちらも、一つ以上のはず）", views, withErrors)
	}
}
