package lexer

import (
	"fmt"

	"vim/internal/diagnostics"
)

// TokenType identifica a categoria gramatical de cada unidade léxica da linguagem vim.
type TokenType int

const (
	// Controle / Especiais
	TokenEOF TokenType = iota
	TokenIllegal

	// Identificadores e Literais
	TokenIdent
	TokenIntLit
	TokenStringLit

	// Palavras-chave de Declaração e Controle
	TokenLet
	TokenConst
	TokenFn
	TokenReturn
	TokenIf
	TokenElse
	TokenWhile
	TokenFor
	TokenTrue
	TokenFalse

	// Palavras-chave de Tipos Primitivos
	TokenIntType
	TokenBoolType
	TokenStringType
	TokenVoidType

	// Operadores Aritméticos
	TokenPlus    // +
	TokenMinus   // -
	TokenStar    // *
	TokenSlash   // /
	TokenPercent // %

	// Operadores de Atribuição, Relacionais, Lógicos e Seta
	TokenAssign // =
	TokenEq     // ==
	TokenNotEq  // !=
	TokenBang   // !
	TokenLt     // <
	TokenLtEq   // <=
	TokenGt     // >
	TokenGtEq   // >=
	TokenAnd    // &&
	TokenOr     // ||
	TokenArrow  // ->

	// Delimitadores e Pontuação
	TokenLParen    // (
	TokenRParen    // )
	TokenLBrace    // {
	TokenRBrace    // }
	TokenLBracket  // [
	TokenRBracket  // ]
	TokenComma     // ,
	TokenColon     // :
	TokenSemicolon // ;
)

var tokenNames = map[TokenType]string{
	TokenEOF:        "EOF",
	TokenIllegal:    "ILLEGAL",
	TokenIdent:      "IDENT",
	TokenIntLit:     "INT_LIT",
	TokenStringLit:  "STRING_LIT",
	TokenLet:        "LET",
	TokenConst:      "CONST",
	TokenFn:         "FN",
	TokenReturn:     "RETURN",
	TokenIf:         "IF",
	TokenElse:       "ELSE",
	TokenWhile:      "WHILE",
	TokenFor:        "FOR",
	TokenTrue:       "TRUE",
	TokenFalse:      "FALSE",
	TokenIntType:    "TYPE_INT",
	TokenBoolType:   "TYPE_BOOL",
	TokenStringType: "TYPE_STRING",
	TokenVoidType:   "TYPE_VOID",
	TokenPlus:       "PLUS",
	TokenMinus:      "MINUS",
	TokenStar:       "STAR",
	TokenSlash:      "SLASH",
	TokenPercent:    "PERCENT",
	TokenAssign:     "ASSIGN",
	TokenEq:         "EQ",
	TokenNotEq:      "NOT_EQ",
	TokenBang:       "BANG",
	TokenLt:         "LT",
	TokenLtEq:       "LT_EQ",
	TokenGt:         "GT",
	TokenGtEq:       "GT_EQ",
	TokenAnd:        "AND",
	TokenOr:         "OR",
	TokenArrow:      "ARROW",
	TokenLParen:     "LPAREN",
	TokenRParen:     "RPAREN",
	TokenLBrace:     "LBRACE",
	TokenRBrace:     "RBRACE",
	TokenLBracket:   "LBRACKET",
	TokenRBracket:   "RBRACKET",
	TokenComma:      "COMMA",
	TokenColon:      "COLON",
	TokenSemicolon:  "SEMICOLON",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TOKEN(%d)", int(t))
}

// Tabela hash O(1) de palavras reservadas da linguagem vim.
var keywords = map[string]TokenType{
	"let":    TokenLet,
	"const":  TokenConst,
	"fn":     TokenFn,
	"return": TokenReturn,
	"if":     TokenIf,
	"else":   TokenElse,
	"while":  TokenWhile,
	"for":    TokenFor,
	"true":   TokenTrue,
	"false":  TokenFalse,
	"int":    TokenIntType,
	"bool":   TokenBoolType,
	"string": TokenStringType,
	"void":   TokenVoidType,
}

// LookupIdent classifica em tempo O(1) uma sequência alfanumérica como palavra-chave ou identificador.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TokenIdent
}

// Token representa uma unidade léxica imutável com seu intervalo exato (Span) no código-fonte.
type Token struct {
	Type              TokenType
	Lexeme            string           // Texto exato extraído do código-fonte [StartOffset:EndOffset]
	IntVal            int64            // Preenchido quando Type == TokenIntLit (0..MaxInt64)
	UintVal           uint64           // Magnitude sem sinal para suportar -9223372036854775808 (MinInt64)
	MinInt64Candidate bool             // Verdadeiro quando o literal é exatamente 9223372036854775808
	StrVal            string           // Valor decodificado (com escapes processados) quando Type == TokenStringLit
	Span              diagnostics.Span // Coordenadas completas de início e fim
}

func (t Token) String() string {
	switch t.Type {
	case TokenEOF:
		return fmt.Sprintf("%-12s %-20s [%s]", t.Type, "<eof>", t.Span)
	case TokenStringLit:
		return fmt.Sprintf("%-12s %-20q [%s]", t.Type, t.StrVal, t.Span)
	default:
		return fmt.Sprintf("%-12s %-20q [%s]", t.Type, t.Lexeme, t.Span)
	}
}
