# O que falta para concluir a troca de linguagem (Delphi → Go)

Atualizado em 2026-11-08 a partir de `go run ./cmd/opdiff -root ..` (números
medidos) e de `MIGRATION_PROGRESS.md`, `MIGRATION_AUDIT.md` e
`KNOWN_ISSUES.md`. Mesma ideia da lista do início da conversa, atualizada.

## 1. Placar

| | Início | Agora |
|---|---:|---:|
| Códigos de pacote do canal com contraparte Go | 56 / 140 | **132 / 140** |
| Pendentes | 84 | **8** |

**Atenção:** o `opdiff` conta como "coberto" qualquer código com `case` no Go.
Dez desses 132 estão **incompletos** (seção 3), então o número honesto de
códigos *totalmente* funcionais é menor.

## 2. Os 8 códigos que ainda não existem

| Sistema | Códigos | Qtde | Depende de | Observação |
|---|---|---:|---|---|
| **Ferramenta de GM** (restante) | `0x3214 0x3219 0x3221 0x3225` + `0x5000` (console remoto) | 5 | informações do servidor, criar mob, conta do jogador, backup de conta | 19 dos 27 feitos nos incrementos 39 e 40 |
| **Loja de NPC** | `0x313 0x314` | 2 | **dados originais das lojas** | **Bloqueado**: não inventar dados |
| **Quest** | `0x32F` | 1 | estado das quests do personagem; NPCs | `AbandonQuest`; o cupom (`0x3E05`) já foi portado no incremento 26 |
| | **Total** | **8** | | |

## 3. Cobertos, mas incompletos (não contam como pendência no `opdiff`)

| Código | O que falta |
|---|---|
| `0x32D` Atributo de item | **Encantamento** (tipos 1, 11 e 13: `EnchantItem` ~330 linhas, `EnchantMount`). Hoje responde "não disponível". Refino (0) e troca de aparência (2) estão prontos |
| `0xE51` Relíquia | Pronto, mas **inalcançável**: o templo nunca abre (NPC do templo, pedras e guardas não existem no Go). O reenvio a todos (`UpdateReliquaresForAll`) já está feito (incremento 15) |
| `0x23FE` / `0x23FB` Atributos detalhados | Números **parciais**: faltam efeitos de equipamento/buff e PvP |
| `0x31E` Barra de atalhos | Skill de pran (`SrcType` 3) recusada |
| `0x22D` Item no chat | Megafone recusado (precisa do item de megafone) |
| `0x3E04` Criar personagem | Skills iniciais **inferidas** da regra do código; bônus de indicação não portado; ver a divergência dos `.acc` |
| `0xB52` Info dos templos | Um canal = uma nação; pergunta por canal ignorada |
| `0xF05` Troca de canal | Líder de grupo com outros membros (e raide) não troca junto; buffs se perdem; só com `-multicanal`; nunca testada com cliente real (incremento 36) |
| `0x361` Título ativo | Pronto, mas só existem os títulos que já estão no banco: ganhar progresso e subir de nível um título não foi portado (incremento 34) |
| `0x359` Item de evento | Pronto; a ferramenta de GM que cria itens de evento (`0x3212`, `0x3299`) continua pendente (incremento 35) |

## 4. Sistemas que não são "um código de pacote" (a maior parte do trabalho)

