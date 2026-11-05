package lexer

import (
	"strings"
	"testing"

	"vim/internal/diagnostics"
)

func TestKeywordsAndOperatorsComplete(t *testing.T) {
	src := `
let const fn return if else while for true false
int bool string void
+ - * / % = == != ! < <= > >= && || ->
( ) { } [ ] , : ;
meu_ident_123 42 "ola\nmundo\t\"ok\"\\"
`
	diag := diagnostics.NewBag()
	sc := New("completo.vim", src, diag)
	tokens := sc.ScanAll()

	if diag.HasErrors() {
		t.Fatalf("não esperava erros léxicos, mas obteve %d", diag.ErrorCount())
	}

	expected := []TokenType{
		TokenLet, TokenConst, TokenFn, TokenReturn, TokenIf, TokenElse, TokenWhile, TokenFor, TokenTrue, TokenFalse,
		TokenIntType, TokenBoolType, TokenStringType, TokenVoidType,
		TokenPlus, TokenMinus, TokenStar, TokenSlash, TokenPercent,
		TokenAssign, TokenEq, TokenNotEq, TokenBang, TokenLt, TokenLtEq, TokenGt, TokenGtEq, TokenAnd, TokenOr, TokenArrow,
		TokenLParen, TokenRParen, TokenLBrace, TokenRBrace, TokenLBracket, TokenRBracket, TokenComma, TokenColon, TokenSemicolon,
		TokenIdent, TokenIntLit, TokenStringLit,
		TokenEOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("contagem de tokens divergente: esperado %d, obtido %d", len(expected), len(tokens))
	}

	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token[%d]: esperado %s, obtido %s (lexema=%q)", i, exp, tokens[i].Type, tokens[i].Lexeme)
		}
	}

	// Verifica decodificação de escapes na string e valor do inteiro
	intTok := tokens[len(tokens)-3]
	if intTok.IntVal != 42 {
		t.Errorf("valor inteiro esperado 42, obtido %d", intTok.IntVal)
	}

	strTok := tokens[len(tokens)-2]
	wantStr := "ola\nmundo\t\"ok\"\\"
	if strTok.StrVal != wantStr {
		t.Errorf("string decodificada incorreta: esperado %q, obtido %q", wantStr, strTok.StrVal)
	}
}

func TestExactSpanOffsetsAndColumns(t *testing.T) {
	src := "let total: int = 1234;\r\nreturn total;"
	sc := New("spans.vim", src, diagnostics.NewBag())
	tokens := sc.ScanAll()

	// "let" -> [0..3), linha 1, col 1..4
	if got := tokens[0].Span; got.StartOffset != 0 || got.EndOffset != 3 || got.StartLine != 1 || got.StartCol != 1 || got.EndCol != 4 {
		t.Fatalf("span incorreto para 'let': %+v", got)
	}

	// "1234" -> tokens: [0]=let, [1]=total, [2]=:, [3]=int, [4]==, [5]=1234, [6]=;, [7]=return
	// "let total: int = " tem 17 bytes -> [17..21), linha 1, col 18..22
	numTok := tokens[5]
	if numTok.Lexeme != "1234" {
		t.Fatalf("esperava '1234', obteve %q", numTok.Lexeme)
	}
	if got := numTok.Span; got.StartOffset != 17 || got.EndOffset != 21 || got.StartLine != 1 || got.StartCol != 18 || got.EndCol != 22 {
		t.Fatalf("span incorreto para '1234': %+v", got)
	}

	// Segunda linha após CRLF (\r\n = 2 bytes): "return" começa no offset 24, linha 2, col 1..7
	retTok := tokens[7]
	if retTok.Type != TokenReturn {
		t.Fatalf("esperava RETURN, obteve %s", retTok.Type)
	}
	if got := retTok.Span; got.StartOffset != 24 || got.EndOffset != 30 || got.StartLine != 2 || got.StartCol != 1 || got.EndCol != 7 {
		t.Fatalf("span incorreto para 'return' após CRLF: %+v", got)
	}
}

func TestNestedBlockComments(t *testing.T) {
	src := `
let a: int = 1;
/* comentário externo
   /* comentário aninhado nível 2
      /* nível 3 com símbolos: */ // ainda dentro do nível 2!
   */
   ainda estamos no nível 1, portanto let falso = 999; deve ser ignorado!
*/
let b: int = 2;
`
	diag := diagnostics.NewBag()
	tokens := New("aninhado.vim", src, diag).ScanAll()

	if diag.HasErrors() {
		t.Fatalf("comentário aninhado válido gerou erro inesperado: %+v", diag.Items())
	}

	// Deve produzir apenas: let a : int = 1 ; let b : int = 2 ; EOF (15 tokens)
	if len(tokens) != 15 {
		t.Fatalf("esperava 15 tokens (ignorando todo o bloco aninhado), obteve %d: %v", len(tokens), tokens)
	}
	if tokens[8].Lexeme != "b" {
		t.Fatalf("esperava identificador 'b' após comentário aninhado, obteve %q", tokens[8].Lexeme)
	}
}

