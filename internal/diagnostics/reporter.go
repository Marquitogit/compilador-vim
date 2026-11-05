package diagnostics

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const tabWidth = 4

// SourceMap indexa as linhas de um código-fonte para renderização rápida de diagnósticos.
type SourceMap struct {
	File  string
	Src   string
	lines []string
}

// NewSourceMap prepara as linhas do código-fonte (normalizando CRLF do Windows).
func NewSourceMap(file, src string) *SourceMap {
	rawLines := strings.Split(src, "\n")
	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return &SourceMap{
		File:  file,
		Src:   src,
		lines: lines,
	}
}

// LineCount retorna o número total de linhas lógicas do arquivo.
func (sm *SourceMap) LineCount() int {
	if len(sm.Src) == 0 {
		return 0
	}
	return len(sm.lines)
}

// Line retorna a linha 1-indexada ou string vazia caso esteja fora do intervalo.
func (sm *SourceMap) Line(lineNum int) (string, bool) {
	if lineNum < 1 || lineNum > len(sm.lines) {
		return "", false
	}
	return sm.lines[lineNum-1], true
}

// Render formata todos os diagnósticos acumulados no estilo visual do Rust/Clang.
func Render(sm *SourceMap, bag *Bag) string {
	var sb strings.Builder
	items := bag.Items()
	for i, d := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(RenderDiagnostic(sm, d))
	}
	if bag.HasErrors() {
		sb.WriteString(fmt.Sprintf("\ncompilação abortada devido a %d erro(s).\n", bag.ErrorCount()))
	}
	return sb.String()
}

// RenderDiagnostic formata um único diagnóstico com trecho do código e sublinhado ^^^ exato.
func RenderDiagnostic(sm *SourceMap, d Diagnostic) string {
	var sb strings.Builder

	file := d.Span.File
	if file == "" && sm != nil {
		file = sm.File
	}
	if file == "" {
		file = "<entrada>"
	}

	// Cabeçalho: erro[E0001]: mensagem
	if d.Code != "" {
		sb.WriteString(fmt.Sprintf("%s[%s]: %s\n", d.Severity, d.Code, d.Message))
	} else {
		sb.WriteString(fmt.Sprintf("%s: %s\n", d.Severity, d.Message))
	}

	startLine := max(1, d.Span.StartLine)
	startCol := max(1, d.Span.StartCol)
	endLine := max(startLine, d.Span.EndLine)
	endCol := d.Span.EndCol

	// Margem numerada alinhada pela maior linha exibida
	gutterWidth := len(strconv.Itoa(endLine))
	pad := strings.Repeat(" ", gutterWidth)

	sb.WriteString(fmt.Sprintf("%s --> %s:%d:%d\n", pad, file, startLine, startCol))

	if sm == nil {
		return sb.String()
	}

	rawLine, ok := sm.Line(startLine)
	if !ok {
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s |\n", pad))

	if startLine == endLine {
		expandedLine, caretPad, caretCount := buildUnderline(rawLine, startCol, endCol)
		sb.WriteString(fmt.Sprintf("%*d | %s\n", gutterWidth, startLine, expandedLine))
		sb.WriteString(fmt.Sprintf("%s | %s%s", pad, caretPad, strings.Repeat("^", caretCount)))
		if d.Label != "" {
			sb.WriteByte(' ')
			sb.WriteString(d.Label)
		}
		sb.WriteByte('\n')
	} else {
		// Span multilinha: destaca do StartCol até o final da primeira linha e referencia a linha final.
		runeLen := utf8.RuneCountInString(rawLine)
		expandedStart, caretPad, caretCount := buildUnderline(rawLine, startCol, runeLen+1)
		sb.WriteString(fmt.Sprintf("%*d | %s\n", gutterWidth, startLine, expandedStart))
		sb.WriteString(fmt.Sprintf("%s | %s%s", pad, caretPad, strings.Repeat("^", caretCount)))
		if d.Label != "" {
			sb.WriteByte(' ')
			sb.WriteString(d.Label)
		}
		sb.WriteByte('\n')

		if endLine > startLine+1 {
			sb.WriteString(fmt.Sprintf("%s ...\n", pad))
		}
		if lastRaw, okLast := sm.Line(endLine); okLast {
			expandedEnd, _, endCarets := buildUnderline(lastRaw, 1, max(2, endCol))
			sb.WriteString(fmt.Sprintf("%*d | %s\n", gutterWidth, endLine, expandedEnd))
			sb.WriteString(fmt.Sprintf("%s | %s\n", pad, strings.Repeat("^", endCarets)))
		}
	}

	if d.Help != "" {
		sb.WriteString(fmt.Sprintf("%s |\n", pad))
		sb.WriteString(fmt.Sprintf("%s = ajuda: %s\n", pad, d.Help))
	}

	return sb.String()
}

// buildUnderline expande tabs em espaços fixos e calcula o recuo e a largura exata dos marcadores '^'.
func buildUnderline(rawLine string, startCol, endCol int) (expanded string, pad string, carets int) {
	var expBuf strings.Builder
	var padBuf strings.Builder

	col := 1
	for _, r := range rawLine {
		width := 1
		if r == '\t' {
			width = tabWidth
			expBuf.WriteString(strings.Repeat(" ", tabWidth))
		} else {
			expBuf.WriteRune(r)
		}

		if col < startCol {
			padBuf.WriteString(strings.Repeat(" ", width))
		} else if col >= startCol && col < endCol {
			carets += width
		}
		col++
	}

	// Caso o erro aponte após o último caractere visível (ex: EOF na linha)
	for col < startCol {
		padBuf.WriteByte(' ')
		col++
	}
	if carets < 1 {
		carets = 1
	}

	return expBuf.String(), padBuf.String(), carets
}
