package gobase

import (
	"testing"

	"github.com/mew-ton/hozo/plugins/go/internal/spec/gobasetest"
)

// TestRequirements は、go 自身の要求（plugins/go/spec/go.hozo の hozo.gobase.Requirement）を、仕様から作った口
// （gobasetest.RunRequirements）で走らせる。要求を足せば、requirements のメソッドを書くまで、ここがコンパイルできない。
func TestRequirements(t *testing.T) {
	gobasetest.RunRequirements(t, requirements{})
}

// requirements は、要求ごとの、テストの入れ物。どれも、そのケースの入力（goldenFixtures の req と also。runFixtures）を走らせ、
// ケースに固有の確かめを足す。
type requirements struct{}

func (requirements) Targets(t *testing.T) { runFixtures(t, "Targets") }

func (requirements) Layout(t *testing.T) {
	runFixtures(t, "Layout")
	t.Run("typesPackageText", typesPackageText)
}

func (requirements) Names(t *testing.T) { runFixtures(t, "Names") }

func (requirements) Types(t *testing.T) {
	runFixtures(t, "Types")
	t.Run("literalTypeDecls", literalTypeDecls)
}

func (requirements) OpShape(t *testing.T) {
	t.Run("everyFunctionTakesCtxAndReturnsError", everyFunctionTakesCtxAndReturnsError)
	t.Run("pureOpsHaveTheSameShape", pureOpsHaveTheSameShape)
}

func (requirements) Ports(t *testing.T) {
	runFixtures(t, "Ports")
	t.Run("incompleteUnitDoesNotCompile", incompleteUnitDoesNotCompile)
	t.Run("leafOfAnotherShapeDoesNotCompile", leafOfAnotherShapeDoesNotCompile)
}

func (requirements) Wiring(t *testing.T) {
	runFixtures(t, "Wiring")
	t.Run("elseArmSameCode", elseArmSameCode)
	t.Run("literalMatchBodies", literalMatchBodies)
	t.Run("literalPredicateGuard", literalPredicateGuard)
	t.Run("literalTypeBodies", literalTypeBodies)
	t.Run("optionalLiteralPredicateGuard", optionalLiteralPredicateGuard)
	t.Run("noGeneratedPanic", noGeneratedPanic)
}

func (requirements) Decorators(t *testing.T) { runFixtures(t, "Decorators") }

func (requirements) Seams(t *testing.T) { runFixtures(t, "Seams") }

func (requirements) Wraps(t *testing.T) { runFixtures(t, "Wraps") }

func (requirements) UnitTestPort(t *testing.T) {
	runFixtures(t, "UnitTestPort")
	t.Run("incompleteCasesDoNotCompile", incompleteCasesDoNotCompile)
	t.Run("unitTestNames", unitTestNames)
	t.Run("unitTestSeamFails", unitTestSeamFails)
	t.Run("unitTestSplitOutput", unitTestSplitOutput)
	t.Run("catchCasesFail", catchCasesFail)
	t.Run("coverageLines", coverageLines)
}

func (requirements) Fakes(t *testing.T) { runFixtures(t, "Fakes") }

func (requirements) Comments(t *testing.T) { runFixtures(t, "Comments") }

func (requirements) Determinism(t *testing.T) {
	t.Run("sameNamespaceOtherDir", sameNamespaceOtherDir)
	t.Run("readsNothing", readsNothing)
}

// TestEveryFixtureIsRequired は、入力（goldenFixtures）が、どれも、要求のケースを一つ持つこと: req のない入力は、どの要求からも
// 走らない。
func TestEveryFixtureIsRequired(t *testing.T) {
	cases := map[string]bool{
		"Targets": true, "Layout": true, "Names": true, "Types": true, "OpShape": true, "Ports": true, "Wiring": true, "Decorators": true,
		"Seams": true, "Wraps": true, "UnitTestPort": true, "Fakes": true, "Comments": true, "Determinism": true,
	}
	for _, fx := range goldenFixtures {
		if !cases[fx.req] {
			t.Errorf("入力 %s の req %q は、要求のケースでない", fx.name, fx.req)
		}
		for _, also := range fx.also {
			if !cases[also] {
				t.Errorf("入力 %s の also %q は、要求のケースでない", fx.name, also)
			}
		}
	}
}
