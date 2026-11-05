package parser

import (
	"fmt"

	"vim/internal/diagnostics"
	"vim/internal/lexer"
)

// Códigos padronizados de diagnósticos sintáticos.
const (
	ErrNestedFunction = "E0101"
	ErrInvalidLValue  = "E0102"
	ErrExpectedToken  = "E0103"
	ErrExpectedExpr   = "E0104"
	ErrExpectedType   = "E0105"
	ErrUnexpectedTok  = "E0106"
)

// Parser implementa o analisador sintático descendente recursivo acoplado ao Pratt Parser.
type Parser struct {
	tokens    []lexer.Token
	pos       int
	needsSync bool
	diag      *diagnostics.Bag
}

// New cria uma vim instância do Parser sobre uma fatia de tokens já escaneada.
func New(tokens []lexer.Token, diag *diagnostics.Bag) *Parser {
	if diag == nil {
		diag = diagnostics.NewBag()
	}
	return &Parser{
		tokens: tokens,
		pos:    0,
		diag:   diag,
	}
}

// ParseSource executa conveniência ponta a ponta: Scanner -> Parser (sem desugaring).
func ParseSource(file, src string, diag *diagnostics.Bag) (*Program, []lexer.Token) {
	if diag == nil {
		diag = diagnostics.NewBag()
	}
	sc := lexer.New(file, src, diag)
	tokens := sc.ScanAll()
	p := New(tokens, diag)
	return p.ParseProgram(), tokens
}

// ParseProgram analisa a raiz `Program ::= TopLevelDecl* EOF`.
// Declarações `fn` são permitidas estritamente neste nível superior.
func (p *Parser) ParseProgram() *Program {
	var decls []Stmt

	var startSpan diagnostics.Span
	if len(p.tokens) > 0 {
		startSpan = p.tokens[0].Span
	}

	for !p.isAtEnd() {
		startPos := p.pos
		var decl Stmt

		if p.check(lexer.TokenFn) {
			decl = p.parseFnDecl()
		} else if p.check(lexer.TokenRBrace) {
			bad := p.advance()
			p.diag.Error(
				ErrUnexpectedTok,
				bad.Span,
				"delimitador '}' inesperado no escopo global",
				"chave de fechamento sem '{' correspondente",
				"remova este '}' excedente",
			)
			continue
		} else {
			decl = p.parseStmt()
		}

		if decl != nil {
			decls = append(decls, decl)
		} else if p.needsSync {
			if p.pos == startPos && !p.isAtEnd() {
				p.advance()
			}
			p.synchronize(false)
		} else if p.pos == startPos && !p.isAtEnd() {
			p.advance()
		}
	}

	progSpan := startSpan
	if len(decls) > 0 {
		progSpan = decls[0].Span().Merge(decls[len(decls)-1].Span())
	} else if len(p.tokens) > 0 {
		progSpan = p.tokens[len(p.tokens)-1].Span
	}

	prog := &Program{
		Decls:    decls,
		SpanInfo: progSpan,
	}
	validateIntBounds(prog, p.diag)
	return prog
}

