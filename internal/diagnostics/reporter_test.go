package diagnostics

import (
	"strings"
	"testing"
)

func TestSpanAndUnderlineWidth(t *testing.T) {
	src := "let taxa: int = @invalido + 10;\n"
	sm := NewSourceMap("teste.vim", src)
	bag := NewBag()

	// "@invalido" começa na coluna 17 (offset 16) e termina na coluna 26 (offset 25) -> 9 caracteres
	span := Span{
		File:        "teste.vim",
		StartOffset: 16,
		EndOffset:   25,
		StartLine:   1,
		StartCol:    17,
		EndLine:     1,
		EndCol:      26,
	}
	bag.Error("E0001", span, "token inválido encontrado", "nove caracteres destacados", "remova o símbolo '@'")

	out := Render(sm, bag)

	if !strings.Contains(out, "erro[E0001]: token inválido encontrado") {
		t.Fatalf("cabeçalho ausente na saída:\n%s", out)
	}
	if !strings.Contains(out, "--> teste.vim:1:17") {
		t.Fatalf("localização inicial incorreta:\n%s", out)
	}
	if !strings.Contains(out, "^^^^^^^^^ nove caracteres destacados") {
		t.Fatalf("esperava exatamente 9 marcadores '^' cobrindo todo o span:\n%s", out)
	}
}

func TestTabAlignmentInDiagnostic(t *testing.T) {
	src := "\t\tlet x: int = $;\n"
	sm := NewSourceMap("tabs.vim", src)
	bag := NewBag()

	// Dois tabs (colunas 1 e 2) + "let x: int = " (13 chars, colunas 3..15) -> '$' está na coluna 16..17
	span := Span{
		File:        "tabs.vim",
		StartOffset: 15,
		EndOffset:   16,
		StartLine:   1,
		StartCol:    16,
		EndLine:     1,
		EndCol:      17,
	}
	bag.Error("E0001", span, "caractere inválido '$'", "aqui", "")

	out := Render(sm, bag)
	// Dois tabs expandidos = 8 espaços + 13 espaços = 21 espaços antes de '^'
	expectedCaretLine := "  | " + strings.Repeat(" ", 21) + "^ aqui"
	if !strings.Contains(out, expectedCaretLine) {
		t.Fatalf("alinhamento com tabs incorreto.\nEsperado trecho:\n%s\nObtido:\n%s", expectedCaretLine, out)
	}
}
