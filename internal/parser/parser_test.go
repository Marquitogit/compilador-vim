package parser

import (
	"math"
	"strings"
	"testing"

	"vim/internal/diagnostics"
	"vim/internal/lexer"
)

func TestPrattOperatorPrecedenceAndAssociativity(t *testing.T) {
	cases := []struct {
		src      string
		wantExpr string
	}{
		// Multiplicação tem maior precedência que adição
		{
			src:      "let r: int = 1 + 2 * 3;",
			wantExpr: "(1 + (2 * 3))",
		},
		// Parênteses sobrescrevem precedência
		{
			src:      "let r: int = (1 + 2) * 3;",
			wantExpr: "((1 + 2) * 3)",
		},
		// Associatividade à esquerda em subtração e divisão
		{
			src:      "let r: int = 10 - 4 - 2;",
			wantExpr: "((10 - 4) - 2)",
		},
		{
			src:      "let r: int = 20 / 5 * 2;",
			wantExpr: "((20 / 5) * 2)",
		},
		// Hierarquia completa: || < && < == < < < + < * < unário < pós-fixo
		{
			src:      "let b: bool = a || b && c == d + e * -arr[0] && !validar(x);",
			wantExpr: "(a || ((b && (c == (d + (e * (-(arr[0])))))) && (!validar(x))))",
		},
	}

	for _, tc := range cases {
		diag := diagnostics.NewBag()
		prog, _ := ParseSource("prec.vim", tc.src, diag)
		if diag.HasErrors() {
			t.Fatalf("erro inesperado em %q: %+v", tc.src, diag.Items())
		}
		vd, ok := prog.Decls[0].(*VarDecl)
		if !ok {
			t.Fatalf("esperava *VarDecl, obteve %T", prog.Decls[0])
		}
		got := exprToSExpr(vd.Value)
		if got != tc.wantExpr {
			t.Errorf("precedência incorreta para %q:\n  esperado: %s\n  obtido  : %s", tc.src, tc.wantExpr, got)
		}
	}
}

func TestChainedCallsAndMultidimensionalIndexing(t *testing.T) {
	src := `
let m: [][]int = [[1, 2], [3, 4]];
m[0][1] = obterLinha(0)[1] + 10;
`
	diag := diagnostics.NewBag()
	prog, _ := ParseSource("chain.vim", src, diag)
	if diag.HasErrors() {
		t.Fatalf("não esperava erros, obteve: %+v", diag.Items())
	}

	assign, ok := prog.Decls[1].(*AssignStmt)
	if !ok {
		t.Fatalf("esperava *AssignStmt, obteve %T", prog.Decls[1])
	}

	if gotTarget := exprToSExpr(assign.Target); gotTarget != "((m[0])[1])" {
		t.Errorf("alvo indexado incorreto: %s", gotTarget)
	}
	if gotVal := exprToSExpr(assign.Value); gotVal != "((obterLinha(0)[1]) + 10)" {
		t.Errorf("valor encadeado incorreto: %s", gotVal)
	}
}