func TestUnclosedNestedBlockCommentPointsToOpening(t *testing.T) {
	src := "let x: int = 1;\n/* nível 1 aberto na linha 2\n   /* nível 2 fechado */ ainda aberto\nlet y: int = 2;\n"
	diag := diagnostics.NewBag()
	_ = New("bloco_aberto.vim", src, diag).ScanAll()

	if diag.ErrorCount() != 1 {
		t.Fatalf("esperava exatamente 1 erro de bloco não fechado, obteve %d", diag.ErrorCount())
	}
	errItem := diag.Items()[0]
	if errItem.Code != ErrUnterminatedBlock {
		t.Fatalf("esperava código %s, obteve %s", ErrUnterminatedBlock, errItem.Code)
	}
	// Deve apontar para o '/*' da linha 2, coluna 1..3 (e não para o EOF na linha 5!)
	if errItem.Span.StartLine != 2 || errItem.Span.StartCol != 1 || errItem.Span.EndCol != 3 {
		t.Fatalf("span do bloco não fechado deveria apontar para 2:1..3, mas apontou para %+v", errItem.Span)
	}
}

func TestUnterminatedStringTracksOpeningQuoteAndRecovers(t *testing.T) {
	src := "let ok: int = 1;\nlet msg: string = \"texto sem fechar\nlet depois: int = 2;\n\n"
	sm := diagnostics.NewSourceMap("str_aberta.vim", src)
	diag := diagnostics.NewBag()
	tokens := New("str_aberta.vim", src, diag).ScanAll()

	if diag.ErrorCount() != 1 {
		t.Fatalf("esperava 1 erro léxico, obteve %d", diag.ErrorCount())
	}

	d := diag.Items()[0]
	if d.Code != ErrUnterminatedStr {
		t.Fatalf("esperava código %s, obteve %s", ErrUnterminatedStr, d.Code)
	}
	// As aspas de abertura '"' estão na linha 2, coluna 19 (e NÃO no EOF na linha 5!)
	if d.Span.StartLine != 2 || d.Span.StartCol != 19 {
		t.Fatalf("erro deveria apontar para linha 2, coluna 19 (aspas de abertura), mas obteve %+v", d.Span)
	}

	// Verifica renderização visual cobrindo exatamente '"texto sem fechar' (17 caracteres)
	rendered := diagnostics.Render(sm, diag)
	if !strings.Contains(rendered, "--> str_aberta.vim:2:19") {
		t.Fatalf("renderização não apontou para 2:19:\n%s", rendered)
	}
	if !strings.Contains(rendered, strings.Repeat("^", 17)) {
		t.Fatalf("esperava sublinhado de 17 '^' cobrindo toda a string não fechada:\n%s", rendered)
	}

	// Verifica recuperação: a linha 3 ("let depois: int = 2;") deve ter sido tokenizada normalmente!
	foundDepois := false
	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Lexeme == "depois" {
			foundDepois = true
			break
		}
	}
	if !foundDepois {
		t.Fatalf("scanner falhou em recuperar após string não fechada; 'depois' não foi encontrado")
	}
}

func TestErrorAccumulationMultipleLexicalErrors(t *testing.T) {
	src := `
let a: int = @ + #;
let b: bool = true & false | true;
let c: string = "escape \q ruim";
let d: int = §;
let final_valido: int = 100;
`
	diag := diagnostics.NewBag()
	tokens := New("erros_multiplos.vim", src, diag).ScanAll()

	// Esperados 6 erros acumulados em uma única passagem:
	// 1) '@'  2) '#'  3) '&' isolado  4) '|' isolado  5) '\q' inválido  6) '§' multibyte UTF-8 (apenas 1 erro, não 2!)
	if diag.ErrorCount() != 6 {
		t.Fatalf("esperava acumular exatamente 6 erros léxicos, mas obteve %d:\n%+v", diag.ErrorCount(), diag.Items())
	}

	// Garante que o código após todos os erros continuou sendo escaneado
	foundFinal := false
	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Lexeme == "final_valido" {
			foundFinal = true
			break
		}
	}
	if !foundFinal {
		t.Fatalf("acumulação de erros interrompeu o scanner prematuramente")
	}
}

func TestInt64OverflowError(t *testing.T) {
	src := "let grande: int = 99999999999999999999999999999;"
	diag := diagnostics.NewBag()
	_ = New("overflow.vim", src, diag).ScanAll()

	if diag.ErrorCount() != 1 || diag.Items()[0].Code != ErrIntOverflow {
		t.Fatalf("esperava erro %s de overflow de 64 bits, obteve: %+v", ErrIntOverflow, diag.Items())
	}
}