// parseFnDecl analisa `FnDecl ::= "fn" IDENT "(" ParamList? ")" ( "->" Type )? Block`.
func (p *Parser) parseFnDecl() Stmt {
	fnTok := p.advance() // consome 'fn'

	nameTok, ok := p.expect(lexer.TokenIdent, "esperado nome da função após 'fn'", "informe um identificador válido, ex: 'fn calcular(...) -> int'")
	if !ok {
		return nil
	}

	if _, ok := p.expect(lexer.TokenLParen, fmt.Sprintf("esperado '(' após o nome da função %q", nameTok.Lexeme), ""); !ok {
		return nil
	}

	var params []Param
	if !p.check(lexer.TokenRParen) {
		for {
			paramName, ok := p.expect(lexer.TokenIdent, "esperado nome do parâmetro na assinatura da função", "formato esperado: 'nome: tipo'")
			if !ok {
				return nil
			}
			if _, ok := p.expect(lexer.TokenColon, fmt.Sprintf("esperado ':' após o parâmetro %q", paramName.Lexeme), "todo parâmetro requer tipo explícito, ex: 'x: int'"); !ok {
				return nil
			}
			paramType := p.parseType()
			if paramType == nil {
				return nil
			}
			params = append(params, Param{
				Name:     paramName.Lexeme,
				NameSpan: paramName.Span,
				Type:     paramType,
				SpanInfo: paramName.Span.Merge(paramType.Span()),
			})

			if !p.match(lexer.TokenComma) {
				break
			}
			if p.check(lexer.TokenRParen) {
				break
			}
		}
	}

	rparenTok, ok := p.expect(lexer.TokenRParen, "esperado ')' após lista de parâmetros", "")
	if !ok {
		return nil
	}

	var retType TypeNode
	if p.match(lexer.TokenArrow) {
		retType = p.parseType()
		if retType == nil {
			return nil
		}
	} else {
		// Retorno omitido equivale explicitamente a `void`
		retType = &PrimType{
			Name:     "void",
			SpanInfo: rparenTok.Span,
		}
	}

	body := p.parseBlockStmt()
	if body == nil {
		return nil
	}

	return &FnDecl{
		Name:       nameTok.Lexeme,
		NameSpan:   nameTok.Span,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		SpanInfo:   fnTok.Span.Merge(body.Span()),
	}
}

// parseStmt analisa qualquer comando válido dentro de blocos ou fora de `FnDecl`.
// Proíbe explicitamente declarações `fn` aninhadas.
func (p *Parser) parseStmt() Stmt {
	switch p.peek().Type {
	case lexer.TokenFn:
		fnTok := p.peek()
		errSpan := fnTok.Span
		if p.peekNext().Type == lexer.TokenIdent {
			errSpan = errSpan.Merge(p.peekNext().Span)
		}
		p.diag.Error(
			ErrNestedFunction,
			errSpan,
			"declaração de função aninhada não é permitida",
			"funções só podem ser declaradas no escopo global (top-level)",
			"mova esta função para fora de blocos '{ ... }' ou de outras funções",
		)
		// Consome a estrutura da função aninhada para não gerar erros falsos no '}' seguinte
		_ = p.parseFnDecl()
		return nil

	case lexer.TokenLet:
		return p.parseVarDecl(true)

	case lexer.TokenConst:
		return p.parseConstDecl()

	case lexer.TokenIf:
		return p.parseIfStmt()

	case lexer.TokenWhile:
		return p.parseWhileStmt()

	case lexer.TokenFor:
		return p.parseForStmt()

	case lexer.TokenReturn:
		return p.parseReturnStmt()

	case lexer.TokenLBrace:
		return p.parseBlockStmt()

	default:
		return p.parseAssignOrExprStmt()
	}
}

// parseVarDecl analisa `let IDENT ":" Type "=" Expr (";")?`.
func (p *Parser) parseVarDecl(requireSemi bool) Stmt {
	letTok := p.advance() // consome 'let'

	nameTok, ok := p.expect(lexer.TokenIdent, "esperado nome da variável após 'let'", "exemplo: 'let contador: int = 0;'")
	if !ok {
		return nil
	}

	if _, ok := p.expect(lexer.TokenColon, fmt.Sprintf("esperado ':' e tipo estático após a variável %q", nameTok.Lexeme), "a linguagem vim exige anotação explícita de tipo, ex: 'let x: int = 10;'"); !ok {
		return nil
	}

	typ := p.parseType()
	if typ == nil {
		return nil
	}

	if _, ok := p.expect(lexer.TokenAssign, fmt.Sprintf("esperado '=' para inicializar a variável %q", nameTok.Lexeme), "toda variável deve ser inicializada na declaração"); !ok {
		return nil
	}

	val := p.parseExpr(precLowest)
	if val == nil {
		return nil
	}

	endSpan := val.Span()
	if requireSemi {
		semiTok, ok := p.expect(lexer.TokenSemicolon, "esperado ';' ao final da declaração 'let'", "")
		if !ok {
			return nil
		}
		endSpan = semiTok.Span
	}

	return &VarDecl{
		Name:     nameTok.Lexeme,
		NameSpan: nameTok.Span,
		Type:     typ,
		Value:    val,
		SpanInfo: letTok.Span.Merge(endSpan),
	}
}

