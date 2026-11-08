# Problemas conhecidos, limitações e bloqueios

Atualizado em 2026-11-07.

## Ambiente

* **`go test -race` não executa nesta máquina**: `go: -race requires cgo` e não
  há `gcc` no PATH. O `README.md` pede `-race` antes de PR. Todos os testes
  rodaram **sem** o detector. Para habilitar: instalar um compilador C
  (MSYS2 `mingw-w64-ucrt-x86_64-gcc` ou WinLibs) e rodar
  `CGO_ENABLED=1 go test ./... -race`. Até lá, a segurança de concorrência dos
  códigos novos (a `friendState` tem trava e teste em paralelo) está
  *argumentada*, não comprovada pelo detector.
* **Python não está instalado** (só o atalho da Microsoft Store). O gerador
  `gen/emit.py` não pôde ser executado; as 319 structs já geradas estão
  versionadas e testadas, e a ferramenta nova de auditoria (`cmd/opdiff`) foi
  escrita em Go. Se o gerador precisar rodar de novo, instalar Python 3 ou
  portá-lo para Go.
* **Sem MySQL e sem cliente Aika Global** disponíveis aqui. `internal/store` é
  testado só nas partes puras; as consultas SQL (inclusive as novas de
  `store/friends.go`) **não foram executadas** contra um banco.
* **Sem repositório Git** no diretório recebido. Ponto de restauração:
  `Downloads/Servidor-de-teste-main.zip`.

## Lista de amigos (incremento 1)

* **Cidade (`City`) vai zerada** nos pacotes `$870` e `$96F`. Na origem vem de
  `GetCurrentCityID`/`CurrentCity`, que depende da tabela de mapas
  (`Map.bin`, hoje lida só em parte — 1.024 de 69.184 bytes) e ainda não está
  portada. O cliente provavelmente mostra "cidade desconhecida" para o amigo.
  **Não validado** contra o cliente.
* **`$975` não é reenviado ao mudar de cidade/teleportar** (a origem chama
  `RefreshMeToFriends` no `Teleport`). Depende do item acima.
* **Amigos só dentro do mesmo canal.** A origem varre `Servers[0..N]`; o Go sobe
  um canal só. Quando a troca de canal for portada, `onlineFriend` precisa
  consultar todos os canais.
* **`Header.Index` de `$96F`, `$975` e `$971` vai zero.** Na origem os dois
  primeiros eram zerados (`ZeroMemory`) e o `$971` tinha lixo de pilha. Se o
  cliente exigir um índice, será visível ao testar com o cliente real.
* O apelido (`Nick`) que o destinatário vê numa conversa é o do **autor**. A
  origem repassava o que o cliente mandou. Se o cliente real preenche `Nick`
  com o nome do *destinatário* no pacote de ida, nada muda para quem envia
  (o eco é o pacote original); só vale confirmar o lado de quem recebe.
* Mensagens de erro foram escritas **sem acentos**, como o restante do Go
  (`"Jogador nao encontrado."`), enquanto o Delphi usava acentos em ANSI. Se o
  cliente exibir caracteres errados com um dos dois, ajustar em
  `game/friends.go`.

## Bloqueios

* **Loja de NPC (`0x313`, `0x314`)**: faltam os dados originais das lojas.
  Não serão inventados.

## Dívida herdada (já documentada em `PORTE.md`)

* `ItemList.bin` tem 26.000 itens e a tabela comporta 20.478; 5.522 nunca são
  lidos (idem 1 skill, `Map.bin`). Comportamento da origem, reproduzido.
* Estado de conta por sessão (cash, armazém): dois canais ao mesmo tempo
  divergiriam. Precisa ser resolvido junto com a troca de canal.

## Renovar item (incremento 3)

