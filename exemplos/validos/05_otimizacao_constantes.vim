/* ====================================================================
   Exemplo 05: Demonstração do Otimizador (--opt)
   Compare o bytecode antes e depois da otimização:
     .\nova.exe --dump-ir --stats exemplos/validos/05_otimizacao_constantes.nova
     .\nova.exe --opt --dump-ir --stats exemplos/validos/05_otimizacao_constantes.nova
   ==================================================================== */

const HORAS_DIA: int = 24;
const MINUTOS_HORA: int = 60;
const SEGUNDOS_MIN: int = 60;
const MODO_DEBUG: bool = false;

fn segundosEmDias(dias: int) -> int {
    // Eliminado integralmente por Constant Folding + Dead Code Elimination (--opt):
    if (MODO_DEBUG || (100 < 50 && !false)) {
        print("LOG DEBUG: este ramo inteiro desaparece do bytecode!");
        let inutil: int = 999 * 888 + 777;
        print(inutil);
    }

    // Simplificado diretamente para o corpo do 'if' (condição constante verdadeira):
    if (2 + 2 == 4 && !MODO_DEBUG) {
        return dias * (HORAS_DIA * MINUTOS_HORA * SEGUNDOS_MIN) + (50 - 10 * 5);
    }

    // Código morto após retorno garantido (também eliminado com --opt):
    let inalcancavel: int = 1 + 2 + 3 + 4;
    return inalcancavel;
}

let cabecalho: string = "Segundos " + "em " + "7 dias:";
let total: int = segundosEmDias(7);
print(cabecalho, total);
