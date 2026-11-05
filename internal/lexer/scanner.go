package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"vim/internal/diagnostics"
)

// Códigos padronizados de diagnósticos léxicos.
const (
	ErrInvalidChar       = "E0001"
	ErrUnterminatedStr   = "E0002"
	ErrInvalidEscape     = "E0003"
	ErrUnterminatedBlock = "E0004"
	ErrIntOverflow       = "E0005"
	ErrInvalidOperator   = "E0006"
)

// Scanner implementa um analisador léxico manual (sem regex ou geradores externos),
// com acumulação de erros, spans completos [início..fim) e comentários de bloco aninhados.
type Scanner struct {
	file   string
	src    string
	offset int // Índice atual em bytes (0-indexed)
	line   int // Linha atual (1-indexed)
	col    int // Coluna atual em runas (1-indexed)
	diag   *diagnostics.Bag
}

// New cria um novo Scanner vinculado a um arquivo, seu conteúdo e um acumulador de diagnósticos.
func New(file, src string, diag *diagnostics.Bag) *Scanner {
	if diag == nil {
		diag = diagnostics.NewBag()
	}
	return &Scanner{
		file:   file,
		src:    src,
		offset: 0,
		line:   1,
		col:    1,
		diag:   diag,
	}
}

// Diagnostics retorna o acumulador de erros associado ao Scanner.
func (s *Scanner) Diagnostics() *diagnostics.Bag {
	return s.diag
}

