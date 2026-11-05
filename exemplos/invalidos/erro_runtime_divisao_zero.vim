/* ====================================================================
   Caso Demonstrativo de Erro em Tempo de Execução (E0301): Divisão por Zero
   - Aprovado estaticamente por: .\nova.exe --check exemplos/invalidos/erro_runtime_divisao_zero.nova
   - Capturado na Stack VM em  : .\nova.exe --run exemplos/invalidos/erro_runtime_divisao_zero.nova
   ==================================================================== */

fn calcularMedia(somaTotal: int, quantidadeItens: int) -> int {
    return somaTotal / quantidadeItens;
}

let itensAtivos: int = 3;
while (itensAtivos > 0) {
    itensAtivos = itensAtivos - 1;
}

// Em tempo de execução, 'itensAtivos' chega a 0 e dispara E0301 na linha 8!
let media: int = calcularMedia(150, itensAtivos);
print("Media:", media);
