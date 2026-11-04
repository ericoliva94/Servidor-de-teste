# Porte do servidor Aika para Go

Porte de `Server/Src` (Object Pascal, Delphi 12) para Go. O cliente continua
sendo o **Aika Global** original, binário fechado da Hanbitsoft: o Go substitui
o servidor falando o mesmo protocolo, byte a byte.

Alvo medido na origem: **53 units compiladas, 85.116 linhas** de Object Pascal
(contadas a partir de `Server/AikaServer.dproj`).

## Como rodar

```bash
cd GoServer && go test ./...                      # a bateria inteira
go run ./cmd/aikaserver -ini ../Server/Bin/AikaServer.ini   # sobe o login
go run ./cmd/aikaserver -no-db                    # sobe sem banco, so para ver a carga
```

O servidor abre as portas **8831** (login) e **8822** (canal), carrega as 16
tabelas `.bin` de `Server/Bin/Data` e atende:

- `$81` no login: valida o token contra `accounts` e responde `$82`.
- `$685` no canal: valida conta e token, lê os 3 personagens de `characters`
  e responde a lista em `$901`.
- HTTP na **8090**: `/member/aika_get_token.asp` gera o token e grava em
  `accounts`, mais as rotas de contagem de personagens, lista de servidores e
  reset de flag. É o que o launcher chama antes de conectar.

### Console de administração (porta 5555)

Fica **desligado** sem token. Para abrir:

```bash
AIKA_CONSOLE_TOKEN=um-segredo-longo go run ./cmd/aikaserver -ini ../Server/Bin/AikaServer.ini
```

O token vem do ambiente, não do `.ini`, para não ser copiado junto com o
arquivo de configuração. A porta escuta em `127.0.0.1:5555`; mudar isso é no
`ConsoleAddr` do config. Uma linha por comando, e a primeira linha tem de ser o
`AUTH`:

```bash
printf 'AUTH um-segredo-longo\n/who\n/addgold Alfa 1000\nquit\n' | nc 127.0.0.1 5555
```

Na origem essa porta aceitava qualquer conexão **sem autenticação nenhuma** e
executava o que chegasse com poder de Admin; a seção do console, no fim deste
arquivo, detalha isso.

## Regras do porte

Valem para todo arquivo novo. Existem porque o original falhava justamente
nesses pontos — as 36 correções do commit anterior em `Server/Src` são a lista
de evidências.

1. **Nada de panic por entrada da rede.** Toda função que recebe buffer valida
   tamanho antes de indexar e devolve `error`. Cada pacote tem um teste que
   joga milhares de entradas aleatórias e falha se houver panic.
2. **Índice vindo do cliente é validado na borda.** No Delphi, `Inventory[Packet.Slot]`
   com `Slot` WORD indexava fora de um array `[0..63]`. Em Go isso é panic, não
   corrupção silenciosa — então o limite é checado antes, não depois.
3. **O teste fixa o comportamento, inclusive o esquisito.** Onde o original tem
   uma peculiaridade que o cliente depende (o checksum cego a corrupção do
   corpo, a cauda não cifrada em pacotes de tamanho não múltiplo de 4), existe
   um teste nomeado que trava aquilo, para ninguém "consertar" depois e quebrar
   a compatibilidade.
4. **Layout binário travado por teste.** Toda struct que viaja na rede ou vai
   para o banco tem um teste que compara `binary.Size` com o `sizeof()` do
   Delphi.
5. **Divergência é documentada no código.** Onde o porte não é fiel, o comentário
   diz o que mudou e por quê (ver "Divergências" abaixo).

## Estado

| Fase | Conteúdo | Origem | Estado |
|---|---|---|---|
| 1 | Módulo, tipos base (`Item`, `Position`, `ItemEffect`) | `MiscData.pas` | **pronto** |
| 2 | Criptografia e enquadramento TCP | `EncDec.pas`, `PlayerThread.pas` | **pronto** |
| 3 | Structs binárias: 319 records | `Packets.pas`, `PlayerData.pas`, `MiscData.pas`, `FilesData.pas`, `GuildData.pas`, `PartyData.pas`, `Entity*.pas` | **pronto** |
| 4 | Leitura das tabelas `.bin` | `Load.pas` (2.719) | **pronto** (16 tabelas; CSV de drop/quest pendente) |
| 5 | Acesso a MySQL | `SQL.pas` | **pronto** |
| 6a | Núcleo de entidade: buffs, cooldown, visíveis, alvos | `BaseMob.pas` | **pronto** |
| 6b | Combate: tipo de dano e pipeline completo no alvo | `BaseMob.pas`, `Util.pas` | **pronto** (`AttackParseForMobs`) |
| 6c | Jogador, NPC, pet | `Player.pas` (12.859), `NPC.pas`, `PET.pas` | a fazer |
| 7 | Handlers | `PacketHandlers.pas` (13.686), `NPCHandlers.pas`, `AuthHandlers.pas`, `CommandHandlers.pas` | a fazer |
| 8 | Sistemas | itens/refino, dungeon, guild, party, mail, auction, nation, eventos, ranked | a fazer |
| 9 | Laços de atualização | `UpdateThreads.pas` (3.942) | a fazer |
| 10a | Servidor TCP, login e `main` que sobe | `LoginSocket.pas`, `AuthHandlers.pas` | **pronto** |
| 10b | Canal 8822: `CheckLogin` e lista de personagens | `ServerSocket.pas`, `PacketHandlers.pas`, `Player.pas` | **pronto** |
| 10c | Token HTTP 8090 | `TokenSocket.pas`, `AuthHandlers.pas` | **pronto** |
| 10d | Entrada no mundo: numérica (`$F02`) e `SendToWorld` (`$925`) | `PacketHandlers.pas`, `Player.pas` | **pronto** |
| 10e | Registro do canal, movimento (`$301`), spawn (`$349`) e remoção (`$101`) | `ServerSocket.pas`, `BaseMob.pas`, `PacketHandlers.pas` | **pronto** |
| 10f | Inventário e equipamento do banco, bônus de refino no HP/MP | `Player.pas`, `ItemFunctions.pas` | **pronto** |
| 10g | Conversa (`$F86`), rotação (`$305`), info de personagem (`$22C`/`$19E`), hora (`$202`) | `PacketHandlers.pas`, `Player.pas` | **pronto** |
| 10h | Inventário em jogo: mover (`$70F`), descartar (`$32C`), empilhar, gravar na saída | `PacketHandlers.pas`, `ItemFunctions.pas`, `Player.pas` | **pronto** (inventário↔equipamento; armazém e baú na 10t; pran a fazer) |
| 10i | Atributos derivados (`GetCurrentScore`), ataque básico (`$302`), morte e revive (`$303`) | `BaseMob.pas`, `PacketHandlers.pas` | **pronto** (sem skill, dungeon, pran nem mascote) |
| 10j | Mobs: carga dos dois CSV, spawn (`$35E`), visibilidade, golpe, morte, experiência (`$108`) e respawn | `ServerSocket.pas`, `MOB.pas`, `Player.pas` | **pronto** (sem IA de movimento nem ataque do mob) |
| 10k | IA de mob: patrulha, perseguição e golpe no jogador | `MOB.pas` (`MobHandler`, `MobMoviment`, `AttackPlayer`) | **pronto** (sem skill de mob nem guarda) |
| 10l | Grupo: convidar (`$322`), aceitar (`$323`), sair/expulsar (`$324`), desfazer (`$325`), liderança (`$34B`), pedir entrada (`$38F`), estado (`$326`), posição (`$11D`) e conversa de grupo | `PartyData.pas`, `PacketHandlers.pas`, `Player.pas` | **pronto** (sem raid) |
| 10m | Skills: aprender (`$31C`), lançar (`$320`), zerar (`$32A`), cancelar (`$327`), listas (`$106`/`$107`), recarga, custo de MP e buff | `SkillFunctions.pas`, `PacketHandlers.pas`, `BaseMob.pas` | **pronto** (motor genérico; os casos especiais por skill ficam para a fase seguinte) |
| 10n | Drop: carga das 13 listas, sorteio, item no chão (`$33B`), coleta (`$336`) e expiração | `Load.pas`, `BaseMob.pas` (`DropItemFor`), `ServerSocket.pas` (`CreateMapObject`) | **pronto** |
| 10o | Guilda: criar (`$341`), convite (`$F7B`/`$67D`/`$F7E`), expulsar (`$F2D`), sair (`$F1C`), rank (`$F1D`), mestre (`$619`), avisos (`$F20`), site (`$F21`), permissões (`$F22`), info (`$965`), membros (`$97F`) e conversa de guilda | `GuildData.pas`, `PacketHandlers.pas`, `Player.pas` | **pronto** (baú na 10t; sem aliança, skill de guilda nem cerco) |
| 10p | Correio: conferir (`$3F16`), enviar (`$3F15`), lista (`$3F17`), abrir/apagar (`$3F18`), retirar anexo (`$3F1A`), não lidas (`$3F1B`) | `MailFunctions.pas`, `CharacterMail.pas` | **pronto** (caixa no MySQL; ver abaixo) |
| 10q | Leilão: registrar (`$3F0B`), comprar (`$3F0C`), lista (`$3F0D`), cancelar (`$3F10`), minhas ofertas (`$3F11`) e vencimento | `AuctionFunctions.pas`, `PacketHandlers.pas` | **pronto** (ofertas no MySQL; entrega pelo correio) |
| 10r | Dungeon: pedir entrada (`$334`), saguão (`$145`), confirmar (`$355`), instância por grupo, IA do mob, exp, drop e saída | `Dungeon.pas`, `Player.pas` (`CreateDungeonInstance`), `ServerSocket.pas` (`InitDungeons`) | **pronto** (sem raid; ver abaixo) |
| 10s | Console 5555: socket de comando com autenticação, e os comandos de GM pelo sussurro | `ConsoleSocket.pas`, `CommandHandlers.pas` | **pronto** (ver abaixo) |
| 10t | Armazém e baú de guilda: opção de NPC (`$30F`/`$348`), armazém (`$137`/`$310`), baú (`$D58`/`$F10`/`$D18`/`$F2F`), ouro (`$F59`) e arrasto de item nos dois | `PacketHandlers.pas` (`OpenNPC`, `ChangeGold`, `MoveItem`), `GuildData.pas` (`OpenChest`), `Player.pas` (`SendStorage`) | **pronto** (ver abaixo) |
| 10u | Nação, eventos, ranked, pet, pran, cerco, NPC/loja, central de pran | `Nation/`, `Events/`, `Rankeds/`, `PET.pas`, `NPC.pas` | a fazer |

As fases 1 e 2 vêm primeiro porque são o piso: sem enquadramento e cifra
corretos, nada mais pode ser testado contra o cliente real.

## O gerador (`gen/`)

As 319 structs binárias não foram digitadas à mão: `gen/emit.py` lê as units
Delphi, resolve os tipos entre elas e emite o Go. Digitar 2.145 linhas de
`packed record` à mão erraria algum offset, e um offset errado num pacote é
desincronização com o cliente.

```bash
cd GoServer/gen && python3 emit.py   # regenera internal/*/types_gen.go
```