// parseConstDecl analisa `const IDENT ":" Type "=" Expr ";"`.
func (p *Parser) parseConstDecl() Stmt {
	constTok := p.advance() // consome 'const'

	nameTok, ok := p.expect(lexer.TokenIdent, "esperado nome da constante após 'const'", "exemplo: 'const MAX: int = 100;'")
	if !ok {
		return nil
	}

	if _, ok := p.expect(lexer.TokenColon, fmt.Sprintf("esperado ':' e tipo estático após a constante %q", nameTok.Lexeme), "exemplo: 'const MAX: int = 100;'"); !ok {
		return nil
	}

	typ := p.parseType()
	if typ == nil {
		return nil
	}

	if _, ok := p.expect(lexer.TokenAssign, fmt.Sprintf("esperado '=' para definir o valor da constante %q", nameTok.Lexeme), ""); !ok {
		return nil
	}

	val := p.parseExpr(precLowest)
	if val == nil {
		return nil
	}

	semiTok, ok := p.expect(lexer.TokenSemicolon, "esperado ';' ao final da declaração 'const'", "")
	if !ok {
		return nil
	}

	return &ConstDecl{
		Name:     nameTok.Lexeme,
		NameSpan: nameTok.Span,
		Type:     typ,
		Value:    val,
		SpanInfo: constTok.Span.Merge(semiTok.Span),
	}
}

// parseBlockStmt analisa `"{" Statement* "}"` com sincronização interna por comando.
func (p *Parser) parseBlockStmt() *BlockStmt {
	openTok, ok := p.expect(lexer.TokenLBrace, "esperado '{' para iniciar bloco de comandos", "")
	if !ok {
		return nil
	}

	var stmts []Stmt
	for !p.check(lexer.TokenRBrace) && !p.isAtEnd() {
		startPos := p.pos
		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		} else if p.needsSync {
			if p.pos == startPos && !p.check(lexer.TokenRBrace) && !p.isAtEnd() {
				p.advance()
			}
			p.synchronize(true)
		} else if p.pos == startPos && !p.check(lexer.TokenRBrace) && !p.isAtEnd() {
			p.advance()
		}
	}

	closeTok, ok := p.expect(
		lexer.TokenRBrace,
		"esperado '}' para fechar o bloco",
		fmt.Sprintf("bloco '{' aberto na linha %d, coluna %d", openTok.Span.StartLine, openTok.Span.StartCol),
	)
	if !ok {
		return nil
	}

	return &BlockStmt{
		Stmts:    stmts,
		SpanInfo: openTok.Span.Merge(closeTok.Span),
	}
}

// parseIfStmt analisa `"if" "(" Expr ")" Block ( "else" ( IfStmt | Block ) )?`.
func (p *Parser) parseIfStmt() Stmt {
	ifTok := p.advance() // consome 'if'

	if _, ok := p.expect(lexer.TokenLParen, "esperado '(' após 'if'", "exemplo: 'if (x > 0) { ... }'"); !ok {
		return nil
	}
	cond := p.parseExpr(precLowest)
	if cond == nil {
		return nil
	}
	if _, ok := p.expect(lexer.TokenRParen, "esperado ')' após a condição do 'if'", ""); !ok {
		return nil
	}

	thenBlock := p.parseBlockStmt()
	if thenBlock == nil {
		return nil
	}

	endSpan := thenBlock.Span()
	var elseBranch Stmt
	if p.match(lexer.TokenElse) {
		if p.check(lexer.TokenIf) {
			elseBranch = p.parseIfStmt()
		} else {
			elseBranch = p.parseBlockStmt()
		}
		if elseBranch == nil {
			return nil
		}
		endSpan = elseBranch.Span()
	}

	return &IfStmt{
		Cond:     cond,
		Then:     thenBlock,
		Else:     elseBranch,
		SpanInfo: ifTok.Span.Merge(endSpan),
	}
}

