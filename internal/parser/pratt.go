package parser

import (
	"fmt"
	"math"

	"vim/internal/lexer"
)

// precedence define os níveis de Binding Power (poder de ligação) do Pratt Parser.
type precedence int

const (
	precLowest     precedence = iota
	precOr                    // 1. || (associatividade à esquerda, curto-circuito)
	precAnd                   // 2. && (associatividade à esquerda, curto-circuito)
	precEquality              // 3. == != (associatividade à esquerda)
	precComparison            // 4. < <= > >= (associatividade à esquerda)
	precTerm                  // 5. + - (associatividade à esquerda)
	precFactor                // 6. * / % (associatividade à esquerda)
	precUnary                 // 7. ! - (prefixo unário)
	precPostfix               // 8. (...) chamada de função e [...] indexação
)

// bindingPower consulta a precedência infixa/pós-fixa de um tipo de token em O(1).
var bindingPower = map[lexer.TokenType]precedence{
	lexer.TokenOr:       precOr,
	lexer.TokenAnd:      precAnd,
	lexer.TokenEq:       precEquality,
	lexer.TokenNotEq:    precEquality,
	lexer.TokenLt:       precComparison,
	lexer.TokenLtEq:     precComparison,
	lexer.TokenGt:       precComparison,
	lexer.TokenGtEq:     precComparison,
	lexer.TokenPlus:     precTerm,
	lexer.TokenMinus:    precTerm,
	lexer.TokenStar:     precFactor,
	lexer.TokenSlash:    precFactor,
	lexer.TokenPercent:  precFactor,
	lexer.TokenLParen:   precPostfix,
	lexer.TokenLBracket: precPostfix,
}

func (p *Parser) curPrecedence() precedence {
	if prec, ok := bindingPower[p.peek().Type]; ok {
		return prec
	}
	return precLowest
}

// parseExpr implementa o núcleo do algoritmo de Pratt (Top-Down Operator Precedence).
func (p *Parser) parseExpr(minPrec precedence) Expr {
	left := p.parsePrefix()
	if left == nil {
		return nil
	}

	for !p.isAtEnd() && minPrec < p.curPrecedence() {
		tok := p.peek()
		switch tok.Type {
		case lexer.TokenPlus, lexer.TokenMinus,
			lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
			lexer.TokenEq, lexer.TokenNotEq,
			lexer.TokenLt, lexer.TokenLtEq, lexer.TokenGt, lexer.TokenGtEq,
			lexer.TokenAnd, lexer.TokenOr:
			left = p.parseBinaryExpr(left)
			if left == nil {
				return nil
			}
		case lexer.TokenLParen:
			left = p.parseCallExpr(left)
			if left == nil {
				return nil
			}
		case lexer.TokenLBracket:
			left = p.parseIndexExpr(left)
			if left == nil {
				return nil
			}
		default:
			return left
		}
	}

	return left
}

// parsePrefix trata átomos primários e operadores unários prefixos (`nud` - null denotation).
func (p *Parser) parsePrefix() Expr {
	tok := p.peek()

	switch tok.Type {
	case lexer.TokenIntLit:
		p.advance()
		return &IntLit{
			Value:            tok.IntVal,
			Raw:              tok.Lexeme,
			RequiresNegation: tok.MinInt64Candidate,
			SpanInfo:         tok.Span,
		}

	case lexer.TokenStringLit:
		p.advance()
		return &StringLit{
			Value:    tok.StrVal,
			Raw:      tok.Lexeme,
			SpanInfo: tok.Span,
		}

	case lexer.TokenTrue:
		p.advance()
		return &BoolLit{
			Value:    true,
			SpanInfo: tok.Span,
		}

	case lexer.TokenFalse:
		p.advance()
		return &BoolLit{
			Value:    false,
			SpanInfo: tok.Span,
		}

	case lexer.TokenIdent:
		p.advance()
		return &IdentExpr{
			Name:     tok.Lexeme,
			SpanInfo: tok.Span,
		}

	case lexer.TokenBang, lexer.TokenMinus:
		opTok := p.advance()
		operand := p.parseExpr(precUnary)
		if operand == nil {
			return nil
		}
		// Caso especial: -9223372036854775808 produz exatamente math.MinInt64 sem overflow.
		if opTok.Type == lexer.TokenMinus {
			if lit, ok := operand.(*IntLit); ok && lit.RequiresNegation {
				return &IntLit{
					Value:            math.MinInt64,
					Raw:              "-" + lit.Raw,
					RequiresNegation: false,
					SpanInfo:         opTok.Span.Merge(operand.Span()),
				}
			}
		}
		return &UnaryExpr{
			Op:       opTok.Type,
			OpLexeme: opTok.Lexeme,
			Operand:  operand,
			SpanInfo: opTok.Span.Merge(operand.Span()),
		}

	case lexer.TokenLParen:
		openTok := p.advance() // consome '('
		inner := p.parseExpr(precLowest)
		if inner == nil {
			return nil
		}
		closeTok, ok := p.expect(lexer.TokenRParen, "esperado ')' após expressão entre parênteses", fmt.Sprintf("'(' aberto na linha %d, coluna %d", openTok.Span.StartLine, openTok.Span.StartCol))
		if !ok {
			return nil
		}
		inner.SetSpan(openTok.Span.Merge(closeTok.Span))
		return inner

	case lexer.TokenLBracket:
		return p.parseArrayLit()

	case lexer.TokenIllegal:
		// O erro léxico já foi reportado pelo Scanner; apenas avança para evitar cascata duplicada.
		p.advance()
		p.needsSync = true
		return nil

	default:
		shown := tok.Lexeme
		if tok.Type == lexer.TokenEOF {
			shown = "fim do arquivo (EOF)"
		} else {
			shown = fmt.Sprintf("%q", shown)
		}
		p.diag.Error(
			ErrExpectedExpr,
			tok.Span,
			fmt.Sprintf("expressão esperada, mas foi encontrado %s", shown),
			"início de expressão inválido",
			"esperava literal, identificador, '(', '[' ou operador unário ('!' ou '-')",
		)
		p.needsSync = true
		return nil
	}
}

