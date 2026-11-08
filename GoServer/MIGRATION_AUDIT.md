# Auditoria da migração Delphi → Go

Auditoria feita sobre o código real em 2026-11-07. Os números abaixo foram
**medidos**, não copiados do enunciado. Para refazer a contagem de pacotes:

```bash
cd GoServer && go run ./cmd/opdiff -root ..
```

## 1. Premissas do enunciado que não bateram

| Enunciado | Realidade no repositório |
|---|---|
| "Delphi **e Python** implementam o jogo" | **Não há lógica de jogo em Python.** Os 8 `.py` são ferramentas: `GoServer/gen/*.py` (gerador das structs), `Scripts/aika_installer_gui.py` (instalador), `Server/Bin/WebPage/WebPage.py`, `Server/Bin/WebPainel/app.py`, `Server/Src/AdminWeb/WebAdmin.py` (painéis web) e `Server/Bin/Data/descript.py` (utilitário de dados). Toda regra de negócio, todo pacote e toda montagem de resposta está em Delphi. A única fonte de verdade para o comportamento é `Server/Src/*.pas`. |
| 56 de 140 códigos de pacote | **Confirmado**: 56 dos 140 `case` de `ServerSocket.pas` tinham contraparte Go. Hoje são **62** (ver §4). |
| ~80.800 linhas Delphi pendentes | Plausível. O `.dproj` compila 53 units / 85.116 linhas; o Go já cobre as fases 1–10v. Não recontei linha a linha por pendência — ver §6. |
| Estado inicial "29 de 34 linhas da tabela de fases" | Confere com `PORTE.md` (fases 1–10v prontas, 10w "a fazer"). |

Também não existe `git` no diretório recebido (nem `.git`): o único ponto de
restauração é `Servidor-de-teste-main.zip` em `Downloads`.

## 2. Estrutura

| Pasta | Conteúdo | Tocar? |
|---|---|---|
| `Server/Src` | Fonte Delphi 12 (referência do comportamento) | **Não alterar** |
| `Server/_backup_original` | Cópia do fonte antes das 36 correções | **Não alterar** |
| `Server/Bin`, `Server/Database` | Dados `.bin`, CSV, schema `aika_db4` | Só leitura |
| `Server/Tools/AikaProxy` | Proxy .NET 10 (contorna perda de pacote do binário Delphi) | Fora do escopo |
| `GoServer` | Porte em Go (módulo `aika`, Go 1.24, único dependente externo: driver MySQL) | **Onde se trabalha** |
| `Client` | `SL.bin` do cliente Aika Global (binário fechado) | Só leitura |

Arquivos Delphi duplicados/mortos dentro de `Server/Src`, **fora** do `.dproj`:
`BaseMob_old.pas` (9.852), `ServerSocket2.pas` (4.527), `LoginSocket2.pas`,
`TokenSocket2.pas`, `AuthHandlers2.pas`, `Dungeon antigo.pas` (1.191). Não são
referência de comportamento.

## 3. Tamanho do que existe

| | Arquivos | Linhas |
|---|---:|---:|
| Go (código) | 76 | 26.188 |
| Go (testes) | 52 | 22.758 |
| Delphi compilado (`.dproj`) | 53 units | 85.116 |

Maiores units Delphi, em ordem: `PacketHandlers.pas` 13.686, `Player.pas`
12.859, `BaseMob.pas` 12.542, `ItemFunctions.pas` 5.695, `ServerSocket.pas`
5.097, `UpdateThreads.pas` 3.942, `NPCHandlers.pas` 3.505, `MOB.pas` 3.327.

## 4. Inventário de funcionalidades

Estados: **V** = completo e validado por teste automatizado · **I** =
implementado, sem validação contra o cliente real · **P** = parcial ·
**N** = não implementado · **B** = bloqueado.

"V" aqui significa *testado por teste automatizado deste repositório*, nunca
*testado contra o cliente Aika Global* — esse teste não foi possível (§7).

### 4.1 Já em Go (62 dos 140 códigos do canal + login)