O gerador calcula o `sizeof` do Delphi por conta própria, inclusive o
alinhamento dos `record` sem `packed` (nesses ele emite campos `_ [n]byte`
para o layout empacotado do Go casar com o da origem). A conta foi validada
contra a única âncora independente que existe no código: `TQuestFromList`, que
o `FilesData.pas` documenta como 2332 bytes e o `Load.pas` lê em blocos de
`$91C` — o gerador chega aos mesmos 2332 sozinho.

`gen/NAO_PORTADO.md` lista o que ele deliberadamente não gera: 5 records que
contêm `TList`, classe ou string de tamanho variável. Esses não têm layout
binário fixo nem no Delphi, então viram tipo nativo de Go (slice, ponteiro,
`string`) nas fases seguintes.

Dois achados do gerador que valem registro, porque não são óbvios:

- **Padding só pode usar o identificador branco.** Um campo de alinhamento
  chamado `_pad1` é não exportado e `binary.Read` tenta escrevê-lo por
  reflexão, o que entra em pânico. Só o nome `_` é pulado. `binary.Size` não
  detecta isso, por isso existe o teste de ida e volta.
- **`Boolean` do Delphi virou `uint8`, não `bool`.** `binary.Read` aceita
  qualquer byte num `bool` do Go, mas `binary.Write` devolve `0x01`: um pacote
  que o servidor ecoa de volta sairia alterado. `uint8` preserva o byte.

## O que os dados reais revelaram

A fase 4 lê os `.bin` de `Server/Bin/Data`, e os testes rodam contra esses
arquivos de verdade. Três coisas apareceram, todas **pré-existentes** no
servidor Delphi — o porte as reproduz, mas agora elas saem no log de carga em
vez de passar caladas:

| Arquivo | O que acontece |
|---|---|
| `ItemList.bin` | tem 26.000 itens, mas `TItemList` é `ARRAY [0..20477]`: **5.522 itens do arquivo nunca são lidos** |
| `SkillData.bin` | tem 12.000 skills, `TSkillData` é `ARRAY [0..11998]`: 1 skill fora |
| `Map.bin` | tem 69.184 bytes (4.324 limites), `TFileMapsData` guarda 64: **só 1.024 bytes são lidos** |

Nos três casos o Delphi lê com `Read(f, Tabela)` de arquivo tipado, que
consome exatamente `sizeof(Tabela)` e ignora o resto — o porte faz igual e
reporta a sobra em `Table.Truncated`.

Também confirmado contra dado real: `ItemList.bin` **não tem cabeçalho**. Os
4 bytes de sobra estão no fim do arquivo, não no início — os nomes dos itens
caem exatamente nos offsets 0 e 64 de cada registro de 464 bytes, e o item 10
é `WarriorFace1` no byte 4640 = 10 × 464. Isso está travado por teste.

## Código morto encontrado na origem

Não portado por não fazer nada no servidor atual. Fica registrado aqui para
que a ausência seja uma decisão, e não um esquecimento.

**`Util.IntMV`** (`Util.pas:11` e `84`) — declarada na interface, com corpo
vazio (só um comentário `//if()`) e sem atribuir `Result`, então devolve lixo.
Nunca é chamada.

**`TPrediction`** (`BaseMob.pas:8-18`, implementação em `12503-12540`) — a
interpolação de posição de mob. `SetDestination` escreve nela, mas o único
leitor, `TBaseMob.CurrentPosition`, está **inteiramente dentro de um bloco
comentado** (`1506-1528`), e `SetDestination` em si não é chamado de lugar
nenhum. Dois bugs ficaram dormentes aí dentro:

- `Elapsed` devolve `Timer.ElapsedTicks` (ticks de alta resolução) mas `ETA` é
  calculado em milissegundos, então `Delta = Elapsed/ETA` estoura e
  `Interpolate` sempre devolve o destino direto.
- `CalcETA` faz `speed := speed * 190` com `speed: Byte`: para qualquer
  velocidade acima de 1 o valor trunca em 256. Com `speed = 25` (o usado em
  `MobMove`), `25*190 = 4750` vira 142.

Se a interpolação for reativada algum dia, os dois precisam de decisão de
projeto — não de tradução.

## Aritmética com limite: a pegadinha do `Dec`

`Util.pas` declara `Dec` e `Inc` sobrepondo os intrínsecos do Delphi para
`Word`, `Byte` e `Int64`. O efeito é que `Dec(X, N)` no código original
**limita em zero** — mas só para esses tipos. Para `Cardinal` não existe
sobrecarga, o intrínseco entra e o estouro é silencioso: `100 - 500` num
`Cardinal` vira 4.294.966.896.

Traduzir isso errado troca "fica em zero" por "vira 4 bilhões", que num campo
de ouro ou cash é exploit. `internal/num` tem uma função por semântica, com o
nome dizendo qual é qual — inclusive as duas que a origem chama igual:

| `Util.pas` | `internal/num` | Comportamento |
|---|---|---|
| `DecInt` | `num.DecInt` | limita em zero |
| `Dec(var x: integer)` | `num.DecIntRaw` | **não** limita, fica negativo |
| `Dec(var x: Word)`, `DecWORD` | `num.DecWord` | limita em `[0, 65535]` |
| `Dec(var x: Byte)` | `num.DecByte` | limita em `[0, 255]` |
| `Dec(X, N)` com `Cardinal` | `num.DecCardinal` | limita em zero (a origem estoura) |

## Peculiaridades mantidas de propósito

Comportamento estranho da origem que o cliente ou o balanceamento já dependem.
Cada um tem teste nomeado, para que uma "correção" futura falhe o teste em vez
de mudar o jogo em silêncio.

**Dano acima de 999999 vira 1, não o teto.** É a guarda de `AttackParse`
contra valor absurdo (`if(Dano > 999999) then Dano := 1`).

**Absorção que não cobre o golpe não consome o escudo.** No ramo em que o
escudo é menor que o dano, o Delphi desconta o escudo do dano mas não chama
`DecreasseMobAbility`. Com três buffs de absorção ativos, o mesmo valor é
descontado três vezes do golpe (300 − 30 − 30 − 30 = 210).

**A posição 0 da roleta de dano nunca sai.** `RandomRange(1, total+1)` sorteia
em `[1, total]`, embora a posição 0 seja pintada pela primeira chance.

**O token era derivado da senha e do segundo corrente.**
`MD5(MD5(senha) + MD5(DateTimeToStr(Now)))` tem resolução de segundo, então
dois pedidos no mesmo segundo com a mesma senha davam o **mesmo token**, e o
valor era previsível para quem soubesse o hash. O cliente trata o token como
texto opaco de 32 caracteres e só o devolve, então o porte usa bytes
aleatórios: transparente na rede e estritamente mais seguro.

**Nível do personagem vai como `Level - 1` na lista.** `SendCharList` manda
`Account.Characters[i].Base.Level - 1`. Num slot vazio (`Level = 0`) isso
estoura o `WORD` e vira 65535; o porte usa piso zero, que é a única diferença.

**A amplificação usa divisão inteira.** `(Dano div 100) * pct`, então 25% de
1250 dá 120 e não 125.

**A conversão de HP não custa mana.** O custo é
`Help1 * (GetMobAbility(EF_MP_EFFICIENCY) div 100)`. O valor desse efeito é um
número pequeno — o trecho comentado em `BaseMob.pas:9815` o usa como
multiplicador de `Dano div 200` — então a divisão inteira por 100 dá zero.
Quase certamente era `Help1 * eff div 100`, mas como a escala pretendida é
ambígua o porte mantém a conta da origem: mudar custo de recurso alteraria o
balanceamento sem ter como validar contra o cliente.

## Divergências do original

Cada uma está comentada no ponto do código.

**`protocol.Encrypt`** — `EncDec.pas` tem duas sobrecargas de `Encrypt`. A que
recebe array grava a chave no byte 2 e o checksum no byte 3; a que recebe
ponteiro grava ao contrário, e é essa que `Decrypt` espera. As duas são
incompatíveis entre si. Os dois chamadores reais (`Player.pas:4601`,
`LoginSocket.pas:265`) passam ponteiro, então a sobrecarga de array é código
morto e não foi portada.

**`protocol.Decrypt`** — no Delphi devolvia `False` quando o pacote era
*válido*. Nenhum chamador lia o retorno. Em Go o retorno tem o sentido óbvio
(`true` = checksum fecha); o conteúdo decifrado é idêntico.

**`protocol.Framer.isHello`** — `TPlayerThread.IsHello` tinha dois palpites que
erram quando o primeiro segmento TCP chega curto: tratava *qualquer* 4 bytes
isolados como hello, e comparava o tamanho do offset 4 contra o que já havia
chegado em vez do pacote inteiro. Nos dois casos o servidor descartava 4 bytes
de um pacote legítimo e a conexão dessincronizava. Em Go vale só a assinatura
documentada no próprio comentário do Delphi (`?? ?? 11 1F`) mais a variante
`11 F3`.

**`protocol.Encrypt`, validação de tamanho** — o original gravava o uptime em
`BufferDest[8]` sem checar se o pacote tinha 12 bytes, estourando o bloco
alocado em pacotes curtos. Agora pacote menor que o cabeçalho é erro.

**`model.Item.Expire`** — `TItem.GetExpire` copiava 3 bytes para dentro de um
`UnixDate` que nunca era inicializado, então o byte baixo era lixo de pilha e a
data variava até 255 s entre duas leituras do mesmo item (o próprio arquivo
marca o trecho com *"erro aqui por conta do x64"*). Em Go o byte baixo é zero,
o que deixa a leitura estável e no mesmo passo de 256 s em que a escrita grava.

**`model.Item.EquipSellPrice`** — `GetEquipSellPrice` fazia `MIN / MAX` sem
checar `MAX`: item com durabilidade máxima zero levantava divisão por zero.
Agora vale preço zero.

**`game.Channel`** — substitui `Servers[n].Players: ARRAY [1..1000] OF TPlayer`.
No original o índice do vetor *era* o `ClientID`, e cada acesso precisava
acertar a faixa sozinho; o teste que aparece em `MOB.pas`, `PET.pas`,
`NPC.pas`, `BaseMob.pas` e `Dungeon.pas` é sempre o mesmo
`if (i <= MAX_CONNECTIONS)`, **sem limite de baixo**, então `i = 0` lia e
escrevia antes do início do vetor. Aqui o acesso é por mapa, os ids vão de 1 a
`MAX_USERS` e id inválido simplesmente não encontra ninguém. A busca por nome
(`GetPlayerByUsername`, que varria as 1000 posições a cada chamada) virou
índice.

**`netsrv.DisconnectHandler`** — a limpeza do jogador vivia em
`Connection.Destroy`, chamado de dezenas de pontos espalhados. Quando um
caminho de saída não chamava, o jogador ficava no vetor com `Status` igual a
`Playing` e continuava na lista de visíveis dos outros, que viam um boneco
parado para sempre. Agora a saída é um ponto só, no fim da goroutine da
conexão, e roda até se o tratamento tiver entrado em pânico.