func TestForDesugaringPreservesSemanticsAndSpans(t *testing.T) {
	src := `
for (let i: int = 0; i < 5; i = i + 1) {
    print(i);
}
`
	diag := diagnostics.NewBag()
	rawProg, _ := ParseSource("for.vim", src, diag)
	if diag.HasErrors() {
		t.Fatalf("erro inesperado no parse do for: %+v", diag.Items())
	}

	rawFor, ok := rawProg.Decls[0].(*ForStmt)
	if !ok {
		t.Fatalf("esperava *ForStmt antes do desugar, obteve %T", rawProg.Decls[0])
	}

	desugared, count := DesugarWithCount(rawProg)
	if count != 1 {
		t.Fatalf("esperava 1 laço desaçucarado, obteve %d", count)
	}

	outerBlock, ok := desugared.Decls[0].(*BlockStmt)
	if !ok || !outerBlock.IsDesugaredFor {
		t.Fatalf("esperava *BlockStmt marcado como IsDesugaredFor, obteve %T", desugared.Decls[0])
	}
	if outerBlock.Span() != rawFor.Span() {
		t.Errorf("span externo não preservado: esperado %v, obtido %v", rawFor.Span(), outerBlock.Span())
	}
	if len(outerBlock.Stmts) != 2 {
		t.Fatalf("bloco desaçucarado deve conter [init, while], obteve %d stmts", len(outerBlock.Stmts))
	}

	if _, ok := outerBlock.Stmts[0].(*VarDecl); !ok {
		t.Errorf("primeiro comando deveria ser *VarDecl (let i), obteve %T", outerBlock.Stmts[0])
	}

	whileNode, ok := outerBlock.Stmts[1].(*WhileStmt)
	if !ok {
		t.Fatalf("segundo comando deveria ser *WhileStmt, obteve %T", outerBlock.Stmts[1])
	}
	// Span da condição `i < 5` deve permanecer idêntico ao código original (linha 2)
	if whileNode.Cond.Span().StartLine != 2 {
		t.Errorf("span da condição perdeu a linha original: %+v", whileNode.Cond.Span())
	}
	if len(whileNode.Body.Stmts) != 2 {
		t.Fatalf("corpo do while deveria conter [print(i), i = i + 1], obteve %d stmts", len(whileNode.Body.Stmts))
	}
	if _, ok := whileNode.Body.Stmts[1].(*AssignStmt); !ok {
		t.Errorf("último comando do while deveria ser *AssignStmt (step), obteve %T", whileNode.Body.Stmts[1])
	}
}

func TestNestedFunctionForbiddenAndInvalidLValueAndPanicRecovery(t *testing.T) {
	src := `
fn externa() -> void {
    fn proibida() -> int {
        return 1;
    }
    10 + 20 = 30;
    let quebrada: int = * 5;
    let sobrevivente: int = 99;
}
`
	diag := diagnostics.NewBag()
	prog, _ := ParseSource("erros_sintaticos.vim", src, diag)

	// Deve acumular exatamente os 3 erros sintáticos independentes sem abortar no primeiro:
	// 1) E0101: função aninhada `fn proibida`
	// 2) E0102: lado esquerdo inválido `10 + 20 = 30`
	// 3) E0104: expressão inválida `* 5`
	if diag.ErrorCount() != 3 {
		t.Fatalf("esperava acumular exatamente 3 erros sintáticos, obteve %d:\n%+v", diag.ErrorCount(), diag.Items())
	}

	codes := []string{diag.Items()[0].Code, diag.Items()[1].Code, diag.Items()[2].Code}
	if codes[0] != ErrNestedFunction || codes[1] != ErrInvalidLValue || codes[2] != ErrExpectedExpr {
		t.Errorf("códigos de erro inesperados: %v", codes)
	}

	// Garante que após sincronizar, `let sobrevivente: int = 99;` foi incluída na AST de `externa`!
	fnDecl, ok := prog.Decls[0].(*FnDecl)
	if !ok || len(fnDecl.Body.Stmts) != 1 {
		t.Fatalf("esperava que 'let sobrevivente' fosse preservada após sincronização, obteve: %+v", prog.Decls)
	}
}

func TestTreeAndDOTFormatters(t *testing.T) {
	src := "fn dobro(x: int) -> int { return x * 2; }"
	diag := diagnostics.NewBag()
	prog, _ := ParseSource("fmt.vim", src, diag)
	if diag.HasErrors() {
		t.Fatalf("erro inesperado: %+v", diag.Items())
	}

	treeOut := FormatTree(prog)
	if !strings.Contains(treeOut, "FnDecl [fn dobro(x: int) -> int]") || !strings.Contains(treeOut, "BinaryExpr (*)") {
		t.Errorf("saída FormatTree incompleta:\n%s", treeOut)
	}

	dotOut := FormatDOT(prog)
	if !strings.HasPrefix(dotOut, "digraph vimAST {") || !strings.Contains(dotOut, "BinaryExpr (*)") {
		t.Errorf("saída FormatDOT inválida:\n%s", dotOut)
	}
}

