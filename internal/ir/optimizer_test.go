package ir

import (
	"strings"
	"testing"

	"vim/internal/diagnostics"
	"vim/internal/parser"
	"vim/internal/semantic"
)

func compileWithAndWithoutOpt(t *testing.T, src string) (rawMod *Module, optMod *Module, stats OptStats) {
	t.Helper()
	diag := diagnostics.NewBag()

	rawProg, _ := parser.ParseSource("opt_test.vim", src, diag)
	rawProg = parser.Desugar(rawProg)
	_ = semantic.Check(rawProg, diag)
	if diag.HasErrors() {
		t.Fatalf("erros inesperados no código de teste: %+v", diag.Items())
	}

	rawMod = Generate(rawProg)

	optProg, optStats := Optimize(rawProg)
	optMod = Generate(optProg)
	return rawMod, optMod, optStats
}

func TestConstantFoldingAndDeadCodeEliminationReducesBytecode(t *testing.T) {
	src := `
const TAXA: int = 10;
const DEBUG: bool = false;

fn calcular(x: int) -> int {
    if (DEBUG || 1 > 2) {
        print("morto");
    }
    while (false) {
        print("loop morto");
    }
    return x + (2 + 3 * 4) * TAXA;
    let aposReturn: int = 999;
    print(aposReturn);
}
print("A" + "B" + "C");
print(calcular(5));
`
	rawMod, optMod, stats := compileWithAndWithoutOpt(t, src)

	if stats.ConstsFolded == 0 || stats.DeadNodesRemoved == 0 {
		t.Fatalf("esperava otimizações não-nulas, obteve %+v", stats)
	}
	if optMod.TotalInstructions() >= rawMod.TotalInstructions() {
		t.Fatalf("esperava redução real de instruções no bytecode com --opt: sem opt=%d, com opt=%d",
			rawMod.TotalInstructions(), optMod.TotalInstructions())
	}

	// Verifica que (2 + 3 * 4) * 10 = 140 foi dobrado em uma única constante na função `calcular`
	dis := Disassemble(optMod)
	if !strings.Contains(dis, "const 140") {
		t.Errorf("esperava encontrar constante dobrada 140 no disassembler:\n%s", dis)
	}
	if !strings.Contains(dis, `"ABC"`) {
		t.Errorf("esperava concatenação dobrada \"ABC\" no disassembler:\n%s", dis)
	}
}

func TestOptimizerNeverFoldsDivByZeroOrMinInt64DivMinusOne(t *testing.T) {
	src := `
fn perigosa(flag: bool) -> int {
    if (flag) {
        return 100 / 0;
    }
    return (-9223372036854775808) / (-1);
}
`
	_, optMod, _ := compileWithAndWithoutOpt(t, src)
	dis := Disassemble(optMod)

	// Ambas as divisões perigosas DEVEM permanecer como instruções DIV para tratamento em runtime!
	if strings.Count(dis, "DIV") != 2 {
		t.Fatalf("otimizador dobrou indevidamente divisão por zero ou MinInt64/-1:\n%s", dis)
	}
}
