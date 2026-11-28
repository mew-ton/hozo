package tests

import (
	"strings"
	"sync"
	"testing"

	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
)

// このファイルは、検査の規則と診断の持ち主（仕様の名前空間 hozo.diagnostics の分類のケース。spec/diagnostics.hozo と
// spec/diagnostics/ の下）を読む。tests/codes_test.go と tests/rules_test.go が、ここから ID と重さを引く。

// diagnosticsNamespace は、診断を要求として持つ名前空間。
const diagnosticsNamespace = "hozo.diagnostics"

// specDiagnostic は、仕様のケース一つ。
type specDiagnostic struct {
	// id は診断の ID（ケースの名前の _ を - に戻したもの。spec/diagnostics.hozo の「穴 30」）。
	id string
	// class は分類の名前（Rule、Hozoc、GoHttp など）。
	class string
	// sev は重さ: "error" / "warning" / "—"（診断を出さない）。欠番なら空。
	sev string
	// retired は欠番（@retired）か。
	retired bool
}

var (
	specDiagnosticsOnce sync.Once
	specDiagnosticsList []specDiagnostic
	specDiagnosticsErr  string
)

// specDiagnostics は、リポジトリのルートの仕様を読んで、hozo.diagnostics の分類のケースを、宣言順に返す。
// どのケースも、重さ（@severity）か欠番（@retired）の、どちらかを、ちょうど一つ持つこと（穴 31）と、ID が分類をまたいで
// 重ならないことも、ここで確かめる。
func specDiagnostics(t *testing.T) []specDiagnostic {
	t.Helper()
	specDiagnosticsOnce.Do(func() {
		p := project.Load("..", "", nil)
		if p.Sema == nil || p.Sema.Program == nil {
			specDiagnosticsErr = "リポジトリのルートの仕様を読めない"
			return
		}
		var problems []string
		seen := map[string]string{}
		for _, ns := range p.Sema.Program.Namespaces {
			if ns.Name != diagnosticsNamespace {
				continue
			}
			for _, d := range ns.Decls {
				c, ok := d.(*ir.Classify)
				class := ""
				if ok {
					class = strings.TrimPrefix(c.Name, diagnosticsNamespace+".")
				}
				if !ok || class == "Severity" {
					continue
				}
				for _, cs := range c.Cases {
					sd := specDiagnostic{id: strings.ReplaceAll(cs.Name, "_", "-"), class: class}
					marks := 0
					for _, app := range cs.Decorators {
						switch app.Decorator {
						case diagnosticsNamespace + ".severity":
							marks++
							sd.sev = severityArg(app)
						case diagnosticsNamespace + ".retired":
							marks++
							sd.retired = true
						}
					}
					if marks != 1 || (!sd.retired && sd.sev == "") {
						problems = append(problems, sd.class+"."+cs.Name+" は、重さ（@severity）か欠番（@retired）の、どちらかを、ちょうど一つ持たない")
					}
					if prev, dup := seen[sd.id]; dup {
						problems = append(problems, sd.id+" が、二つの分類にある（"+prev+" と "+sd.class+"）")
					}
					seen[sd.id] = sd.class
					specDiagnosticsList = append(specDiagnosticsList, sd)
				}
			}
		}
		if len(specDiagnosticsList) == 0 {
			problems = append(problems, "名前空間 "+diagnosticsNamespace+" から、ケースを一つも拾えなかった")
		}
		specDiagnosticsErr = strings.Join(problems, "\n")
	})
	if specDiagnosticsErr != "" {
		t.Fatal(specDiagnosticsErr)
	}
	return specDiagnosticsList
}

// severityArg は @severity の引数（分類 Severity のケース）を、診断の重さの綴りにする。
func severityArg(app ir.Application) string {
	for _, a := range app.Args {
		if v, ok := a.Value.(ir.CaseValue); ok {
			switch v.Case {
			case "Error":
				return "error"
			case "Warning":
				return "warning"
			case "Silent":
				return "—"
			}
		}
	}
	return ""
}

// TestDocsHaveNoDiagnosticTables は、文書（docs/）が、診断の条件と重さの表を持たないこと: 持ち主は、仕様のケース
// （決定 D812）で、文書に表を足せば、持ち主が二つになる。
func TestDocsHaveNoDiagnosticTables(t *testing.T) {
	_, noSeverity, withSeverity := docTables(t)
	for _, at := range append(withSeverity, noSeverity...) {
		t.Errorf("docs/%s に、診断の ID の表がある。条件と重さは、spec/diagnostics/ のケースに書く", at)
	}
}
