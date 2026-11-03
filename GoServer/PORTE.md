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
| 10h | Inventário em jogo: mover (`$70F`), descartar (`$32C`), empilhar, gravar na saída | `PacketHandlers.pas`, `ItemFunctions.pas`, `Player.pas` | **pronto** (inventário↔equipamento; armazém, baú de guilda e pran a fazer) |
| 10i | Atributos derivados (`GetCurrentScore`), ataque básico (`$302`), morte e revive (`$303`) | `BaseMob.pas`, `PacketHandlers.pas` | **pronto** (sem skill, dungeon, pran nem mascote) |
| 10j | Mobs: carga dos dois CSV, spawn (`$35E`), visibilidade, golpe, morte, experiência (`$108`) e respawn | `ServerSocket.pas`, `MOB.pas`, `Player.pas` | **pronto** (sem IA de movimento nem ataque do mob) |
| 10k | IA de mob: patrulha, perseguição e golpe no jogador | `MOB.pas` (`MobHandler`, `MobMoviment`, `AttackPlayer`) | **pronto** (sem skill de mob nem guarda) |
| 10l | Grupo: convidar (`$322`), aceitar (`$323`), sair/expulsar (`$324`), desfazer (`$325`), liderança (`$34B`), pedir entrada (`$38F`), estado (`$326`), posição (`$11D`) e conversa de grupo | `PartyData.pas`, `PacketHandlers.pas`, `Player.pas` | **pronto** (sem raid) |
| 10m | Skills: aprender (`$31C`), lançar (`$320`), zerar (`$32A`), cancelar (`$327`), listas (`$106`/`$107`), recarga, custo de MP e buff | `SkillFunctions.pas`, `PacketHandlers.pas`, `BaseMob.pas` | **pronto** (motor genérico; os casos especiais por skill ficam para a fase seguinte) |
| 10n | Drop: carga das 13 listas, sorteio, item no chão (`$33B`), coleta (`$336`) e expiração | `Load.pas`, `BaseMob.pas` (`DropItemFor`), `ServerSocket.pas` (`CreateMapObject`) | **pronto** |
| 10o | Guilda: criar (`$341`), convite (`$F7B`/`$67D`/`$F7E`), expulsar (`$F2D`), sair (`$F1C`), rank (`$F1D`), mestre (`$619`), avisos (`$F20`), site (`$F21`), permissões (`$F22`), info (`$965`), membros (`$97F`) e conversa de guilda | `GuildData.pas`, `PacketHandlers.pas`, `Player.pas` | **pronto** (sem baú, aliança, skill de guilda nem cerco) |
| 10p | Console 5555 | `ConsoleSocket.pas` | a fazer |

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

**`gamedata.Tables.BuffSeconds`** — `SkillData[Index].Duration` com
`SkillData: ARRAY [0..11998]` acessado sem checagem: índice de buff acima da
faixa lia memória fora da tabela. Agora índice inválido responde 0, e a duração
lida do arquivo é limitada a 24 h para um valor absurdo não virar buff eterno.
