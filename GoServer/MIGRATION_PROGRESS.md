# Progresso da migração

Números medidos com `go run ./cmd/opdiff -root ..`. "Concluído" = implementado
**e** coberto por teste automatizado que passa. Nada aqui foi testado contra o
cliente real (ver `KNOWN_ISSUES.md`).

| Marco | Códigos do canal com contraparte Go | Pendentes |
|---|---:|---:|
| Início da sessão | 56 / 140 | 84 |
| Após incremento 1 (amigos) | 62 / 140 | 78 |
| Após incremento 2 (pilhas de itens) | 64 / 140 | 76 |
| Após incremento 3 (renovar item) | 66 / 140 | 74 |
| Após incremento 4 (refino + aparência, `0x32D` parcial) | 67 / 140 | 73 |
| Após incremento 5 (reparo) | 68 / 140 | 72 |
| Após incremento 6 (forja `0x32B`) | 69 / 140 | 71 |
| Após incremento 7 (diversos de personagem, lote 1) | 76 / 140 | 64 |
| Após incremento 8 (diversos de personagem, lote 2) | 81 / 140 | 59 |
| Após incremento 9 (excluir personagem) | 83 / 140 | 57 |
| Após incremento 10 (criar personagem) | 84 / 140 | 56 |
| Após incremento 11 (comércio) | 88 / 140 | 52 |
| Após incremento 12 (relíquia e devir, núcleo) | 90 / 140 | 50 |
| Após incremento 13 (loja pessoal + posição de membro) | 94 / 140 | 46 |
| Após incremento 14 (gravação periódica; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 15 (relíquias: reenvio a todos + efeitos nos atributos; sem código novo) | 94 / 140 | 46 |
| Após incremento 16 (skills de área + buffs de classe; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 17 (cura e Clériga; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 18 (Guerreiro e Templário; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 19 (dano contínuo; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 20 (Atirador e Dual Gunner; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 21 (Mago e dano mágico das skills; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 22 (skills em área das seis classes; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 23 (skills restantes e buffs reativos; sem código novo de pacote) | 94 / 140 | 46 |
| Após incremento 24 (raid / legião) | 98 / 140 | 42 |
| Após incremento 25 (duelo) | 100 / 140 | 40 |
| Após incremento 26 (cupom `0x3E05`) | **101 / 140** | **39** |
| Após incremento 27 (nação `0xE3A`/`0xF34` e aliança `0xF12`) | **104 / 140** | **36** |
| Após incremento 28 (renomear pran `0x3E02`) | **105 / 140** | **35** |
| Após incremento 29 (modelo de pran, fatia 1) | **105 / 140** | **35** (sem código novo: a pran não tem opcode próprio no canal) |
| Após incremento 30 (pran: passivas e equipamento) | **105 / 140** | **35** (sem opcode novo) |
| Após incremento 31 (pran: exp, nível, comida e devoção) | **105 / 140** | **35** (sem opcode novo) |
| Após incremento 32 (pran: invocar, guardar, vizinhos e evolução) | **105 / 140** | **35** (sem opcode novo) |
| Após incremento 33 (pran: itens no equipamento e na bolsa) | **105 / 140** | **35** (sem opcode novo) |
| Após incremento 34 (título ativo `0x361`) | **106 / 140** | **34** |
| Após incremento 35 (item de evento `0x359`) | **107 / 140** | **33** |
| Após incremento 36 (troca de canal `0xF05`/`0xF06`/`0xF0B`) | **110 / 140** | **30** |
| Após incremento 37 (grupo na troca de canal; sem opcode novo) | **110 / 140** | **30** |
| Após incremento 38 (títulos: ganhar/subir/perder; sem opcode novo) | **110 / 140** | **30** |
| Após incremento 39 (ferramenta de GM, fatia 1: 8 códigos) | **118 / 140** | **22** |
| Após incremento 40 (GM fatias 2 e 3: 14 códigos; itens de título e de nação; teste de canal entre nações) | **132 / 140** | **8** |

Suíte: `go build ./...`, `go vet ./...`, `gofmt -l .` limpos e `go test ./...`
verde nos 29 pacotes com testes. `-race` indisponível na máquina.

## Incremento 1 — Lista de amigos

| Item | Detalhe |
|---|---|
| Estado anterior | N (0 dos 6 códigos) |
| Códigos | `0x372`, `0x673`, `0xF74`, `0xF26`, `0xF27`, `0xF30` (cliente→servidor) e `0x870`, `0x96F`, `0x971`, `0x975` (servidor→cliente) |
| Origem Delphi lida | `EntityFriend.pas`, `PacketHandlers.pas` (`AddFriendRequest`…`CloseFriendWindow`), `Player.pas` (`sendFriendToSocial`, `AddFriend`, `SendFriendLogin/Logout`, `RefreshMeToFriends`…), `Packets.pas` |
| Arquivos novos | `game/friends.go`, `game/friends_test.go`, `store/friends.go`, `cmd/opdiff/main.go` |
| Arquivos alterados | `game/handler.go` (campo `FriendDB`, 6 `case`, gancho de saída), `game/session.go` (campo `Friends`), `game/entry.go` (carga ao entrar no mundo), `cmd/aikaserver/main.go` (ligação), `PORTE.md` |
| Testes | 31 funções de teste novas em `friends_test.go`: fluxo pedir/aceitar/recusar, 6 recusas, falha de banco, remover (online/offline/inexistente), janelas e conversa, entrada/saída, tamanhos dos 10 pacotes vs `sizeof` Delphi, 3.000 entradas aleatórias nos 6 handlers, paralelismo da lista |
| Resultado | Todos passam; suíte completa verde |
| Compatibilidade | Layout/código/tamanho conferidos contra `Packets.pas`. **Não** validado no cliente |
| Defeitos da origem corrigidos | Ver `PORTE.md`, seção "Lista de amigos" (aceite forjado sem pedido; conversa com não-amigo; remoção sem checar amizade; gravação em duas etapas) |
| Pendências | `City` zerada; só um canal; consultas SQL não executadas contra MySQL |
| Próximo incremento | Ver abaixo |

## Próximos incrementos (ordem recomendada)

1. **Itens restantes** (`0x340` reparo ~420 linhas Delphi, `0x21A 0x22A` renovar ~70, `0x32D` atributo ~160, `0x32B` fabricar ~490; os dois primeiros de pilha já feitos): reaproveitam
   `moveItem`/`itemfn`; alto impacto, risco baixo.
2. **Diversos de personagem** (14 códigos): `PKMode`, `ChangeItemBar`,
   `UpdateAction`, status points… usados o tempo todo.
3. **Criar/apagar personagem** (`0x3E01 0x3E04 0x603`): exige escrita em
   `store.Characters`; sem isso não há conta nova.
4. **Comércio** (4) e **loja pessoal** (3): compartilham a validação de
   transferência de item do leilão/correio.
5. **Troca de canal** + estado de conta compartilhado, depois **raid**,
   **duelo**, **nação**, **GM** (protocolo separado).
6. Loja de NPC: bloqueada até haver os dados originais.

## Incremento 2 — Pilhas de itens (`$332` juntar, `$333` separar)

| Item | Detalhe |
|---|---|
| Estado anterior | N |
| Origem lida | `PacketHandlers.pas` `AgroupItem` (3424), `UngroupItem` (3307), `GetEmptySlot` (`ItemFunctions.pas:717`) |
| Arquivos | novos `game/itemstack.go`, `game/itemstack_test.go`; alterado `game/handler.go` (2 `case`) |
| Testes | 16 novos, todos passam; suíte completa verde; `go vet`/`gofmt` limpos |
| Defeitos da origem corrigidos | juntar equipamentos somava o refino; `src == dest` apagava o item; separar equipamento **duplicava** o item; quantidade 0; slot sem bolsa; soma acima do teto perdia itens |
| Divergência deliberada | acima de 1000 a pilha de destino enche e o **resto fica na origem** (a origem somava sem teto num WORD) |
| Pendências | pran (`SlotTypePranInv`) recusado com aviso, como no resto do porte; não validado no cliente |
| Itens ainda pendentes | `0x340`, `0x21A`, `0x22A`, `0x32B`, `0x32D` |

## Incremento 3 — Renovar item (`$21A`, `$22A`)

| Item | Detalhe |
|---|---|
| Estado anterior | N |
| Origem lida | `PacketHandlers.pas` `RenoveItem` (10657), `Packets.pas` `TPacketRenoveItem`, `MiscData.pas` `TItem.SetExpire` |
| Arquivos | novos `game/renew.go`, `game/renew_test.go`; alterado `game/handler.go` (1 `case` para os 2 códigos) |
| Testes | 10 casos novos, todos passam; suíte completa verde |
| Comportamento | tipo 35 (reparador de roupa de pran): alvo com `Classe` 100–104, zera `Min`/`Max`, prazo = agora + `UseEffect` h. Tipo 520 (licença de montaria): alvo de slot de equipamento 9, prazo gravado em dias desde `BASE_DATETIME` |
| Corrigido | slot 0 era excluído (`> 0`); bolsa do slot não conferida; mesmo slot nos dois campos; `UseEffect` 0 consumia a licença e expirava o item |
| Mantido da origem | o prazo conta a partir de **agora**, não do prazo atual (renovar com 30 dias restantes e licença de 7 deixa 7) |
| Pendências | não validado no cliente; texto sem acentos |
| `0x32D` (atributo) | **não feito**: despacha para `ReinforceItem` (~300 linhas, sorteio e tabelas de custo), `EnchantItem` (~330), `ChangeApp` (~45) e `EnchantMount`. Vira incremento próprio |

## Incremento 4 — Atributo de item `$32D`: refino e troca de aparência

| Item | Detalhe |
|---|---|
| Estado anterior | N |
| Origem lida | `PacketHandlers.pas` `ChangeItemAttribute` (4116), `ItemFunctions.pas` `ReinforceItem` (1228), `GetResultRefineItem` (1071), `GetReinforceCust`, `ChangeApp` (1860), `Changeable`, `GlobalDefs.pas` (`ChancesOfRefinament`, `CHANGE_*`), `Player.pas` (`SendClientMessage`, `SendChangeItemResponse`) |
| Arquivos | novos `game/reinforce.go`, `game/reinforce_test.go`; alterados `game/handler.go` (dispatch, código e tamanho da mensagem ao cliente), `gamedata/tables.go` (`ReinforceCost`), `game/friends_test.go` |
| Testes | 40+ casos novos (subtestes incluídos), suíte completa verde, vet/gofmt limpos |
| Portado | `CHANGE_REINFORCE` (0) e `CHANGE_APP` (2), com resposta `$32E`, `$312` e atualização de slots |
| **Não portado** | `CHANGE_ENCHANTS` (1), `CHANGE_MOUNT_ENCHANTS` (11), `CHANGE_PRAN_ENCHANTS` (13): recusados com aviso, nada consumido. Dependem de `EnchantItem` (~330 linhas) e `EnchantMount` |
| **Atenção — `0x32D` conta como coberto no `opdiff`, mas só 2 dos 5 tipos estão feitos** | |

### Correção de protocolo encontrada no caminho

`game.message` (usada por todos os handlers do porte) enviava **código `0x100`,
145 bytes, tipo 0**. O Delphi (`Player.pas:4763`, `Packets.pas:36`) envia
**`$984`, 144 bytes, tipo 16**. Nada no repositório justificava `0x100`. Foi
corrigido: código `0x984`, struct local `clientMessage` de 144 bytes
(`Header.size := 144`), tipo padrão 16 (`MsgYellow`). **Não validado no cliente**:
se o valor antigo tiver vindo de captura de pacote real, reverter a constante
`CodeClientMsg` em `game/handler.go` é suficiente.

## Incremento 5 — Reparar itens (`$340`)

| Item | Detalhe |
|---|---|
| Estado anterior | N |
| Origem lida | `PacketHandlers.pas` `RepairItens` (10233–10655; a maior parte é código comentado, a lógica viva são dois modos e a fórmula de preço), `Packets.pas` `TPacketRepareItens`, `Player.pas` `DecGold` |
| Arquivos | novos `game/repair.go`, `game/repair_test.go`; alterado `game/handler.go` (1 `case`) |
| Testes | 11 novos, todos passam; suíte completa verde |
| Comportamento | modo 0: até 10 itens escolhidos (equipamento ou inventário); modo 1: todo o inventário 0–59 e equipamento 2–7. Preço = `SellPrince × 0,00025 × (Max−Min)`, vezes `Rank−1` se `Rank > 1`; total somado em `float32` e arredondado ao par; `Min := Max`; atualiza ouro e slots e ecoa o pacote |
| Corrigido | `Min > Max` dava custo **negativo** e pagava o jogador (também no modo "tudo", que só excluía `Min = Max`); slot repetido cobrado duas vezes; slot de inventário sem bolsa |
| Pendências | não confere NPC de reparo (como a origem); não validado no cliente |

## Incremento 6 — Forja (`$32B`)

| Item | Detalhe |
|---|---|
| Estado anterior | N |
| Origem lida | `PacketHandlers.pas` `MakeItem` (9742–10231), `Load.pas` `InitMakeItems`, `Functions.pas` `SearchMakeItemIDByRewardID`, `ItemFunctions.pas` (`PutItem`, `RemoveItem`, `GetItemSlotByItemType`, `GetItemSlotAndAmountByIndex`), `GlobalDefs.pas` (`Level_*_Destroy_Range`, `TYPE_ITEM_*`) |
| Arquivos | novos `game/craft.go`, `game/craft_test.go`, `game/itemput.go`, `gamedata/makeitems.go`, `gamedata/makeitems_test.go`; alterados `gamedata/tables.go`, `game/handler.go`, `cmd/aikaserver/main.go` |
| Testes | 30+ casos novos (subtestes incluídos); a carga é testada contra os CSV reais (425 receitas, 2300 ingredientes); suíte completa verde |
| Modos do `$32B` (campo `Null3`) | 1 desmontar equipamento · 3/4 limpar adicionais (Vaizan 513/514/515) · 6 limpar aparência (pedra 517) · qualquer outro: **fabricar** |
| Novo | `Handler.putItem`: inserção de item com empilhamento, divisão por teto de 1000 e rollback sem espaço (o Go não tinha equivalente de `PutItem`) |
| **Achado importante** | a fabricação **nunca funcionava** na origem (ver abaixo) |
| Pendências | itens com prazo/selo (`Expires`/`CanSealed`) saem sem prazo (duração de item não portada); não validado no cliente |

## Incremento 7 — Diversos de personagem, lote 1 + gravação do progresso

| Item | Detalhe |
|---|---|
| Códigos | `0x20A` (saldo premium), `0x213` (pontos de atributo), `0x304` (ação), `0x307` (PvP), `0x329` (cancelar buff), `0x668` (voltar à lista, desativado na origem), `0xF93A` (ping) |
| **Achado** | o porte **não gravava nenhum progresso do personagem**: nível, experiência, ouro, atributos, vida, posição e PvP se perdiam a cada saída (só itens, skills, ouro do armazém e cash eram gravados). Novo `store.Characters.SaveProgress` (um `UPDATE` com parâmetros e a conta dona no `WHERE`) chamado em `OnDisconnect` por `saveCharacter` |
| Arquivos | novos `game/misc.go`, `game/misc_test.go`, `game/charsave.go`, `store/characters_save.go`; alterados `game/handler.go`, `game/session.go` (campo `Action`), `cmd/aikaserver/main.go` |
| Testes | 14 novos, todos passam; suíte completa verde |
| Corrigido | `$213` gastava os pontos com índice inválido e dava a volta no `WORD`; `$F93A` mostraria ~3,3 trilhões de ms sem `ping` prévio |
| Pendente neste sistema | `0x31E` (barra de itens), `0x22D` (link de item no chat), `0x34A` (teleporte salvo), `0x359` (item de evento), `0x361` (título ativo), `0x23FB`/`0x23FE` (todos os atributos) |

## Incremento 8 — Diversos de personagem, lote 2

| Item | Detalhe |
|---|---|
| Códigos feitos | `0x22D` (item linkado no chat → `$F6F`), `0x31E` (barra de atalhos), `0x34A` (posição de teleporte salva), `0x23FE`/`0x23FB` (atributos detalhados próprios e do alvo → `$23FF`/`$23FC`) |
| Persistência nova | `store.ItemBars` (tabela `itembars`, DELETE+INSERT em transação), `Characters.TeleportPositions`/`SaveTeleportPositions` (coluna `tp_positions`). A barra carrega ao entrar no mundo (vai no `$925`, campo `ItemBar`) e grava na saída; as posições de teleporte do mago carregam na entrada e gravam a cada mudança |
| Arquivos | novos `game/charextras.go`, `game/charextras_test.go`, `store/itembars.go`; alterados `game/handler.go`, `game/session.go`, `game/entry.go`, `cmd/aikaserver/main.go` |
| Testes | 20 novos, todos passam; suíte completa verde |
| Corrigido | barra: slot e índice de skill sem limite (`× 16` estourava); teleporte: slot 5 numa lista de 5; atributos do alvo: `Players[TargetID]` sem faixa; item linkado: slot de inventário de 0 a 255 num vetor de 64, `Header.Index` vindo do cliente e o megafone mandando a todos **sem gastar o item de megafone** |
| **Não feitos** | `0x361` (título ativo) e `0x359` (item de evento): ver `KNOWN_ISSUES.md`. Ficam sem `case` de propósito, para o `opdiff` não contá-los como cobertos |

## Incremento 9 — Excluir personagem (`$603`, `$3E01`)

| Item | Detalhe |
|---|---|
| Feito | `0x603` pedir/cancelar exclusão (marca para daqui a 1 dia, `DELETE_DAYS_INC`) e `0x3E01` exclusão definitiva depois do prazo |
| Arquivos | novos `game/chardelete.go`, `game/chardelete_test.go`, `store/chardelete.go`; alterados `game/handler.go`, `cmd/aikaserver/main.go` |
| Testes | 16 novos, todos passam; suíte completa verde |
| Corrigido | slot do cliente sem faixa (`$603`) e `BYTE(Slot)` que transformava o slot 256 em 0 e apagava o primeiro personagem (`$3E01`); `DELETE FROM items WHERE owner_id = <id do personagem>` apagava também itens da CONTA e da GUILDA com o mesmo número (armazém, premium, baú) — agora só os tipos de slot do personagem (0 e 1); a numérica podia ser testada sem limite pelo pedido de exclusão — agora soma no mesmo contador da entrada e tranca; data de exclusão vazia levantava exceção |
| Mantido | sem numérica definida, `$3E01` pula a conferência (só o `$603` a exige); o aviso "ainda não pode excluir" do `$603` aparece justamente com o prazo vencido, como na origem |
| **Não feito** | **criar personagem (`0x3E04`)**: bloqueado por uma divergência nos dados, ver `KNOWN_ISSUES.md` |

## Incremento 10 — Criar personagem (`$3E04`)

| Item | Detalhe |
|---|---|
| Decisão | opção (a): atributos, tamanho do corpo, nível, equipamento e inventário vêm dos `.acc` (parte conferida); skills iniciais pela regra de `CreateBasicCharacter`/`GetSkillIndex` |
| Arquivos | novos `internal/charcreate/{charcreate,charcreate_test}.go`, `store/charcreate.go`, `game/charcreate.go`, `game/charcreate_test.go`; alterados `game/handler.go`, `cmd/aikaserver/main.go` |
| Testes | 10 no pacote `charcreate` (contra os 6 `.acc` reais: classes 1/11/21/31/41/51 e atributos batem com o código do Delphi) + 6 no `game`; suíte completa verde |
| Fluxo | valida (nome 1–14 letras/números, slot 0–2, classe 10–69, cabelo 7700–7731) → modelo da classe → `Characters.Create` (transação: limite de 3, slot livre, nome livre) → itens, skills e barra → expansão inicial do armazém (item 5310, slot 80, só se vazio) → recarrega e reenvia a lista. Falha depois do `INSERT` apaga o personagem |
| Corrigido | nome vazio passava; slot ocupado ganhava segunda linha; nome conferido em consulta separada; personagem ficava pela metade no banco; o slot 80 do armazém era sobrescrito a cada criação |
| Não portado | bônus de indicação (item 4357 ×500): depende do sistema de eventos |

## Incremento 11 — Comércio entre jogadores (`$315`–`$318`)

| Item | Detalhe |
|---|---|
| Códigos | `0x315` convite, `0x316` resposta, `0x317` oferta, `0x318` cancelar |
| Arquivos | novos `game/trade.go`, `game/trade_test.go`; alterados `game/handler.go`, `game/session.go` (campo `Trade`) |
| Testes | 21 novos (subtestes incluídos), todos passam; suíte completa verde |
| Modelo | o servidor é a autoridade: o cliente manda só os **slots** e o ouro; os itens que o outro lado vê saem do inventário do servidor. Cada lado guarda uma fotografia do que ofereceu e a execução confere item a item; troca tudo-ou-nada com restauração dos dois inventários |
| Corrigido | itens vindos do pacote do cliente (mostrar um item e entregar outro); confirmação do parceiro não era desfeita quando a oferta mudava; `ExecuteTrade` testava os itens do próprio jogador no segundo laço, movia itens antes de conferir o ouro e saía no meio; `$316` abria negociação com qualquer id sem convite; convite a si mesmo e a quem não está em jogo; slot do cliente sem faixa; quem saía deixava o parceiro preso |
| Mantido | itens com `TypeTrade > 0` fecham a negociação dos dois com aviso; `$317` com "pronto" confere lugar no inventário do outro; ao cancelar só a janela do outro lado recebe o `$318` |
| Não portado | distância entre os jogadores (a origem não conferia). A loja pessoal no `$318` foi portada no incremento 13 |

## Incremento 12 — Relíquia e devir (`$B52`, `$E51`): núcleo

| Item | Detalhe |
|---|---|
| Códigos | `0xB52` pedir o estado dos templos (resposta `$953`) e `0xE51` entregar (NPC 263) ou puxar (NPC 1793) uma relíquia |
| Arquivos | novos `internal/devir/{devir,devir_test}.go`, `store/devires.go`, `game/devirhandlers.go`, `game/devirhandlers_test.go`; alterados `game/handler.go`, `game/session.go` (`OpenedDevir`/`OpenedTemple`), `cmd/aikaserver/main.go` (flag `-nation`, carga dos templos) |
| Testes | 11 no pacote `devir` + 12 no `game`, todos passam; suíte completa verde |
| Feito | estado dos 5 templos da nação (tabela `devires`, `devir_id` de `(n-1)*5+1`), carga com as regras de `Nation.pas` (timecap < 1e9 é lixo; `TimeToEstabilish` = captura + 2 h), efeitos somados por índice, entregar (nação, vaga habilitada e livre, gravação, aviso à nação, honra `(nível+1)×250` e item 8544 ao jogador e ao grupo a ≤ 20 células), puxar (templo inimigo, liberada após 2 h, uma relíquia por vez, gravação antes de entregar), e o `$953` |
| Corrigido | `SrcSlot` (WORD) sem faixa e puxar de slot vazio entregava o item 0; falha de gravação no roubo restaurava o slot mas o item já estava no inventário (duplicava); `OpenedDevir` 255 indexava o vetor de templos |
| **Bloqueio** | **o templo nunca se abre**: ver `KNOWN_ISSUES.md` |

## Incremento 13 — Loja pessoal (`$319`, `$31A`, `$31B`, fechar no `$318`) e posição de membro (`$338`)

| Item | Detalhe |
|---|---|
| Estado anterior | N (0 dos 4 códigos); o `$318` só fechava comércio |
| Origem lida | `PacketHandlers.pas` `CreatePersonalShop`/`OpenPersonalShop`/`BuyPersonalShopItem`/`ClosePersonalShop` (7248–7391), `TradeCancel` (4461), `UpdateMemberPosition` (6639); `Player.pas` `SendPersonalShop`, `SendPositionParty`; `Packets.pas`/`MiscData.pas` (`TPersonalShopData` 356 B, `TPersonalShopItem` 32 B) |
| Arquivos | novos `game/shop.go`, `game/shop_test.go`; alterados `game/handler.go` (4 `case`, gancho de saída), `game/session.go` (campo `Shop`), `game/trade.go` (`$318` fecha loja; convite recusado com loja aberta), `game/partyhandlers.go` (`$338`) |
| Testes | 34 novos (subtestes incluídos), todos passam; suíte completa verde, `go vet` e `gofmt` limpos. Inclui tamanhos `$170`/`$38`, 8 recusas na criação, 8 na compra, vendedor que mexe no item depois de abrir, 4 compradores concorrentes (só um leva) e 3.000 entradas aleatórias |
| Modelo | o servidor é a autoridade: o cliente manda só **slot e preço**; o item da vitrine sai do inventário do servidor e, na compra, é conferido de novo contra o inventário do dono. Mesma trava do comércio (`tradeMu`), pois loja e troca mexem nos mesmos inventários |
| Corrigido | item da vitrine vinha do cliente (anunciar um, entregar outro); `Packet.Slot` da vitrine sem faixa; a compra entregava o que houvesse no slot do dono (vazio/outro item) pelo preço combinado; slot de bolsa (60–63) e slot vazio aceitos; ouro do vendedor somava sem teto; abrir loja de quem está longe ou sem loja; loja aberta durante troca; sair do jogo deixava loja e visitantes pendurados; `$338` indexava `Players` sem faixa nem checar se o membro estava em jogo |
| Mantido | slot repetido fecha a loja com aviso; item com `TypeTrade > 0` é tirado da vitrine; comprador visitante que cancela só zera a visita (sem eco); produto vendido vira vitrine zerada; loja vazia de produtos não fecha sozinha |
| Divergência deliberada | loja sem nenhum item válido é **recusada** (a origem abria uma loja vazia); preço acima de `MaxGold` recusado; quem abre a loja de outro precisa vê-lo na tela |
| Compatibilidade | códigos, tamanhos e layout conferidos contra `Packets.pas`/`MiscData.pas`. **Não** validado no cliente |
| Pendências | ver `KNOWN_ISSUES.md` ("Loja pessoal"); raid no `$338` |
| Próximo incremento | gravação periódica do progresso + `UpdateReliquaresForAll`, depois efeitos nas contas de jogo (ver `PENDENCIAS.md` §6) |

## Incremento 14 — Gravação periódica do progresso

| Item | Detalhe |
|---|---|
| Estado anterior | gravava só na saída (`OnDisconnect`); queda do processo perdia a sessão inteira |
| Arquivos | alterados `game/charsave.go` (`SaveAllTick`), `cmd/aikaserver/main.go` (`saveLoop`, a cada 2 min); `game/misc_test.go` (3 testes novos) |
| Comportamento | para cada jogador em jogo grava itens, skills, cash/ouro de conta, progresso do personagem e barra de itens, segurando `tradeMu` para nunca fotografar troca ou compra de loja pela metade |
| Testes | gravados só os `Playing`; segunda volta grava o estado novo; sem canal/banco não quebra; convive com compra de loja. Suíte completa verde |
| Pendência | corrida benigna conhecida: handlers escrevem na sessão sem trava e o laço lê de outra goroutine (só o `-race` mediria). Intervalo fixo de 2 min, não vem do `.ini` |
| Próximo incremento | `UpdateReliquaresForAll`, depois efeitos nas contas de jogo |

## Incremento 15 — Relíquias: `UpdateReliquaresForAll` e efeitos nos atributos

| Item | Detalhe |
|---|---|
| Arquivos | novos `game/relicfx.go`; alterados `game/devirhandlers.go` (`sendReliques` `$136`, `updateReliquaresForAll`), `game/entry.go` (`$136` ao entrar; relíquia no cálculo inicial), `game/avatar.go` (`Relic`, `PercentHP/MP`), `game/exp.go`, `stats/derived.go` (`RelicAtkFis/MagPercent`); testes em `devirhandlers_test.go` e `stats/stats_test.go` |
| Reenvio | entregar ou roubar uma relíquia manda a todos em jogo o `$136` (lista de 25 itens; vazia para quem é de outra nação), refaz os atributos e reenvia status, pontos e HP/MP. Antes, o estado novo só chegava a quem pedisse o `$B52` |
| Efeitos aplicados | HP% e MP%, dano físico/mágico % (segundo passo percentual composto, como a origem), crítico, acerto, esquiva (`PARRY`), resistência a estados, recarga, velocidade e ataque duplo |
| Regras da origem mantidas | tudo só vale para jogador da nação dona do canal, exceto o **duplo** (`EF_RELIQUE_DOUBLE` é somado a todos); `PER_RESISTANCE1/2` não têm efeito (a soma está comentada) |
| Divergência deliberada | o MP% da origem usava o efeito de **HP** (`EF_RELIQUE_PER_HP`, erro de cópia); aqui usa `EF_RELIQUE_PER_MP` (148), como `stats.MaxMP` já previa |
| Correção de passagem | subir de nível chamava `Recalc(Bonus{})` e zerava o bônus de equipamento; agora usa `ApplyEquipBonus` |
| Pendências | velocidade: o valor gravado do personagem tem precedência sobre o cálculo (regra antiga do `RecalcDerived`), então `RUNSPEED` da relíquia só aparece se o personagem estiver com velocidade 0; efeitos de dano contra nação/monstro (`ATK_NATION`, `ATK_MONSTER`, `DEF_*`), EXP%, drop e `SKILL_*` não estão ligados; buffs e títulos continuam zerados |

## Incremento 16 — Skills de área e buffs de classe

| Item | Detalhe |
|---|---|
| Sintoma | `useSkill` recusava toda skill de área ("ainda não estão disponíveis"); buff entrava numa lista e **nada mais**: sem aviso ao cliente, sem efeito nos atributos, e **nunca expirava** (ninguém removia os vencidos) |
| Origem lida | `BaseMob.pas` `HandleSkill` (5504, despacho por `SuccessRate`/`Range`), `AddBuff` (3569), `AddBuffEffect`/`RemoveBuffEffect` (3813), `SendAddBuff` (3507), `GetCurrentScore` (3153), `AreaSkill` (8337), `AreaBuff` (8275), `Decreasse/IncreasseMobAbility` (2879) |
| Arquivos | novos `game/buffs.go`, `game/skillarea.go`, `game/buffs_test.go`, `game/skillarea_test.go`; alterados `game/skill.go` (despacho, helpers `skillHitPlayer`/`skillHitMob`), `game/avatar.go` (`Ability`), `game/combat.go` (`sessionTarget.Ability/DecAbility/RemoveBuff` reais), `game/misc.go`, `game/commands.go`, `game/mobai.go`, `game/dungeonhandlers.go`, `game/entry.go` (zera buffs ao entrar), `stats/derived.go` (somadores planos após os percentuais), `cmd/aikaserver/main.go` (`buffLoop`, 1 s) |
| Buffs | `addBuff` soma `EF/EFV[0..3]` da `SkillData` num acumulador (`MOB_EF`), refaz os atributos e manda `$16F`, `$16E`, pontos e status. `dropBuff` desfaz com a regra de `DecreasseMobAbility` (valor negativo soma o oposto; positivo desconta com piso 0). `BuffTick` remove os vencidos a cada segundo. Recastar refaz o efeito do zero (não acumula). Limite de 60 com aviso; `7257` e `9133` recusados, como a origem. Tempo extra da relíquia (`EF 175`) para a nação |
| Efeitos ligados | STR/DEX/INT/CON/SPI, dano e defesa planos e %, crítico, poder crítico, acerto, esquiva, duplo, resistências (crítico, duplo, estado), penetrações, dano de skill, recarga, velocidade, HP/MP planos e %. Também liga o `Ability()` que o pipeline de combate já consultava: escudos e amplificadores de buff passam a existir |
| Skills de área | dano em jogadores e mobs ao redor do centro, raio `Range×1,5`, `MaxTargets`, poupa zona segura, mesma guilda, mesmo grupo, mesma nação sem PK e mortos; buff de duração é aplicado em quem apanha. Buff de área: sem grupo vai só no autor; com grupo, no autor e nos membros a vista dentro do raio. `TargetType = 1` agora é "em si mesmo" |
| Corrigido | centro da área vindo do pacote sem validar (agora cai no autor se estiver longe ou zerado); `Recalc(Bonus{})` ao subir de nível apagava bônus de equipamento (incremento 15); buffs do personagem anterior ficavam na sessão ao trocar de personagem |
| Testes | 18 novos (buff: efeito, remoção, recast, expiração, debuff negativo, HP, limite, índices recusados, skill → cliente; área: acerta, poupa 5 tipos de aliado, PK, raio, `MaxTargets`, centro longe, mobs, buff em área/grupo, `TargetType 1`, debuff em dano). Suíte completa verde |
| Não portado | efeitos próprios por skill das seis classes (`SelfBuffSkill` ~255 linhas, `TargetBuffSkill` ~365, `*AreaSkill` ~1.500): escudos com contador (`DefesaPoints`, `BolhaPoints`), lâmina, `HPRListener`, teleporte, invisibilidade, cura (`CalcAndCure`, El Tycia), skills de pran, dungeon, duelo, guardas/pedras de devir. Essas skills caem no dano/efeito genérico da tabela |

## Incremento 17 — Cura e Clériga

| Item | Detalhe |
|---|---|
| Sintoma | skills de cura (`SuccessRate 0`, sem duração) não tinham ramo no despacho e **não faziam nada**; Mão de Cura, Libertação de Mana, Ressurreição, Limpar e Recuperação não existiam |
| Origem lida | `BaseMob.pas` `CalcAndCure` (5311), `CalcCure2` (5440), `TargetBuffSkill`/`SelfBuffSkill` (ramos da Clériga), `RemoveDebuffs`/`RemoveAllDebuffs`/`RemoveBuffs` (4061–4123), `RemoveBuffEffect` (cura preventiva), `ClericSkill`/`ClericAreaSkill`, `UpdateThreads.pas` (laço `HPRListener`, 780–925) |
| Arquivos | novos `game/cleric.go`, `game/cleric_test.go`; alterados `game/skill.go` (despacho `SuccessRate 0/Range 0` → efeito; ganchos de dano), `game/skillarea.go` (buff de área chama `targetBuffSkill`), `game/buffs.go` (`regenTick`, cura preventiva, `clearBuffs`), `game/combat.go` (`revive` limpa efeitos), `game/session.go` (`NegarCura`, `regen`), `game/handler_test.go` (`asPacket`) |
| Cura | `cureValue` = `AtkMag/2 + Damage` + EF_DAMAGE6/SKILL_DAMAGE6 (autor e alvo) + `CureTax` + EF_UPCURE + EF_PER_CURE_PREPARE + sorteio `RandomRange(20,299)/2+35` − EF_DECURE. **Anticura** (EF 165): troca a cura por perda de vida sem matar, por até 3 curas, e tira o buff 88 ao zerar. Alvo morto não é curado |
| Skills | Cura 122, Flores da Glória 458, Remediar 26, Cura Massa Defensiva 131 e Áurea 162 (curam + buff), Recuperação 138 (cura + todos os debuffs), Glória de Excelis 167 (1 debuff + cura + buff), Cura Concentrada 459, Limpar 140, Remover Debuffs 337, Ressurreição 126 (mesmo grupo, `Damage`% da vida), Mão de Cura 125 e Libertação de Mana 128 (regeneração), Cura Preventiva 134 (cura quando o buff acaba). Variante "em si mesmo" (`TargetType 1`) separada, como a origem |
| Dano da Clériga | Flecha Sagrada (+`Adicional`%), Retorno de Magia (+`Adicional` e tira `Damage` buffs do alvo), Sensor Mágico e Raio Solar (dano 0; Sensor revela invisibilidade 53/77) |
| Regeneração | presa ao buff (`Session.regen`): o buff expira ou é removido e a regeneração acaba junto. A origem tinha `HPRListener/HPRAction/HPRCycles` num laço de thread, que podia ficar ligado depois do buff |
| Corrigido | `revive` chamava `Buffs.Clear()` e deixava o acumulador de efeitos com valores de buffs que não existiam mais (agora `clearBuffs`); testes instáveis ~1 em 270: o `asPacket` caía na colisão do "hello" do framer (`11 1F` nos bytes 2–3, 1 em 65.536 no mundo real) |
| Testes | 23 novos (cálculo da cura com CureTax/Decure, teto de HP, alvo morto, anticura ×3, Limpar/Recuperação, Mão de Cura por segundo e parada, MP, cura preventiva, ressurreição dentro/fora do grupo, cura em área, `TargetType 1`, Flecha/Retorno/Sensor); verificado por mutação (quebrar a fórmula derruba o teste). Suíte completa verde |
| Não portado | Água Benta 457 e Raio Solar 133 como dano contínuo (`SKDListener`), Glória de Excelis como regeneração de 10 ciclos (`HPRAction 4`), `RemoveBuff` de buff de dungeon, cura em mob/pet, raid na ressurreição, mensagens exatas por classe do cliente |

## Incremento 18 — Guerreiro e Templário

| Item | Detalhe |
|---|---|
| Origem lida | `BaseMob.pas` `WarriorSkill` (10944), `TemplarSkill` (11052), `ValidAttack` (5985, resistência a estados e bolha), `AttackParse` (9940–10090: Proteção, União Divina, Bolha, Resoluto), `SelfBuffSkill` (ramos 32/36/42/120), `TargetBuffSkill` (35, 39), `WalkAvanced` (12331) |
| Arquivos | novos `game/warrior.go`, `game/warrior_test.go`; alterados `game/skill.go` (`skillHitPlayer`/`skillHitMob`), `game/cleric.go` (ramos novos, `hpPctMax`), `game/combat.go` e `game/mobai.go` (defesa por pontos e União Divina também nos golpes básicos e de mob), `game/session.go` (`DefesaPoints`, `BolhaPoints`, `UniaoDivina`), `combat/pipeline.go` (Proteção sai ao zerar), helpers de teste de `auth` |
| Dano | Pancada soma `Adicional`% da vida atual do autor; Estocada só testa o golpe (dano 0) e aplica o efeito |
| Estados | Avanço Poderoso, Proficiência e Atração Divina (stun) e Ferida Mortal (silêncio) passam pelo teste de resistência de `ValidAttack`: taxa `Resistence/5 + EF_STATE_RESISTANCE` (do **atacante**, como a origem) + imunidade do alvo contra sorteio 1–254. Resistido: não entra o efeito, não há avanço, as duas pontas são avisadas |
| Avanço | Avanço Poderoso e Carga Divina levam o autor até o alvo (máx. 18 células) com `$301` (velocidade 125) e refazem a lista de visíveis |
| Escudos | **Proteção (32)**: 3 pontos guardados antes do buff, cada golpe gasta 1 e reduz `EF_POINT_DEFENCE`%; o buff sai ao zerar (antes ninguém passava os pontos ao pipeline e o buff nunca saía). **Bolha (36)**: pontos = `EFV[0]`; cada golpe de skill que acerta é anulado, sem efeito de estado; Retorno de Magia gasta `Damage` pontos. **União Divina (35)**: `EF_TRANSFER`% do dano vai para quem lançou (perde vida sem morrer), até `EF_TRANSFER_LIMIT`; se o lançador saiu, o buff cai |
| Outros | El Tycia (39): alvo com vida cheia, autor sem mana. Revitalizar (120): +1% da vida máxima por segundo; buff 42 (Égide): `EFV[2]/2` de vida por segundo |
| Testes | 11 novos (Pancada, Estocada, stun com avanço, stun e silêncio resistidos, Carga longe, Bolha por pontos até acabar, Proteção 3 golpes, União Divina com e sem lançador, El Tycia, Revitalizar); suíte completa verde. Também corrigido o mesmo flake do "hello" no helper de teste do pacote `auth` |
| Não portado | Resoluto (8: pontos + buff 6879 + recuo), Lâmina (150: `EventListener`), Quebrar Armadura/Incitar/Stigma/Nêmesis/Travar Alvo como debuffs genéricos (sem o efeito próprio, ex.: provocação), teleportes (72, 113), Pedra da Alma (460), `Requiem` (EF_ADD_DAMAGE1), Estripador (90), Sacrifício/veneno/polimorfo (15, 55, 74/79, 99, 250) |

## Incremento 19 — Dano contínuo (SKD)

| Item | Detalhe |
|---|---|
| Origem lida | `UpdateThreads.pas` laço `SKDListener` (970–1100), `TargetBuffSkill` ramos 15/55/74/79/250/133/457, `RemoveBuffEffect` (49, 73, 108) |
| Arquivos | novos `game/dot.go`, `game/dot_test.go`; alterados `game/skill.go` (efeito pós-golpe e dano contínuo em mob), `game/cleric.go` (ramo geral e Água Benta), `game/buffs.go` (tick, remoção, ferimento ao acabar), `game/mob.go` (`HitKeepOne`), `game/session.go` e `game/handler.go` (estado) |
| Modelo | o dano contínuo fica **preso ao buff do alvo** (`Session.dots`, por skill): acaba quando o buff acaba ou é removido. A origem guardava **um único** dano contínuo por *autor* (lançar o segundo apagava o primeiro) e o rodava numa thread à parte; aqui dois ou mais convivem. Em mobs, que não têm lista de buffs, um registro no `Handler` expira por `Duration` |
| Valor por segundo | Sacrifício/Estocada (15): `EFV[0]` + sorteio (30–59 acima de 80% da vida, 15–29 abaixo); Veneno (55, 74, 79): `(EFV[0] + sorteio 60–119/30–58) / Duration`; 250: `EFV[0]` + mesmo sorteio; Raio Solar (133): `EFV[0]/2` + 30–58 / 10–28. Em mob o sorteio é entre metade e todo o ataque do autor (físico ou mágico conforme a skill) |
| Regra | nunca mata (`RemoveHP` com `StayOneHP`): fica em 1 de vida, em jogador e em mob |
| Ferimento ao acabar | Contagem Regressiva (49) e Mjolnir (73): `RandomRange(15,90) + EFV[0]`; Eclater (108): o mesmo, mais `Damage` se `EF_ACCELERATION1 > 0`; não mata. Dispara em qualquer remoção do buff, como a origem |
| Água Benta (457) | em si mesmo cria o item de índice `Damage` no inventário ("Inventário cheio." se não couber) |
| Testes | 8 novos (dano por segundo e piso 1, parada com o buff, divisão do veneno, faixa do Raio Solar, recast sem acumular e dois convivendo, cancelar para o dano, mob até acabar sem morrer, ferimento ao acabar, Água Benta); verificado por mutação. Suíte completa verde |
| Não portado | polimorfo (99), Pedra da Alma, dano contínuo em dungeon, mensagem de HP do mob no formato `$103` (usa o pacote de dano normal), o dano contínuo de Água Benta no alvo (sem valor na origem, só buff) |

## Incremento 20 — Atirador e Dual Gunner

| Item | Detalhe |
|---|---|
| Origem lida | `BaseMob.pas` `RiflemanSkill` (11121), `DualGunnerSkill` (11351), `ValidAttack` (tipos `LENT`, `CHOCK`, `PARALISYS`) |
| Arquivos | novos `game/gunner.go`, `game/gunner_test.go`; alterados `game/warrior.go` (tabela de tipos de estado, `statusResisted` genérico), `game/skill.go` (ganchos de ajuste e de efeito no golpe que acertou, em jogador e mob) |
| Estados | lentidão (Tiro na Perna, Veneno de Lentidão), choque (Choque Súbito, Choque Hidra), paralisia (Espinho Venenoso) e stun (Primeiro Encontro, Ponto Vital, Ataque Atordoante, Veneno Hidra) passam pelo teste de resistência, cada um com a sua imunidade (`EF_IM_RUNSPEED/SHOCK/IMMOVABLE`). Resistido: o efeito não entra e as duas pontas são avisadas |
| Só testam o golpe (dano 0) | Eliminação, Perseguidor, Marca do Perseguidor, Espinho Venenoso, Requiem, Choque Súbito, Negar Cura |
| Dano | Contra Golpe: +`Adicional`% e o golpe vira crítico; Sentença: dobra com chance `DamageRange − 20`%; Destino: soma `DefFis >> 3` do alvo; Veneno de Mana: +`Adicional`; Morte Decidida: `(Damage+1000)/100 × max(sorteio 0–99, 30)` |
| Efeito no golpe | Eliminação tira 1 buff do alvo; Postura Fantasma e Destino lançam em si a skill indicada em `Adicional`; Veneno de Mana tira `Adicional` de MP do alvo e dá ao autor; Inspirar Matança cura o autor em `Adicional`% do dano (+25% da vida máxima se matou um jogador) |
| Testes | 9 funções novas (cobrindo 4 estados em subtestes); verificado por mutação. Suíte completa verde |
| Não portado | Estripador (90: crítico no alvo dispara 6279), Dor do Predador, Reação em Cadeia e Bomba Maldita (a origem deixa o efeito só em comentário), imunidade total (`EF_IMMUNITY`) e `EF_COUNT_HIT` nas skills de teste, os `*AreaSkill` do Atirador/Dual |

## Incremento 21 — Mago e dano mágico das skills

| Item | Detalhe |
|---|---|
| **Achado** | `GetDamage` escolhe o tipo pela classe: Guerreiro, Templário, Atirador e Dual (classes 0–3) usam ataque/defesa/penetração **físicos**; Mago e Clériga usam os **mágicos** ao lançar skill. O porte calculava toda skill com o ataque físico, então Mago e Clériga batiam com o número errado |
| Origem lida | `BaseMob.pas` `GetDamage` (4716), `MagicianSkill` (11534) |
| Arquivos | novos `game/mage.go`, `game/mage_test.go`; alterados `game/skill.go` (dano base por classe em jogador e mob, ganchos), `game/gunner.go` (encadeia estados e dano zero do Mago), `game/warrior_test.go` |
| Dano | `skillBaseDamage`/`skillMobBaseDamage`: físico = `AtkFis − (DefFis × (1 − pen)) >> 3`; mágico = `AtkMag − (DefMag × (1 − penMag)) >> 3`, piso 1 |
| Estados | Chama Caótica e Inferno Caótico (stun) e Impedimento (silêncio) passam pelo teste de resistência |
| Só testam o golpe | Sofrimento (250), Polimorfo (99), Mão da Escuridão (226) |
| Efeitos | Corroer cura o autor em `Adicional`% do dano; Vínculo **fere** o autor em `Adicional`% do dano (como a origem, pode matar); Cristalizar Mana soma `Adicional`% da mana atual do alvo jogador |
| Testes | 5 funções novas; verificado por mutação (trocar a base mágica pela física derruba o teste). Suíte completa verde |
| Não portado | transformação de Polimorfo (aparência e dano ×2 do pipeline), Mão da Escuridão como debuff com efeito próprio, redução de dano do equipamento do alvo (`GetEquipedItensDamageReduce/10`) em todo golpe de skill, Fluxo de Mana e demais ramos de `SelfBuffSkill` do Mago |

## Incremento 22 — Skills em área das seis classes

| Item | Detalhe |
|---|---|
| Origem lida | `WarriorAreaSkill`, `TemplarAreaSkill`, `RiflemanAreaSkill`, `DualGunnerAreaSkill`, `MagicianAreaSkill`, `ClericAreaSkill` (11677–12330) |
| Achado | quase todo ramo de `*AreaSkill` é `GetDamage` + um teste de estado. Como o dano de área já passa pelo mesmo `skillHitPlayer` da skill de alvo único, bastou ligar **o estado de cada skill** e as que só testam o golpe, em tabelas (`game/areaskills.go`) |
| Estados | stun: Canção de Guerra, Explosão Radiante, Bruma, Queda Negra, Proeminência, Tempestade, Furacão Negro; lentidão: Área de Impacto, Investida Mortal, Esplendor Caótico, Pecados Mortais, Tempestade de Raios; silêncio: Emissão Divina, Detonação, Rajada Sônica; **medo**: Grito de Medo (novo tipo, imunidade `EF_IM_FEAR`) |
| Só testam o golpe | Grito de Medo, Incitar Multidão (revela invisíveis 53/77), Bruma, Eclater |
| Pecados Mortais | dano contínuo fixo de `EFV[0]` por segundo (sem sorteio), como `SKDAction 2` |
| Testes | 5 funções (8 estados em subtestes, com e sem imunidade; só-teste; Incitar; Pecados; dano mágico em área do Mago); verificado por mutação. Suíte completa verde |
| Não portado | `MoveToTarget` da Investida Mortal (comentado na origem), Sensor Mágico/Raio Solar como skills de área da Clériga já cobertos nos incrementos 17 e 19, dungeon, duelo, guardas e pedras de devir |

## Incremento 23 — Skills restantes e buffs reativos

| Item | Detalhe |
|---|---|
| Origem lida | `SelfBuffSkill` (7621: 124/127/137/160, 89, 72, 113, 150, 201, 208, 53/77), `TargetBuffSkill` (113, 124…), `AttackParse` (9940–10090: Pedra da Alma, Requiem, Estripador, Resoluto), `UpdateThreads.pas` laço `EventListener` (Lâmina) |
| Arquivos | novos `game/skillmisc.go`, `game/skillmisc_test.go`; alterados `game/cleric.go` (roteamento, `castSkill` na regeneração), `game/skill.go` e `game/combat.go` e `game/mobai.go` (buffs reativos em todo golpe), `game/session.go` |
| Skills | **Tempo extra** (124, 127, 137, 160): +`EF_SKILL_ATIME6`×60 s no fim do buff; **Fluxo de Mana** (201): `MaxMP/2` + 0–N% (N por Inteligência 10…50); **208**: Damage 200 devolve 15% do MP e dá o buff, 300 só 15% do HP; **Disparos Rápidos** (89): zera as recargas; **Teleporte** (72) e **Teleporte em Massa** (113): recusados com relíquia no inventário ou fora da nação; **Lâmina** (150): lança a skill `EFV[0]` em área em volta do autor a cada segundo enquanto durar; **Invisibilidade** (53/77): larga no chão as relíquias do inventário |
| Buffs reativos | **Pedra da Alma** (460): golpe que mataria deixa o alvo vivo com 30% HP/25% MP e limpa os debuffs, uma vez; **Requiem** (`EF_ADD_DAMAGE1`): +2× o efeito no dano recebido; **Estripador** (90): crítico no alvo dá o buff 6279; **Resoluto** (8): marca o alvo por 8 s com `Damage` cargas, cada golpe gasta uma e dá o buff 6879 |
| Testes | 11 funções novas; verificado por mutação (Requiem ×2→×1 e Lâmina sem o tick derrubam os testes). Suíte completa verde |
| Não portado | recuo (`WalkBacked`) do alvo do Resoluto, `DisparosRapidosBarReset` (barra de atalhos), Demolição X14 e forma féerica (pet/pran), Teleporte fora de guerra de castelo (`InClastleVerus`), objeto de relíquia com a aparência da origem (320) |

## Incremento 24 — Raid / legião (`$342`, `$343`, `$344`, `$207`)

| Item | Detalhe |
|---|---|
| Estado anterior | N (0 dos 4 códigos); `MaxRaidSlots` só reservado no `$326` |
| Origem lida | `PacketHandlers.pas` `SendRaid`, `AcceptRaid` (ramo de legião), `ExitRaid`, `GiveLeaderRaid`; `PartyData.pas` `CreateRaid`, `AddPartyToRaid`, `RefreshRaid`; `Player.pas` `RefreshParty` (laço dos slots) |
| Arquivos | novos `game/raid.go`, `game/raid_test.go`; alterados `game/party.go` (registro de raids; desfazer um grupo o tira da raid), `game/partyhandlers.go` (`$326` mostra a legião), `game/handler.go`, `game/session.go` (`RaidRequester`) |
| Modelo | uma `Raid` por legião (até 4 grupos, um líder) em vez de `InRaid`, `IsRaidLeader`, `PartyRaidCount` e `PartyAllied[1..3]` espalhados em cada grupo e mantidos à mão nos dois sentidos; `Others()` dá os aliados de um grupo |
| Regras | convite: só o líder de um grupo convida o líder de outro; alvo a ≤ 20 células, em grupo, líder, fora de legião; quem convida precisa liderar a legião (se já houver uma) e a legião ter menos de 4; mesma nação (se ambos têm). Aceitar cria a legião ou entra nela, o grupo novo adota a divisão de exp/item; recusar avisa o convidador. Sair devolve a liderança ao primeiro grupo que ficou e desfaz a legião de um grupo só. Passar a liderança vale para o líder de outro grupo da mesma legião |
| `$326` | o jogador recebe o próprio grupo (com a flag de líder da legião) e um pacote por slot 1–3 com o grupo aliado ou vazio, como o laço de `RefreshParty` |
| Corrigido | aceitar não conferia se quem aceita lidera o grupo nem se o convite ainda valia; `ExitRaid` deixava **qualquer membro** tirar o grupo (agora só o líder do grupo); `GiveLeaderRaid` testava `Party.Index = ClientID` (índice de grupo contra id de jogador) para saber se o alvo lidera, só passava por coincidência |
| Testes | 11 funções (convite, 4 recusas, silêncio sem grupo/alvo inválido, criar com slots do `$326`, recusar, aceite inválido, até 4 grupos e o 5º recusado, sair em três situações, passar liderança em quatro, desfazer grupo, 3.000 entradas aleatórias); verificado por mutação (limite de 4 → 5 derruba o teste). Suíte completa verde |
| Não portado | ramo de **aliança de guildas** do `$343` (`AliianceByLegion`); `SendToRaid`/chat de legião e exp/drop repartidos entre os grupos; atualizar os aliados quando um membro sai ou um grupo é desfeito (só o grupo desfeito deixa a legião) |

## Incremento 25 — Duelo (`$395`, `$396`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `SendRequestDuel`, `DuelResponse`; `Player.pas` `TDuelThread` (380–540), `CreateDuelSession`, `SendDuelTime`, `SendDuelEnd`, `RemoveDuelFlag`; regras de duelo em `BaseMob.pas` (HP 10 em vez de morrer, área só no oponente após 15 s) |
| Arquivos | novos `game/duel.go`, `game/duel_test.go`; alterados `game/combat.go` (golpe letal deixa 10 de vida; só o oponente é alvo e só depois da espera), `game/skillarea.go` (regras de aliado com duelo), `game/buffs.go` (`duelTick` no laço de 1 s), `game/handler.go`, `game/session.go` |
| Fluxo | desafio por id ou pelo nome (nível ≥ 10 dos dois, mesma nação, PvP desligado, fora do mesmo grupo, nenhum dos dois em duelo); resposta recusa/aceita (distância, vivo, PvP); aceitar manda a contagem de 15 s (`$1A2`), cria a bandeira (`$349`, título "A vs. B") e liga o duelo nos dois |
| Durante o duelo | 15 s de espera sem golpes; depois só o oponente leva golpe e área; o golpe que mataria deixa **10 de vida**. Fim: vida ≤ 10 (o outro ganha), 5 s fora da área (perde), 5 min (empate), desconexão (o outro ganha). Todos recebem `$1A0` (1/0), a plateia a mensagem e `$101` que apaga a bandeira |
| Corrigido | a origem não impedia o desafiante já em duelo de desafiar outro, nem conferia a faixa do id do alvo; ids de bandeira cresciam sem teto (agora giram em 1000); o fim dependia de uma thread por duelo com o `Sleep` inteiro |
| Testes | 10 funções (desafio, por nome, 6 recusas, ids inválidos, começar, recusar/aceitar inválido, espera e oponente, nunca morre e fim por 10 HP, empate por tempo, saída da área, desconexão, área só no oponente); verificado por mutação. Suíte completa verde |
| Não portado | `ChangeChannel2` do `$396` (duelo entre canais), o aviso de "volte à área" repetido a cada passo, recompensas/ranking de duelo |

## Incremento 26 — Cupom (`$3E05`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `ReclaimCoupom` (4639–4757), tabela `gm_commands` de `aika_db4.sql` |
| Arquivos | novos `store/coupons.go`, `game/coupon.go`, `game/coupon_test.go`; alterados `game/handler.go` (campo `CouponDB`), `game/session.go`, `cmd/aikaserver/main.go` |
| Fluxo | os 16 caracteres viram `AAAA-BBBB-CCCC-DDDD`; o cupom vale se `runned=1`, `refused=0` e `target_name` vazio. Tipo 3 soma AikaCash, tipo 4 entrega o item (pilha ou até 20 cópias) e tipo 5 soma ouro; depois o cupom fica marcado com o nome do personagem |
| Corrigido | o ouro era somado **duas vezes** e avisado uma; o resgate não era atômico (lia, entregava e só depois marcava, numa conexão à parte: dois resgates simultâneos levavam o mesmo cupom). Agora a linha é travada (`FOR UPDATE`), o prêmio é entregue **dentro** da transação e a marca vai junto; inventário cheio desfaz e o cupom continua livre; só letras e dígitos valem (antes qualquer byte entrava na consulta); 5 erros trancam o jogador por 1 minuto contra tentativa de adivinhar |
| Testes | 9 funções (formato do código, cash, ouro uma vez, item em pilha, inventário cheio sem gastar e depois entrega, inválido/já usado/lixo, tranca por tentativas, sem banco, 4 resgates simultâneos com um ganhador); verificado por mutação (ouro duplicado derruba o teste). A consulta SQL **não** foi exercitada contra MySQL |
| Não portado | `$32F` (abandonar quest): precisa do estado das quests do personagem e dos NPCs |

## Incremento 27 — Nação e aliança (`$E3A`, `$F34`, `$F12`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `UpdateNationTaxes`, `UpdateNationGold`, `RequestGuildToAlly`, ramo `AliianceByLegion` de `AcceptRaid`; `Player.pas` `SendNationInformation`, `IsMarshal`, `IsArchon`, `IsGradeMarshal`, `IsGradeArchon`; `Nation.pas` (`LoadNation`, `SaveNationTaxes`); `GuildData.pas` (`TGuildAlly`); tabelas `nations` e `guilds` |
| Arquivos | novos `nation/nation.go`, `store/nations.go`, `game/nation.go`, `game/nation_test.go`, `guild/ally.go`, `guild/ally_test.go`, `game/alliance.go`, `game/alliance_test.go`; alterados `guild/guild.go` (campo `Ally`, criada sozinha, `leaveAllianceLocked` ao desfazer), `store/guilds.go` (colunas de aliança na carga e na gravação), `game/guildhandlers.go` (`GuildsAlly` no `$965`), `game/raid.go` (ramo de aliança no `$343`), `game/entry.go` (nação no login), `game/handler.go`, `game/session.go`, `cmd/aikaserver/main.go` |
| Nação | `nation.Registry` com as três nações (tesouro, taxas, guildas de grade, aliança entre nações) carregadas de `nations`. `$F34` devolve o tesouro da nação do jogador (0 sem nação). `$E3A`: só o **marechal** (líder da guilda marechal da nação da conta) muda as taxas (cidadão 5–15, visitante 10–20); grava, avisa e reenvia a informação da nação a todos os jogadores dela |
| Informação da nação | `$936` (grade e taxas), `$91A` (cerco), `$967` (nação aliada) e `$F34`, no login (antes das relíquias, como a origem) e a cada mudança; buffs 6600/6601 (marechal / guilda marechal) e 6602/6648 (archon / guildas archon) ligados e desligados conforme o cargo |
| Aliança | `$F12`: o líder da guilda líder da aliança chama outra guilda pelo nome; recusas com as mensagens da origem (não é líder da guilda / da aliança, outra nação, já em aliança, sem vaga, a si mesma, líder offline); o convite vai por `$342` ao líder da outra e o `$343` responde. Aceitar põe a guilda no primeiro slot livre, copia a lista para **todas** as guildas da aliança, reenvia `$965` (agora com a lista) e avisa os membros logados; recusar avisa o líder. Se quem chamou é o marechal, a guilda nova ocupa o primeiro cargo livre de estrategista / juiz / tesoureiro e a grade é gravada |
| Corrigido | taxa convertida para BYTE antes de checar (266 virava 10 e passava); sem vaga o slot era 255 e a entrada escrevia fora do vetor, e a conferência de vaga misturava índices de guildas diferentes; o convite era aceito mesmo que o convidador já não liderasse a aliança (agora tudo é conferido de novo ao aceitar); guilda desfeita deixava o índice morto na lista das aliadas (agora sai de todas; se era a líder, a aliança acaba); `UPDATE` de taxas montado por concatenação de texto |
| Testes | `game/nation_test.go` 8 funções (tesouro, taxas do marechal e gravação, só o marechal, faixas e limites incluindo 266, quatro pacotes, buffs de marechal e membro, buffs de archon, sem registro), `game/alliance_test.go` 8 funções (convite, 5 recusas, só o líder da aliança, aceitar com lista copiada em 3 guildas, recusar, convite envelhecido, aliança cheia, grade do marechal, fora do marechal) e `guild/ally_test.go` 6 funções (guilda nova, guilda do banco, regras de entrada, máximo de 4, cópia para todas, guilda desfeita); verificado por mutação (marechal sem checar a liderança, faixa de taxa, revalidação do convite, limite de vagas, limpeza da aliada). Suíte completa verde |
| Não portado | cerco (`$91A` sai só com os defensores; os atacantes `cerco_guildid_attack_*` não são lidos), nação aliada entre nações (`nation_ally` é lido e enviado, nada o altera), chat de aliança, `SaveNation` completo (só taxas e grade são gravadas), reenvio de status/HP depois de mudar a grade (os buffs já refazem). A consulta SQL de `nations` e as colunas de aliança em `guilds` **não** foram exercitadas contra MySQL |

## Incremento 28 — Renomear pran (`$3E02`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `RenamePran` (9190), `Functions.pas` `IsLetter` e `VerifyNameAlreadyExists`, `Player.pas` (carga de `Pran1`/`Pran2` de `prans` por `acc_id ORDER BY id LIMIT 2`), tabela `prans` |
| Arquivos | novos `store/prans.go`, `game/pran.go`, `game/pran_test.go`; alterados `game/handler.go` (campo `PranDB`, `case`), `cmd/aikaserver/main.go` |
| Fluxo | nome de 1 a 16 letras/dígitos; recusa se já existe em `prans`; batiza a primeira das duas primeiras prans da conta que está sem nome ("Todas suas prans já possuem nome." se não houver); responde o pacote com o id da conta e reenvia os slots 84 e 85 do armazém |
| Corrigido | nome **vazio** passava (`IsLetter` devolve True para texto vazio); conferir o nome e gravá-lo eram passos soltos (dois jogadores pegavam o mesmo nome); agora há uma trava e o `UPDATE` só vale com `name = ''`; consultas parametrizadas em vez de texto concatenado |
| Testes | 4 funções (batiza a segunda quando a primeira tem nome, 5 recusas, sem pran e sem banco, dois pedidos simultâneos com o mesmo nome e um só ganha); verificado por mutação (sem a trava e sem a checagem de nome vazio, ambos derrubam teste). Suíte completa verde |
| Não portado | o modelo de pran inteiro (carga, equipamento, bolsas, habilidades, comida, evolução); a origem só gravava o nome na memória da conta e na gravação periódica, aqui ele vai direto para `prans`. SQL **não** exercitada contra MySQL |

## Incremento 29 — Pran inteira, fatia 1: modelo, carga, gravação e entrada no mundo

| Item | Detalhe |
|---|---|
| Origem lida | `PlayerData.pas` (`TPran`), `Player.pas` (`LoadAccountSQL` bloco de prans, `SaveInGame`, `SendPranToWorld`, `SendPranSpawn`, entrada em `SendToWorld`, `PranIsFairy`), `SkillFunctions.pas` (`GetSkillPranLevel`), `ServerSocket.pas` (`GetFreePranClientID`), tabelas `prans`, `items` (tipos 5 e 6), `skills` (tipo 3), `itembars` (dono = id + 1024000) |
| Arquivos | novos `pran/pran.go`, `pran/ids.go`, `pran/pran_test.go`; alterados `store/prans.go` (`LoadAccount`, `Save`), `game/pran.go`, `game/pran_test.go`, `game/session.go` (`Prans`, `PranClientID`, `SpawnedPran`), `game/entry.go`, `game/handler.go`, `game/charsave.go`, `cmd/aikaserver/main.go` |
| Modelo | `pran.Pran` com atributos, personalidade (6 valores), 16 equipamentos, 42 itens de bolsa, 10 habilidades, barra de 3 e medidas; as duas primeiras prans da conta (`ORDER BY id LIMIT 2`) são `Prans[0]` e `Prans[1]` |
| Carga e gravação | tudo na tabela da origem, com slot fora da faixa ignorado (a origem indexava o vetor com o valor do banco); gravação no `UPDATE` de `prans`, itens (`SaveSet`), habilidades e barra, na saída e na gravação periódica; renomear atualiza o cache (senão a gravação devolveria o nome vazio) |
| Entrada | se `Equip[10].Identific` do personagem é o `ItemID` de uma pran: `$349` (spawn, id de pran 10241–11240, posição numa das 8 casas vizinhas, título "Dono 's Pran", aparência da arma com `DelayUse`) e `$907` (nome, classe, comida, personalidade, níveis de habilidade empacotados, devoção, HP/MP, exp, defesas, equipamentos, bolsas, barra). Pran em forma de fada (classes 61/71/81) não aparece, mas os dados vão |
| Corrigido | a segunda pran comparava a personalidade com a devoção da **primeira**; cada jogador ganhava um ClientID de pran no login mesmo sem pran (agora só quem a invoca, devolvido ao sair) |
| Testes | 9 funções novas (personalidade, níveis empacotados, fada, ids, entrada com spawn e dados, sem item/item de outra pran, fada, renomear + gravação, sair libera id); mutação em personalidade, cache do nome e regra da fada derruba teste. Suíte completa verde |
| Não portado (próximas fatias) | **invocar/guardar a pran** por item (`ItemFunctions`, linhas ~5352–5481), **spawn para quem passa perto** (o `$349` só vai ao dono; falta entrar na lista de visíveis e sumir ao sair), passivas das habilidades (`SetPranPassiveSkill`, ~16 tipos) e atributos do equipamento (`SetPranEquipAtributes`), exp/nível (`AddPranExp`, `AddPranLevel`), comida e devoção (`SendPranDevotionAndFood`), evolução de classe, habilidades e barra de pran, bolsa e equipamento da pran (`ItemFunctions`), mover a pran junto do dono, combate da pran. SQL não exercitada contra MySQL |

## Incremento 30 — Pran, fatia 2: passivas e equipamento

| Item | Detalhe |
|---|---|
| Origem lida | `Player.pas` `SetPranPassiveSkill` (6028–6420), `SetPranEquipAtributes`; `BaseMob.pas` `SetEquipEffect`, `GetCurrentScore` (uso de `EF_PRAN_DAMAGE1/2` e `EF_PRAN_RESISTANCE1/2`); `SkillFunctions`/`SkillData` (familia da passiva = `SkillData[Index+Nivel-1].Index`) |
| Arquivos | novos `game/pranfx.go`, `game/pranfx_test.go`; alterados `game/buffs.go` (EF 182/183/186/187 em `Ability.addTo`), `stats/derived.go` (`PranAtkFis/PranAtkMag` entram depois dos pontos e antes dos percentuais, como `DNFis`), `game/pran.go` (liga os efeitos na entrada), `game/session.go` |
| Passivas | 12 famílias: 193/217/241 ataque (`5/3/4 + 2*lv`), 194/218/242 defesa (`2*lv`), 197 HP (`23+(13+lv)*lv`), 199 crítico (`+lv`), 221 MP (`30+20*lv`), 223 resistência 6 (`+lv`), 245 e 247 HP (`15+(8+lv)*lv`) e MP (`21+(11+lv)*lv`) |
| Equipamento | para cada item da pran: os 3 efeitos gravados valem o dobro e os 3 primeiros `EF/EFV` da tabela entram como estão; item de `ItemType 10` não conta |
| Como liga e desliga | `applyPranEffects` soma ao `Ability` do dono e **guarda a lista somada**; `removePranEffects` subtrai exatamente essa lista. Ligar de novo primeiro desliga. Depois de cada mudança refaz os atributos e reenvia pontos e status |
| Corrigido | a desativação da 2ª pran usava o nível das habilidades da **1ª** nas famílias 197 e 247 (sobrava ou faltava HP/MP); e subir o nível antes de desligar deixava resto — agora o que se desfaz é o que foi somado |
| Testes | 4 funções (12 fórmulas de passiva, efeitos de passivas + equipamento no dono e volta ao desligar, ligar duas vezes sem dobrar e subir de nível sem deixar resto, sem tabela de skills); mutação em fórmula, dobro do efeito gravado, item `ItemType 10` e posição do dano da pran derruba teste. Suíte completa verde |
| Não portado | **efeitos de conjunto** (`Conjuntos[]`/`SetConjuntEffect`) das roupas da pran, o visual de `ItemType 8` (`EFF_5`), expiração de roupa de pran (`Expires`, ainda não portada), `EF_PRAN_REGENHP/MP`, `EF_PRAN_PARRY`, `EF_PRAN_SKILL_DAMAGE` (usados fora do `GetCurrentScore`); desligar ao guardar a pran (não há invocar/guardar ainda) |

## Incremento 31 — Pran, fatia 3: experiência, nível, comida e devoção

| Item | Detalhe |
|---|---|
| Origem lida | `BaseMob.pas` (bloco de exp da pran após o abate, ~6650–6900), `Player.pas` `AddPranExp`, `AddPranLevel` (8158–8700), `SendPranLevelAndExp` (`$116`), `SendPranDevotionAndFood` (`$96B`), `ItemFunctions.pas` `ITEM_TYPE_PRAN_FOOD` (26) e `ITEM_TYPE_PRAN_DIGEST` (230), `PlayerData.pas` (`PRAN_HP_INC_PER_LEVEL`=209, `PRAN_MP_INC_PER_LEVEL`=356), `Load.pas` `InitPranExpList` |
| Arquivos | novos `pran/level.go`, `pran/level_test.go`, `game/pranexp.go`, `game/pranexp_test.go`; alterados `game/combat.go` (gancho no abate), `game/useitem.go` (comida e digestivo), `game/handler.go` (`PranExp`), `cmd/aikaserver/main.go` |
| Experiência | no abate, a pran **invocada** ganha **1/3** da exp do dono. Tetos por fase: nível 0–3 (fada) até o 4; 5–18 (criança) até o 19; 20–48 (adolescente) até o 49; passou do teto → exp fica na linha anterior e sobe os níveis que faltam; nos níveis 4, 19 e 49 a pran só segue depois de evoluir (classes 61/71/81, 62/72/82, 63/73/83: "A sua pran precisa evoluir para ganhar exp."); 50+ sem teto. `AddPranExp` soma e sobe enquanto a exp passar da linha do nível seguinte; envia `$116` |
| Nível | `+209` HP e `+356` MP máximos, vida e mana cheias, "Sua pran subiu de nível.", `$116`; as passivas saem antes e voltam depois (valem com o nível novo); nos níveis mostrados 5, 10, 15 … 50 as habilidades certas ganham um nível (tabela portada, incluindo o 50 sem o slot 0) e, se estão na barra, o índice avança e o cliente recebe `$31E`; reenvia `$907` |
| Comida | item 26: só com pran invocada e abaixo de 121; os itens 8105–8110 mexem na personalidade (+2 na dela, −3 nas outras cinco, piso 0) e somam 1 à devoção (até 226); a fome sobe 15 (teto 121); reenvia `$907`. Pran cheia ou sem pran invocada **não gasta** o item. Digestivo (230): metade da fome, recusa com fome ≤ 13 |
| Corrigido | `PranExpList` era lida sem conferir o índice (agora a tabela responde); soma de exp podia estourar o DWORD; casar a barra pelo índice 0 (barra vazia) fazia habilidades sem índice "estarem na barra" |
| Testes | 8 funções em `pran` (nível, marcos, fronteiras, comida, digestivo) e 12 em `game` (um terço, vários níveis, teto da fase, fronteira de evolução, adulta, sem pran/tabela, passivas refeitas, aviso de barra, comida, comida inválida, digestivo, abate); mutações no um terço, no teto da fase, nas passivas, no teto da comida e no gancho do abate derrubam teste. Suíte completa verde |
| Não portado | exp da pran em abates de **grupo** e de **dungeon** (só o abate individual); `$96B` (a origem nunca o chama); a evolução de classe (o que destrava os níveis 4/19/49); fome que cai com o tempo (não há laço na origem lida); exp por outras fontes |

## Incremento 32 — Pran, fatia 4: invocar, guardar, vizinhos e evolução

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `MoveItem` (3451–4115: `SpawnPran`/`UnspawnPran`, central 84/85), `Player.pas` `SendPranSpawn`, `SendPranUnspawn`, `SendPranToWorld(255)`, `BaseMob.pas` (atualização de visíveis, 440–510 e 935–1000), `NPCHandlers.pas` `FinishQuest` (quests 406 e 407) |
| Arquivos | novos `game/pranworld.go`, `game/pranworld_test.go`, `game/pranevo.go`, `game/pranevo_test.go`; alterados `game/pran.go` (spawn antigo substituído), `game/inventory.go` (pran deixa de ser recusada; central 84/85; sem checagem de classe), `game/spawn.go` (pran junto de `spawnPair`, `despawnPair` e `removeFromEveryone`), `game/inventory_test.go` |
| Invocar / guardar | mover o item de pran (ItemType 10) do inventário para o `Equip[10]` invoca a pran cujo `ItemID` é o `Identific` do item: sai a pran anterior (se houver), depois `$349`, `$907`, passivas e equipamento da pran ligados; o caminho inverso guarda (painel zerado, `$101`, efeitos desligados, id devolvido). Trocar uma pran pela outra faz as duas coisas. A pran não tem classe: qualquer dono a equipa |
| Vizinhos | a pran invocada aparece (`$349` com a posição guardada) para quem passa a ver o dono e some quando ele sai da vista ou do jogo; a posição é uma das 8 casas ao redor do dono, sorteada ao invocar |
| Fada | pran de classe 61/71/81 não vira criatura: o dono recebe o efeito da classe (2, 4 ou 8) e, ao guardar, o efeito 0 |
| Central da pran | os slots 84 e 85 do armazém voltam a aceitar movimentos, mas só de item de ItemType 10, nos dois sentidos |
| Evolução | `evolvePran` porta o bloco das quests 406 (nível 5: fada → criança, `Equip[0]=104`, `Equip[6]=150`, +1 na habilidade de transformação, item de pran do dono vira 104, efeito 0) e 407 (nível 20: criança → adolescente, `Equip[0]=105`, item 105); reenvia `$907`/`$349` e o slot 10, e refaz os efeitos do equipamento novo. Quest 408 é vazia na origem |
| Corrigido | a origem não refazia o equipamento da pran depois de evoluir (o acumulador ficava com o antigo); agora o que se desliga é exatamente o que foi somado |
| Testes | 14 funções novas (invocar com vizinho, guardar e liberar id, trocar uma pela outra, atributos entram e saem, fada, sem checagem de classe, item de outra conta, chegada e saída de vizinho, central, evolução 406 e 407, recusas, sem pran, efeitos refeitos); mutações em invocar, central, nível da evolução, vizinhos, saída, classe e liberação do id derrubam teste (duas delas só depois de endurecer os testes). Suíte completa verde |
| Não portado | **chamar `evolvePran`**: as quests 406/407 terminam no NPC, e o sistema de NPCs e de quests não existe (bloqueio dos `.npc`); mover itens **para dentro** do equipamento e da bolsa da pran (tipos de slot 5 e 6 no `MoveItem`); `ClearQuest` (marca da quest); a pran **andar** junto do dono; exp em grupo e em dungeon |

## Incremento 33 — Pran, fatia 5: itens no equipamento e na bolsa da pran

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `MoveItem` (tipos de slot 5 e 6, 3576–3630, 3733–3790, 3804–3904), `DeleteItem` (3252), `UngroupItem` (3335), `ItemFunctions.pas` `GetItemEquipPranSlot`, `GetEmptyPranSlot`, `GetInvPranMaxSlot`, `Player.pas` `GetPranClassStoneItem` |
| Arquivos | novos `game/pranitems.go`, `game/pranitems_test.go`; alterados `game/inventory.go` (`resolveSlot`, `podeTrocar`, `moveItem`, `deleteItem`), `game/itemstack.go` (`ungroupItem` na bolsa da pran), `game/itemstack_test.go` |
| Equipamento da pran | slots 1 a 5, só com pran invocada. Do inventário: o slot é `ItemType − 18`, o nível do item no máximo o da pran + 1, e a classe do item zero ou igual à "pedra" da forma (100 para 61/62/71/72/81/82, 101 para 63/73/83, 102 para 64/74/84). Ao trocar a peça os efeitos da pran são refeitos e a pran é reenviada (`$349`) |
| Bolsa da pran | slots 0 a 39, com a bolsa que o cobre (`Inventory[40]` para 0–19, `[41]` para 20–39); item preso à conta (`TypeTrade` 1) não entra nem sai por cima de um preso. Apagar (`$32C`, slots 0–41) e separar pilha (`$333`) também valem na bolsa da pran, e o pedaço separado vai para o primeiro slot livre dela |
| Diferenças conscientes | o equipamento da pran só troca com o inventário (a origem deixava o tipo 5 ir ao armazém e ao baú sem conferência); ao tirar uma peça a origem comparava o slot do item com o slot do INVENTÁRIO de destino e recusava quase tudo, aqui compara com o slot da pran |
| Testes | 14 funções (equipar soma efeito e reenvia a pran, tirar devolve, trocar confere a peça que entra, recusa por slot/nível/classe, exige pran invocada, slots 1–5, bolsa exige a bolsa que cobre o slot, item preso, equipamento só com o inventário, reorganizar a bolsa, apagar, separar, separar sem espaço). 12 mutações derrubam teste. Suíte completa verde |
| Não portado | a barra de atalhos da pran como destino de arrastar; formato próprio de `$F0E` para o item de pran (continua o formato normal); roupa/aparência da pran (`Equip[0]`) por item |

## Incremento 34 — Título ativo (`0x361`) e lista de títulos

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `UpdateActiveTitle` (9139), `Player.pas` `SendUpdateActiveTitle` (7220), `SendToWorld` (bloco "Set Titles List", 5243–5324) e `LoadCharacterMisc` (1478), `BaseMob.pas` `SetOnTitleActiveEffect`/`SetOffTitleActiveEffect` (1882–1920), `Functions.pas` `GetTitleLevelValue`, `Load.pas` `InitTitles`, `FilesData.pas` (`TitleLevel`, 56 bytes) |
| Arquivos | novos `titledata/titledata.go`, `titledata/titledata_test.go`, `store/titles.go`, `game/titles.go`, `game/titles_test.go`; alterados `store/characters_save.go` (`active_title` entra no `SaveProgress`), `game/session.go`, `game/handler.go`, `game/entry.go`, `game/charsave.go`, `cmd/aikaserver/main.go` |
| Dados | `Title.bin` (255 títulos × 4 níveis × 56 bytes; o arquivo real tem bytes a mais, ignorados como na origem). Conferido contra o arquivo do repositório: título 1, nível 1 = "Protetor da Natureza", EF 92 / EFV 2 |
| `0x361` | dado 0 tira o título; outro valor procura na lista do jogador um título com nível > 0 e o torna ativo. Os efeitos (até 3 pares EF/EFV do nível, tabela) entram e saem do acumulador pelo valor exato somado. O `$361` vai ao jogador e a quem o vê; depois pontos, status, nível e HP/MP |
| Entrada no mundo | lista carregada da tabela `titles`; o pacote `$925` leva o mapa de níveis por categoria (`GetTitleLevelValue`, 4 bits por título, 12 categorias) e o progresso de cada tipo (4, 7, 8, 9, 10, 11, 12, 13, 15, 16, 23); depois o título ativo é reenviado e o efeito ligado. O título aparece no spawn (`TitleId`/`TitleLevel`) |
| Gravação | `titles` regravada numa transação (DELETE + INSERT; a origem não usava transação) na saída e a cada gravação periódica; `active_title` entra no UPDATE de progresso |
| Corrigido | título que o jogador não tem: a origem desligava o efeito do antigo mas o mantinha ativo, e a próxima troca subtraía o efeito de novo — aqui nada muda; níveis fora de 1..4 e `TitleIndex` fora do vetor do tipo eram indexados sem conferir |
| Testes | 3 em `titledata`, 9 em `game` (mapa de níveis, pacote de entrada, carga com ativo, ativar soma e avisa vizinhos, trocar tira o antigo, título inexistente, fora do jogo, entrada, gravação). Mutações em bit, recursão, categoria, desligar efeito, nível−1, vizinhos, avatar, progresso e `active_title` derrubam teste |
| Não portado | **ganhar e subir de nível um título** (contadores de abate, pesca, arena etc., tipos 4–23): hoje só os títulos que já estão no banco aparecem; `EnsureLeaderTitle` (Marechal/Archon) — a origem alterava uma cópia e não tinha efeito; nenhuma consulta SQL nova foi executada contra MySQL |

## Incremento 35 — Item de evento (`0x359`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `ReceiveEventItem` (8913), `Player.pas` `GetAllEventItems` (10887) e `DiaryItemAvaliable` (10994), `ItemFunctions.pas` `PutItemOnEvent` (496), `GetInvAvailableSlots` (675), `aika_db4.sql` (`items.slot_type` 17, `characters.last_diary_event`) |
| Arquivos | novos `store/events.go`, `game/eventitem.go`, `game/eventitem_test.go`; alterados `game/handler.go` (`EventDB`), `cmd/aikaserver/main.go` |
| Fluxo | sem espaço: "Inventário cheio." e nada mais. Prêmio diário liberado (24 h desde `last_diary_event`, lido com trava e regravado) coloca o item 10467 na fila. Fila vazia: "Não existem itens de evento para receber." Com itens: um por slot livre, depois "Não existem mais itens de evento para receber."; todos os ramos terminam com o sinal `$359` |
| Corrigido | a origem apagava por `item_id` depois de entregar um item: quem tinha duas caixas iguais recebia uma e perdia a outra. Aqui cada linha é apagada pelo próprio `id` **antes** de entregar; se a entrega falhar a linha volta para a fila |
| Testes | 9 funções (entrega, duas linhas iguais, só o que cabe, inventário cheio não toca na fila, fila vazia e sinal, prêmio diário uma vez, linha levada por outro pedido, fora do jogo/sem banco, linha que não cabe volta inteira). 8 mutações derrubam teste |
| Não portado | `GMEventItem` / `GMEventItemForAll` (ferramenta de GM), o prêmio de fundador (comentado na origem); nenhuma consulta SQL nova foi executada contra MySQL |

## Incremento 36 — Troca de canal (`0xF05`, `0xF06`, `0xF0B`) e multi-canal

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `ChangeChannel` (7394), `LoginIntoChannel` (7791), `Load.pas` `InitServers`/`InitServerList`, `Functions.pas` `LoadSL`/`EncDecSL`, `FilesData.pas` `TChannelFromList`, `ServerSocket.pas` (porta 8822 para todos os canais) |
| Arquivos | novos `serverlist/serverlist.go` (+ teste), `game/channelchange.go`, `game/channelchange_test.go`; alterados `game/handler.go` (`Handoffs`, `ChannelStatus`, casos `0xF05/F06/F0B`, liberação no `OnDisconnect`), `game/session.go`, `cmd/aikaserver/main.go` (um `Handler` por canal; opções `-multicanal` e `-sl`) |
| Multi-canal | `-multicanal` sobe os `Channels` do `.ini`, cada um no IP do `SL.bin` e na porta 8822 (127.0.0.11 e 127.0.0.12 no arquivo do repositório). Cada canal tem o próprio mundo (jogadores, mobs, grupos, itens no chão, dungeons, ids de pran); dividem guildas, nações, templos e a fila de troca. **Sem a opção o servidor sobe um canal só em `:8822`, exatamente como antes** |
| `0xF05` | o cliente pede o canal em `Info1`. Recusas: "Canal destino não válido. ERR_1" (não existe), "ERR_2" (desligado), "ERR_3" (é este mesmo). Aceito: registra a ordem, responde `$F05` (conta e canal) e solta a conexão, o que grava o personagem |
| `0xF06` | no canal novo o cliente manda conta (`Serial`) e usuário. A ordem precisa ser deste canal, do mesmo usuário e do mesmo IP, ter menos de 30 s, e o canal de origem precisa ter terminado de gravar (espera até 10 s); vale uma vez. O personagem é relido do banco e entra no mundo direto (sem lista nem numérica) |
| `0xF0B` | o Go já manda tudo o que a origem mandava depois dele logo ao entrar no mundo; aqui só é reconhecido |
| Diferença de projeto | a origem levava conta e personagem inteiros na memória (`ChangeChannelList`) e esperava o jogador antigo sumir; aqui só a ordem passa de um canal ao outro e o estado vem do banco |
| Testes | 3 em `serverlist` (inclui o `SL.bin` real), 9 em `game` (recusas, líder de grupo, troca completa com dois canais, conferência de ordem/usuário/endereço/canal/validade, sessão já autenticada e conta diferente, ordens não se sobrepõem, espera pela gravação, `0xF0B`). 15 mutações derrubam teste. Servidor subido com `-no-db -multicanal`: duas portas abertas (127.0.0.11:8822 e 127.0.0.12:8822) |
| Não portado | levar a legião (raide) junto (o grupo já viaja, incremento 37); buffs, que a origem levava na memória; duelo entre canais (`0x396` → `ChangeChannel2`); amigos entre canais; o guarda de baú de guilda (`ChestTick`) avisa só quem está no canal que varreu |

## Incremento 37 — Grupo na troca de canal

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `ChangeChannel` (ramos "telar apenas minha party", 7689–7773) e `LoginIntoChannel` (ramo "então é minha party", 8068–8107) |
| Arquivos | alterados `game/channelchange.go`, `game/channelchange_test.go`, `game/handler.go` (`moved`, `movedMu`) |
| Fluxo | o líder de um grupo com outros membros leva junto quem está a até `DISTANCE_TO_WATCH` dele: cada um recebe o `$F05`, tem a ordem registrada e a conexão solta. Quem está longe sai do grupo e fica. No canal novo o primeiro a chegar cria o grupo (com a divisão de exp/item do original), os outros entram, e o líder verdadeiro assume ao chegar. Duas viagens de grupo ao mesmo tempo não se misturam (chave por viagem) |
| Testes | grupo viaja junto (dois jogadores, dois canais, ordem de chegada invertida, líder e divisão preservados, grupo velho acaba), membro longe fica, líder de legião é avisado e fica, dois grupos intercalados. 6 de 7 mutações derrubam teste; a da chave por viagem só é pega no teste direto de `rejoinParty`, não na chamada de `changeChannel` |
| Não portado | **legião (raide)**: o líder de um grupo em legião é avisado e fica (os grupos aliados e `PartyAllied` não são recriados); buffs |

## Incremento 38 — Títulos: ganhar, subir e perder (`AddTitle`, `UpdateTitleLevel`, `RemoveTitle`)

| Item | Detalhe |
|---|---|
| Origem lida | `Player.pas` 9073–9247 e `SendTitleUpdate` (7003), `BaseMob.pas` 7449–7581 (único gatilho de progresso na origem: o título 27 no abate de jogador), `ItemFunctions.pas`/`NPCHandlers.pas`/`UpdateThreads.pas`/`CommandHandlers.pas` (chamadores) |
| Arquivos | alterados `game/titles.go`, `game/titles_test.go` |
| Implementado | `addTitle` (primeiro slot livre; com aviso o progresso nasce na meta do nível, manda `$17D` e a mensagem), `updateTitleLevel` (refaz o efeito se for o ativo), `removeTitle` (se era o ativo, deixa de ser e o efeito sai), `sendTitleUpdate` (`$17D`) |
| Corrigido | título repetido entrava duas vezes; id e nível fora da faixa indexavam a tabela; remover o título ativo deixava o efeito ligado e o título "ativo" |
| Achado | **na origem o único contador de progresso de título é o do título 27, no abate de jogador (PvP).** O Go não tem PvP entre jogadores, então esse gatilho não existe e nada alimenta o progresso por abate. Os demais títulos só mudam por item, NPC, comando ou o laço de `UpdateThreads` (título 45), que ainda não chamam estas funções |
| Não portado | cópia dos títulos 17 e 18 para os outros personagens da conta; os chamadores acima; o gatilho do título 27 (depende de PvP) |
| Testes | 4 funções (adicionar com aviso e recusas, lista cheia, subir nível do ativo, remover o ativo) |

## Incremento 39 — Ferramenta de GM, fatia 1 (`0x3202`, `0x3204`–`0x3207`, `0x3210`, `0x3212`, `0x3299`)

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` `CheckGMLogin` (10798) e os `GM*` correspondentes (10893–11773), `MiscData.pas` `TMasterPrives`, `Player.pas` `CheckModeratorLogged`/`CheckGameMasterLogged`/`CheckAdminLogged`, `aika_db4.sql` (`gm_accounts`) |
| Arquivos | novos `store/gm.go`, `game/gm.go`, `game/gm_test.go`; alterados `game/handler.go` (`GMDB`, `Channels`, casos), `game/session.go`, `cmd/aikaserver/main.go` |
| Login | `$3202` → `$3203` (1 ou −1). Usuário em `gm_accounts`, conta não bloqueada, senha: o cliente manda o MD5 e a conta guarda a senha em texto (como na origem). Cada erro soma ao contador; com o contador em 5 o erro seguinte bloqueia a conta; acerto zera. Comparação em tempo constante |
| Privilégios | moderador (3): mover e gritar; game master (4): também desconectar; administrador (5): também ouro, cash e itens de evento. Sem login de painel nada acontece |
| Ações | mover jogador por nome (`$3204`); mensagem de servidor no canal ou em todos (`$3205`, prefixo `<[GameMaster] nome>`, corte em 90); ouro e cash somar/tirar/zerar com teto (`$3206`, `$3207`); derrubar jogador (`$3210`); item de evento a um jogador (`$3212`) ou a todos os personagens (`$3299`, um INSERT só). O alvo é achado por nome em qualquer canal, ou por id no canal do painel. Toda ação vai para o log com a conta de painel |
| Corrigido | a origem derrubava todo jogador de mesmo nome em todos os canais; ficava calada quando o banco caía no login; somava ouro sem teto; item de evento por jogador não conferia item/quantidade |
| Testes | 9 funções (login, bloqueio por senha, sem login, matriz de privilégios, ouro/cash com teto e zerar, outro canal e id, chat local e global com corte, posição inválida). Mutações em comparação de senha, bloqueio, status, zerar erros, privilégios, tetos, corte e chat global derrubam teste; a da posição é equivalente (`teleportTo` já valida) |
| Não portado de propósito | (corrigido no incremento 40: o fluxo de aprovação, os comprovantes e o cupom foram portados; ver abaixo) |
| Não portado (falta fazer) | `0x3208` (nível, com assinatura de comando para quem não é admin), `0x3209` (buffs), `0x3211` (banir: pede escrever `accounts.account_status` e `ban_days`), `0x3214` (informações do servidor), `0x3219` (criar mob), `0x3238` (efeito), `0x324A` (apagar prans), `0x322D` (lista de GMs), `0x5000` (console remoto; o Go já tem o console por token) |

## Incremento 40 — Ferramenta de GM, fatias 2 e 3, e teste da troca de canal entre nações

**Correção de um erro meu (incremento 39).** Escrevi que a aprovação de comandos "executa o SQL guardado em `gm_commands.command`". **Isso estava errado.** Relida a origem (`GMApproveCommand`, 12151–12286), a aprovação só lê o tipo, o alvo e a quantidade e aplica nível (1), item de evento (2) ou ativa o cupom (3 e 4); o texto de `command` nunca é executado. O fluxo foi então portado inteiro, com tipos fechados.

| Item | Detalhe |
|---|---|
| Origem lida | `PacketHandlers.pas` 11199–11440 (nível, buffs), 11481 (banir), 12043–12342 (nomes, pendentes, aprovar, reprovar, efeito), 12345–12446 (cupom), 12448–12860 (comprovantes, zerar prans), `ItemFunctions.pas` (`AddTitle` nos itens, `ITEM_TYPE_SET_ACCOUNT_NATION`), `ServerSocket.pas` (nação do canal) |
| Arquivos | novos `game/gm2.go`, `game/gm3.go`, `game/useitem_nation.go`, `store/gmfounders.go`, `store/accountnation.go` e os testes; alterados `game/gm.go`, `game/useitem.go`, `game/handler.go`, `store/gm.go`, `cmd/aikaserver/main.go` |
| Códigos novos (opdiff 118 → 132) | `0x3208` nível, `0x3209` buffs, `0x3211` banir, `0x3229` pendentes, `0x322D` nomes de painel, `0x3234` reprovar, `0x3236` aprovar, `0x3238` efeito, `0x323A` pedir cupom, `0x3240`/`0x3242` buscar comprovante, `0x3246` cadastrar comprovante, `0x3248` validar comprovante, `0x324A` zerar prans |
| Nível | administrador soma (nível do **alvo** + N), tira ou zera; game master não muda o nível, **deixa um pedido** (`gm_commands` tipo 1, alvo e quantidade) para um administrador aprovar |
| Banir | game master ou acima: grava `accounts.account_status = 8` (e `ban_days`, se não for permanente — 1000 dias ou mais) **na hora**, avisa todos os canais com o motivo e derruba a conexão; ninguém bane a si mesmo |
| Aprovar / reprovar | só administrador. O pedido é reservado no banco antes de aplicar (dois administradores não o aplicam duas vezes); nível e item exigem que o personagem exista (senão o pedido continua pendente); nível vale ao vivo ou grava `level/experience` fora do jogo; item de evento entra na fila do jogador; cupom só passa a valer. Lista de pendentes só para administrador (a origem não conferia) |
| Cupom | moderador ou acima; PinCode de 10/20/50/100 de AikaCash ou item de fundador; **código sorteado** (`crypto/rand`), não mais o MD5 da hora; o pedido sai em nome da conta de painel logada |
| Comprovantes | buscar por id da transação ou por parte do nome (game master+; o texto é parâmetro, a origem o concatenava no SQL), cadastrar (administrador; faixa de valor → produto 14134–14137 e cupom de founder já aprovado, tudo numa transação; transação repetida, valor ≤ 0 ou data zero são recusados) e validar (uma vez) |
| Zerar prans | game master+: apaga as quests 39/40/41/406/407 e os itens 100–105 (equipado, inventário e armazém da conta) de quem tem pran, numa transação. **Nenhuma pran é apagada**, apesar do nome |
| Itens que dão título | `AddTitle` agora é chamado pelo uso das caixas dos fundadores (1134–1137 → títulos 80–83) e dos itens 10766/10767/10768 (títulos 78/94/95); título repetido não gasta o item |
| **Nação do canal** | cada canal tem a nação do `SL.bin` e o item de escolha de nação (tipo 300, efeito 99) dá à conta **a nação do canal em que é usado**, grava na hora (`accounts.nation`, só se ainda for 0), avisa quem vê (`$130`) e dá o título 18. Com `-multicanal` os templos (devir) também são da nação do canal |
| Teste da troca de canal entre nações | `TestTrocaDeCanalEntreNacoesPorSocket`: dois servidores TCP reais em portas locais, canal 0 (nação 1) e canal 1 (nação 2) com templos diferentes; um cliente faz login, numérica, entra no mundo, pede o canal 1, recebe `$F05`, é desconectado, reconecta no outro canal com `$F06` e entra no mundo com o mesmo personagem; o canal 0 fica vazio e o 1 com um jogador. Passa |
| Testes | `gm_test.go` (nível, banir, buffs), `gm2_test.go` (6), `gm3_test.go` (4), `useitem_nation_test.go` (3), `channelsocket_test.go`. Mutações: privilégios, reserva do pedido, tipos de cupom, faixas de valor, validação, nação do canal, título repetido — pegas, exceto a corrida "outro administrador chegou antes" (só existe com concorrência) e `gmBuffs` (o teste só confere privilégio e que não quebra) |
| Não portado | `0x3214` (informações do servidor), `0x3219` (criar mob), `0x3221` (conta do jogador), `0x3225` (backup de conta) e `0x5000` (console remoto); o gatilho do título 27 (PvP); a cópia dos títulos 17/18 aos outros personagens; `0x313`/`0x314` (loja de NPC, sem dados); `0x32F` (abandonar quest) |
