package diagnostics

// Severity define a gravidade de um diagnóstico emitido pelo compilador.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "aviso"
	default:
		return "erro"
	}
}

// Diagnostic representa uma mensagem estruturada de erro ou aviso vinculada a um Span.
type Diagnostic struct {
	Severity Severity
	Code     string // Ex: "E0001" (léxico), "E0101" (sintático), "E0201" (semântico)
	Message  string // Descrição principal do problema
	Label    string // Rótulo curto posicionado logo após o sublinhado ^^^
	Help     string // Sugestão opcional de correção
	Span     Span
}

// Bag acumula diagnósticos ao longo das fases para reportar todos os erros de uma só vez.
type Bag struct {
	items      []Diagnostic
	errorCount int
}

// NewBag cria um novo acumulador vazio de diagnósticos.
func NewBag() *Bag {
	return &Bag{}
}

// Add registra um diagnóstico arbitrário no acumulador.
func (b *Bag) Add(d Diagnostic) {
	if d.Severity == SeverityError {
		b.errorCount++
	}
	b.items = append(b.items, d)
}

// Error adiciona um diagnóstico de erro com código, mensagem, rótulo e dica opcional.
func (b *Bag) Error(code string, span Span, message, label, help string) {
	b.Add(Diagnostic{
		Severity: SeverityError,
		Code:     code,
		Message:  message,
		Label:    label,
		Help:     help,
		Span:     span,
	})
}

// HasErrors informa se ao menos um erro foi acumulado.
func (b *Bag) HasErrors() bool {
	return b.errorCount > 0
}

// ErrorCount retorna o total de erros registrados.
func (b *Bag) ErrorCount() int {
	return b.errorCount
}

// Items retorna uma cópia da lista de diagnósticos acumulados em ordem de ocorrência.
func (b *Bag) Items() []Diagnostic {
	out := make([]Diagnostic, len(b.items))
	copy(out, b.items)
	return out
}
