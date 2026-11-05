package diagnostics

import "fmt"

// Position representa uma coordenada exata no código-fonte.
// Offset é 0-indexado (em bytes); Line e Column são 1-indexados (em runas).
type Position struct {
	Offset int
	Line   int
	Column int
}

// Span representa um intervalo semiaberto [StartOffset, EndOffset) no código-fonte,
// preservando linha e coluna tanto do início quanto do fim do token ou nó da AST.
type Span struct {
	File        string
	StartOffset int
	EndOffset   int
	StartLine   int
	StartCol    int
	EndLine     int
	EndCol      int
}

// NewSpan constrói um Span a partir das posições inicial (inclusiva) e final (exclusiva).
func NewSpan(file string, start, end Position) Span {
	return Span{
		File:        file,
		StartOffset: start.Offset,
		EndOffset:   end.Offset,
		StartLine:   start.Line,
		StartCol:    start.Column,
		EndLine:     end.Line,
		EndCol:      end.Column,
	}
}

// Merge combina dois spans do mesmo arquivo em um único Span cobrindo do início do primeiro ao fim do segundo.
func (s Span) Merge(other Span) Span {
	res := s
	if other.StartOffset < res.StartOffset {
		res.StartOffset = other.StartOffset
		res.StartLine = other.StartLine
		res.StartCol = other.StartCol
	}
	if other.EndOffset > res.EndOffset {
		res.EndOffset = other.EndOffset
		res.EndLine = other.EndLine
		res.EndCol = other.EndCol
	}
	return res
}

// ByteLen retorna o comprimento em bytes coberto pelo Span.
func (s Span) ByteLen() int {
	if s.EndOffset < s.StartOffset {
		return 0
	}
	return s.EndOffset - s.StartOffset
}

// String formata a localização no padrão "arquivo:linha:coluna (início..fim)".
func (s Span) String() string {
	file := s.File
	if file == "" {
		file = "<entrada>"
	}
	if s.StartLine == s.EndLine {
		return fmt.Sprintf("%s:%d:%d..%d", file, s.StartLine, s.StartCol, s.EndCol)
	}
	return fmt.Sprintf("%s:%d:%d..%d:%d", file, s.StartLine, s.StartCol, s.EndLine, s.EndCol)
}
