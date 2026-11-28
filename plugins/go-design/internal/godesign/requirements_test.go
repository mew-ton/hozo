package godesign

import (
	"testing"

	"github.com/mew-ton/hozo/plugins/go-design/internal/spec/godesigntest"
)

// TestRequirements は、go-design 自身の要求（plugins/go-design/spec/go-design.hozo の hozo.godesign.Requirement）を、仕様から作った口
// （godesigntest.RunRequirements。作るのは、go-design 自身）で走らせる。要求を足せば、requirements のメソッドを書くまで、ここが
// コンパイルできない。
func TestRequirements(t *testing.T) {
	godesigntest.RunRequirements(t, requirements{})
}

// requirements は、要求ごとの、テストの入れ物。確かめる中身は golden_test.go の関数。
type requirements struct{}

func (requirements) Targets(t *testing.T) {
	t.Run("unitTargets", unitTargets)
	t.Run("warnNothing", warnNothing)
}

func (requirements) Output(t *testing.T) {
	t.Run("contents", outputContents)
}

func (requirements) FlowPort(t *testing.T) {
	t.Run("contents", flowContents)
	t.Run("behavior", flowBehavior)
	t.Run("mismatch", flowMismatch)
	t.Run("incomplete", func(t *testing.T) { incomplete(t, flowOmissions) })
}

func (requirements) RequirementPort(t *testing.T) {
	t.Run("contents", requirementContents)
	t.Run("behavior", requirementBehavior)
	t.Run("incomplete", func(t *testing.T) { incomplete(t, requirementOmissions) })
}

func (requirements) Designs(t *testing.T) {
	t.Run("contents", designContents)
	t.Run("warnUnreferenced", warnUnreferenced)
}
