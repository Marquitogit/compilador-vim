/* ====================================================================
   Caso Demonstrativo de Erro em Tempo de Execução (E0302): Índice Fora dos Limites
   - Aprovado estaticamente por: .\nova.exe --check exemplos/invalidos/erro_runtime_indice_fora.nova
   - Capturado na Stack VM em  : .\nova.exe --run exemplos/invalidos/erro_runtime_indice_fora.nova
   ==================================================================== */

let notas: []int = [85, 92, 78, 96];
let soma: int = 0;

// Bug clássico de off-by-one ('<=' em vez de '<'): na última iteração i == 4!
for (let i: int = 0; i <= len(notas); i = i + 1) {
    soma = soma + notas[i];
}

print("Soma:", soma);