**`game.movement`, cabeçalho** — `MovementCommand` repassava o pacote do jeito
que chegou (`SendToVisible(Packet, Packet.Header.Size, False)`), e
`Header.Index` é escrito pelo cliente: bastava mandar o id de outro jogador
para fazer o boneco dele andar na tela de todos os visíveis. Agora o
cabeçalho repassado leva o id do autor.

**`game.ValidPosition` e o salto absurdo** — em `MovementCommand` a checagem de
`IsValid` e **toda** a verificação de velocidade estão dentro de um bloco
comentado: nunca rodaram. NaN e infinito iam direto para `LastPos` e dali para
todas as contas de distância. Agora coordenada não finita, negativa ou fora do
mapa é recusada, e salto maior que `DISTANCE_TO_FORGET` num pacote devolve o
jogador à última posição válida. Não há desconexão: o limite não foi calibrado
contra lag real, e derrubar quem tem conexão ruim seria pior que o problema.

**Histerese de visibilidade** — `GlobalDefs.pas` declara `DISTANCE_TO_WATCH`
(50) e `DISTANCE_TO_FORGET` (60), mas o trecho que usa a segunda está
comentado em `BaseMob.pas:1056`, então o original usava um limite só nos dois
sentidos. Com um limite só, quem andava em cima da borda entrava e saía da
lista a cada passo e levava um spawn e um remove por passo. Aqui entra a 50 e
só sai acima de 60, que é para o que a segunda constante existe.

**`stats.MaxHP` / `stats.MaxMP`** — porte de `GetCurrentHP`/`GetCurrentMP`, que
apesar do nome devolvem o **máximo**. Lá a conta era em `DWORD` e cada `Inc`
somava valores que podiam ser negativos (`GetMobAbility` devolve `Integer`),
dando a volta para perto de 4 bilhões; o piso `if (hp_inc <= 0)` nunca pegava
isso porque num `DWORD` só o zero satisfaz `<= 0`. Em Go a conta é em `int`
com sinal e o piso de 1 vale. `GetCurrentMP` também usava
`ReliqEffect[EF_RELIQUE_PER_HP]` — a constante de HP — no cálculo do MP; aqui o
percentual chega somado de fora e quem o monta escolhe o efeito certo.

**`game.buildCreateMob`, campo de guilda** — `GetCreateMob` montava
`GuildIndexAndNation` concatenando texto hexadecimal
(`StrToInt('$' + IntToStr(Nation) + IntToHex(GuildIndex, 3))`). Com índice de
guilda acima de `$FFF` o `IntToHex` devolvia 4 dígitos e o número montado
passava de 16 bits. Agora a nação fica nos bits altos e o índice é mascarado
nos 12 baixos.

**`game.buildCreateMob`, limite de buffs** — o laço
`for Index in Self._buffs.Keys do ... Packet.Buffs[i]` incrementava `i` sem
teto, e a partir do 60º buff escrevia por cima do resto do pacote. Agora para
no tamanho do vetor.

**`game.buildCreateMob`, MaxMP** — `Packet.MaxMP := Character^.CurrentScore.MaxHP`
no ramo de mob/NPC: erro de cópia que fazia mob reportar MP máximo igual ao HP.
Não repetido.

**Nível de título e nível de personagem** — `Packet.Titlelevel := ActiveTitle.Level - 1`
num `BYTE` e `dst.Level := Level - 1` num `WORD` davam 255 e 65535 para o valor
zero (sem título, slot vazio). Os dois têm piso em zero agora.

**`store.Characters.ByAccount`, posição de entrada** — o original usava
`posx`/`posy` do banco direto. Personagem criado por fora, ou com a coluna
nula, entrava em `0,0`: canto do mapa, fora de qualquer cidade. Agora posição
gravada só vale se for utilizável, e o resto cai no ponto de partida. Mesma
ideia para `speedmove` zerado, que deixava o boneco preso no lugar, e para
`curhp` zerado, que fazia o personagem entrar morto e travado.

**`store.Items`, slot do item** — o slot vinha do banco e indexava
`Equip[ItemSlot]` e `Inventory[ItemSlot]` **sem nenhuma checagem de faixa**.
Um slot errado escrevia no meio do registro do personagem, que é o que vai no
`$925` e é gravado de volta. Agora slot fora da faixa é ignorado e reportado, e
duas linhas no mesmo slot mantêm a primeira em vez de uma sumir em silêncio.

**`Player.pas:1056`, equipamento no personagem errado** — o carregamento de
equipamento usava o índice do laço como slot do personagem (`Slot := i` dentro
de `for i := 0 to CharCount-1`), não o slot que veio do banco. Uma conta com
personagens nos slots 0 e 2 lia os itens de `Characters[1]`, que está vazio, e
o personagem do slot 2 entrava sem equipamento. No Go cada personagem é
carregado pelo próprio id.

**`itemfn.ReinforceHPMPInc`** — `GetItemReinforceHPMPInc` indexava
`HealthIncrementPoints[Refine]`, um `ARRAY [0..15]`, com `Refine` vindo de
`Round(Refi / 16)`, que chega a 4095 num `Refi` adulterado: leitura muito além
do fim do vetor, dentro de outros registros da tabela. Agora nível fora de
0..15 vale zero.

**`itemfn.Reinforce3Index`** — com `UseEffect` acima de 105 a faixa ficava em
zero e o resultado era o próprio `UseEffect`, que depois indexava `Reinforce3`
sem checagem. Agora não há índice para esse caso e o bônus é zero.

**`itemfn.ReinforceFromItem`, mantido de propósito** — `Round(item.Refi / 16)`
é divisão em ponto flutuante com arredondamento bancário, **não** `div 16`. Com
`Refi` 8 o refino é 0 e com 24 é 2. `Packet.ItemEff[7]`, no mesmo arquivo, usa
`div 16`. Os dois valores diferem para metade dos refinos e os dois foram
portados como estão, cada um no seu lugar.

**`game.clientSay`, autor da fala** — `SendClientSay` repassava o pacote do
jeito que chegou, e tanto `Header.Index` quanto `Nick` são escritos pelo
cliente: dava para falar no nome de outra pessoa. Agora os dois vão com os
dados do autor. O campo `Fala` é `ARRAY [0..127] OF AnsiChar` e era copiado sem
limite: texto sem terminador fazia a leitura seguir pelos campos vizinhos do
pacote; agora é lido com limite e reescrito com terminador.

**`game.shout`, momento de marcar o relógio** — `ShoutTime` só era gravado
**depois** do laço que envia o grito a todos. Uma exceção no meio do laço
deixava o jogador sem limite nenhum. Agora o instante é marcado antes do envio.

**`game.requestCharInfo`** — a guarda era
`if (Packet.Index >= 0) or (Packet.Index <= MAX_CONNECTIONS)`. Com `Index`
sendo `DWORD`, o primeiro teste é sempre verdadeiro e o `or` deixa a condição
sempre verdadeira; o ramo seguinte fazia
`@Servers[ChannelIndex].Players[Packet.Index]` com um valor de até 4 bilhões
num vetor de 1000 posições. Agora o índice é validado contra o limite real do
canal.

**`game.Channel.FindByCharacter`** — `GetPlayer` e `GetPlayerByName` varriam as
1000 posições do vetor a cada chamada, e eram chamados de dentro de laços.
Agora é um índice por nome, mantido na entrada e na saída do jogador.

**Canais de conversa ainda não portados** — grupo, guilda, aliança, nação e
megafone respondem uma mensagem dizendo que não estão disponíveis. Na origem o
jogador digitava e nada acontecia, sem nenhum retorno.

**`game.moveItem`, ponteiro nulo** — `MoveItem` iniciava `srcItem` e `destItem`
em `nil` e só os preenchia dentro dos ramos do `case`. Um `SrcType` ou
`DestType` fora da lista deixava o ponteiro nulo e, poucas linhas abaixo,
`if((ItemList[destItem.Index].ItemType = 8) and (ItemList[srcItem.Index].ItemType = 8))`
o dereferenciava: **qualquer cliente derrubava a thread do próprio jogador
mandando um tipo de slot qualquer**. No Go os dois lados são resolvidos antes
de qualquer uso e tipo não suportado é recusado.

Arrastar um slot sobre ele mesmo também seguia adiante na origem: com pilha de
itens, `AgroupItem(src, dest)` com `src == dest` dobrava a quantidade. Agora é
recusado.

**`itemfn.DecreaseAmount`** — o teste era `(item.Refi - Quanti) > 0` com os dois
`WORD`: a subtração dava a volta, e tirar 2 unidades de uma pilha de 1 deixava
**65535**. Agora a conta é com sinal.

**`itemfn.AddAmount`** — `SetItemAmount(..., Somar=True)` fazia
`Inc(item.Refi, quant)` num `WORD` sem teto; somar acima de 65535 dava a volta e
a pilha virava um punhado. Agora há teto em `MAX_SLOT_AMOUNT`.

**`store.Items.SaveSet`** — a origem montava o `INSERT` concatenando texto,
inclusive `Identific`, que é `Integer` com sinal e vem de dados do jogador.
Agora os valores vão como parâmetros. A estratégia `DELETE` + `INSERT` dentro de
uma transação foi mantida: é a que casa com o formato da tabela, que não tem
chave única por slot.

**Gravação na saída** — `SaveInGame` era chamado de vários pontos e a gravação
de desconexão dependia de o caminho de saída chegar lá; quando não chegava,
tudo que o jogador tinha movido na sessão era perdido. Agora a gravação fica no
mesmo ponto único de saída que tira o jogador do canal, com prazo próprio para
não segurar a goroutine da conexão.

**`game.sendStatus`** — `SendStatus` terminava chamando `RequestAllAttributes`
com um buffer local `Array [0..12] of Byte` zerado só nos **12** primeiros,
passado a um handler que o lê como pacote: o 13º byte ia com lixo de pilha.

**`game.sendCurrentHPMP`** — `SendCurrentHPMP` terminava com `Sleep(1)`, uma
pausa dentro do tratamento de pacote que segurava a thread do jogador.

**Mantido de propósito** — `ReinforceFromItem` usa `Round` e o `ItemEff[7]` usa
`div 16` (ver acima); a pilha junta fica no slot de **origem**, não no de
destino; com um dos lados já cheio os dois itens trocam de lugar em vez de
somar; os slots 0 e 1 do equipamento não saem (`srcSlot > 1`); e o slot do
inventário só funciona se a bolsa correspondente (60 a 63) estiver lá.

**`game.attack`, alcance** — o ataque básico entre jogadores **não checava
distância nenhuma**: bastava o `ClientID` do alvo para acertá-lo de qualquer
ponto do mapa. Agora o alvo precisa estar dentro de `AttackRange`.

**`game.attack`, índice de skill** — `DataSkill := @SkillData[Packet.Skill]` com
`Skill` vindo do cliente como `WORD` num `ARRAY [0..11998]`: lia a tabela fora
do fim. Nesta fase skill é recusada de todo, e o índice nunca é usado.

**`game.attack`, o aviso de velocidade** — a origem mandava "Outros programas
detectados." e, se a mensagem repetisse dentro de 5 s, marcava `SocketClosed`,
derrubando o jogador. O golpe rápido demais agora é só descartado: o limite não
foi calibrado contra lag real.