| Sistema | Estado | Por que importa |
|---|---|---|
| **NPCs** (objetos, diálogo, `OpenNPC` completo) | só armazém e baú de guilda. **Bloqueado por dados**: posição, nome e opções de cada NPC vêm de `C:DatabaseNPCs*.npc` (`Load.pas:1478`, `NPC.pas:57`), e esses arquivos não existem no repositório nem nesta máquina | Destrava loja de NPC, templos/devires, quests, teleportes. É a dependência mais cruzada |
| **Nação + Devir aberto** (pedras, guardas, `OpenDevir`/`CloseDevir`, áreas seguras, threads) | só o estado dos templos | Faz o `0xE51` funcionar de verdade |
| **Efeitos nas contas de jogo** (`GetMobAbility`: relíquia, equipamento, buffs, títulos) | equipamento, set e **relíquias (parcial)** ligados; buffs e títulos zerados | Buffs e títulos não mudam atributos; relíquia ainda sem EXP%, drop, dano contra nação/monstro e skills (incremento 15) |
| **Buffs ao cliente** | feito (incrementos 16–18): `$16F`, `$16E`, expiração e efeitos genéricos | Faltam os efeitos por skill de cada classe (ver KNOWN_ISSUES.md) |
| **Pran e pet** (`PET.pas`, central, evolução, roupas) | pran feita (incrementos 28–33: nome, passivas, equipamento, exp, comida, invocar, evolução, itens); pet e a pran andar junto do dono não | Falta chamar a evolução (depende de NPC/quest), exp em grupo e dungeon, roupas |
| **Skills de mob e guardas** | mobs só patrulham e golpeiam | Chefes e mobs com skill |
| **Cerco de castelo** | não portado | Guilda + nação |
| **`UpdateThreads.pas`** (3.942 linhas) | parcialmente substituído por tickers do canal | Faltam os laços de nação, eventos, ranked, buffs |
| **Eventos, diário, ranked, cupom, comprovantes** | evento/diário (`0x359`) e cupom (`0x3E05`) feitos; ranked e comprovantes não | Parte da GM |
| **Quest** | `Quest.bin` carregado; sem handlers | `0x32F` e quests em NPC |
| **Duração de item** (`Expires`, `CanSealed`, `SetItemDuration`) | itens saem sem prazo | Fabricação, renovar, bolsas |
| **Cidade do personagem** (`GetCurrentCityID`, `Map.bin` lido só em 1.024 de 69.184 bytes) | zerada | Lista de amigos, teleporte |
| **Multi-canal** | feito com `-multicanal` (incremento 36); sem a opção, 1 canal | Faltam grupo/raide na troca, amigos e duelo entre canais |

## 5. Dívida técnica e de qualidade

- ~~Gravação periódica do progresso~~ — feita no incremento 14 (a cada 2 min; corrida benigna documentada em MIGRATION_PROGRESS.md).
- **`-race` não roda** nesta máquina (falta `gcc`). Há corridas conhecidas: `PartyRequester`/`GuildRequester` e handlers de inventário sem trava.
- **Nenhuma consulta SQL nova foi executada contra MySQL** em teste automatizado; só o servidor real as exercita.
- **Nenhum teste contra o cliente Aika Global.** Revisar em especial: mensagem ao cliente (`0x984`, 144 bytes), cidade zerada, `Header.Index` de pacotes de amigo, regra nova do comércio (desfaz confirmação do parceiro).
- **Divergência dos modelos `.acc`** (7.249 bytes) contra `TBasicCharacter` calculado (5.081): pode indicar erro do gerador em `TQuest`/`TSkillsList`/`TBuffsList`, e afetar o `$925`.
- **Python não instalado**: o gerador `gen/emit.py` não roda; portar para Go ou instalar Python.
- **Dados suspeitos**: ligação dos mutantes às listas de drop parece trocada (Buto Mutante usa a lista "Planta"); `ItemList.bin` tem 5.522 itens além da capacidade da tabela; `MobPos.bin` ausente; Marauder Hold com 93 pontos (limite 50).
- **Modo de teste dos mutantes** (`-teste-mutantes`, `-teste-def-max`): lembrar de rodar sem a flag para voltar ao normal.
- Sem repositório Git: único ponto de restauração é o `.zip` em `Downloads`.

## 6. Ordem sugerida (por dependência)

1. ~~Gravação periódica~~ e ~~`UpdateReliquaresForAll`~~ (feitos, incrementos 14 e 15).
2. **Buffs ao cliente + efeitos de buff/título nas contas** (relíquia e equipamento já ligados).
3. ~~Loja pessoal (3) e posição de membro (1)~~ — feitos no incremento 13.
4. **Sistema de NPCs** → destrava loja de NPC (quando houver dados), templos/devir aberto, quests, teleportes.
5. ~~Multi-canal → troca de canal~~ (feito, incremento 36; falta o grupo).
6. **Raid** (4), **Nação/aliança** (3).
7. **Pran/pet** → renomear pran (1), barra, roupas.
8. ~~Eventos/diário/cupom/títulos~~ (`0x359`, `0x361`, `0x3E05`): feitos; falta o progresso dos títulos.
9. **Ferramenta de GM** (27), em pacote próprio.
10. Cerco de castelo e o restante de `UpdateThreads`.

