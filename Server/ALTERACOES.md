# Alterações no servidor (25/10/2026)

Linguagem: Delphi 12 (Object Pascal), VCL + FireDAC (MySQL) + Winsock2 + Indy (HTTP).
Os originais dos arquivos alterados estão em `_backup_original\Src`.

## Rodando localmente

1. Feche o AikaEmu (AuthServer, GameServer e WebServer): ele usa as mesmas portas.
2. Rode `Bin\Iniciar_Servidor.bat`. Ele abre o `AikaServer.exe` e o `AikaProxy`.
3. Portas: login 8831, canais 127.0.0.1:8822 e 127.0.0.2:8822 (AikaProxy, que
   repassa para o servidor em 127.0.0.11/127.0.0.12), token HTTP 8090,
   admin HTTPS 9571, console 5555. O `Setting.txt` do cliente já aponta para
   `http://localhost:8090/`.
4. Os botões "Soft Reset" e "Fechar servidor" da janela do AikaServer fazem
   exatamente isso; não clique neles com jogadores online.

## AikaProxy (`Tools\AikaProxy`, publicado em `Bin\AikaProxy`)

O `AikaServer.exe` pré-compilado descarta o 2º pacote quando dois chegam no mesmo
segmento TCP (confirmado em teste: 3 pacotes juntos = 1 resposta). Enquanto o
servidor não é recompilado, o proxy separa o fluxo do cliente em pacotes inteiros
e entrega um por vez (intervalo mínimo `GapMs` = 40 ms, em `AikaProxy.json`).
Com ele, 3 pacotes juntos = 3 respostas. O `Bin\SL.bin` do servidor agora usa
127.0.0.11/127.0.0.12 para os canais (original em `SL.bin.orig`); o `SL.bin` do
cliente não muda. Depois de recompilar com `PlayerThread.pas` corrigido, o proxy
pode ser removido (volte o `SL.bin.orig`).

## Cliente

O servidor fala o protocolo do cliente **Aika Global** (ver `Client\Client Aika!.txt`).
O cliente AikaBR 2024 usa outra criptografia (cabeçalho mascarado, "V2") e outro
layout de pacotes, que este servidor não implementa.

Aika Global instalado em `C:\Users\erico\Downloads\AIKA\AikaOnlineGlobal`
(instalador oficial verificado: SHA-256 da CBM e assinatura Hanbitsoft):
- `SL.bin` = o do pacote (`Client\SL.bin`, canais em 127.0.0.1/127.0.0.2).
- `Setting.txt` linha 13 (servidor de token) = `http://localhost:8090/`; o patch
  continua no servidor oficial da CBM.
- Originais em `SL.bin.orig` / `Setting.txt.orig`. Se o launcher atualizar e
  sobrescrever esses arquivos, rode `Usar_Servidor_Local.bat` na pasta do cliente.
- Abra o jogo por `AikaLauncher_TK.exe`.
4. Contas (copiadas do AikaEmu, mesma senha): `admin`/`admin` (GM, account_type 5)
   e `teste`/`teste`.

## Banco de dados

- `Database\aika_db4.sql`: schema reconstruído a partir das queries do código (o
  pacote não tinha dump). Usuário root com a mesma senha do AikaEmu.
- `Database\import_aikaemu_accounts.sql`: copia `aikaemu_auth.accounts` para o
  `aika_db4` (o AikaEmu guarda o MD5, que é o que este servidor compara).
- `sql_mode` global do MySQL ficou `NO_ENGINE_SUBSTITUTION`: no modo estrito o
  código legado falha em vários INSERT/UPDATE e o dado não é salvo.
- `Bin\AikaServer.ini`, `Bin\config.ini` e `Bin\WebPage\WebPage.py` usam essa
  conexão (originais em `*.ini.orig`).

## Correções de perda de dados / disconnect (precisam recompilar no Delphi)

| Arquivo | Problema | Correção |
|---|---|---|
| `Threads\PlayerThread.pas` | Só o 1º pacote de cada `recv()` era tratado; os outros eram descartados e pacotes partidos eram processados pela metade (dessincroniza e derruba o cliente). | Buffer por conexão que separa o fluxo TCP em pacotes pelo tamanho do cabeçalho; trata o "hello" de 4 bytes. |
| `Threads\PlayerThread.pas` | `RecvPackets` chamado 2x por volta; `fCritSect` liberado sem `Enter`; `PlayersThreads` decrementado 2x; exceção no handler matava a thread sem desconectar. | Loop com `select()`, limpeza única em `Terminate`, exceções logadas. |
| `Threads\PlayerThread.pas` | Conexões que caíam antes do login nunca tinham o socket fechado. | `closesocket` em `Terminate` quando o `Disconnect` não fechou. |
| `Mob\Player.pas` (`SendPacket`) | Envio parcial ignorado; `WSAEWOULDBLOCK` descartava o pacote; várias threads escreviam no mesmo socket ao mesmo tempo (bytes misturados). | Trava por socket, laço até enviar tudo, espera o buffer liberar (até ~2 s). |
| `Connections\ServerSocket.pas` | Erro no handler de movimento ($301) ou de info de personagem ($22C) desconectava o jogador. | Só registra o erro. |
| `Connections\ServerSocket.pas` | `SaveTemplesDB` gravava `devir_id` 1..5 para toda nação: relíquias da nação 2 nunca eram salvas. | Usa o `DevirId` carregado. Buffer de envio do socket aumentado para 256 KB. |
| `Connections\SQL.pas` | No retry a query era recriada sem os parâmetros; erros de escrita eram engolidos sem log; reconexão em loop sem pausa. | Reconecta mantendo query/parâmetros, só repete quando o servidor caiu, loga toda falha, define `sql_mode` da sessão. |
| `Connections\LoginSocket.pas` | `RecvBytes: WORD` transformava `-1` em 65535: conexões com erro nunca eram liberadas (os 200 slots acabavam). `Exit` matava a thread de accept. `Leave` sem `Enter`. | `Integer`, `Continue`, travas removidas, socket fechado quando não há slot. |
| `Functions\Functions.pas`, `Mob\Player.pas` | `Integer(Ponteiro)` trunca endereços no Win64 (access violation). | `NativeInt`. |

`PlayerThread.pas` e `LoginSocket.pas` foram convertidos de Windows-1252 para UTF-8
com BOM (o Delphi 12 lê normalmente).
