// Caso de Rejeição Semântica: Análise de Retorno Garantido e Return Fora de Função

fn calcularDesconto(valor: int) -> int {
    if (valor > 100) {
        return valor - 15;
    } else if (valor > 50) {
        return valor - 5;
    }
    // Erro E0208: faltou o ramo 'else' ou um 'return' final quando valor <= 50!
}

// Erro E0207: comando 'return' proibido no escopo global
return 0;
