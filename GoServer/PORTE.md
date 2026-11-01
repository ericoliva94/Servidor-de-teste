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
| 10d | Console, entrada no mundo, movimento | `ConsoleSocket.pas`, `Player.pas` | a fazer |

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
