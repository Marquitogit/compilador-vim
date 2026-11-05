@echo off
chcp 65001 > nul
cls
echo =====================================================================
echo           DEMONSTRAÇÃO DO COMPILADOR VIM (LINGUAGEM DIDÁTICA)
echo =====================================================================
echo.

echo [PASSO 1/4] Executando Algoritmo: Fatorial Recursivo
echo Comando: vim.exe --run exemplos\validos\01_fatorial_recursivo.vim
echo ---------------------------------------------------------------------
vim.exe --run exemplos\validos\01_fatorial_recursivo.vim
echo.
pause

cls
echo [PASSO 2/4] Executando Bubble Sort com Arrays Mutáveis por Referência
echo Comando: vim.exe --opt --run --stats exemplos\validos\04_arrays_bubblesort.vim
echo ---------------------------------------------------------------------
vim.exe --opt --run --stats exemplos\validos\04_arrays_bubblesort.vim
echo.
pause

cls
echo [PASSO 3/4] Otimizador de Código (--opt): 57 instruções -> 4 instruções!
echo Comando: vim.exe --opt --dump-ir exemplos\validos\05_otimizacao_constantes.vim
echo ---------------------------------------------------------------------
vim.exe --opt --dump-ir exemplos\validos\05_otimizacao_constantes.vim
echo.
pause

cls
echo [PASSO 4/4] Diagnóstico Visual de Erro em Tempo de Execução (E0301)
echo Comando: vim.exe --run exemplos\invalidos\erro_runtime_divisao_zero.vim
echo ---------------------------------------------------------------------
vim.exe --run exemplos\invalidos\erro_runtime_divisao_zero.vim
echo.
echo =====================================================================
echo                 FIM DA DEMONSTRAÇÃO PRÁTICA
echo =====================================================================
pause
