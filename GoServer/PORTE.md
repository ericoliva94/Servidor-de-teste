# Porte do servidor Aika para Go

Porte de `Server/Src` (Object Pascal, Delphi 12) para Go. O cliente continua
sendo o **Aika Global** original, binário fechado da Hanbitsoft: o Go substitui
o servidor falando o mesmo protocolo, byte a byte.

Alvo medido na origem: **53 units compiladas, 85.116 linhas** de Object Pascal
(contadas a partir de `Server/AikaServer.dproj`).

## Como rodar os testes

```bash
cd GoServer && go test ./...
```

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
| 6b | Combate: sorteio de tipo de dano, aritmética com limite | `BaseMob.pas`, `Util.pas` | **parcial** (tipo de dano pronto; pipeline de dano a fazer) |
| 6c | Jogador, NPC, pet | `Player.pas` (12.859), `NPC.pas`, `PET.pas` | a fazer |
| 7 | Handlers | `PacketHandlers.pas` (13.686), `NPCHandlers.pas`, `AuthHandlers.pas`, `CommandHandlers.pas` | a fazer |
| 8 | Sistemas | itens/refino, dungeon, guild, party, mail, auction, nation, eventos, ranked | a fazer |
| 9 | Laços de atualização | `UpdateThreads.pas` (3.942) | a fazer |
| 10 | Servidores e `main` | `ServerSocket.pas`, `LoginSocket.pas`, `TokenSocket.pas`, `ConsoleSocket.pas` | a fazer |

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
