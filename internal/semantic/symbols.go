package semantic

import "vim/internal/diagnostics"

// SymbolKind categoriza a natureza semântica de uma entrada na Tabela de Símbolos.
type SymbolKind string

const (
	KindVar       SymbolKind = "var"
	KindConst     SymbolKind = "const"
	KindParam     SymbolKind = "param"
	KindFn        SymbolKind = "fn"
	KindIntrinsic SymbolKind = "intrinsic"
)

// Symbol armazena os metadados estáticos de um identificador declarado.
type Symbol struct {
	Name      string
	Kind      SymbolKind
	Type      Type
	DefSpan   diagnostics.Span
	IsMutable bool
}

// Scope representa um nível léxico na Tabela de Símbolos encadeada (global -> função -> blocos `{}`).
type Scope struct {
	Name     string
	Parent   *Scope
	Symbols  map[string]*Symbol
	Order    []*Symbol // Preserva ordem determinística de inserção para inspeção didática
	Children []*Scope
}

// NewScope cria um novo escopo encadeado ao escopo pai informado.
func NewScope(name string, parent *Scope) *Scope {
	sc := &Scope{
		Name:    name,
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
	if parent != nil {
		parent.Children = append(parent.Children, sc)
	}
	return sc
}

// Define registra um símbolo no escopo atual.
// Caso já exista outro símbolo com o mesmo nome neste mesmo escopo léxico, retorna (existente, false).
func (s *Scope) Define(sym *Symbol) (*Symbol, bool) {
	if existing, exists := s.Symbols[sym.Name]; exists {
		return existing, false
	}
	s.Symbols[sym.Name] = sym
	s.Order = append(s.Order, sym)
	return nil, true
}

// LookupLocal consulta apenas o escopo léxico corrente (sem subir para os pais).
func (s *Scope) LookupLocal(name string) (*Symbol, bool) {
	sym, ok := s.Symbols[name]
	return sym, ok
}

// Lookup percorre a cadeia de escopos do mais interno até o escopo global (permitindo shadowing).
func (s *Scope) Lookup(name string) (*Symbol, bool) {
	for cur := s; cur != nil; cur = cur.Parent {
		if sym, ok := cur.Symbols[name]; ok {
			return sym, true
		}
	}
	return nil, false
}

// CountScopes retorna recursivamente o total de escopos na subárvore iniciada em `s`.
func (s *Scope) CountScopes() int {
	if s == nil {
		return 0
	}
	total := 1
	for _, ch := range s.Children {
		total += ch.CountScopes()
	}
	return total
}

// CountUserSymbols retorna recursivamente o total de símbolos declarados pelo usuário (excluindo intrínsecos).
func (s *Scope) CountUserSymbols() int {
	if s == nil {
		return 0
	}
	total := 0
	for _, sym := range s.Order {
		if sym.Kind != KindIntrinsic {
			total++
		}
	}
	for _, ch := range s.Children {
		total += ch.CountUserSymbols()
	}
	return total
}
