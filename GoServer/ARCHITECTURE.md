# Arquitetura do servidor Go

Decisões já tomadas no código (ver `PORTE.md` para o porquê, caso a caso) e as
que esta auditoria recomenda. Nenhuma refatoração foi feita só por estética.

## Estrutura

```
cmd/aikaserver   main: config, MySQL, tabelas, sobe login (8831), canal (8822), HTTP token (8090), console (5555)
cmd/opdiff       auditoria: opcodes do Delphi sem contraparte Go
internal/protocol   cifra e enquadramento (byte a byte igual ao EncDec.pas)
internal/netsrv     servidor TCP, uma goroutine por conexão, ponto único de saída (DisconnectHandler)
internal/packets    structs binárias GERADAS (types_gen.go) + teste de binary.Size vs sizeof Delphi
internal/model, entity, stats, combat, skillfn, itemfn   regras de jogo puras, sem I/O
internal/game       handlers do canal; Session por conexão; Channel (registro); *Registry por sistema
internal/store      MySQL: uma struct por tabela, consultas nomeadas, transações
```

## Decisões vigentes

1. **Handler único, interfaces por persistência.** `game.Handler` tem um campo
   de interface por armazenamento (`MailDB`, `AuctionDB`, `GuildDB`,
   `FriendDB`…). Os testes injetam uma implementação em memória; `main` liga o
   `store.*`. Sem banco, o campo fica nil e o recurso cai para memória ou é
   recusado — nunca panic.
2. **Estado por sessão, não vetor global.** `Session` pendurada na `netsrv.Conn`
   substitui `Servers[n].Players[1..1000]`. `Channel` indexa por id, nome e
   usuário; id inválido não acha ninguém.
3. **Um ponto de saída.** Gravação, grupo, guilda, dungeon e (agora) amigos
   limpam em `Handler.OnDisconnect`, não em cada caminho que o Delphi lembrava.
4. **Validar na borda.** Todo índice vindo do cliente é conferido antes do uso;
   cada handler tem teste com entrada aleatória.
5. **Travas onde há estado compartilhado entre sessões.** `friendState` tem
   `sync.Mutex` porque aceitar/remover amizade muda a lista do *outro* jogador.
   (`PartyRequester`/`GuildRequester` ainda são escritos sem trava por outra
   goroutine — dívida a pagar assim que o `-race` estiver disponível.)

## Recomendado (não feito)

* **Multi-canal (parcial, incremento 36).** Com `-multicanal` o processo sobe um
  `Handler` e um `Channel` por entrada do `Channels` do `.ini`, cada um no IP do
  `SL.bin` (porta 8822). Os canais dividem guildas, nações, templos e o
  registro `Handoffs` da troca de canal; o personagem passa pelo banco, não pela
  memória. Faltam levar o grupo na troca, amigos e duelo entre canais, e um
  repositório de conta único para cash e armazém (hoje cada sessão relê o
  banco).
* **Tabela de despacho no lugar do `switch`** de `HandlePacket` (já tem ~55
  `case`): só vale quando a lista passar de 100 e se medir ganho; hoje o
  `switch` do Go compila para busca binária e é legível.
* **Protocolo de GM separado**: pacote `internal/gm` com seu próprio listener e
  autenticação, sem passar por `game.Handler`.