// ScanAll percorre todo o código-fonte até o EOF, acumulando todos os tokens e erros léxicos.
func (s *Scanner) ScanAll() []Token {
	// Pré-alocação heurística leve (~1 token a cada 5 bytes) para minimizar realocações.
	tokens := make([]Token, 0, max(8, len(s.src)/5))
	for {
		tok := s.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens
}

// NextToken extrai e retorna o próximo token do fluxo de entrada.
func (s *Scanner) NextToken() Token {
	s.skipTrivia()

	start := s.pos()
	if s.isAtEnd() {
		return Token{
			Type:   TokenEOF,
			Lexeme: "",
			Span:   diagnostics.NewSpan(s.file, start, start),
		}
	}

	b := s.peek()

	// 1. Identificadores e Palavras-chave (consulta O(1) na tabela hash)
	if isIdentStart(b) {
		return s.scanIdentifier(start)
	}

	// 2. Literais Inteiros
	if isDigit(b) {
		return s.scanNumber(start)
	}

	// 3. Literais de String com rastreamento da linha/coluna das aspas de abertura
	if b == '"' {
		return s.scanString(start)
	}

	// 4. Operadores, Delimitadores e Recuperação de Erros Léxicos
	s.advanceByte()
	switch b {
	case '(':
		return s.makeToken(TokenLParen, start)
	case ')':
		return s.makeToken(TokenRParen, start)
	case '{':
		return s.makeToken(TokenLBrace, start)
	case '}':
		return s.makeToken(TokenRBrace, start)
	case '[':
		return s.makeToken(TokenLBracket, start)
	case ']':
		return s.makeToken(TokenRBracket, start)
	case ',':
		return s.makeToken(TokenComma, start)
	case ':':
		return s.makeToken(TokenColon, start)
	case ';':
		return s.makeToken(TokenSemicolon, start)
	case '+':
		return s.makeToken(TokenPlus, start)
	case '-':
		if s.match('>') {
			return s.makeToken(TokenArrow, start)
		}
		return s.makeToken(TokenMinus, start)
	case '*':
		return s.makeToken(TokenStar, start)
	case '/':
		return s.makeToken(TokenSlash, start)
	case '%':
		return s.makeToken(TokenPercent, start)
	case '=':
		if s.match('=') {
			return s.makeToken(TokenEq, start)
		}
		return s.makeToken(TokenAssign, start)
	case '!':
		if s.match('=') {
			return s.makeToken(TokenNotEq, start)
		}
		return s.makeToken(TokenBang, start)
	case '<':
		if s.match('=') {
			return s.makeToken(TokenLtEq, start)
		}
		return s.makeToken(TokenLt, start)
	case '>':
		if s.match('=') {
			return s.makeToken(TokenGtEq, start)
		}
		return s.makeToken(TokenGt, start)
	case '&':
		if s.match('&') {
			return s.makeToken(TokenAnd, start)
		}
		span := diagnostics.NewSpan(s.file, start, s.pos())
		s.diag.Error(
			ErrInvalidOperator,
			span,
			"operador '&' isolado não é suportado",
			"operador incompleto",
			"use '&&' para conjunção lógica booleana",
		)
		return s.makeToken(TokenIllegal, start)
	case '|':
		if s.match('|') {
			return s.makeToken(TokenOr, start)
		}
		span := diagnostics.NewSpan(s.file, start, s.pos())
		s.diag.Error(
			ErrInvalidOperator,
			span,
			"operador '|' isolado não é suportado",
			"operador incompleto",
			"use '||' para disjunção lógica booleana",
		)
		return s.makeToken(TokenIllegal, start)
	default:
		// Rebobina 1 byte para decodificar corretamente runas UTF-8 multibyte (ex: '§', 'ç', '€')
		// e emitir apenas um único diagnóstico preciso cobrindo toda a runa inválida.
		s.offset = start.Offset
		s.line = start.Line
		s.col = start.Column
		r := s.advanceRune()
		span := diagnostics.NewSpan(s.file, start, s.pos())
		s.diag.Error(
			ErrInvalidChar,
			span,
			fmt.Sprintf("caractere inválido %q (U+%04X)", r, r),
			"caractere não reconhecido pela gramática",
			"remova este símbolo ou envolva-o em aspas duplas se for texto",
		)
		return s.makeToken(TokenIllegal, start)
	}
}

// skipTrivia descarta espaços em branco, comentários de linha (//) e comentários de bloco aninhados (/* /* */ */).
func (s *Scanner) skipTrivia() {
	for !s.isAtEnd() {
		b := s.peek()
		switch b {
		case ' ', '\t', '\r', '\n':
			s.advanceByte()
		case '/':
			if s.peekNext() == '/' {
				// Comentário de linha: consome até '\n' ou EOF
				s.advanceByte()
				s.advanceByte()
				for !s.isAtEnd() && s.peek() != '\n' {
					s.advanceRune()
				}
			} else if s.peekNext() == '*' {
				s.skipNestedBlockComment()
			} else {
				return
			}
		default:
			return
		}
	}
}

// skipNestedBlockComment processa comentários de bloco com suporte a aninhamento arbitrário via contador de profundidade.
func (s *Scanner) skipNestedBlockComment() {
	var openSpans []diagnostics.Span

	firstStart := s.pos()
	s.advanceByte() // '/'
	s.advanceByte() // '*'
	openSpans = append(openSpans, diagnostics.NewSpan(s.file, firstStart, s.pos()))
	depth := 1

	for depth > 0 && !s.isAtEnd() {
		if s.peek() == '/' && s.peekNext() == '*' {
			nestedStart := s.pos()
			s.advanceByte() // '/'
			s.advanceByte() // '*'
			openSpans = append(openSpans, diagnostics.NewSpan(s.file, nestedStart, s.pos()))
			depth++
			continue
		}
		if s.peek() == '*' && s.peekNext() == '/' {
			s.advanceByte() // '*'
			s.advanceByte() // '/'
			depth--
			openSpans = openSpans[:len(openSpans)-1]
			continue
		}
		s.advanceRune()
	}

	if depth > 0 {
		unclosed := openSpans[len(openSpans)-1]
		help := "adicione '*/' para fechar este comentário de bloco"
		if len(openSpans) > 1 {
			outer := openSpans[0]
			help = fmt.Sprintf(
				"comentário externo iniciado em %d:%d; faltam %d fechamento(s) '*/'",
				outer.StartLine, outer.StartCol, depth,
			)
		}
		s.diag.Error(
			ErrUnterminatedBlock,
			unclosed,
			fmt.Sprintf("comentário de bloco não fechado (profundidade pendente: %d)", depth),
			"comentário aberto aqui nunca foi fechado",
			help,
		)
	}
}

// scanIdentifier lê [a-zA-Z_][a-zA-Z0-9_]* e consulta a tabela O(1) de palavras reservadas.
func (s *Scanner) scanIdentifier(start diagnostics.Position) Token {
	for !s.isAtEnd() && isIdentPart(s.peek()) {
		s.advanceByte()
	}
	lexeme := s.src[start.Offset:s.offset]
	tokType := LookupIdent(lexeme)
	return Token{
		Type:   tokType,
		Lexeme: lexeme,
		Span:   diagnostics.NewSpan(s.file, start, s.pos()),
	}
}

const maxInt64Magnitude uint64 = 1 << 63 // 9223372036854775808 (válido apenas quando precedido por '-' unário)

// scanNumber lê literais inteiros decimais e valida estouro de 64 bits (preservando candidato a MinInt64).
func (s *Scanner) scanNumber(start diagnostics.Position) Token {
	for !s.isAtEnd() && isDigit(s.peek()) {
		s.advanceByte()
	}
	lexeme := s.src[start.Offset:s.offset]
	span := diagnostics.NewSpan(s.file, start, s.pos())

	uval, err := strconv.ParseUint(lexeme, 10, 64)
	if err != nil || uval > maxInt64Magnitude {
		s.diag.Error(
			ErrIntOverflow,
			span,
			fmt.Sprintf("literal inteiro '%s' excede a capacidade de 64 bits", lexeme),
			"estouro numérico (overflow)",
			"utilize um valor inteiro entre -9223372036854775808 e 9223372036854775807",
		)
		return Token{
			Type:   TokenIllegal,
			Lexeme: lexeme,
			Span:   span,
		}
	}

	if uval == maxInt64Magnitude {
		return Token{
			Type:              TokenIntLit,
			Lexeme:            lexeme,
			UintVal:           uval,
			MinInt64Candidate: true,
			Span:              span,
		}
	}

	return Token{
		Type:    TokenIntLit,
		Lexeme:  lexeme,
		IntVal:  int64(uval),
		UintVal: uval,
		Span:    span,
	}
}

// scanString lê um literal de string preservando a posição exata das aspas de abertura.
// Caso não seja fechada antes da quebra de linha ou do EOF, aponta para as aspas iniciais e recupera o scan.
func (s *Scanner) scanString(start diagnostics.Position) Token {
	s.advanceByte() // consome '"' de abertura

	var decoded strings.Builder
	for !s.isAtEnd() && s.peek() != '"' && s.peek() != '\n' && s.peek() != '\r' {
		if s.peek() == '\\' {
			escStart := s.pos()
			s.advanceByte() // consome '\'

			if s.isAtEnd() || s.peek() == '\n' || s.peek() == '\r' {
				break
			}

			escRune := s.advanceRune()
			switch escRune {
			case 'n':
				decoded.WriteByte('\n')
			case 't':
				decoded.WriteByte('\t')
			case 'r':
				decoded.WriteByte('\r')
			case '"':
				decoded.WriteByte('"')
			case '\\':
				decoded.WriteByte('\\')
			case '0':
				decoded.WriteByte(0)
			default:
				escSpan := diagnostics.NewSpan(s.file, escStart, s.pos())
				s.diag.Error(
					ErrInvalidEscape,
					escSpan,
					fmt.Sprintf("sequência de escape inválida '\\%c'", escRune),
					"escape desconhecido",
					`sequências válidas: \n, \t, \r, \", \\, \0`,
				)
			}
			continue
		}
		decoded.WriteRune(s.advanceRune())
	}

	if s.isAtEnd() || s.peek() != '"' {
		span := diagnostics.NewSpan(s.file, start, s.pos())
		s.diag.Error(
			ErrUnterminatedStr,
			span,
			"literal de string não terminado",
			fmt.Sprintf("string aberta na linha %d, coluna %d não foi fechada", start.Line, start.Column),
			fmt.Sprintf("adicione '\"' antes do fim da linha %d para fechar o literal", start.Line),
		)
		return Token{
			Type:   TokenIllegal,
			Lexeme: s.src[start.Offset:s.offset],
			Span:   span,
		}
	}

	s.advanceByte() // consome '"' de fechamento
	span := diagnostics.NewSpan(s.file, start, s.pos())
	return Token{
		Type:   TokenStringLit,
		Lexeme: s.src[start.Offset:s.offset],
		StrVal: decoded.String(),
		Span:   span,
	}
}

func (s *Scanner) makeToken(tokType TokenType, start diagnostics.Position) Token {
	return Token{
		Type:   tokType,
		Lexeme: s.src[start.Offset:s.offset],
		Span:   diagnostics.NewSpan(s.file, start, s.pos()),
	}
}

func (s *Scanner) pos() diagnostics.Position {
	return diagnostics.Position{
		Offset: s.offset,
		Line:   s.line,
		Column: s.col,
	}
}

func (s *Scanner) isAtEnd() bool {
	return s.offset >= len(s.src)
}

func (s *Scanner) peek() byte {
	if s.isAtEnd() {
		return 0
	}
	return s.src[s.offset]
}

func (s *Scanner) peekNext() byte {
	if s.offset+1 >= len(s.src) {
		return 0
	}
	return s.src[s.offset+1]
}

func (s *Scanner) match(expected byte) bool {
	if s.isAtEnd() || s.src[s.offset] != expected {
		return false
	}
	s.advanceByte()
	return true
}

func (s *Scanner) advanceByte() byte {
	b := s.src[s.offset]
	s.offset++
	if b == '\n' {
		s.line++
		s.col = 1
	} else if b != '\r' {
		s.col++
	}
	return b
}

func (s *Scanner) advanceRune() rune {
	r, width := utf8.DecodeRuneInString(s.src[s.offset:])
	s.offset += width
	if r == '\n' {
		s.line++
		s.col = 1
	} else if r != '\r' {
		s.col++
	}
	return r
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || isDigit(b)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
