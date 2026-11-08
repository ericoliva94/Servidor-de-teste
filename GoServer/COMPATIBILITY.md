# Compatibilidade com o cliente Aika Global

## O que está conferido (automático)

* Cifra e enquadramento: `internal/protocol` contra as rotinas de `EncDec.pas`.
* Layout de **319 records** (`Packets.pas`, `PlayerData.pas`, `MiscData.pas`…):
  `binary.Size` igual ao `sizeof` do Delphi, e teste de ida e volta.
* Tabelas `.bin` e CSV: lidas dos arquivos reais de `Server/Bin/Data`.
* Códigos e tamanhos dos pacotes de amigo (`$372 $673 $870 $96F $971 $975 $F74
  $F26 $F27 $F30`): `TestTamanhosDosPacotesDeAmigo`.
* Cobertura de opcodes: `go run ./cmd/opdiff` (132/140 no canal).

## O que NÃO está conferido

* **Nenhum teste contra o cliente real.** Não há cliente nem MySQL na máquina
  de desenvolvimento. Cada divergência intencional do Delphi está em `PORTE.md`;
  as que podem afetar o cliente (campos zerados, mensagens sem acento) estão
  também em `KNOWN_ISSUES.md`.
* **Troca de canal e multi-canal** (`0xF05`/`0xF06`/`0xF0B`): nunca vista por um
  cliente. Em particular, supõe-se que o cliente conecte ao IP do `SL.bin` do
  canal pedido e mande o `$F06` com o id da conta; foi lido do Delphi, não
  observado.
* Ordem exata em que o cliente espera pacotes em sequência (ex.: `$870` antes de
  `$96F` ao entrar). Foi seguida a ordem do Delphi (`Player.pas:6725`).

## Regra

Mudança de protocolo externo (código, tamanho, ordem de campo) é proibida sem
teste que a justifique. Mudança interna é livre se os testes de layout passam.