* **O prazo novo conta a partir de agora**, como na origem. Renovar uma montaria
  que ainda tem 30 dias com uma licença de 7 dias deixa 7 (a mensagem diz "por
  mais N dias"). Mantido por fidelidade; se for considerado defeito, trocar
  `h.now()` por `max(agora, prazo atual)` em `game/renew.go`.
* `BASE_DATETIME` ('31/12/1969 22:00') foi lido como UTC. A origem usa hora
  local; só afeta a contagem de dias da montaria se a máquina não estiver em
  UTC−2. Não validado.

## Refino e aparência (incremento 4)

* **Mensagem ao cliente corrigida de `0x100`/145 bytes para `0x984`/144 bytes e
  tipo 16** (ver `MIGRATION_PROGRESS.md`). Afeta TODAS as mensagens do porte
  (amigos, itens, grupo, guilda…). Não testado com o cliente real.
* `0x32D` só tem refino e aparência. Encantamento (tipos 1, 11, 13) é recusado
  com aviso; o `opdiff` mostra o código como coberto mesmo assim.
* A troca de aparência não confere se os dois itens são do mesmo tipo de
  equipamento (como na origem). Se o cliente mostrar peça trocada, adicionar a
  comparação de `itemfn.EquipSlot` em `changeAppearance`.
* O material de refino só é validado por `Rank` e por empilhar (a origem nem
  isso); o `ItemType` correto do "Hira Kaize" não está em nenhum dado do
  repositório. Se existir regra de tipo, precisa ser informada.
* Mensagens sem acentos, como no resto do Go.

## Reparo (incremento 5)

* **Sem conferência de NPC.** Como na origem, o `$340` repara de qualquer lugar
  do mapa (o cliente só manda perto do NPC, o servidor não confere). Para exigir
  o NPC, usar `Session.OpenedNPC`/`OpenedOption` como o armazém faz — falta saber
  qual opção de NPC é a de reparo.
* O preço usa `float32` no total, como a origem, para bater com o valor que o
  cliente exibe; se o cliente mostrar valor diferente, a causa provável é a
  ordem das somas.
* Reparo de equipamento não recalcula atributos (a origem também não).

## Forja (incremento 6)

* **Decidido (2026-11-07): manter como a origem — fabrica sempre.** A origem fabrica sempre (ver
  `PORTE.md`); as 10 receitas com `TaxSuccess` 500 parecem pensadas para 50%.
  Para ativar a chance real, trocar a conta em `craftItem` por
  `1+IntN(1000) <= TaxSuccess`. Não mudei porque é balanceamento.
* **Fabricação inferida.** O desbloqueio da fabricação (limite de slot só nos
  modos de slot) é dedução do código; se o cliente enviar outro valor em
  `ItemID` na fabricação, a receita não será achada ("Item nao encontrado na
  forja.").
* Item fabricado ou obtido na desmontagem com `Expires`/`CanSealed` sai sem
  prazo/selo. Pran (`ItemType` 10) e mascote também não têm tratamento.
* Tratamento especial de `PutItem` não portado: item 5300 (bolsa) e aviso de
  tesouro sagrado (`ItemType` 40).

## Diversos de personagem, lote 1 (incremento 7)

* **A gravação do progresso só roda na saída** (`OnDisconnect`). Queda do
  processo perde tudo da sessão; a origem também gravava em pontos soltos
  (`SaveInGame`). Um temporizador de gravação periódica é o próximo passo
  natural. A consulta SQL nova **não foi executada** contra MySQL.
* `active_title` **não** é gravado: o `Avatar` guarda o título em 8 bits e
  gravar de volta truncaria títulos acima de 255. Entra com o handler `0x361`.
* **Buffs não são enviados ao cliente ao serem aplicados** (`applyBuff` só põe
  na lista); o `$16E` agora existe (`sendRefreshBuffs`) e é usado só no
  cancelamento. Chamá-lo também em `applyBuff` é o próximo passo, mas não está
  confirmado como a origem avisa o cliente na aplicação (`SendAddBuff`, `$?`).
* PvP (`0x307`): a recusa de ligar o PvP durante duelo não existe (duelo não
  portado).
* `0x668` responde só "função desativada", como a origem.

## Diversos de personagem, lote 2 (incremento 8)

* **Atributos detalhados são parciais** (`0x23FE`/`0x23FB`). A origem soma a
  esses campos efeitos de equipamento e buff (`GetMobAbility`: regeneração,
  imunidades) e `PvPDamage`/`PvPDefense`; o porte ainda não calcula esses
  efeitos, então a tela mostra só a parte do personagem (regeneração =
  Constituição × 3; resistências = `Resistence`; dano/defesa PvP = 0). O
  cliente vai exibir números **menores** que os reais para quem usa
  equipamento com esses efeitos.
* **`0x361` Título ativo — bloqueado por dependência.** Precisa da lista de 96
  títulos do personagem (cada um com nível) e dos efeitos de título
  (`SetOnTitleActiveEffect`), que o porte não carrega. Só `active_title`
  (um número) é lido hoje.
* **`0x359` Item de evento — bloqueado por dependência.** Chama
  `GetAllEventItems`/`PutItemOnEvent`/`DiaryItemAvaliable`, o sistema de
  eventos e do diário, ainda a fazer (fase 10x).
* **Skill de pran na barra (`SrcType` 3)** recusada com aviso.
* A barra aceita qualquer skill dentro da tabela, mesmo não aprendida (a
  origem também não conferia).
* O item linkado no megafone é recusado (a origem mandava a todos sem gastar o
  item); o grito usa o mesmo limite de 5 s do grito comum, mas, como na
  origem, vai a todos (o grito comum filtra por nação).
* Posições de teleporte vazias são gravadas como `0,0;` para manter o slot;
  uma posição realmente em (0,0) é tratada como vazia.

## Criar personagem (`0x3E04`) — implementado com skills pela regra (decidido)

* **Os modelos de personagem inicial não batem com o layout do Go.** O Delphi
  cria o personagem copiando `InitialAccounts[classe]`, lido de
  `Server/Bin/Data/BaseAccs/*.acc` (6 arquivos de **7.249 bytes**), gravados
  com `Write(F, TBasicCharacter)`. O gerador (`gen/emit.py`) calcula
  `TBasicCharacter` em **5.081 bytes** (`model.BasicCharacter`). Diferença de
  2.168 bytes. Conferi à mão contra o `Guerreiro.acc`: o começo bate
  exatamente (classe no offset 33, atributos no 36, nível no 188, equipamento
  no 344, inventário no 668), mas **a partir daí o arquivo e o layout divergem**:
  na região onde o Go espera as skills (offset 4.265) o arquivo está zerado, e
  os dados que parecem skills estão em ~6.432. Ou o `.acc` foi gravado por uma
  versão do servidor com outro `TCharacterDB`, ou o gerador erra o tamanho de
  algum tipo no meio (suspeitos: `TQuest`, `TSkillsList`, `TBuffsList`).
  O registro `model.Character` (4.216 bytes), usado no `$925`, também depende
  desse cálculo.
* O que **dá para usar com segurança** do `.acc`: classe, atributos, tamanhos
  do corpo, nível, equipamento (slots 2 a 15) e inventário — tudo antes do
  offset ~1.948, conferido. O que **não dá**: as skills iniciais. A
  alternativa seria a regra de `CreateBasicCharacter`/`GetSkillIndex`
  (usada quando o `.acc` não existe), mas ela produz equipamento diferente do
  `.acc` real (ex.: `Equip[2]` = 2834 no arquivo, ausente no código).
* **Decisão que preciso de você:** (a) criar o personagem com atributos/itens
  do `.acc` e skills pela regra do código, aceitando a inferência; (b) você
  informa o `TCharacterDB` correto (ou o tamanho real de `TBasicCharacter` na
  build do Delphi) para eu consertar o gerador antes; ou (c) deixar a criação
  de fora por enquanto.
* O resto da criação está mapeado e é direto: validações (14 caracteres, só
  letras/números, nome único, slot ≤ 2, classe 10–69, cabelo 7700–7731),
  `Equip[0]` = classe e `Equip[1]` = cabelo, bolsa 5300 em `Inventory[60]`,
  balas nas classes 3 e 4, posição por cidade, e a gravação transacional
  (`characters`, `items`, `skills`, `itembars`). A origem também sobrescrevia o
  slot 80 do armazém da conta (`Storage.Itens[80] := 5310`) a cada criação.

## Exclusão de personagem (incremento 9)

* Mensagens sem acentos, como o resto do Go. Não validado no cliente.
* A exclusão tira o personagem da guilda em memória (`Guilds.Leave`); a
  persistência da guilda segue o mesmo caminho de `exitGuild`.
* Correio e leilão do personagem excluído ficam no banco (a origem também não
  os apagava).
* O `Delete` do `store` usa uma transação, mas **não foi executado contra
  MySQL**.

### Criação de personagem — o que ficou aproximado (incremento 10)

* **Skills iniciais = inferência.** Vêm da regra do código (`GetSkillIndex`),
  não do `.acc`. Se o `.acc` real tiver outras skills, os personagens novos
  começarão com um conjunto diferente do de produção. A divergência de tamanho
  entre os `.acc` (7.249 bytes) e o registro calculado pelo gerador (5.081)
  continua sem explicação e pode afetar o `$925`.
* **HP e MP iniciais:** gravados como `0x7FFFFFFF` e limitados ao máximo no
  primeiro login (a origem gravava 0 e a entrada trata 0 como 1 de vida, o que
  deixaria o personagem novo com 1 HP). Escolha minha; reverter em
  `store.FullVitals`.
* **Barra inicial:** só guerreiro e clériga recebem a quarta skill básica no
  atalho 3 (regra de `CreateBasicCharacter`); os `.acc` não foram lidos nesse
  trecho (offset > 3.400, não conferido).
* **Cabelo e face:** `Equip[0]` = índice da classe do pacote e `Equip[1]` =
  cabelo; `APP` fica zero, como na origem.
* **Posição:** cidades 0 e 1 têm coordenadas; qualquer outro `Local` grava
  (0,0) e o login usa o ponto de partida padrão.
* Se faltar um `.acc` (ou ele estiver inválido), aquela classe não pode ser
  criada e o servidor avisa no log. A origem gerava o arquivo com
  `CreateBasicCharacter`; não portado porque o equipamento que o código gera
  é diferente do dos arquivos de produção.
* Não validado no cliente nem contra MySQL.

## Comércio (incremento 11)

* **Regra nova, não existe na origem:** qualquer mudança na oferta (itens ou
  ouro) desfaz "pronto" e "confirmado" do **parceiro**. Se o cliente real
  mandar o novo estado de "confirmado" do parceiro por conta própria, vai
  divergir do servidor; aparece como a janela do parceiro voltar a "não
  confirmado" depois de uma mudança. Não validado no cliente.
* A origem não conferia distância entre os jogadores; o porte também não.
* Mensagens sem acentos.
* `tradeMu` é uma trava global (uma troca por vez em todo o servidor). Basta
  enquanto trocas são raras; se virar gargalo, trocar por trava por par.
* **O inventário ainda é mexido por outros handlers sem trava** (mover item,
  descartar...). A execução da troca confere cada item contra a fotografia e
  cancela se algo mudou, mas dois handlers escrevendo no mesmo slot no mesmo
  instante continuam sendo uma corrida que só o `-race` revelaria (indisponível
  nesta máquina).

## Relíquia e devir (incremento 12) — núcleo pronto, templo não abre

* **`$E51` só funciona se `Session.OpenedDevir`/`OpenedTemple` forem
  preenchidos, e nada os preenche.** Na origem quem faz isso é o diálogo do
  NPC do templo (`PacketHandlers.pas` ~1689–2300, ~600 linhas), que por sua vez
  depende das **pedras e dos guardas do devir** (`StonesDied`/`GuardsDied`,
  `OpenDevir`/`CloseDevir`, a thread `TDevirOpennedThread`, `SecureAreas`).
  Nada disso está no Go (o porte não tem NPCs nem a fase "nação"). Hoje, num
  servidor Go de verdade, o `$E51` responde "Nenhum templo aberto."; os
  testes preenchem os campos à mão. **O `opdiff` conta o `0xE51` como coberto,
  mas ele é inalcançável até o diálogo do NPC do templo ser portado.**
* O `$B52` funciona (lê o estado carregado do banco).
* **Os efeitos das relíquias (`ReliqEffect`) ainda não chegam às contas de
  jogo** (regeneração, dano etc.): o `Registry.EffectValue` guarda o valor,
  mas `GetMobAbility`/`stats` não o consultam. É o mesmo buraco dos atributos
  detalhados (`0x23FE`).
* **Um servidor Go = uma nação.** A origem tem um `TServerSocket` por canal,
  cada um de uma nação (`Servers[ch].NationID`); o Go sobe um canal e usa a
  flag `-nation` (padrão 1) para escolher quais 5 templos carregar. A pergunta
  por "canal" do `$B52` é ignorada.
* Não portado: `UpdateReliquaresForAll` (empurrar o estado novo a todos os
  jogadores depois de entregar/roubar; hoje cada cliente só vê o novo estado
  quando pede o `$B52`), `Player.UpdateReliquareOpennedDevir`, `CloseDevir`, a
  raid aliada na divisão de honra, e o item raro de tesouro sagrado dar o
  aviso à nação inteira ao ser pego (`PutItem` com `ItemType` 40).
* `TimeCapped`/`TimeToEstablish` voltam do banco só pelo `timecap`
  (`+2 h`), como a origem; o `APP` da relíquia volta igual ao índice.
* Mensagens sem acentos. Nada foi executado contra MySQL.

## Loja pessoal e posição de membro (incremento 13)

* **Não validado no cliente.** Em especial: o `Header.Index` `$7535` da vitrine
  (`$319`) foi copiado da origem; o eco do `$318` ao fechar a própria loja usa o
  `ClientID` do dono (a origem ecoava o `Header.Index` recebido); e o
  `respawnToVisible` reenvia o `$349` com o nome da loja no lugar do título e
  `EffectType` 2, como `GetCreateMob` já fazia.
* **Regras novas, não existem na origem:** loja sem item válido é recusada; o
  comprador precisa ver o dono na tela; abrir loja e negociar são exclusivos
  (convite de troca é recusado com loja aberta); o preço é limitado a
  `MaxGold`. Se o cliente real mandar um caso que a origem aceitava, vai
  aparecer como "Nenhum item válido para vender." ou sem reação ao clicar na loja.
* O dono pode mover/gastar o item depois de abrir a loja: a vitrine **não** é
  atualizada na hora; o produto só é retirado quando alguém tenta comprá-lo
  ("O item não está mais disponível."). Atualizar a vitrine a cada mudança de
  inventário exigiria um gancho em `moveItem`/`deleteItem`/uso de item.
* Andar ou morrer com a loja aberta **não** fecha a loja (a origem também não).
* `$338`: raid (grupos aliados) não portada; a origem deixou a dúvida anotada
  (`//ver como ficam as raids aqui`).
* Mensagens sem acentos. Mesma trava global `tradeMu` do comércio.

## Skills de área e buffs (incremento 16)

* **Genérico, não por skill.** O efeito de cada buff vem só de `EF/EFV[0..3]` da
  `SkillData`; o dano em área é o mesmo cálculo da skill de alvo único. Tudo que
  a origem faz em `SelfBuffSkill`/`TargetBuffSkill`/`*AreaSkill` por índice de
  skill (escudos com contador, lâmina, regeneração por evento, teleporte,
  invisibilidade, cura, execução, empurrão...) **não** existe: a skill sai, mas
  sem esse efeito. É o próximo passo natural, classe por classe.
* **Cura:** feita no incremento 17 para a Clériga e o Remediar; El Tycia (39,
  templário) segue sem efeito.
* **Pran** continua recusado ("Habilidades de pran ainda não estão disponíveis").
* **Dungeon e duelo:** área não atinge mobs de dungeon (já era o caso do alvo
  único) nem respeita duelo/aliança de guildas.
* **Percentuais de HP/MP:** a origem aplica marechal, relíquia e buff em passos
  seguidos sobre o total; `stats.MaxHP` soma os percentuais e aplica uma vez.
  Pode diferir em alguns pontos de HP com vários percentuais ativos.
* Buffs **não são gravados**: sair do jogo os perde (a origem os guardava em
  `PlayerCharacter.Buffs`).

## Cura e Clériga (incremento 17)

* **Dano contínuo (`SKDListener`)** de Água Benta (457) e Raio Solar (133) não
  existe: Raio Solar entra como debuff genérico da tabela, sem o dano por
  segundo. O mesmo vale para as outras skills de dano contínuo das demais classes.
* **Glória de Excelis (167)** cura uma vez e dá o buff; a regeneração de 10
  ciclos (`HPRAction 4`) da origem não foi ligada porque o ponto que a liga não
  está na parte lida.
* A **ordem em que debuffs saem** na Limpar é a do índice; a origem percorria um
  dicionário sem ordem. Pode divergir de qual debuff sai primeiro.
* Cura só em **jogadores**; mobs, pets e pran como alvo não existem.
* Mensagens sem acentos; "Seu HP foi restaurado em N" usa o tipo amarelo (16)
  como a origem.
* O flake de `asPacket` (colisão do "hello") foi corrigido nos testes; no mundo
  real continua 1 em 65.536 por primeira conexão, como o framer documenta.

## Guerreiro e Templário (incremento 18)

* **Bolha só vale para skill**, não para golpe básico (a origem também: exige
  `Skill > 0`). Atenção: o cliente real pode mostrar a mensagem de resistência
  em tipo diferente do amarelo (a origem usa tipo 16 com parâmetros 1,1).
* **Resistência a estados usa a taxa do atacante** (`Resistence` do autor da
  skill), como no Delphi. Parece invertido, mas é o comportamento original.
* **Resoluto, Lâmina, Pedra da Alma, Requiem, Estripador** e as skills de dano
  contínuo (veneno, Sacrifício) não existem; as de debuff entram só com o efeito
  da tabela (`EF/EFV`), sem provocação, recuo ou sangramento.
* O avanço é reaplicado em quem **está a vista**; não confere obstáculo/parede
  (a origem também não).
* União Divina procura o lançador **pelo nome**; renomear/trocar de personagem
  durante o buff solta o buff.

## Dano contínuo (incremento 19)

* O tick é de **1 segundo** do laço de buffs, não a thread de 1 s da origem; a
  fase dentro do segundo pode divergir. Número de ticks ≈ `Duration` (a origem
  fazia `Duration + 1`).
* Dano contínuo em mob é avisado com o **pacote de dano normal** (`$102`, tipo
  normal), não com o `$103` de HP que o `RemoveHP` da origem mandava; o cliente
  pode mostrar o número de dano.
* Mobs sob dano contínuo **não** são levados em conta por `onMobDeath`/drop se
  morrerem por outro golpe depois: o registro só expira (por tempo ou morte).
* Não há dano contínuo em dungeon.

## Atirador e Dual Gunner (incremento 20)

* **Reação em Cadeia, Bomba Maldita e Dor do Predador** têm o efeito só descrito
  em comentário no Delphi (dano por quantidade de debuff/buff, vida a cada
  ataque); entram como dano comum com o efeito da tabela.
* **Estripador (90)** não dispara o buff 6279 quando o alvo leva crítico.
* **Imunidade total** (`EF_IMMUNITY`) e bloqueio por `EF_COUNT_HIT` não são
  consultados nas skills de só-teste (Eliminação, Perseguidor...): o golpe vale
  se não errar.
* O sorteio de Sentença usa `<=`, como a origem (`Helper <= DamageRange − 20`),
  então a chance real é de 1 ponto a mais do que o número da tabela sugere.

## Mago (incremento 21)

* **Polimorfo** entra como buff com o efeito da tabela; o jogador **não** muda
  de aparência e o pipeline não aplica o "golpe vira duplo crítico".
* **Vínculo** pode matar o próprio Mago (a origem usa `RemoveHP` sem o piso de 1).
* O dano de skill ainda não desconta a **redução de dano do equipamento** do alvo
  (`GetEquipedItensDamageReduce div 10`), que `GetDamage` aplica em alvo jogador.
* Skills em área do Mago (Meteoro etc.) seguem no dano genérico de área, agora
  mágico para Mago e Clériga.
* Todo teste de classe precisa de skill com a `Classe` da própria classe; um
  Mago com skill de `Classe 1` é recusado ("não pertence a sua classe").

## Skills em área (incremento 22)

* O centro e o raio da área seguem a regra do incremento 16; skills de **longa
  duração** (`UsingLongSkill`: Tempestade de Raios, Enxame de Escuridão) não
  repetem o golpe com o tempo, só acertam uma vez.
* Área em **dungeon**, **duelo**, **guardas e pedras de devir** e **pets** não
  existe.
* Os testes de classe que usam Atirador/Dual precisam de bala no slot 15, como
  em jogo; sem ela a skill é recusada ("sem balas").

## Skills restantes (incremento 23)

* As relíquias largadas pela invisibilidade viram **objeto de item comum** no
  chão, não o objeto de relíquia (`CreateMapObject` tipo 320) da origem.
* O Resoluto marca e dá o buff, mas **não empurra** o alvo para trás.
* O Teleporte não conhece guerra de castelo (cerco não portado).
* Pedra da Alma consome o buff mesmo se o dano seguinte ainda matar o alvo
  (a origem também setava a vida e depois descontava o dano).
* Lâmina lança a área uma vez por segundo pelo laço de buffs; a origem usava um
  laço próprio (`EventListener`) com outro intervalo.

## Raid (incremento 24)

* O **chat de legião**, a divisão de **exp e drop** entre os grupos da legião e o
  `SendToRaid` não foram portados: a legião só aparece na tela de grupo.
* Quando um grupo é **desfeito** (ou todos saem), ele sai da legião, mas os
  grupos que ficaram **não são reenviados** na hora; veem a lista nova no
  próximo `$326`.
* O ramo de aliança de guildas do `$343` foi portado no incremento 27.
* `ExitRaid` agora exige o **líder do grupo**; se o cliente real deixa membros
  comuns pedirem a saída, isso aparece como "Só o líder do grupo pode sair da
  legião."

## Duelo (incremento 25)

* Sem multi-canal, o duelo só existe dentro do canal (a origem chamava
  `ChangeChannel2` no `$396`).
* Quem está em duelo **não** fica protegido de mobs: o golpe de mob que mataria
  também deixa 10 de vida (a regra está em `applyDamage`), mas o duelo só acaba
  no próximo tick.
* O titulo da bandeira usa os nomes dos personagens; nomes longos são cortados
  em 31 caracteres.

## Nação e aliança (incremento 27)

* A **gravação das guildas** continua só no bau (`SaveChestGold`): entrar, sair
  e mudar rank não gravam. A aliança grava as guildas afetadas com `GuildDB.Save`
  ao aceitar, mas se o banco falhar o estado só existe em memória até lá.
* O **cerco** não está portado: o `$91A` leva só os defensores (grade) e os
  atacantes saem vazios. A nação aliada (`nation_ally`) é enviada como está.
* Só `taxas` e `guildas de grade` são gravadas em `nations`; tesouro só é lido
  (nada o altera ainda).
* Quem não tem nação usa a do canal para o `$936`, como a origem; já o marechal
  exige a nação da conta (também como a origem).
* Não há **chat de aliança**. As SQL de `nations` e das colunas de aliança em
  `guilds` não foram exercitadas contra MySQL.

## NPCs (verificado depois do incremento 27)

* O sistema de NPCs **não foi iniciado**: a origem carrega cada NPC de
  `C:\Database\NPCs\<nome>.npc` (cabeçalho com opções + personagem base,
  posição e ClientID). Nenhum `.npc` existe no repositório nem em
  `C:\Database` desta máquina (só `ACCS` e `BaseAccs`). Sem eles, qualquer
  posição, nome ou menu de NPC seria inventado. Só `NPCOptionsText.bin`
  (textos das opções) e `Dialog.bin` estão disponíveis.

## Pran (incremento 29)

* Só a **entrada no mundo** está portada. Invocar e guardar a pran pelo item,
  vê-la andar com o dono, passivas, exp, comida, evolução, bolsa e equipamento
  da pran ainda não existem; o `$349` vai apenas ao dono, nunca a quem passa por
  perto.
* Quem tem a pran com o item no `Equip[10]` vê os números dela, mas as passivas
  e os bônus do equipamento da pran **não** entram nos atributos do dono.
* O item de pran continua sendo recusado nos movimentos de inventário
  (`ErrPranNotPorted`) e a barra de pran responde "Pran ainda nao esta
  disponivel".
* As consultas de `prans`, `items` (tipos 5 e 6), `skills` (tipo 3) e
  `itembars` da pran não foram exercitadas contra MySQL.

## Pran: passivas e equipamento (incremento 30)

* Os efeitos só ligam na entrada no mundo com a pran no `Equip[10]`. Como ainda
  não há invocar/guardar nem trocar o equipamento da pran em jogo, nada os
  desliga ou refaz até o próximo login.
* Faltam efeitos de **conjunto** da roupa da pran e `EF_PRAN_REGENHP/MP`,
  `EF_PRAN_PARRY` e `EF_PRAN_SKILL_DAMAGE`.
* Roupa de pran **vencida** ainda conta (`Expires` não está portada).

## Pran: experiência, nível e comida (incremento 31)

* A exp da pran sai só do abate **individual** de mob (`onMobDeath`); abates em
  grupo e em dungeon não a entregam.
* Sem **evolução de classe**, uma pran parada nos níveis 4, 19 ou 49 não passa
  dali (é a regra da origem; a evolução é uma fatia futura).
* **A conferir com o cliente real:** nos marcos de nível a origem sobe o `Level`
  da habilidade **e** avança o `Index` dela quando está na barra; como a
  passiva lê `SkillData[Index + Level - 1]`, isso pode andar duas linhas por
  marco. Foi mantido como está; só um teste com dados reais confirma.
* A barra da pran (`ItemBar`) em memória não acompanha o `Index` novo (a origem
  só avisava o cliente), então o marco seguinte não reencontra essa habilidade
  na barra.

## Pran: invocar, guardar e evolução (incremento 32)

* `evolvePran` está pronto e testado, mas **inalcançável**: as quests 406 e 407
  só terminam em NPC, e o sistema de NPCs e de quests não existe. Sem ele, uma
  pran parada nos níveis 4, 19 ou 49 não passa dali em jogo.
* O item de pran do dono (`Equip[10]`) tem o **índice trocado** na evolução
  (104 ou 105), como a origem; o `Identific`, que liga o item à pran, continua.
* Não é possível ainda mover itens para o equipamento ou para a bolsa **da
  pran** (tipos de slot 5 e 6 do `MoveItem`): a pran carregada do banco mostra
  o que tem, mas o jogador não troca as roupas dela em jogo.
* A pran não **anda** atrás do dono: ela fica na casa sorteada ao invocar e
  reaparece ali para quem chega depois.
* Pran e dungeon: a origem mostrava e escondia a pran ao entrar e sair de
  dungeon; isso não foi portado.

## Pran: itens no equipamento e na bolsa (incremento 33)

- **Não testado com o cliente real.** Validado só por testes automatizados.
- O equipamento da pran só troca com o inventário do dono. A origem deixava o tipo de slot 5 ir ao armazém e ao baú de guilda sem nenhuma conferência; isso foi fechado de propósito.
- Ao tirar uma peça do equipamento da pran a origem comparava o slot da peça com o slot do **inventário** de destino (quase sempre recusava); o Go compara com o slot da pran. Vale conferir com um cliente real se alguma peça deveria ser recusada.
- O refresh de slot (`$F0E`) do item de pran e da montaria continua no formato normal; a origem tinha formatos próprios.
- Apagar na bolsa da pran aceita os slots 0 a 41, como a origem (as duas bolsas podem ser apagadas); mover só os slots 0 a 39.

## Títulos (incremento 34)

- **Só os títulos que já estão no banco aparecem.** Ganhar progresso e subir de nível um título (abates, pesca, arena etc.) não existe: o contador de cada tipo não é alimentado.
- A tabela `titles` é regravada por completo (DELETE + INSERT numa transação) a cada gravação periódica e na saída. Nenhuma dessas consultas foi executada contra MySQL real.
- Título ativo com nível que não existe na tabela de títulos (`Title.bin`) fica sem efeito, sem erro. O `Title.bin` do repositório tem bytes além dos 255 títulos que a origem lia; o excedente é ignorado como na origem e não se sabe o que é.
- `EnsureLeaderTitle` (títulos de Marechal/Archon) não foi portado: na origem ele alterava uma cópia do registro e nunca teve efeito.

## Item de evento (incremento 35)

- A fila de itens de evento é a tabela `items` com `slot_type = 17`. Nenhuma consulta nova foi executada contra MySQL real.
- O prêmio diário (item 10467) conta 24 h pelo relógio do servidor, gravado em `characters.last_diary_event` como texto (segundos desde 1970). Coluna vazia ou inválida vale como "nunca recebeu".
- Se o inventário encher no meio da entrega, o item que não coube volta para a fila; um item empilhável de muitas unidades pode ocupar mais de um slot.

## Troca de canal e multi-canal (incremento 36)

- **Nunca rodou com o cliente real.** A troca foi validada com dois handlers no mesmo processo; o servidor foi subido com `-multicanal` e as duas portas abriram, mas nenhum cliente trocou de canal.
- `-multicanal` é opcional. Sem ele sobe um canal só em `:8822`. Com ele, cada canal escuta no IP do `SL.bin` (porta 8822): no arquivo do repositório, `127.0.0.11` e `127.0.0.12`. Isso muda de `:8822` (todas as interfaces) para um IP específico; um cliente que se conecta por outro endereço não alcança o canal.
- **O grupo troca de canal junto com o líder desde o incremento 37; a legião (raide) não.** Um membro comum, ou quem joga sozinho, troca normalmente e sai do grupo ao soltar a conexão.
- O personagem passa de um canal ao outro **pelo banco**: o canal de origem grava ao soltar a conexão e o de destino espera até 10 s por essa gravação. Sem banco (`-no-db`) não há o que reler. **Buffs não são gravados** e se perdem na troca (a origem os levava na memória).
- A ordem de troca só vale para o mesmo usuário vindo do mesmo IP, uma vez, por 30 s. O `Serial` do `$F06` é o id da conta, como na origem; não há segredo além disso.
- O guarda de baú de guilda (`ChestTick`) varre as guildas compartilhadas, mas só avisa do fechamento quem está no canal que fez a varredura; em dois canais, um jogador do outro pode não receber o aviso de fechamento.
- Amigos e duelo entre canais continuam sem suporte (cada canal conhece só os próprios jogadores).

## Grupo na troca de canal (incremento 37)

- Nunca vista por um cliente. O líder e quem está perto dele recebem `$F05` ao mesmo tempo; a ordem e a quem chega primeiro no canal novo foram só testadas com dois handlers no mesmo processo.
- A legião (raide) não viaja: o líder é avisado e fica. Quem está longe do líder sai do grupo.
- Se o líder demorar a chegar, o grupo existe antes com outro líder, e a liderança passa quando ele chega.

## Títulos: ganhar e perder (incremento 38)

- `addTitle`/`updateTitleLevel`/`removeTitle` existem e têm teste, mas **nada no Go os chama ainda**: os chamadores da origem são item de uso, NPC, comando e o laço de eventos. O único contador de progresso da origem (título 27, abate de jogador) depende de PvP, que o Go não tem.
- A lista de títulos só é gravada na saída e na gravação periódica; um título dado agora e perdido numa queda do processo antes da gravação some.

## Ferramenta de GM (incremento 39)

- **Nunca falou com o painel real.** Os pacotes foram conferidos contra `Packets.pas`; o painel (WebPainel/MasterEditor) não foi usado.
- A conta de painel é a de `gm_accounts` e o login é aceito em qualquer conexão do canal, como na origem. A senha fica em texto no banco (como na origem) e o cliente manda o MD5. Quem ler o banco lê as senhas dos GMs.
- Só 8 dos 27 códigos de painel foram portados. **[Corrigido no incremento 40]** Eu havia escrito aqui que a aprovação executa SQL guardado em `gm_commands.command`; isso estava errado e a aprovação foi portada (só tipos fechados). (CORRIGIDO no incremento 40: a aprovação NÃO executa esse texto; foi portada com tipos fechados.)
- O painel age em todos os canais pelo nome do personagem; o `TargetID` só vale no canal da conexão do painel.
- `$3205` com `Type2 = 1` alcança todos os canais; os demais mandam só ao canal.
- Ouro e cash têm teto (`MaxGold` e 2³²−1); a origem somava sem teto.

## Ferramenta de GM, fatias 2 e 3 (incremento 40)

- **Nunca falou com o painel real nem com MySQL real** (as consultas novas não foram executadas; o acesso direto ao banco a partir desta sessão foi negado). Os pacotes foram conferidos contra `Packets.pas`.
- O ramo "pedido de nível" do game master grava um texto informativo em `gm_commands.command`; esse texto não é executado por nada.
- A lista de pendentes passou a exigir administrador (a origem não exigia). Se o painel original pede a lista com outro privilégio, ela virá vazia.
- Quem bane precisa ser game master ou acima e o alvo precisa estar em jogo; banir um jogador offline não existe (nem na origem).
- Aprovar nível/item exige o personagem existir; a origem marcava o pedido como feito mesmo sem ele.
- O item de escolher nação grava `accounts.nation` só se for 0. Num servidor de um canal só, a nação escolhida é a do flag `-nation` (padrão 1).
- `0x3225` (backup de conta), `0x3221`, `0x3214`, `0x3219` e `0x5000` continuam sem porte.