// parseBinaryExpr trata operadores binários infixos com associatividade à esquerda (`led` - left denotation).
func (p *Parser) parseBinaryExpr(left Expr) Expr {
	opTok := p.advance()
	prec := bindingPower[opTok.Type]
	right := p.parseExpr(prec)
	if right == nil {
		return nil
	}
	return &BinaryExpr{
		Left:     left,
		Op:       opTok.Type,
		OpLexeme: opTok.Lexeme,
		Right:    right,
		SpanInfo: left.Span().Merge(right.Span()),
	}
}

// parseCallExpr trata o sufixo pós-fixo de invocação `callee(arg1, arg2, ...)`.
func (p *Parser) parseCallExpr(callee Expr) Expr {
	openTok := p.advance() // consome '('
	var args []Expr

	if !p.check(lexer.TokenRParen) {
		for {
			arg := p.parseExpr(precLowest)
			if arg == nil {
				return nil
			}
			args = append(args, arg)
			if !p.match(lexer.TokenComma) {
				break
			}
			if p.check(lexer.TokenRParen) {
				break
			}
		}
	}

	closeTok, ok := p.expect(
		lexer.TokenRParen,
		"esperado ')' ao final da lista de argumentos da chamada",
		fmt.Sprintf("'(' da chamada aberto em %d:%d", openTok.Span.StartLine, openTok.Span.StartCol),
	)
	if !ok {
		return nil
	}

	return &CallExpr{
		Callee:   callee,
		Args:     args,
		SpanInfo: callee.Span().Merge(closeTok.Span),
	}
}

// parseIndexExpr trata o sufixo pós-fixo de indexação `collection[index]`.
func (p *Parser) parseIndexExpr(collection Expr) Expr {
	openTok := p.advance() // consome '['
	index := p.parseExpr(precLowest)
	if index == nil {
		return nil
	}

	closeTok, ok := p.expect(
		lexer.TokenRBracket,
		"esperado ']' após expressão de índice do array",
		fmt.Sprintf("'[' aberto em %d:%d", openTok.Span.StartLine, openTok.Span.StartCol),
	)
	if !ok {
		return nil
	}

	return &IndexExpr{
		Collection: collection,
		Index:      index,
		SpanInfo:   collection.Span().Merge(closeTok.Span),
	}
}

// parseArrayLit lê literais de array `[e1, e2, ...]` (incluindo array vazio `[]`).
func (p *Parser) parseArrayLit() Expr {
	openTok := p.advance() // consome '['
	var elems []Expr

	if !p.check(lexer.TokenRBracket) {
		for {
			el := p.parseExpr(precLowest)
			if el == nil {
				return nil
			}
			elems = append(elems, el)
			if !p.match(lexer.TokenComma) {
				break
			}
			if p.check(lexer.TokenRBracket) {
				break
			}
		}
	}

	closeTok, ok := p.expect(
		lexer.TokenRBracket,
		"esperado ']' para fechar o literal de array",
		fmt.Sprintf("literal de array iniciado em %d:%d", openTok.Span.StartLine, openTok.Span.StartCol),
	)
	if !ok {
		return nil
	}

	return &ArrayLit{
		Elements: elems,
		SpanInfo: openTok.Span.Merge(closeTok.Span),
	}
}
