# Ferramentas da investigação do Maestri

São as ferramentas usadas para produzir `docs/01` a `docs/03`. Por higiene de "sala limpa", **nenhum material descompilado do Maestri foi guardado aqui**: nada de pseudocódigo, desmontagem ou dumps de strings. Fica só o que é ferramenta própria e o resumo dos achados.

| Arquivo | Uso |
|---|---|
| `kb/findings.md` | Achados consolidados durante a investigação, com endereços e evidências, em notas de trabalho. A versão final e organizada está em `docs/01`. |
| `tools/hop_bridge.py` | Ponte para o Hopper. Roda dentro dele com `hopper -e <bin> -l FAT --aarch64 -l Mach-O -Y hop_bridge.py`, segura um socket Unix na thread Python do Hopper e executa trechos enviados por `hop.py`. Serve quando o REA não consegue iniciar o Hopper a tempo (binários grandes) ou com o Hopper demo (que não salva `.hop`). Os caminhos `BASE` no início dos dois arquivos precisam ser ajustados. |
| `tools/hop.py` | Cliente da ponte: `python3 hop.py 'result = len(H.procs())'`. |
| `tools/extract_decomp.py`, `annotate_dir.py`, `swift_small_strings.py` | Separam resultados de decompilação por função e decodificam strings curtas do Swift (as que ficam dentro das instruções). |
| `tools/sockproxy.py` | Proxy que grava o tráfego de um socket Unix (para capturar o protocolo de uma CLI). |
| `tools/ptylog.py`, `ptylog2.py`, `ptylog_heavy.py` | Rodam dentro de um terminal e gravam, com horário, cada byte recebido (colagem delimitada, Enters). Simulam um agente "trabalhando" com padrões de saída configuráveis. |