// parseWhileStmt analisa `"while" "(" Expr ")" Block`.
func (p *Parser) parseWhileStmt() Stmt {
	whileTok := p.advance() // consome 'while'

	if _, ok := p.expect(lexer.TokenLParen, "esperado '(' após 'while'", "exemplo: 'while (i < 10) { ... }'"); !ok {
		return nil
	}
	cond := p.parseExpr(precLowest)
	if cond == nil {
		return nil
	}
	if _, ok := p.expect(lexer.TokenRParen, "esperado ')' após a condição do 'while'", ""); !ok {
		return nil
	}

	body := p.parseBlockStmt()
	if body == nil {
		return nil
	}

	return &WhileStmt{
		Cond:     cond,
		Body:     body,
		SpanInfo: whileTok.Span.Merge(body.Span()),
	}
}

// parseForStmt analisa `"for" "(" ForInit ";" Expr? ";" ForStep? ")" Block`.
func (p *Parser) parseForStmt() Stmt {
	forTok := p.advance() // consome 'for'

	if _, ok := p.expect(lexer.TokenLParen, "esperado '(' após 'for'", "exemplo: 'for (let i: int = 0; i < 10; i = i + 1) { ... }'"); !ok {
		return nil
	}

	// 1. Cláusula de inicialização (ForInit)
	var initStmt Stmt
	if p.match(lexer.TokenSemicolon) {
		initStmt = nil
	} else if p.check(lexer.TokenLet) {
		initStmt = p.parseVarDecl(true) // consome o ';' ao final
		if initStmt == nil {
			return nil
		}
	} else {
		initStmt = p.parseForAssignNoSemi(true)
		if initStmt == nil {
			return nil
		}
		if _, ok := p.expect(lexer.TokenSemicolon, "esperado ';' após inicialização do laço 'for'", ""); !ok {
			return nil
		}
	}

	// 2. Cláusula de condição (Expr?)
	var cond Expr
	if !p.check(lexer.TokenSemicolon) {
		cond = p.parseExpr(precLowest)
		if cond == nil {
			return nil
		}
	}
	if _, ok := p.expect(lexer.TokenSemicolon, "esperado ';' após condição do laço 'for'", ""); !ok {
		return nil
	}

	// 3. Cláusula de passo (ForStep?)
	var stepStmt Stmt
	if !p.check(lexer.TokenRParen) {
		stepStmt = p.parseForAssignNoSemi(false)
		if stepStmt == nil {
			return nil
		}
	}

	if _, ok := p.expect(lexer.TokenRParen, "esperado ')' após cláusula de passo do 'for'", ""); !ok {
		return nil
	}

	body := p.parseBlockStmt()
	if body == nil {
		return nil
	}

	return &ForStmt{
		Init:     initStmt,
		Cond:     cond,
		Step:     stepStmt,
		Body:     body,
		SpanInfo: forTok.Span.Merge(body.Span()),
	}
}

// parseForAssignNoSemi analisa atribuição ou expressão sem ';' nas cláusulas do `for`.
func (p *Parser) parseForAssignNoSemi(mustBeAssign bool) Stmt {
	lhs := p.parseExpr(precLowest)
	if lhs == nil {
		return nil
	}

	if p.match(lexer.TokenAssign) {
		if !IsValidLValue(lhs) {
			p.diag.Error(
				ErrInvalidLValue,
				lhs.Span(),
				"lado esquerdo da atribuição inválido",
				"esta expressão não é um L-Value atribuível",
				"use uma variável (ex: 'i = 0') ou posição de array (ex: 'arr[i] = 0')",
			)
			_ = p.parseExpr(precLowest)
			return nil
		}
		rhs := p.parseExpr(precLowest)
		if rhs == nil {
			return nil
		}
		return &AssignStmt{
			Target:   lhs,
			Value:    rhs,
			SpanInfo: lhs.Span().Merge(rhs.Span()),
		}
	}

	if mustBeAssign {
		p.diag.Error(
			ErrExpectedToken,
			lhs.Span(),
			"inicialização inválida no cabeçalho do 'for'",
			"esperado 'let ...' ou atribuição com '='",
			"exemplo: 'for (let i: int = 0; i < n; i = i + 1)'",
		)
		p.needsSync = true
		return nil
	}

	return &ExprStmt{
		Expr:     lhs,
		SpanInfo: lhs.Span(),
	}
}