| Funcionalidade | Origem Delphi | Destino Go | Estado |
|---|---|---|---|
| Cifra e enquadramento TCP | `EncDec.pas`, `PlayerThread.pas` | `internal/protocol`, `internal/netsrv` | V |
| 319 structs binárias | `Packets.pas` e outras | `internal/packets` (gerado), `internal/model` | V (tamanhos vs `sizeof`) |
| Tabelas `.bin` e CSV | `Load.pas` | `internal/gamedata`, `mobdata`, `dropdata`, `dungeondata` | V |
| MySQL | `SQL.pas` | `internal/store` | I (sem MySQL de teste na CI) |
| Login / token HTTP | `LoginSocket`, `AuthHandlers`, `TokenSocket` | `internal/auth`, `tokensrv` | V |
| Canal, entrada no mundo, lista de personagens | `ServerSocket`, `Player` | `internal/game` | V |
| Movimento, visibilidade, chat | `BaseMob`, `PacketHandlers` | `game/movement.go`, `spawn.go`, `chat.go` | V |
| Combate, skills, buffs, atributos | `BaseMob`, `SkillFunctions` | `combat`, `skillfn`, `stats`, `entity` | V |
| Mobs, IA, drop, experiência | `MOB.pas`, `Load.pas` | `game/mob*.go`, `dropdata` | V |
| Grupo | `PartyData`, `PacketHandlers` | `game/party*.go` | V (sem raid) |
| Guilda | `GuildData`, `PacketHandlers` | `internal/guild`, `game/guildhandlers.go` | V (sem aliança/cerco) |
| Correio | `MailFunctions` | `internal/mail`, `game/mailhandlers.go` | V |
| Leilão | `AuctionFunctions` | `internal/auction`, `game/auctionhandlers.go` | V |
| Dungeon | `Dungeon.pas` | `internal/dungeon`, `game/dungeonhandlers.go` | V (sem raid) |
| Inventário, armazém, baú de guilda | `PacketHandlers`, `ItemFunctions` | `game/inventory.go`, `storage.go` | V |
| Usar item, inventário premium, selo | `ItemFunctions` | `game/useitem.go`, `premium.go` | P (ver `PORTE.md`) |
| Console 5555 e comandos de GM por sussurro | `ConsoleSocket`, `CommandHandlers` | `internal/console`, `game/commands.go` | V |
| **Lista de amigos** (6 códigos) | `EntityFriend.pas`, `PacketHandlers`, `Player.pas` | `game/friends.go`, `store/friends.go` | **V** (incremento 1, esta sessão) |

### 4.2 Pendentes — 78 códigos do canal

Agrupamento meu, por afinidade de código; difere em 1–2 códigos do agrupamento
do enunciado (que não lista os códigos). Lista exata e atualizada:
`go run ./cmd/opdiff`.