**`game.wearWeapon`** — `Dec(Equip[6].MIN, 1)` num `BYTE` sem piso: arma com
durabilidade zero dava a volta para **255** e voltava a funcionar sozinha. Agora
o piso é zero.

**`game.revive`** — `RevivePlayer` não checava se o jogador estava morto:
bastava mandar o `$303` para encher a vida de graça.

**`game.rand`** — `Randomize` era chamado dentro de `GetDamage` **e** de
`SendDamage`, isto é, o gerador global era reiniciado a cada golpe, com a hora
como semente: dois golpes no mesmo instante saíam com a mesma sequência. Agora
o gerador da biblioteca é usado como está.

**`combat.Roller` na sessão** — a roleta de tipo de dano é um vetor de 1000
posições reusado a cada golpe. Fica na sessão, não numa global, para dois
jogadores atacando ao mesmo tempo não sortearem no mesmo vetor.

**`game.sessionTarget.Notify`** — as mensagens de absorção saíam por
`Servers[ChannelId].Players[mob.ClientID].SendClientMessage`, com `Players`
sendo `ARRAY [1..1000]` e `mob.ClientID` podendo ser id de mob (acima de 1000):
índice fora do vetor, e isso aparece **oito vezes** só em
`AttackParseForMobs`. Agora quem entrega a mensagem é a casca que sabe que o
alvo é jogador.

**`stats.Compute`** — porte de `GetCurrentScore`. As constantes multiplicativas
(2.6 por força e por agilidade no dano físico, 3.2 por inteligência no mágico, e
as demais) são as da origem, **inclusive as que lá estão comentadas e não
somam** — essas ficaram de fora aqui também, para o resultado ser o mesmo. Os
limites de cada campo são os das funções de `Util.pas` (`IncWord` satura em
65535, `IncByte` e `IncCritical` em 255, `IncCooldown` em 70, `IncSpeedMove` em
[15, 70]). `Resistence` é o único campo que usa `Round` em vez de `Trunc`, e
isso foi mantido.

**`Avatar.RecalcDerived`, velocidade** — `GetCurrentScore` zerava
`PlayerCharacter.SpeedMove` e recalculava do zero, sobrescrevendo o valor da
coluna `speedmove`: montaria e buff de velocidade sumiam a cada recálculo.
Agora o valor gravado só cede quando é zero.

**`mobdata`, vetores no limite exato** — `MobsP: Array [0..49]` e
`TMobS: Array [0..449]`, indexados **sem nenhuma checagem**. Os dados de hoje
chegam exatamente no limite: 372 espécies e 49 pontos de spawn na maior delas.
Bastava um ponto a mais em qualquer espécie, ou a 451ª espécie no arquivo, para
corromper memória em silêncio. No Go o excedente é recusado e reportado.

**`mobdata`, linha curta e campo inválido** — a origem chamava
`FileStrings[17]` e `.ToInteger()` sem checar: uma linha curta ou um campo com
lixo levantava exceção no meio da carga, e o servidor subia **sem o resto dos
mobs**, sem dizer nada. Agora a linha é reportada e a carga segue.

**`mobdata`, espécie sem atributos** — o piso de 3500 de HP estava dentro do
ramo que casou o nome entre os dois CSV. Espécie presente no `MonsterListCSV`
e ausente do `AllMobsInfo` ficava com HP zero: nascia morta. Agora recebe o
piso e um aviso. (Nos dados reais os dois arquivos batem: 372 espécies, 5865
pontos, zero avisos.)

**`game.MobWorld.Hit`** — `Dec(Self.HP, Dano)` sem trava nenhuma, com uma
thread por jogador: dois golpes simultâneos podiam perder um dos descontos ou
**matar o mob duas vezes**, e a segunda morte dava experiência de novo. Agora o
desconto é sob trava e a morte acontece uma vez só.

**`game.MobWorld.Respawn`, virada de ano** — o teste da origem era
`if (Self.DeadTime.Year >= Now.Year)`. Um mob morto em 31 de dezembro **nunca
voltava** depois da meia-noite. Agora a conta é sobre a diferença de tempo.

**`game.RespawnTick`** — o respawn acontecia dentro de
`UpdateSpawnToPlayers`, isto é, o mob só voltava quando algum jogador passava
perto o bastante para a rotina rodar com ele; sem ninguém por perto o mob
ficava morto indefinidamente. Agora o laço é do servidor.

**`game.buildSpawnMob`** — em `SpawnMob`, `Packet.CurHP := InitHP` (o máximo da
espécie) e `Packet.MaxHp := MobsP[].HP` (a vida atual) estavam **trocados**:
mob machucado aparecia com a barra cheia. E `Packet.CurMp := InitHP` pegava o
MP do HP, o mesmo erro de cópia de `GetCreateMob`.

**`game.sendMobSpawn`** — `SpawnMob` terminava com `Self.SendPacket` chamado
**duas vezes seguidas** (a primeira sem ponto e vírgula), mandando o pacote de
spawn em dobro a cada mob que entra na tela.

**`game.addExp`** — `AddExp` fazia `ExpList[Self.Character.Base.Level]` com
`ExpList` sendo vetor dinâmico e `Level` um `WORD`: nível acima do tamanho da
tabela lia fora do vetor, e é o que acontece ao chegar no teto se o arquivo for
menor que `LEVEL_CAP`. Agora a tabela diz se o nível existe. O parâmetro `out
ExpPreReliq` só era atribuído no ramo `EXP_TYPE_MOB`, ficando indefinido nos
outros dois — ele não foi portado porque nenhum chamador lê o valor.

**`game.addLevel`** — `Inc(SkillPoint, ...)` e `Inc(Status, ...)` num `WORD`
davam a volta a partir de 65535. Agora saturam.

**`EXP_MULTIPLIER` zero** — a origem multiplicava sem checar; zero no `.ini`
fazia **todo mob dar zero de experiência**. Agora zero vale 1, como o valor de
`GlobalDefs.pas`.

**`game.hitMob`, alcance** — mesma falta de checagem do ataque entre jogadores:
bastava o `ClientID` do mob. Agora o mob precisa estar dentro de
`AttackRange`.

**`game.mobPatrolStep`, o mob que nunca chegava** — a chegada era
`if (CurrentPos.Distance(DestPos) <= 0)`, e `Distance` devolve um `WORD`
truncado. Com passo de 1.5 e limite de 2 células por eixo, o mob para a **1.5
célula** do destino: a distância truncada vale 1, o teste nunca fecha, o mob
fica preso ali para sempre e nunca inverte o sentido. Pior: `MovimentsTime`
continua subindo, então a cada dois ticks sai um pacote de movimento **com a
mesma posição**, para todo mob em patrulha do mapa. Agora a chegada é o momento
em que nenhum eixo consegue mais andar.

**`DestMoveWait` nunca lido** — a origem carrega a coluna do CSV e usa
`InitMoveWait` nos **dois** sentidos da patrulha. A coluna existia e não fazia
nada. Agora cada sentido usa a sua.

**`game.mobAttackStep`, alvo que foi embora** — o teste era
`if(Players[AttackerID].Status < Playing) then Exit`, que só saía do
procedimento: o mob ficava preso num alvo que desconectou e **nunca voltava ao
ponto de spawn**. Agora ele esquece o alvo. Também ganhou um alcance de
desistência (`MobLeashRange`), que a origem não tinha: lá a perseguição não
terminava.

**`game.stepToward`** — a origem não validava a posição do mob em momento
nenhum. Um destino errado no CSV levava o mob para fora do mapa, e dali para as
contas de distância de todos os jogadores. Agora o passo que sairia do mapa é
descartado.

**`game.activeMobs`** — nove threads por canal (três de spawn e três de ataque
a cada 1000 ms, três de movimento a cada 600 ms) varriam cada uma um terço do
vetor de 22.500 posições, **mesmo com o mapa vazio**. Agora são dois tickers com
os mesmos tempos, e só os mobs que algum jogador está vendo entram na varredura.

**`game.mobTarget.Notify`** — no reflexo de dano a origem mandava a mensagem
por `Players[mob.ClientID].SendClientMessage`, com `ClientID` de mob acima de
1000 num vetor de 1000 posições. Mob não recebe mensagem, e agora isso é
explícito.

**`game.PartyRegistry`** — substitui o vetor `Parties: ARRAY [1..30]` **e** o
ponteiro `Players[i].Party: PParty` que o original guardava dentro do registro
do jogador. Aquele ponteiro apontava para dentro de um vetor global que
`DestroyParty` zerava com `ZeroMemory`, e seguia sendo lido depois — inclusive
em `RefreshParty`, que faz `Self.Party.Members` **sem checar nil**. Aqui o
grupo é um objeto no registro e quem não tem grupo simplesmente não aparece no
índice.

**Convite reutilizável** — `AcceptParty` lia `Player.PartyRequester` e **não o
limpava**: o mesmo `$323` entrava no grupo de novo, e nada impedia um cliente
de repetir o pacote. Agora o convite vale uma vez.

**`game.sendPartyUpdate`, estouro do vetor do pacote** — `RefreshParty`
incrementava `J` dentro de `for i in Self.Party.Members` sem teto, e
`Packet.Name[J]`, `Packet.PlayerIndex[J]` etc. têm 6 posições. Com mais membros
que isso, escrevia fora do pacote. Agora o laço para no tamanho do vetor.

**`game.leaveParty`** — a saída do grupo na desconexão dependia do caminho de
desconexão chamar `RemoveMember`; quando não chamava, o grupo ficava com um
membro que não estava mais no servidor, e `RefreshParty` seguia lendo
`Players[i]` daquele slot — que podia já ter sido reaproveitado por outro
jogador. Agora a saída fica no mesmo ponto único que tira o jogador do canal.

**Limite de 30 grupos** — o laço de busca de `CreateParty` simplesmente não
achava slot livre e devolvia `False` **sem avisar ninguém**: o jogador clicava
e nada acontecia. Agora o limite é explícito e o pedido é recusado com
mensagem.

**Nível no pacote de grupo** — `Packet.Level[J] := Level - 1` num `BYTE`: nível
0 ia como 255. Piso em zero.

**Conversa de grupo** — passou de "não disponível" para funcionando; guilda,
aliança, nação e megafone seguem recusados com aviso.

**`LearnSkill`, os pontos de atributo apagados** — o ramo
`if (SkillPoint = 1) then CurrentScore.Status := 0` zerava os pontos de
**atributo** junto com o último ponto de habilidade. Aprender uma skill com
exatamente 1 ponto sobrando apagava a reserva de status inteira do personagem.
É o único caso em que o jogador perde dado por usar o jogo normalmente, e não
foi portado.

**`skillfn.Data`** — `SkillData[Packet.SkillIndex]` com o índice vindo do
cliente como `DWORD` num `ARRAY [0..11998]`, lido **cinco vezes antes de
qualquer validação**: um índice qualquer lia memória fora da tabela, e o que
viesse de lixo decidia nível mínimo, pontos, custo e classe.

**`skillfn.IncLevel`, nível sem teto** — `inc(Skills.Basics[i].Level)` não
tinha limite. Passando de 15, `Index + Level` entrava no **grupo de skill
seguinte** da tabela, e o jogador usava skill de outra árvore de graça.