// parseReturnStmt analisa `"return" Expr? ";"`.
func (p *Parser) parseReturnStmt() Stmt {
	retTok := p.advance() // consome 'return'

	var val Expr
	if !p.check(lexer.TokenSemicolon) {
		val = p.parseExpr(precLowest)
		if val == nil {
			return nil
		}
	}

	semiTok, ok := p.expect(lexer.TokenSemicolon, "esperado ';' após comando 'return'", "")
	if !ok {
		return nil
	}

	return &ReturnStmt{
		Value:    val,
		SpanInfo: retTok.Span.Merge(semiTok.Span),
	}
}

// parseAssignOrExprStmt resolve de forma determinística (sem backtracking) `AssignStmt` vs `ExprStmt`.
func (p *Parser) parseAssignOrExprStmt() Stmt {
	lhs := p.parseExpr(precLowest)
	if lhs == nil {
		return nil
	}

	if p.match(lexer.TokenAssign) {
		if !IsValidLValue(lhs) {
			p.diag.Error(
				ErrInvalidLValue,
				lhs.Span(),
				"lado esquerdo da atribuição inválido",
				"esta expressão não pode receber atribuição (não é um L-Value)",
				"apenas identificadores ('x = ...') ou acessos indexados ('arr[i] = ...') podem preceder '='",
			)
			// Continua consumindo o lado direito e ';' para não gerar erros em cascata
			if rhs := p.parseExpr(precLowest); rhs != nil {
				_, _ = p.expect(lexer.TokenSemicolon, "esperado ';' após atribuição", "")
			}
			return nil
		}

		rhs := p.parseExpr(precLowest)
		if rhs == nil {
			return nil
		}
		semiTok, ok := p.expect(lexer.TokenSemicolon, "esperado ';' ao final da atribuição", "")
		if !ok {
			return nil
		}
		return &AssignStmt{
			Target:   lhs,
			Value:    rhs,
			SpanInfo: lhs.Span().Merge(semiTok.Span),
		}
	}

	semiTok, ok := p.expect(lexer.TokenSemicolon, "esperado ';' após expressão", "")
	if !ok {
		return nil
	}
	return &ExprStmt{
		Expr:     lhs,
		SpanInfo: lhs.Span().Merge(semiTok.Span),
	}
}

// parseType analisa `Type ::= PrimType | "[" "]" Type`.
func (p *Parser) parseType() TypeNode {
	tok := p.peek()
	switch tok.Type {
	case lexer.TokenIntType, lexer.TokenBoolType, lexer.TokenStringType, lexer.TokenVoidType:
		p.advance()
		return &PrimType{
			Name:     tok.Lexeme,
			SpanInfo: tok.Span,
		}

	case lexer.TokenLBracket:
		openTok := p.advance() // consome '['
		if _, ok := p.expect(lexer.TokenRBracket, "esperado ']' na anotação de tipo de array", "tipos de array usam o prefixo '[]Tipo', ex: '[]int'"); !ok {
			return nil
		}
		elem := p.parseType()
		if elem == nil {
			return nil
		}
		return &ArrayType{
			Elem:     elem,
			SpanInfo: openTok.Span.Merge(elem.Span()),
		}

	default:
		p.diag.Error(
			ErrExpectedType,
			tok.Span,
			fmt.Sprintf("anotação de tipo esperada, mas foi encontrado %q", tok.Lexeme),
			"tipo inválido",
			"tipos válidos: 'int', 'bool', 'string', 'void' ou arrays '[]Tipo'",
		)
		p.needsSync = true
		return nil
	}
}

