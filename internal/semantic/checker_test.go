package semantic

import (
	"strings"
	"testing"

	"vim/internal/diagnostics"
	"vim/internal/parser"
)

func checkSource(src string) (*Result, *diagnostics.Bag) {
	diag := diagnostics.NewBag()
	prog, _ := parser.ParseSource("sem_test.vim", src, diag)
	prog = parser.Desugar(prog)
	res := Check(prog, diag)
	return res, diag
}

func TestTwoPassMutualRecursionAndShadowing(t *testing.T) {
	src := `
// Chamada antes da declaração (forward call)
let ok: bool = ehPar(6);

fn ehPar(n: int) -> bool {
    if (n == 0) {
        return true;
    }
    return ehImpar(n - 1);
}

fn ehImpar(n: int) -> bool {
    if (n == 0) {
        return false;
    }
    return ehPar(n - 1);
}

// Shadowing válido entre escopos distintos (global -> bloco -> sub-bloco)
let x: int = 10;
{
    let x: string = "sombreado";
    print(len(x));
    {
        let x: bool = true;
        print(x);
    }
}
`
	res, diag := checkSource(src)
	if diag.HasErrors() {
		t.Fatalf("não esperava erros semânticos, mas obteve: %+v", diag.Items())
	}
	if res.ScopeCount < 5 {
		t.Errorf("esperava ao menos 5 escopos criados, obteve %d", res.ScopeCount)
	}
}

func TestRedeclarationInSameScopeRejected(t *testing.T) {
	src := `
let taxa: int = 10;
let taxa: int = 20;

fn duplicada() -> void {}
fn duplicada() -> void {}
`
	_, diag := checkSource(src)
	if diag.ErrorCount() != 2 {
		t.Fatalf("esperava 2 erros E0202 de redeclaração, obteve %d: %+v", diag.ErrorCount(), diag.Items())
	}
	for _, d := range diag.Items() {
		if d.Code != ErrRedeclared {
			t.Errorf("esperava código %s, obteve %s", ErrRedeclared, d.Code)
		}
		if !strings.Contains(d.Help, "linha") {
			t.Errorf("dica de redeclaração deveria apontar para a linha original: %q", d.Help)
		}
	}
}

func TestCascadeErrorSuppressionWithTypeError(t *testing.T) {
	// `inexistente` gera E0201 -> TypeError; `+ 10`, `* 2` e `let total: int = ...` NÃO devem gerar E0203 redundantes!
	src := `let total: int = (inexistente + 10) * 2;`
	_, diag := checkSource(src)

	if diag.ErrorCount() != 1 {
		t.Fatalf("supressão de cascata falhou: esperava exatamente 1 erro (E0201), mas obteve %d: %+v",
			diag.ErrorCount(), diag.Items())
	}
	if diag.Items()[0].Code != ErrUndeclared {
		t.Errorf("esperava %s, obteve %s", ErrUndeclared, diag.Items()[0].Code)
	}
}

func TestContextualEmptyArrayAndAmbiguousRejection(t *testing.T) {
	validSrc := `
fn criarVazio() -> []int {
    return [];
}
fn consumir(v: []string) -> void {
    print(len(v));
}
let nums: []int = [];
nums = [];
consumir([]);
let matriz: [][]int = [[], [1, 2]];
push(matriz, []);
`
	_, diagValid := checkSource(validSrc)
	if diagValid.HasErrors() {
		t.Fatalf("arrays vazios contextuais válidos falharam: %+v", diagValid.Items())
	}

	invalidSrc := `let t: int = len([]);`
	_, diagInvalid := checkSource(invalidSrc)
	if diagInvalid.ErrorCount() != 1 || diagInvalid.Items()[0].Code != ErrAmbiguousEmptyArr {
		t.Fatalf("esperava erro %s para 'len([])' sem tipo determinável, obteve: %+v",
			ErrAmbiguousEmptyArr, diagInvalid.Items())
	}
}

