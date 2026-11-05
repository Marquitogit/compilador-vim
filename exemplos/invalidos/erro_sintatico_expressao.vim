// Demonstração de Recuperação e Acumulação de Erros Sintáticos (Fase 2)

fn testarSintaxe(x: int) -> int {
    // Erro 1 [E0101]: função aninhada dentro de outra função (proibido)
    fn internaProibida(a: int) -> int {
        return a + 1;
    }

    // Erro 2 [E0102]: lado esquerdo da atribuição não é um L-Value válido
    (x + 10) = 50;

    // Erro 3 [E0104]: operando ausente em expressão aritmética (Pratt Parser)
    let calculo: int = 10 + * 3;

    // Após sincronização em modo pânico, o comando válido abaixo é preservado!
    let valido: int = x * 2;
    return valido;
}
