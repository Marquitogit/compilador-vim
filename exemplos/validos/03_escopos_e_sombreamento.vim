/* ====================================================================
   Exemplo 03: Escopos Aninhados, Shadowing e Recursão Mútua
   Demonstra:
   - Passada 1/2: chamada antecipada e recursão mútua (ehPar <-> ehImpar)
   - Shadowing legal entre escopo global, função e blocos internos `{}`
   - Inferência contextual de array vazio `let historico: []int = [];`
   ==================================================================== */

const TITULO: string = "Analise de Escopos";

// Chamada antes das declarações de ehPar e ehImpar (forward reference)
let seisEhPar: bool = ehPar(6);

fn ehPar(n: int) -> bool {
    if (n == 0) {
        return true;
    }
    return ehImpar(n - 1);
}

fn ehImpar(n: int) -> bool {
    if (n == 0) {
        return false;
    }
    return ehPar(n - 1);
}

let valor: int = 100;
let historico: []int = [];
push(historico, valor);

{
    // Shadowing permitido em escopo léxico mais interno (com tipo diferente!)
    let valor: string = TITULO + " - Bloco Nivel 1";
    print(valor);

    {
        let valor: bool = seisEhPar && len(historico) == 1;
        print("Bloco Nivel 2:", valor);
    }
}

print("Valor global intacto:", valor);