## 7. Atualização de 2026-11-10 (porte por rotina)

Contagem por rotina (não por código de pacote): das 53 units do `.dproj`,
cerca de 20 mil linhas de Pascal não tinham o nome da rotina no Go. Parte disso
era falso pendente (portado com outro nome: correio, `LoadNation`/`SaveNation`).

Feito nesta rodada, todos com testes (`go test ./...` verde):

| Rotina Delphi | Go | Observação |
|---|---|---|
| `TBaseMob.MobKilled` | `game/mobkilled.go` | exp por diferença de nível (cinza/azul/amarelo/laranja/roxo), bônus de relíquia (aplicado **uma vez**; a origem somava duas), divisão em grupo (`ExpAlocate`), dono do item (`ItemAlocate`, uma decisão por abate; a origem dava item a todos no modo "em ordem"), mensagem "Adquiriu N exp". **Falta**: raide (mob > 999.999 HP) e contagem de quests |
| `TBaseMob.PlayerKilled` | `game/playerkilled.go` | relíquias soltas, Efeito Duradouro, buff 6471, PvP/honra/caveira/título 27, grupo perto. **Falta**: `InClastleVerus` |
| `TBaseMob.GetDamageType3` | `combat/damagetype3.go` | é o sorteio que a origem usa em `SendDamage` e nas skills; o Go usava o `GetDamageType` (roleta), que a origem não chama. Os 5 pontos de rolagem foram trocados. `GetDamageType2` (1 uso, skill 11021) não foi portado |
| `SearchSkillsPassive` / `SetActiveSkillPassive` | `game/passives.go` | uma tabela só; aprender, resetar e entrar no jogo dão o mesmo total |
| `SendUnreadMails` (threads) | `game/charsave.go` | reenvio do `$3F1B` a cada gravação periódica |
| `Destroy/RemoveMember/ExitAlliance` | `guild/ally.go`, `game/allianceleave.go` | **sem caminho por pacote**: a origem chama de opção de NPC. Faltam as recusas de "cadastrado na guerra" (cerco) |
| `TTimeItensThread.CheckItens` | `game/itemexpiry.go` | inventário, equipamento e armazém, a cada 30 s. `SetItemDuration` ligado em `putItem`. **Falta**: itens da pran |
| `TPranFoodThread.CheckFood` | `game/pranfood.go` | a cada 200 s |
| `GMRequestServerInformation` (`$3214`) | `game/gmserverinfo.go` | jogadores e relíquias por nação, canais de `Channels` |
| `GMSendSpawnMob` (`$3219`) | `game/gmspawn.go` | mob temporário (até 50 por espécie, não reaparece). A origem usava id 9148+, que colide com a faixa de dungeon; aqui é o primeiro id livre da faixa de mobs |

Não feito (e por quê):

* **Nação/templos** (`CheckStones`, `CheckGuards`, `CheckReliques`): dependem do
  devir aberto (pedras e guardas), que depende dos NPCs.
* **Quests** (`SendQuests`, `GetQuest`, `UpdateQuest`, `0x32F`): não há estado de
  quest por personagem nem tabela no banco do Go.
* **GM restante** (`GMRequestPlayerAccount` `$3221`, backup de conta `$3225`/`$3227`):
  leem e gravam os arquivos `.acc` em `DATABASE_PATH\\ACCS\\`, formato que o Go
  substituiu pelo MySQL; portar exigiria definir o que o backup significa no
  banco. O console remoto `0x5000` fica pendente. Precisa de decisões de segurança (backup e
  consulta de conta).
* **Cerco de castelo** (`UpdateSiegeStatus` 669 linhas e `EnterInCastle`):
  depende de guilda e nação completas e do estado de cerco, que não existe.
* **`InitNPCS`** e loja de NPC: continuam bloqueados por dados.