// synchronize implementa a recuperação em modo pânico, descartando tokens até um ponto seguro.
func (p *Parser) synchronize(stopAtRBrace bool) {
	p.needsSync = false
	for !p.isAtEnd() {
		if p.pos > 0 && p.previous().Type == lexer.TokenSemicolon {
			return
		}

		switch p.peek().Type {
		case lexer.TokenLet,
			lexer.TokenConst,
			lexer.TokenFn,
			lexer.TokenIf,
			lexer.TokenWhile,
			lexer.TokenFor,
			lexer.TokenReturn:
			return
		case lexer.TokenRBrace:
			if stopAtRBrace {
				return
			}
		}

		p.advance()
	}
}

// ============================================================================
// Utilitários de Navegação de Tokens
// ============================================================================

func (p *Parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekNext() lexer.Token {
	if p.pos+1 >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) previous() lexer.Token {
	return p.tokens[p.pos-1]
}

func (p *Parser) isAtEnd() bool {
	return p.peek().Type == lexer.TokenEOF
}

func (p *Parser) check(t lexer.TokenType) bool {
	if p.isAtEnd() {
		return t == lexer.TokenEOF
	}
	return p.peek().Type == t
}

func (p *Parser) advance() lexer.Token {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.previous()
}

func (p *Parser) match(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) expect(expected lexer.TokenType, message, help string) (lexer.Token, bool) {
	if p.check(expected) {
		return p.advance(), true
	}
	got := p.peek()
	label := fmt.Sprintf("encontrado %q", got.Lexeme)
	if got.Type == lexer.TokenEOF {
		label = "fim inesperado do arquivo"
	}
	p.diag.Error(ErrExpectedToken, got.Span, message, label, help)
	p.needsSync = true
	return lexer.Token{}, false
}

// validateIntBounds verifica se algum literal 9223372036854775808 permaneceu positivo (sem '-' unário).
func validateIntBounds(n Node, diag *diagnostics.Bag) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *Program:
		for _, d := range v.Decls {
			validateIntBounds(d, diag)
		}
	case *FnDecl:
		validateIntBounds(v.Body, diag)
	case *VarDecl:
		validateIntBounds(v.Value, diag)
	case *ConstDecl:
		validateIntBounds(v.Value, diag)
	case *BlockStmt:
		for _, s := range v.Stmts {
			validateIntBounds(s, diag)
		}
	case *AssignStmt:
		validateIntBounds(v.Target, diag)
		validateIntBounds(v.Value, diag)
	case *IfStmt:
		validateIntBounds(v.Cond, diag)
		validateIntBounds(v.Then, diag)
		validateIntBounds(v.Else, diag)
	case *WhileStmt:
		validateIntBounds(v.Cond, diag)
		validateIntBounds(v.Body, diag)
	case *ForStmt:
		validateIntBounds(v.Init, diag)
		validateIntBounds(v.Cond, diag)
		validateIntBounds(v.Step, diag)
		validateIntBounds(v.Body, diag)
	case *ReturnStmt:
		validateIntBounds(v.Value, diag)
	case *ExprStmt:
		validateIntBounds(v.Expr, diag)
	case *ArrayLit:
		for _, el := range v.Elements {
			validateIntBounds(el, diag)
		}
	case *UnaryExpr:
		validateIntBounds(v.Operand, diag)
	case *BinaryExpr:
		validateIntBounds(v.Left, diag)
		validateIntBounds(v.Right, diag)
	case *CallExpr:
		validateIntBounds(v.Callee, diag)
		for _, a := range v.Args {
			validateIntBounds(a, diag)
		}
	case *IndexExpr:
		validateIntBounds(v.Collection, diag)
		validateIntBounds(v.Index, diag)
	case *IntLit:
		if v.RequiresNegation {
			diag.Error(
				lexer.ErrIntOverflow,
				v.SpanInfo,
				fmt.Sprintf("literal inteiro '%s' excede o máximo positivo de 64 bits (9223372036854775807)", v.Raw),
				"estouro numérico (overflow)",
				"este valor só é válido quando precedido por '-' (-9223372036854775808)",
			)
		}
	}
}