| Sistema | Códigos | Delphi (handlers) | Dependências | Estado | Prioridade |
|---|---|---|---|---|---|
| Ferramenta de GM | `0x3202`…`0x3299` (26) + `0x5000` | `TPacketHandlers.GM*`, `CheckGMLogin`, `HandleRemoteConsole` | Protocolo separado (porta/handshake próprios); precisa de `Player.AccountType`; `GMRequestCreateComprovant`/`Cupom` dependem do esquema de comprovantes | N | Média (alto risco: poder de admin) |
| Diversos de personagem | feitos: `0x20A 0x213 0x304 0x307 0x329 0x668 0xF93A`; pendentes: `0x359` (eventos) e `0x361` (títulos), ambos bloqueados por dependência; feitos também `0x22D 0x31E 0x34A 0x23FB 0x23FE` (2 pendentes) | `GetStatusPoint`, `UpdateAction`, `PKMode`, `ChangeItemBar`, `RemoveBuff`, `TeleportSetPosition`, `UpdateActiveTitle`, `RequestAllAttributes*`, ping… | `0x23FB` envolve coleta de relíquia | N | **Alta** (usados o tempo todo) |
| Itens | (nenhum pendente; `0x32D` parcial — só encantamento falta; `0x332 0x333 0x21A 0x22A` feitos) | `RenoveItem`, `MakeItem`, `ChangeItemAttribute`, `RepairItens` | `itemfn`, receitas | P | **Alta** |
| Comércio entre jogadores | `0x315 0x316 0x317 0x318` — **feitos** | `TradeRequest/Response/Refresh/Cancel` | — | V | — |
| Raid | `0x207 0x342 0x343 0x344` | `GiveLeaderRaid`, `SendRaid`, `AcceptRaid`, `ExitRaid` | Grupo (pronto); `MaxRaidSlots` já reservado | N | Média |
| Criar/apagar/renomear | feitos: `0x603 0x3E01 0x3E04`; pendente: `0x3E02` (renomear pran — depende de pran) | `CreateCharacter`, `RenamePran` | layout de `TBasicCharacter` | P | **Alta** |
| Loja pessoal | `0x319 0x31A 0x31B` | `CreatePersonalShop`, `OpenPersonalShop`, `BuyPersonalShopItem` | Inventário; reaproveita a lógica de compra do leilão | N | Média |
| Relíquia e devir | `0xB52 0xE51` — **núcleo feito**, templo não abre | `RequestUpdateReliquare`, `MoveItemToReliquare` | NPC do templo, pedras e guardas (não portados) | P | Baixa |
| Nação e aliança | `0xE3A 0xF34 0xF12` | `UpdateNationTaxes`, `UpdateNationGold`, `RequestGuildToAlly` | `Nation.pas` (720 linhas); aliança depende da guilda | N | Baixa |
| Duelo | `0x395 0x396` | `SendRequestDuel`, `DuelResponse` | `0x396` chama `ChangeChannel2` | N | Baixa |
| Quest e cupom | `0x32F 0x3E05` | `AbandonQuest`, `ReclaimCoupom` | Carga dos CSV de quest (pendente na fase 4) | N | Baixa |
| Troca de canal | `0xF05 0xF06 0xF0B` — **feito (incremento 36)**, com `-multicanal` | `ChangeChannel`, `LoginIntoChannel`, `SendToWorldSends` | Faltam grupo/raide na troca, amigos e duelo entre canais | P | Média |
| Loja de NPC | `0x313 0x314` | `BuyNPCItens`, `SellNPCItens` | **Bloqueado**: faltam dados originais das lojas de NPC | **B** | — |
| Pet / pran | (vários, dentro de handlers existentes) | `PET.pas` (566), `Player.pas` pran | `ItemFunctions`; recusado com aviso hoje | N | Média |
| Cerco de castelo | — | `GuildData`/`Nation` | Guilda + nação | N | Baixa |
| Laços de atualização | — | `UpdateThreads.pas` (3.942) | Já parcialmente substituído por tickers do canal (`MobWorld`, leilão, drop) | P | Média |

## 5. Obsoleto / duplicado

* **Código morto da origem** já documentado em `PORTE.md`: `Util.IntMV`,
  `TPrediction`, `PredadorInvBarReset`, a sobrecarga de array de `Encrypt`.
* **Duplicado**: `ServerSocket2/LoginSocket2/TokenSocket2/AuthHandlers2.pas`,
  `BaseMob_old.pas`, `Dungeon antigo.pas` (fora do `.dproj`).
* **Interface gráfica**: `Forms/*` (ex.: `SendPacketForm.pas`).

## 6. O que não foi medido

* Linhas Delphi pendentes por sistema: o enunciado dá ~80,8 mil; não recontei.
  Uma contagem honesta exige separar corpo de handler de código morto dentro de
  `PacketHandlers.pas`/`Player.pas`. Fica para quando cada sistema for iniciado
  (cada incremento lê o trecho exato).
* "25 incrementos / 20–30 h": estimativa do enunciado, não verificada. Um
  incremento pequeno (amigos: 357 + ~300 linhas Delphi, 6 códigos) levou uma
  sessão curta, incluindo testes. Sistemas com dependências (criar personagem,
  troca de canal) serão bem maiores.

## 7. Limites desta auditoria

* Sem cliente Aika Global e sem MySQL de teste na máquina: nada foi validado
  contra o cliente real. "Compatível" significa: layout binário, código e
  tamanho conferidos contra `Packets.pas`, e comportamento portado do Delphi.
* `go test -race` **não roda** nesta máquina (sem `gcc`/cgo).
  Ver `KNOWN_ISSUES.md`.