**`game.useSkill`, repasse antes da validação** — `UseSkill` chamava
`Player.Base.SendToVisible(Packet, ...)` na **primeira linha**: skill
inexistente, de outra classe, sem MP ou em recarga ainda aparecia na tela de
todo mundo. Agora o repasse acontece depois de a skill ser aceita.

**`game.useSkill`, skill não aprendida** — a origem não conferia se o jogador
tinha a skill: bastava mandar o índice no `$320` e ela saía. Só o nível mínimo
do personagem era testado, e a classe era checada apenas em `LearnSkill`.

**`game.sendSkillLevels`** — em `SendPlayerSkillsLevel` o `SendPacket` vinha
**antes** de o vetor `Skills` ser preenchido, e o `Move` que o preenchia
escrevia num registro local já enviado e a ponto de sair de escopo. O cliente
recebia a máscara sempre zerada: **os níveis de skill nunca chegavam**.

**`skillfn.LevelMask`** — `GetSkillLevel` devolvia `2^(Level+1) - 2` e o
tamanho do `Move` (2 ou 4 bytes) num slot de 2 bytes. Com nível 16 ou mais o
`Move` de 4 bytes escrevia por cima da máscara da skill vizinha. Agora o nível
é limitado ao teto do grupo antes da conta.

**`store.Skills.ByCharacter`** — o slot vinha do banco e indexava
`Skills.Basics[ItemSlot]` (6 posições) e `Skills.Others[ItemSlot]` (40) **sem
checagem de faixa**: um slot de 40 ou mais escrevia depois do fim de `Others`,
dentro do registro do personagem — o mesmo registro que vai no `$925` e é
gravado de volta.

**`PredadorInvBarReset`, código morto** — monta o pacote `$12C` inteiro,
calcula o cooldown novo e **nunca chama `SendPacket`**. Não foi portado.

**Custo de MP descontado depois da validação** — na origem o MP saía dentro de
`AttackTarget`, depois de vários `Exit` possíveis; aqui o desconto é o último
passo antes de a skill sair, então nenhuma recusa cobra mana.

**`dropdata.Category.Roll`, a raridade subia em vez de descer** — quando a
raridade sorteada não tinha item, `DropItemFor` fazia
`ItemTypeFrom := ItemTypeFrom + 1; goto`, isto é, normal caía em superior,
superior em rara e rara em lendária. **Com os dados que acompanham o
servidor isso é grave:** as categorias de Planta, Croshu Azul e Croshu
Vermelho têm item **apenas** na raridade lendária, e a faixa normal do sorteio
cobre 67 dos 100 valores — dois terços das mortes desses mobs entregavam um
item lendário. No Go a busca desce para a raridade mais comum, e só sobe
quando não há nada abaixo. Os quatro avisos que o servidor imprime na carga
apontam exatamente essas categorias.

**`DropItemFor`, o laço que não termina** — os dois ramos da origem tratavam a
lendária vazia de formas diferentes: no de mob comum, `Exit` (nenhum drop); no
de chefe (`InitHP > 999999`), `ItemTypeFrom := 1; goto ReCase`, voltando à
primeira raridade. Com as quatro raridades vazias esse segundo ramo **gira para
sempre dentro da morte do mob**. Nenhuma categoria dos arquivos de hoje está
com as quatro vazias, então não é alcançável agora — mas basta um arquivo vazio
para travar o servidor ali.

**`TLoad.InitDropList`** — o mesmo bloco de ~60 linhas copiado **treze vezes**,
uma por arquivo, cada cópia com seus quatro contadores e seu `case` de quatro
ramos: cerca de 800 linhas que no Go são um laço. Cada cópia fazia também
`SetLength(..., Count + 1)` por linha lida, realocando o vetor a cada item. E
`FileStrings[2]` sem checar o número de colunas: uma linha curta levantava
exceção e abortava a carga, deixando os arquivos seguintes sem ler.

**`game.MapObjects`, objeto que nunca expira** — a origem não apagava o objeto
de drop por tempo: ele ficava no vetor `OBJ` até alguém coletar, e com o vetor
cheio `CreateMapObject` passava a recusar **todo** drop novo com "Erro ao criar
o objeto no mapa. Abra um ticket no suporte." Agora o objeto expira e o laço do
servidor limpa o chão.

**`game.collectMapItem`** — `Servers[].OBJ[Packet.Index]` com o índice vindo do
cliente e sem checar a faixa do vetor, e **sem checar distância**: bastava o
índice para pegar um item do outro lado do mapa. A coleta também mexia no vetor
sem trava, com uma thread por jogador: dois jogadores clicando no mesmo item
podiam levar os dois. Agora o item sai do chão sob trava, antes de ser
entregue.

**`guild.Guild.MemberAt`, índice −1** — `OpenChest` escrevia
`Self.Members[Self.FindMemberFromCharIndex(CharIndex)].Rank` **direto**, e
`FindMemberFromCharIndex` devolve **−1** quando não acha: aquilo lia
`Members[-1]`, antes do início do vetor, e o byte lido virava o rank usado na
checagem de permissão.

**`guild.Guild.Capacity`** — `MaxGuildPlayersOfLevel[Self.Level]` com `Level`
sendo `DWORD` e o vetor `ARRAY [0..9]`: guilda de nível acima de 9 lia fora do
vetor, e o limite de membros saía de um byte qualquer da memória.

**`guild.Guild.RankConfigOf`** — `Self.RanksConfig[Rank]` com `Rank` sendo
`Integer` e o vetor `ARRAY [0..4]`. O rank vem do registro do membro, que vem
do banco: um rank fora de 0..4 lia fora do vetor e as permissões saíam de
memória qualquer.

**`guild.Guild.SetRanksConfig`** — `Move(RanksConfig, Self.RanksConfig, 4)`
copiava **4 bytes num vetor de 5**: a configuração do rank 4 **nunca** era
atualizada, ficando com o que estava na memória desde a criação da guilda.

**`store.Guilds.loadMembers`** — `Guilds[Slot].Members[R]` com `R` apenas
incrementando por linha, **sem checar** o limite `ARRAY [0..127]`. Uma guilda
com 129 linhas em `guilds_players` escrevia depois do fim de `Members` — e o
que vem depois no registro é o **baú da guilda**. O nome do membro era copiado
com `Move(..., Length(nome))`, sem limite: um nome de mais de 20 caracteres
passava por cima de `Rank`, `ClassInfo`, `Level` e `Logged`.

**`Logged` lido do banco** — a origem gravava a coluna e a lia de volta na
carga, o que deixava membro marcado como online depois de o servidor cair. No
Go quem está online é quem está no canal agora.

**`game.enterGuild`** — personagem com `guildindex` no banco e sem linha em
`guilds_players` não está em guilda nenhuma, mas a origem mandava o pacote de
guilda com o índice gravado e o cliente mostrava uma guilda que não existe.
Agora o caso é reportado e o índice é limpo.

**Convite reutilizável** — como no grupo, `InviteToGuildAccept` lia o
requisitante e não o limpava.

**`TGuild.TCloseChestThread`** — `OpenChest` criava uma `TThread` **por
abertura** e nunca a liberava, com um laço `Sleep(50)` até o baú fechar. O
baú não foi portado; quando for, isso vira um prazo no laço do servidor.

**O correio da origem estava não funcional** —
`TMailFunctions.GetMails` tem o corpo **esvaziado**: é só `Result := True`,
com a leitura do arquivo comentada. Disso decorre tudo o que segue.

- **Toda carta recebida apagava a caixa inteira do destinatário.** `AddMail`
  chama `GetMails` (que não lê nada e devolve `True`), deixa `MailCount` em
  zero, faz `SetLength(Mails, 1)` e grava o arquivo com **uma** carta. A carta
  anterior, com os itens anexados, desaparecia do disco.
- **Nenhuma carta podia ser lida de volta.** `GetMail` e `GetMailSlot` varrem o
  vetor que `GetMails` não preencheu.
- Os laços `for i := Length(Mails) - 1 downto 0` com `i` declarado `BYTE`
  (`GetMail`) ou `DWORD` (a sobrecarga) partem de **−1** num vetor vazio, o que
  como inteiro sem sinal vira 255 ou 4294967295 e indexa um vetor nulo.

Portar isso ao pé da letra seria portar perda de dado. No Go a caixa fica no
MySQL, como o resto do estado de jogo, com os mesmos pacotes. O esquema das
tabelas novas está em `GoServer/SCHEMA.sql`.

**`SendCharacterMail`, item perdido na falha** — os itens saíam do inventário
do remetente **antes** de `AddMail`, e um `AddMail` que falhasse devolvia
`False` sem repor nada: o item sumia. Agora a carta é gravada primeiro e o
inventário só muda depois.

**`ConfirmSendMail`, destinatário inexistente** — a origem só guardava o nome
em `Player.CanSendMailTo` e conferia a existência depois, na gravação, que
criava o arquivo do destinatário **mesmo que ele não existisse**, deixando
caixa de correio de personagem inexistente no disco. Agora o nome é conferido
contra a tabela `characters`.

**`LasMailIndex` / `IncMailIndex`** — o contador de índice de carta vivia num
arquivo de 8 bytes (`Data/Mails/LastIndex.t3c`), lido e reescrito a cada envio
**sem trava nenhuma**: dois envios ao mesmo tempo liam o mesmo valor e as duas
cartas ficavam com o mesmo índice — e o índice é a chave que identifica a carta
na caixa.

**`SendRefreshMails`, total não enviado** — `Packet.MailsAmount` está
comentado, então o cliente nunca sabia quantas cartas a caixa tem.

**`RemoveExpiredMails`** — apagava a carta vencida **sem olhar o anexo**: item
e ouro iam com ela. Agora a carta vencida com anexo fica.

**Apagar carta com anexo** — a origem aceitava, e o item ia embora com a carta.
Agora é recusado com mensagem.

**`gamedata.Tables.BuffSeconds`** — `SkillData[Index].Duration` com
`SkillData: ARRAY [0..11998]` acessado sem checagem: índice de buff acima da
faixa lia memória fora da tabela. Agora índice inválido responde 0, e a duração
lida do arquivo é limitada a 24 h para um valor absurdo não virar buff eterno.

### Leilão (`AuctionFunctions.pas`)

**`RequestBuyItem` entregava o item antes de conferir o ouro do comprador.**
A ordem na origem é: monta a carta do item para o comprador, monta a carta do
ouro para o vendedor, insere o anexo com `RegisterItemDatabase`, e **só então**
lê `SellingPrice` e sai com `Exit` se o comprador não tiver ouro. Quem mandasse
o pacote de compra sem dinheiro levava o item de graça, e a carta do ouro já
estava na caixa do vendedor. No porte a conferência (`auction.CanBuy`) vem
antes de qualquer gravação, e o fechamento da oferta vem antes das cartas.

**`RequestCancelOffer` não conferia o dono.** A oferta era buscada pelo índice
que vinha no pacote e cancelada sem comparar o vendedor com quem pediu:
qualquer jogador cancelava a oferta de qualquer outro, e o item ia para a caixa
de quem cancelou. Agora `auction.CanCancel` exige que o `CharIndex` do pedido
seja o do vendedor.