func TestParenSpanMinInt64AndTrailingCommas(t *testing.T) {
	// 1. Span de expressões parentizadas cobre '(' até ')'
	diagSpan := diagnostics.NewBag()
	_, _ = ParseSource("paren.vim", "(x + 10) = 50;", diagSpan)
	if diagSpan.ErrorCount() != 1 {
		t.Fatalf("esperava 1 erro de L-Value, obteve %d", diagSpan.ErrorCount())
	}
	sp := diagSpan.Items()[0].Span
	// "(x + 10)" ocupa colunas 1..9 (8 caracteres incluindo os parênteses)
	if sp.StartCol != 1 || sp.EndCol != 9 {
		t.Errorf("span de '(x + 10)' deveria cobrir 1..9 (incluindo parênteses), obteve %+v", sp)
	}

	// 2. Suporte a math.MinInt64 (-9223372036854775808) e rejeição de +9223372036854775808
	diagMin := diagnostics.NewBag()
	progMin, _ := ParseSource("minint.vim", "let minimo: int = -9223372036854775808;", diagMin)
	if diagMin.HasErrors() {
		t.Fatalf("não esperava erro em -9223372036854775808: %+v", diagMin.Items())
	}
	litMin := progMin.Decls[0].(*VarDecl).Value.(*IntLit)
	if litMin.Value != math.MinInt64 {
		t.Errorf("valor esperado math.MinInt64 (%d), obtido %d", int64(math.MinInt64), litMin.Value)
	}

	diagPosOverflow := diagnostics.NewBag()
	_, _ = ParseSource("posoverflow.vim", "let estoura: int = 9223372036854775808;", diagPosOverflow)
	if diagPosOverflow.ErrorCount() != 1 || diagPosOverflow.Items()[0].Code != lexer.ErrIntOverflow {
		t.Fatalf("esperava erro E0005 para +9223372036854775808 sem '-', obteve: %+v", diagPosOverflow.Items())
	}

	// 3. Trailing commas em parâmetros, arrays e chamadas
	srcTrailing := `
fn somar(a: int, b: int,) -> int {
    return a + b;
}
let lista: []int = [1, 2, 3,];
let res: int = somar(lista[0], lista[1],);
`
	diagTrail := diagnostics.NewBag()
	_, _ = ParseSource("trailing.vim", srcTrailing, diagTrail)
	if diagTrail.HasErrors() {
		t.Fatalf("trailing commas deveriam ser aceitas sem erros, mas obteve: %+v", diagTrail.Items())
	}
}

// exprToSExpr auxilia os testes convertendo expressões em forma parentizada explícita.
func exprToSExpr(e Expr) string {
	switch v := e.(type) {
	case *IntLit:
		return v.Raw
	case *IdentExpr:
		return v.Name
	case *BoolLit:
		if v.Value {
			return "true"
		}
		return "false"
	case *UnaryExpr:
		return "(" + v.OpLexeme + exprToSExpr(v.Operand) + ")"
	case *BinaryExpr:
		return "(" + exprToSExpr(v.Left) + " " + v.OpLexeme + " " + exprToSExpr(v.Right) + ")"
	case *CallExpr:
		args := make([]string, len(v.Args))
		for i, a := range v.Args {
			args[i] = exprToSExpr(a)
		}
		return exprToSExpr(v.Callee) + "(" + strings.Join(args, ", ") + ")"
	case *IndexExpr:
		return "(" + exprToSExpr(v.Collection) + "[" + exprToSExpr(v.Index) + "])"
	default:
		return "?"
	}
}