func TestVoidProhibitedInVariablesParamsAndArrays(t *testing.T) {
	src := `
fn ruim(p: void) -> []void {
    return [];
}
let v: void = ruim;
let a: []void = [];
`
	_, diag := checkSource(src)
	// p: void (1), []void no retorno (2), v: void (3), a: []void (4) -> todos E0204 (e TypeError suprime cascata!)
	if diag.ErrorCount() != 4 {
		t.Fatalf("esperava 4 erros E0204 sobre uso ilegal de 'void', obteve %d: %+v", diag.ErrorCount(), diag.Items())
	}
	for _, d := range diag.Items() {
		if d.Code != ErrInvalidVoid {
			t.Errorf("esperava %s, obteve %s (%s)", ErrInvalidVoid, d.Code, d.Message)
		}
	}
}

func TestConstImmutabilityDirectIndexedAndPush(t *testing.T) {
	src := `
const MAX: int = 100;
MAX = 200;

const VETOR: []int = [1, 2, 3];
VETOR[0] = 99;
push(VETOR, 4);

const MATRIZ: [][]int = [[1], [2]];
MATRIZ[1][0] = 88;
`
	_, diag := checkSource(src)
	if diag.ErrorCount() != 4 {
		t.Fatalf("esperava 4 erros E0205 de violação de 'const', obteve %d: %+v", diag.ErrorCount(), diag.Items())
	}
	for _, d := range diag.Items() {
		if d.Code != ErrImmutableAssign {
			t.Errorf("esperava %s, obteve %s", ErrImmutableAssign, d.Code)
		}
	}
}

func TestOperatorsConditionsAndIntrinsicsTyping(t *testing.T) {
	validSrc := `
let saudacao: string = "Ola, " + read();
let n: int = read_int();
let lista: []int = [10, 20];
push(lista, n);
if (len(lista) > 2 && saudacao != "") {
    print("Tamanho:", len(lista), lista);
}
`
	_, diagValid := checkSource(validSrc)
	if diagValid.HasErrors() {
		t.Fatalf("operações e intrínsecos válidos geraram erro: %+v", diagValid.Items())
	}

	invalidSrc := `
let s: string = "a" + 1;
let b1: bool = 1 && true;
let b2: bool = [1] == [1];
if (42) {
    print();
}
let tam: int = len(123);
`
	_, diagInvalid := checkSource(invalidSrc)
	// 1) "a" + 1 (E0203)
	// 2) 1 && true (E0203)
	// 3) [1] == [1] (E0203)
	// 4) if (42) (E0203)
	// 5) print() sem args (E0206)
	// 6) len(123) (E0203)
	if diagInvalid.ErrorCount() != 6 {
		t.Fatalf("esperava 6 erros de tipos/intrínsecos, obteve %d: %+v", diagInvalid.ErrorCount(), diagInvalid.Items())
	}
}

func TestDefiniteReturnAnalysisAndReturnOutsideFn(t *testing.T) {
	validSrc := `
fn classificar(x: int) -> string {
    if (x > 0) {
        return "positivo";
    } else if (x < 0) {
        return "negativo";
    } else {
        return "zero";
    }
}
`
	_, diagValid := checkSource(validSrc)
	if diagValid.HasErrors() {
		t.Fatalf("if/else completo com retornos deveria ser aceito: %+v", diagValid.Items())
	}

	invalidSrc := `
fn incompleta(x: int) -> int {
    if (x > 0) {
        return x;
    } else if (x < 0) {
        return -x;
    }
    // faltou else ou return final!
}

return 0;
`
	_, diagInvalid := checkSource(invalidSrc)
	if diagInvalid.ErrorCount() != 2 {
		t.Fatalf("esperava 2 erros (E0208 e E0207), obteve %d: %+v", diagInvalid.ErrorCount(), diagInvalid.Items())
	}
	if diagInvalid.Items()[0].Code != ErrMissingReturn {
		t.Errorf("esperava %s, obteve %s", ErrMissingReturn, diagInvalid.Items()[0].Code)
	}
	if diagInvalid.Items()[1].Code != ErrReturnOutsideFn {
		t.Errorf("esperava %s, obteve %s", ErrReturnOutsideFn, diagInvalid.Items()[1].Code)
	}
}