**Dois compradores levavam o mesmo item.** Com uma thread por jogador, duas
compras simultâneas da mesma oferta liam `Active = 1` e seguiam as duas. O
fechamento agora é um `UPDATE auction_offers SET active = 0 WHERE offer_index =
? AND active = 1`: `RowsAffected > 0` elege um único vencedor, e o outro recebe
recusa sem perder ouro.

**`CheckAuctionItems` tinha o corpo esvaziado** (só `Result := True`): a oferta
vencida ficava marcada como ativa para sempre e o item **nunca** voltava ao
vendedor. O porte roda `AuctionExpireTick` num ticker de 1 minuto, que fecha a
oferta vencida e devolve o item por carta.

**`SELECT max(id)` fora da transação** em `RegisterItemDatabase`: dois
registros ao mesmo tempo liam o mesmo id e o segundo sobrescrevia o anexo do
primeiro. O índice da oferta agora vem de um contador no banco.

**`Inventory[Slot]` com `Slot` vindo do cliente.** `RequestRegisterItem` lê o
slot como `WORD` e indexa `Inventory: ARRAY [0..63]` sem checar; o
`ZeroMemory(@Inventory[Slot], SizeOf(TItem))` do fim **escreve** 20 bytes fora
do vetor para qualquer slot acima de 63 — corrompendo o que estiver depois do
inventário no registro do jogador. Agora o slot é conferido contra
`MovableInventorySlots`.

**Mais de 12 ofertas próprias recusava a lista inteira.** `SendCadastredItems`
sai sem enviar nada quando a contagem passa de 12, então o jogador com 13
ofertas não via **nenhuma**. Agora a lista é cortada no teto e as 12 primeiras
aparecem.

**Falha ao registrar tirava o item do inventário.** Mesmo padrão do correio: a
oferta é gravada primeiro, e o item só sai do inventário (e a taxa só é
cobrada) depois da gravação dar certo.

**Taxa de registro: a conta da origem foi mantida.** `Trunc(Price / 1000) *
Round(Hours / 3)` — com o `Round` do Delphi (metade para o par), preço abaixo
de 1000 dá taxa **zero**, e 3 h de prazo também. Isso está no original e
continua assim; o teste `TestComissao` registra a tabela de valores.

**Item com `TypeTrade` de não comercializável** era aceito no leilão: a origem
confere o `TypeTrade` na troca entre jogadores, mas não no registro da oferta.
Agora é recusado.

### Dungeon (`Dungeon.pas`, `InitDungeons`, `CreateDungeonInstance`)

**A dungeon não funciona na origem, e está desligada de propósito.** A chamada
de carga está comentada — `//Self.InitDungeons;`, `ServerSocket.pas:493` — e
com razão: o laço de carga é `for i := 0 to 2 do` sobre vetores declarados
`Array [0 .. 0] of TDungeon` (`ServerSocket.pas:63-67`). Cada `TDungeon` tem 45
espécies de até 50 pontos, e cada ponto carrega um `TBaseMob` inteiro, então a
carga grava **duas fichas de dungeon inteiras**, megabytes, fora do vetor. Os
campos que vêm logo depois no mesmo objeto são `Parties: ARRAY [1..30]`,
`Devires`, `DevirNpc`, `DevirGuards`, `DevirStones`, `SecureAreas`,
`ReliqEffect`, `CastleObjects`, `CastleSiegeHandler` e os ponteiros de thread
do servidor. A última dungeon carregada (Kinary) escreve além de todos eles.

Com a carga desligada, todo `DGUrsula[x]` lido em jogo devolve zeros: nível
mínimo 0, tabela de mobs vazia, e `SpawnInDungeonPosition` igual a (0, 0) — a
posição para onde o grupo era teleportado. No porte a carga roda, as três
dificuldades ficam num slice indexado com checagem, e o `Dungeons.csv` real
rende 5 dungeons × 3 dificuldades e 2874 pontos de spawn.

**Uma instância para o servidor inteiro.** `DungeonInstances: Array [1 .. 1]`
(`GlobalDefs.pas:18`). O segundo grupo a entrar recebia
`GetFreeDungeonInstance = 255` e a rotina fazia só `Exit` — **depois** de todos
os membros já terem sido teleportados para dentro e marcados `InDungeon`
(`Player.pas:11143-11162`). Resultado: grupo preso num mapa sem mob nenhum,
com `DungeonInstanceID = 0`, que é índice fora do vetor `[1..1]` e era usado em
todo acesso seguinte. No porte a instância é criada **antes** de alguém entrar,
e cada grupo tem a sua.

**O vetor de pontos estoura com os dados que acompanham o servidor.** Nas três
dificuldades do Marauder Hold uma espécie tem **93** pontos de spawn, e
`MobsP` é `Array [0 .. 49]`. A carga grava `MobsP[cntControl]` sem checagem
(`ServerSocket.pas:1332`): 43 registros por cima da espécie seguinte. O porte
recusa o excedente e avisa no log — é o que aparece ao subir o servidor.

**Um jogador confirmava a entrada pelos outros.** `DungeonLobbyConfirm` credita
a confirmação a `Packet.Header.Index`, um campo que vem do cliente
(`PacketHandlers.pas:9699`). Mandando um `$355` com o índice de cada membro, um
jogador sozinho enchia o vetor de confirmações e arrastava o grupo inteiro para
dentro. No porte a confirmação é sempre de quem mandou o pacote.

**O saguão não tinha prazo e as confirmações viviam no grupo.**
`SetLength(Party.DungeonLobbyConfirm, Members.Count)` rodava a cada
confirmação, então um membro que saísse do grupo no meio encurtava o vetor e
descartava confirmações já dadas; e como o vetor ficava no registro do grupo,
um saguão novo herdava as confirmações do anterior. Um membro que nunca
respondesse deixava o grupo preso para sempre. No porte o saguão é um objeto
próprio, com prazo de 60 s.

**O nível do grupo era conferido nos jogadores errados.** No caso do NPC do
Kinary, `for I := Low(Party.Members.ToArray) to High(...)` e
`Players[I].Base.Character.Level` (`PacketHandlers.pas:9557`): `I` é o índice
no vetor de membros, usado como índice de **conexão**. Conferia o nível de
quem estivesse nas conexões 0, 1, 2…, não dos membros. Os outros quatro NPCs
usam `for I in Party.Members`, que está certo.

**A dificuldade é um DWORD do cliente.** `DGMines1[Packet.Dificult].LevelMin`
num vetor de um elemento (`PacketHandlers.pas:9489`), e o mesmo índice volta em
`CreateDungeonInstance` para ler a posição de entrada e a tabela de mobs
inteira. Qualquer valor diferente de 0 lia memória alheia e seguia com ela. No
porte a dificuldade passa por `Table.Get`, que recusa fora de 0..2.

**O `$334` não exigia estar em jogo.** No `case` do despacho os vizinhos têm
`if (Player.IsInstantiated)`; esse não (`ServerSocket.pas:3080`).

**"Miss" tirava vida.** No golpe do mob de dungeon o sorteio marcava
`TDamageType.Miss` para 2..15 e o dano era descontado do HP do mesmo jeito: o
cliente escrevia Miss e o jogador perdia vida. `Critical` também não mudava o
dano. No porte Miss zera e Critical aumenta, como no golpe de mob do mundo.

**O golpe lia o HP de um registro e escrevia em outro.**
`PlayerBase.PlayerCharacter.Base.CurrentScore.CurHP` para conferir, e
`Base.Character.CurrentScore.CurHP` para descontar (`Dungeon.pas:150` e 257).

**A exp do grupo era dividida uma vez por membro.**

```pascal
for i in Party.Members do
  MobExp := (MobExp div Party.Members.Count);
```

(`BaseMob.pas:6189`) — a divisão é refeita sobre o valor já dividido. Num grupo
de quatro: 250, 62, 15 e 3, em vez de 250 para cada. O teste
`TestExpDoGrupoEDivididaUmaVez` fixa isso.

**O drop de dungeon está todo comentado** (`BaseMob.pas:6988-7022`), então o mob
de dungeon caía no drop do mundo indexado por `Mobs.TMobS[mob.Mobid]`, com
`Mobid` sendo o **slot na instância** (1..450): a lista de drop de uma espécie
do mundo escolhida pelo número do slot. E o bloco comentado, se voltasse, tinha
`ItemTax := RandomRange(1, cnt)` (nunca sorteia o índice 0 nem o último) e
deixava `ItemTax` **sem valor** quando a lista estava vazia.

**A coroa do drop só casava com os números do Zantorian.** O
`case DungeonDropIndex of 11, 21, 31: … 12, 22, 32: … 13, 23, 33:` aparece
igual nos cinco blocos de `CreateDungeonInstance`, mas Marauder usa 41..93,
Mina 101..303 e Kinary 401..603: para **quatro das cinco dungeons** nenhum caso
casava e a lista de drop ficava zerada. São 297 espécies sem drop nenhum
(hold 57, cabin 51, mina 57, kinary 132). No porte a coroa sai do último dígito
do índice, que é o que o nome do arquivo quer dizer.

**A thread da instância usava um ponteiro liberado.** `TDungeonMainThread.Execute`
lê `DungeonInstances[i].Party` uma vez, antes do laço, e usa esse ponteiro pela
vida toda; o fechamento põe `Party := nil` e o grupo pode ser desfeito. No
porte a instância guarda os `CharIndex` dos membros e um tick único varre as
instâncias abertas.

**A queda da conexão não soltava o jogador da instância.** `Connection.Destroy`
limpava os campos do jogador, mas não mexia em `InstanceOnline` nem tirava o
membro da lista. Como havia um único slot de instância no servidor, uma queda
dentro da dungeon travava a dungeon **de todo o servidor** até reiniciar.

**`IsInsideDungeon` é escrito em dois lugares e lido em nenhum** — e os dois
lugares o põem em `True` justamente quando o jogador está sendo **expulso** da
dungeon (`Dungeon.pas:294` e 300).

**O mob perseguia teleportando.** `Randomize; Rand := Random(7);
Position := jogador.Neighbors[Rand].pos` (`Dungeon.pas:746-750`): o mob pulava
para o lado do jogador num único passo, `Randomize` reposicionava a semente
global a cada tick, e `Random(7)` nunca sorteia o oitavo vizinho. No porte o
mob anda 1,5 célula por tick na direção do alvo.

**Dois golpes simultâneos matavam o mesmo mob duas vezes**, com uma thread por
jogador e `Dec(CurrentHP, Dano)` sem trava — a mesma forma do mob do mundo.
Aqui o golpe passa por `Instance.Hit`, sob mutex, e `TestGolpeConcorrenteMataUmaVez`
roda com `-race`.

**O jogador dentro da dungeon recebia os mobs do mundo.** Os dois laços de
visibilidade rodavam para ele: o da dungeon e o do mundo, que não sabia da
dungeon. No porte, quem está dentro vê os mobs da instância e mais nada.

**Sem `Exit` no fim do bloco de dungeon do ataque** (`PacketHandlers.pas:5753`),
a execução caía no `if (Packet.Index <= MAX_CONNECTIONS)` logo abaixo e batia
uma segunda vez, agora num jogador.

**A coluna de nível do MobInfo de dungeon não é nível.** A origem lê
`MobLevel := MobFileStrings[7]`, e nos arquivos reais essa coluna vale 269..1292
(Zantorian), 298..1338 (Mina, Kinary) e 28..1320 (Marauder) — acima do nível
máximo do jogo, que tem 152 entradas no `ExpList.bin`, e andando poucas
unidades de uma dificuldade para a outra, como um id sequencial. O valor é lido
como na origem e só serve para a cor do nome no cliente ("for a name collor on
client", `MOB.pas:322`); quem precisa de nível para valer usa o `LevelMin` da
dungeon. O teste `TestColunaSeteNaoEhNivel` registra isso e avisa se os
arquivos mudarem.

Fora do porte, por ora: raid na dungeon (`PlayersParty2..4` do `$145`), skill de
mob de dungeon e os NPCs de dentro (`NPCS: Array [2600..2609]`, nunca
preenchido na origem).

### Console 5555 (`ConsoleSocket.pas`, `CommandHandlers.pas`)

**A porta 5555 era uma porta dos fundos sem senha.** `HandleClient` lê a linha,
procura um Admin online no vetor de conexões, e executa o comando **como esse
Admin** (`ConsoleSocket.pas:159-186`). Não há autenticação em lugar nenhum: nem
senha, nem token, nem lista de endereços. O endereço vinha do `.ini` e aceitava
`0.0.0.0`. Quem alcançasse a porta tinha o servidor: `/additem`, `/addgold`,
`/setrank`, `/dc`.

No porte o console **não sobe sem token** (`AIKA_CONSOLE_TOKEN`), e sem token
ele diz por que no log em vez de abrir a porta. O endereço padrão é
`127.0.0.1:5555`. A primeira linha da conexão tem de ser `AUTH <token>`,
comparada em tempo constante, e o resto só é aceito depois disso.

**O comando rodava como um Admin sorteado.** Duas consequências: sem Admin
online nada funcionava (`"Nenhum Admin online para processar"`), e com Admin
online um comando sem nome de jogador agia **sobre esse Admin** — `/addgold
1000000` pelo console dava o ouro a ele. No porte o console roda como o próprio
servidor e os comandos que precisam de alvo exigem o nome do jogador.

**`ProcessCommands` não conferia permissão nenhuma.** A checagem
`AccountType >= GameMaster` vive em quem chama (`PacketHandlers.pas:1361`, o
caminho do sussurro); o console chamava sem nada. No porte a permissão é do
comando: `/setrank` exige Admin, o resto GameMaster, `/who` Moderador.

**Um `recv` só, sem enquadramento.** `recv(ClientSock, Buffer, 256, 0)` e o que
vier é o comando inteiro; um comando partido em dois segmentos TCP era truncado
e executado pela metade. Depois a conexão era fechada: um comando por conexão.
No porte o enquadramento é por linha e a conexão aceita vários comandos.

**Respondia `OK` sempre.** `send(ClientSock, 'OK'#0, 3, 0)` roda depois de
`ProcessCommands`, incondicionalmente: comando inexistente, comando que falhou
e comando que funcionou davam a mesma resposta ao painel. No porte a resposta é
o resultado do comando.

**Linha acima de 200 bytes era descartada em silêncio** (`if Received > 200 then
Exit`), sem resposta: o painel ficava sem saber.

**`HandleClient` rodava dentro do laço de `accept`**, então um cliente parado
travava a porta inteira. E quando o `accept` falhava, `Sleep(10); Continue` para
sempre — sem log, sem recuperação, laço ocupado.

**Uma exceção num comando matava a thread do console.** `HandleClient` tem
`try...finally` só para fechar o socket, e `Execute` não tem nada: a exceção
sobe e encerra a thread. E as exceções são fáceis de provocar, porque
`ProcessCommands` indexa a lista de parâmetros sem conferir a contagem:

| Comando sem parâmetro | O que fazia |
|---|---|
| `/dc` | `Disconnect(Player, CMD[0])` numa lista vazia |
| `/setdntype` | `SetDnType(CMD[0])` idem |
| `/addeff` | `Data[0].ToInteger()` sem `try` |
| `/dmg` | pior: `try Dano := Data[0].ToInt64 finally RemoveHP(Dano)` — `finally`, não `except`, então o dano era aplicado com valor zero **e** a exceção subia |

Então `/dmg` sem argumento, enviado à porta 5555, derrubava o console e o painel
parava de responder até o servidor reiniciar. No porte cada parâmetro passa por
um acessador que devolve vazio fora da faixa, e o goroutine da conexão tem
`recover`.

**`CMD` (`TStringList`) nunca é liberado.** É criado no começo de
`ProcessCommands` e nenhum dos caminhos — nem os `Exit` adiantados, nem o fim da
rotina — chama `Free`. Um vazamento por comando executado.

**O despacho tinha três partes que não conversavam:** uma corrente de
`if Command = '...'` com `Exit`, um `case AnsiIndexStr(Command, [...])` com 31
nomes, e `addtitle`/`updatetitle`/`removetitle` nas duas. A corrente roda
primeiro e sai, então **os casos 19, 20 e 21 do `case` nunca executavam**. E
`'npc'` está na lista de nomes (índice 30) sem ramo correspondente: caía no
`else`, "Comando inválido.".

**`/setrank` não conferia a faixa nem exigia nome.**
`TAccountType(Rank)` com `Rank` acima de 5 dá um enum fora da faixa, e a
comparação `>= GameMaster` passa a valer para qualquer valor alto; sem nome, o
comando se aplicava a quem digitou, então qualquer GM se promovia a Admin. No
porte o nome é obrigatório, o valor é conferido, e o comando exige Admin.

**`/addlevel` lia o nível de quem digitou, não do alvo.**

```pascal
LevelExp := ExpList[Player.Character.Base.Level + (Level - 1)] + 1;
```

`Player` é quem digitou, mesmo quando o alvo é outro jogador. Pior: `Level` é
WORD, então `/addlevel 0` dá `65535` no índice, cai no `except`, e o *fallback* é
`LevelExp := High(ExpList)` — o último **índice** (151), não o último valor. Em
seguida `AddExp := LevelExp - UInt64(Exp)` dá a volta no UInt64 e o personagem
recebe ~1,8×10¹⁹ de experiência.

**`/addgold` somava sem teto.** `Inc(Gold, Value)` com `Value` vindo de
`Data[0].ToInt64`: dois comandos com o máximo do Int64 davam a volta e deixavam
o ouro negativo. O porte tem teto.

**`/dmg` só sabia bater em quem digitou** — não aceitava nome de alvo. **`/move`
não validava a coordenada.** **`/addmob` tem corpo vazio** e **`/equip`
responde "Ainda não setado."**: comandos que existem e não fazem nada.

Comandos portados: `additem`, `addgold`, `addexp`, `addlevel`, `addbuff`,
`removebuff`, `move`, `dc`, `dmg`, `setrank`, mais `who` e `help`, que não
existem na origem e o console precisa (o nome do jogador agora é obrigatório).
Os outros 24 nomes da origem estão no registro e respondem dizendo de que
sistema dependem, em vez de não fazer nada em silêncio.

### Armazém e baú de guilda (`OpenNPC`, `ChangeGold`, `MoveItem`, `TGuild.OpenChest`)

#### O baú de guilda não abria na origem

`TGuild.OpenChest` recebe o personagem assim (`PacketHandlers.pas:2513`):

```pascal
if (Guilds[Player.Character.GuildSlot].OpenChest(Player.Character.Index,
  Player.ChannelIndex) = False) then
```

e começa por:

```pascal
if Servers[ChannelId].GetPlayerByCharIndex(CharIndex, Player) = False then
  Exit;
```

`Player.Character.Index` é o campo `Index: DWORD` de `TPlayerCharacter`
(`PlayerData.pas:306`). Ele é **lido em vinte lugares e nunca atribuído em
nenhum**: `grep -rn "Character\.Index *:="` no código inteiro não devolve uma
linha. Vale sempre zero. `GetPlayerByCharIndex(0, …)` não acha ninguém,
`OpenChest` devolve `False`, e o ramo `$B` da janela de NPC sai zerando
`OpennedNPC` e `OpennedOption`. **O baú da guilda nunca abriu.** É o mesmo
diagnóstico da dungeon: o sistema está no código, nunca rodou.

O mesmo campo zerado aparece em outros três lugares:

- `Guild.SendMemberLogout(Self.Character.Index)` (`Player.pas:629`, `733`,
  `4923`, `5074`): a saída nunca marca o membro como offline, então a lista da
  guilda mostra todo mundo conectado para sempre.
- `Guilds[GuildSlot].Members[MemberId].CharIndex := Self.Character.Index`
  (`Player.pas:5226`): o remendo que tenta recadastrar o jogador na guilda
  grava **zero** no `CharIndex` de um membro — e é um slot de membro novo a
  cada login, porque `FindMemberFromCharIndex(0)` nunca acha.
- `Packet.Character.CharIndex := Self.Character.Index` (`Player.pas:5557`): o
  pacote sai com zero.

No porte o dono do baú é o `CharIndex` do personagem selecionado
(`Session.CharID()`), que é o mesmo id usado pelo resto do porte.

#### `Guilds[0]` escreve antes do vetor

`Guilds` é declarado `ARRAY [1 .. 50] OF TGuild` (`GlobalDefs.pas:43`) — **o
limite de baixo é 1**. E `GetGuildSlotByID` devolve **zero** quando não acha a
guilda (`ServerSocket.pas:4232`). Os seis caminhos do baú fazem

```pascal
Guild := @Guilds[Player.Character.GuildSlot];
```

conferindo apenas `GuildIndex <= 0`, nunca o slot. Com `GuildSlot = 0` isso é
um registro `TGuild` inteiro — milhares de bytes, 128 membros e 50 itens —
**antes** do início do vetor. O que vem antes de `Guilds` em `GlobalDefs.pas`
são `InstantiatedNPCs`, `PlayersThreads` e, acima deles, as tabelas de dado
carregadas do disco: `MapsData`, `MobPos`, `ScrollTeleportPosition`,
`Recipes`. Escrever `Guild.Chest.Gold` ou `@Guild.Chest.Items[slot]` ali
corrompe dado de jogo em silêncio.

Esse padrão já tinha sido notado na origem: `SendToWorldSends` ganhou um
remendo explícito (`"CORREÇÃO DEFINITIVA DO BUG"`, `Player.pas:5198`) que
valida `GuildSlot >= Low(Guilds)` antes de usar. O remendo não foi aplicado a
nenhum dos caminhos do baú. No porte a guilda vem de
`guild.Registry.Of(charIndex)`, um mapa: índice inválido não acha nada.

#### `Members[-1]`

`OpenChest` lê a permissão antes de qualquer validação:

```pascal
if Self.GetRankConfig(Self.Members[Self.FindMemberFromCharIndex(CharIndex)
  ].Rank).OpenGWH = False then
```

`FindMemberFromCharIndex` devolve `-1` quando o personagem não está na lista.
`Members[-1].Rank` é uma leitura antes do início do vetor de membros. O mesmo
padrão aparece em `MoveItem` e em `ChangeGold`, nas duas vezes que elas leem
`UseGWH`. No porte `Guild.CanOf(charIndex)` devolve permissão vazia para quem
não é membro, e `openGuildChest` recusa quem `FindMember` não acha.

#### `MemberInChest = -1` abre o baú para qualquer um

`OpenChest` grava o resultado de `FindMemberFromCharIndex` **sem conferir**:

```pascal
Self.MemberInChest := Self.FindMemberFromCharIndex(CharIndex);
```

Para um não-membro isso é `-1`. A partir daí o teste de posse que todos os
caminhos usam,

```pascal
if Guild.MemberInChest <> Guild.FindMemberFromCharIndex(Player.Character.Index) then
  Exit;
```

compara `-1` com `-1`: **passa**. Qualquer personagem que não esteja na lista
de membros daquela guilda mexe no baú dela. E o laço da thread de prazo
(`while not(MemberInChest < 0) …`) sai na primeira volta com `-1`, então nem o
prazo salva.

No porte o dono é um `CharIndex` e o baú fechado tem dono zero, que não é
`CharIndex` de ninguém.

#### Uma thread do sistema operacional por abertura de baú

`TCloseChestThread` é criada a cada `OpenChest`:

```pascal
Self.CloseChestThread := TCloseChestThread.Create(True);
Self.CloseChestThread.Guild := @Self;
Self.CloseChestThread.Start;
```

- Sem `FreeOnTerminate`, e nada chama `Free`: o objeto `TThread` vaza a cada
  abertura.
- O campo `CloseChestThread` é sobrescrito sem olhar o que estava lá: abrir o
  baú duas vezes deixa a primeira thread rodando e sem referência.
- O laço é `Sleep(50)` — para um prazo de dois minutos, 2.400 voltas por baú
  aberto, cada uma lendo `Now` e `MinutesBetween`.
- `Guild := @Self` guarda o endereço de um registro do vetor global. A criação
  de guilda faz `ZeroMemory(@Guilds[I], sizeof(Guilds[I]))`
  (`PacketHandlers.pas:8272`): a thread continua lendo o registro depois de
  ele ter sido zerado e reusado por outra guilda.

No porte não há thread por baú: `Registry.SweepChests` roda no tique de 15
segundos de `chestLoop` e percorre os baús abertos.

#### O prazo avisava mas não fechava — o baú ficava trancado até o reinício

Ao vencer o prazo, a thread faz exatamente isto e termina:

```pascal
Player.SendSignal(Player.Character.Base.ClientId, $F2F);
Break;
```

Não chama `CloseChest`. O único caminho que chama `CloseChest` é o pacote
`$F2F` **vindo do cliente** (`PacketHandlers.pas:8283`). Se o cliente não
responder, ou se o jogador cair, `MemberInChest` fica apontando para ele e o
baú da guilda fica trancado **para todos os membros até o servidor
reiniciar**. Nada no caminho de desconexão chama `CloseChest` — é a mesma
classe de vazamento do slot de dungeon.

E o handler `$F2F` compara com um campo diferente do que `OpenChest` gravou:

```pascal
if Guild.MemberInChest = Guild.FindMemberFromCharIndex(Player.Base.Character.CharIndex)
```

`Player.Base.Character.CharIndex` é o id real; `OpenChest` gravou o índice
achado por `Player.Character.Index`, que é zero. Os dois lados nunca podiam
casar.

No porte o baú fecha em quatro caminhos: o pacote do cliente, o prazo
(`ChestTick`), fechar a janela do NPC e a desconexão (`chestOnDisconnect`).

#### Dois membros duplicavam o item

`srcItem` e `destItem` são ponteiros crus para `Guild.Chest.Items[slot]`, na
memória global, e a troca é um `Move` de três linhas sem nenhuma proteção.
Dois membros da mesma guilda arrastando o mesmo slot ao mesmo tempo levavam
cada um uma cópia. O porte faz o movimento inteiro dentro de `Chest.Do`, com o
mutex do baú tomado; `TestBauNaoDuplicaItemComMuitosMembros` prova com 32
goroutines e `-race` que exatamente um leva o item.

#### Ouro: `abs(math.MinInt64)` e `UInt64(Value)`

`ChangeGold` recebe `Value: Int64` do cliente e usa `abs(Packet.Value)` nas
guardas de saldo. `abs(Low(Int64))` não cabe no `Int64` e **volta negativo**:

```pascal
if Player.Account.Header.Storage.Gold < abs(Packet.Value) then Exit;
if (Player.Character.Base.Gold + abs(Packet.Value)) > 2000000000 then …
```

Com `Value = -9223372036854775808` as duas comparações viram comparações com
um número negativo gigante e **passam**, e as somas seguintes levam o ouro da
bolsa e do armazém para valores absurdos. No porte esse valor é recusado logo
na entrada, com registro no log.

No baú a conta é feita em `UInt64`:

```pascal
Guild.Chest.Gold := Guild.Chest.Gold + UInt64(Packet.Value);
```

Para `Value` negativo o `UInt64()` é um número enorme que, por dar a volta,
acerta a subtração por acidente. A guarda que deveria impedir a retirada acima
do saldo compara um `UInt64` com um `Int64`. E no **depósito** não há teto
nenhum — o teto de 2.000.000.000 existe só no ramo da retirada, e ali ele é
conferido contra o ouro da *bolsa*, não contra o do baú. Um baú podia crescer
até dar a volta em 1,8e19 e ficar quase zerado. No porte `Chest.MoveGold` faz
a conta em `int64`, recusa a retirada acima do saldo e aplica
`MaxChestGold = 2.000.000.000` nos dois sentidos.

#### A guarda da janela de NPC existia só em metade dos caminhos

A opção `$7` (armazém) do case de `OpenNPC` chama `SendStorage` e **sai pelo
fim do case**, onde está

```pascal
Player.OpennedNPC := Packet.Index;
Player.OpennedOption := Packet.Type1;
```

Só que vários ramos saem antes com `Exit`, e o resultado é um par incoerente.
A consequência é visível no próprio código da origem: a guarda do depósito de
ouro no armazém está **comentada**:

```pascal
{if not(Player.OpennedOption = 7) or (Player.OpennedNPC <= 0) then
  Exit;}
```

e a do arrasto a partir do armazém também (`PacketHandlers.pas:3514`). No
destino do arrasto ela está ativa (`3669`). O efeito prático é que **tirar**
item e ouro do armazém funcionava de qualquer ponto do mapa, sem NPC nenhum, e
**guardar** exigia o NPC. No porte `storageOpen` vale para os dois lados, e
cada ramo do `openNPC` marca o que abriu.

#### Carga do baú com slot fora do vetor

`Load.pas:212` lê o slot do banco e escreve direto:

```pascal
ItemSlot := Byte(GuildQuery.Query.FieldByName('slot').AsInteger);
Guilds[Slot].Chest.Items[ItemSlot].Index := …
```

`Items` é `ARRAY [0..49]`. Um slot de 50 a 255 escreve depois do fim do vetor,
por cima de `Chest.Gold`, de `MemberInChest`, do **ponteiro**
`CloseChestThread` e do início da guilda seguinte. É o mesmo padrão já
corrigido no inventário e no armazém. No porte a faixa é conferida em
`store.Items.ByOwner` e de novo em `Chest.LoadItems`, que reporta o que
ignorou.

#### O ouro do armazém vinha truncado do banco

`Player.pas:875` lê um campo `Int64` com `AsInteger`:

```pascal
Self.Account.Header.Storage.Gold := PlayerSQLComp.Query.FieldByName
  ('storage_gold').AsInteger;
```

Um armazém com mais de 2.147.483.647 de ouro voltava truncado do banco, e
acima de 4.294.967.295 voltava negativo. No porte a coluna é lida como `int64`
e o negativo é cortado.

#### Gravação

Na origem o armazém e o baú só eram gravados por caminhos periódicos
(`TPlayer.SaveInGame` para o armazém, `TFunctions.SaveGuilds` para o baú), e
`SaveGuilds` reescrevia a guilda inteira e todos os membros num `UPDATE`
montado por concatenação de texto. Um depósito feito pouco antes de o servidor
cair era perdido.

No porte:

- o ouro do armazém é gravado a cada movimento (`accounts.storage_gold`) e na
  saída do jogador;
- os itens do armazém são gravados na saída, junto do inventário, com o id da
  **conta** como `owner_id` e `slot_type` 2 — igual à origem;
- o ouro do baú é gravado a cada movimento por `SaveChestGold`, um `UPDATE` de
  uma coluna;
- os itens do baú são gravados a cada arrasto e no fechamento, com o **índice
  da guilda** como `owner_id` e `slot_type` 3 — igual à origem.

#### Diferenças deliberadas

- **`$119` é o diálogo de dungeon, não `$348`.** Uma anotação anterior deste
  porte dizia que `CodeDungeonDialog` era `$348`; `$348` é o `CloseNPCOption`
  (`ServerSocket.pas:3171`). O diálogo é `$119`
  (`NPCHandlers.pas:586`). Corrigido, e `ShowDungeonDialog` agora está
  portado — com isso a opção 26 do NPC passa a marcar a janela e o fluxo de
  dungeon da fase 10r fica alcançável pelo cliente.
- **O nível mínimo do diálogo vem da tabela.** A origem lia sempre
  `DG<nome>[0].LevelMin`, o da primeira dificuldade, mesmo que o jogador fosse
  escolher outra.
- **O baú só troca com inventário e armazém.** A origem deixava qualquer
  combinação que passasse pelos dois `case` cair no `Move` final; equipamento
  já era barrado pela guarda do topo.
- **Os slots 84 e 85 do armazém são recusados.** São a central de pran, que a
  origem só deixava receber item de `ItemType` 10 — e lá a checagem usava
  `Player.Character.Base.Inventory[Packet.srcSlot]` mesmo quando a origem do
  arrasto não era o inventário, lendo o item errado. Pran não está portado.
- **Os slots 80 a 83 não se movem.** São as expansões do armazém, que a origem
  exigia presentes para liberar cada grupo de 20 slots e que o seu próprio
  `if (Packet.srcSlot < 80)` já barrava.
- **A "Símbolo da Confiança" ainda não abre o armazém.** O item de
  `ITEM_TYPE_STORAGE_OPEN` (226) abria o armazém pelo inventário
  (`ItemFunctions.pas:4902`); isso depende do handler de usar item (`$31D`),
  que não está portado. O armazém abre pelo NPC.
- **O teto da bolsa no saque é o `MaxGold` do porte** (999.999.999.999), não o
  2.000.000.000 da origem, para ficar coerente com o `/addgold` já portado. O
  teto do armazém e do baú é o 2.000.000.000 da origem.

#### Correção no próprio teste da fase 10r

`TestMobDaDungeonPegaAlvoEBate` foi escrito sem fonte de sorteio fixa, e
`dungeonMobAttack` cai no ramo `Miss` quando o sorteio dá de 2 a 15: o teste
falhava sozinho em cerca de 14% das rodadas. `handlerComDungeon` agora injeta
`randFixo{v: 50}`, como os testes de IA de mob já faziam.
