package game

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"aika/internal/auth"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/store"
)

// Comandos de GM. Porte de TCommandHandlers.ProcessCommands
// (CommandHandlers.pas:98-305).
//
// Na origem o despacho tinha tres partes que nao conversavam: uma corrente de
// "if Command = '...'" com Exit, um "case AnsiIndexStr(Command, [...])" com
// trinta e um nomes, e tres nomes (addtitle, updatetitle, removetitle) nas
// duas, sendo que a corrente roda primeiro e sai — os casos 19, 20 e 21 do
// case nunca executavam. O nome 'npc' esta na lista e nao tem ramo: caia no
// else, "Comando invalido.".
//
// ProcessCommands tambem nao conferia permissao nenhuma: a checagem
// "AccountType >= GameMaster" vive em quem chama (PacketHandlers.pas:1361), e
// o console da porta 5555 chamava sem checar nada. Aqui a permissao e do
// comando.

// CommandLevel e o nivel de conta que um comando exige.
type CommandLevel uint8

// Origem e de onde o comando veio.
type Origem int

const (
	// OrigemChat e o comando digitado no jogo, pelo sussurro.
	OrigemChat Origem = iota
	// OrigemConsole e o comando que chegou pela porta de administracao.
	OrigemConsole
)

// CommandContext reune o que um comando precisa.
type CommandContext struct {
	// Caller e quem digitou, nil quando veio do console.
	Caller *Session
	// CallerConn e a conexao de quem digitou, nil no console.
	CallerConn *netsrv.Conn
	// Origem diz se veio do chat ou do console.
	Origem Origem
	// Args sao os parametros, ja separados por espaco.
	Args []string
}

// Arg devolve o parametro n, ou "" quando nao existe.
//
// Na origem os parametros eram um TStringList indexado direto: "Data[0]" com
// a lista vazia levanta excecao. Em /dmg isso passava por um "finally" em vez
// de um "except", entao o dano era aplicado com valor zero E a excecao subia;
// vindo do console, que nao tinha try/except, a excecao matava a thread do
// console e o painel parava de responder ate o servidor reiniciar.
func (cc *CommandContext) Arg(n int) string {
	if n < 0 || n >= len(cc.Args) {
		return ""
	}
	return cc.Args[n]
}

// Int devolve o parametro n como inteiro.
func (cc *CommandContext) Int(n int) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(cc.Arg(n)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Command e um comando de administracao.
type Command struct {
	Name string
	// Level e o tipo de conta minimo. O console nao tem conta e por isso
	// roda qualquer nivel, mas ele proprio exige token.
	Level CommandLevel
	// Usage e a linha de ajuda.
	Usage string
	// Run roda o comando e devolve o texto da resposta.
	Run func(h *Handler, ctx context.Context, cc *CommandContext) string
}

// comandos e o registro, por nome em minusculas.
var comandos = map[string]*Command{}

func registrar(c *Command) { comandos[c.Name] = c }

// CommandNames devolve os nomes em ordem, para a ajuda.
func CommandNames() []string {
	out := make([]string, 0, len(comandos))
	for n := range comandos {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// RunCommand roda um comando e devolve o texto da resposta.
//
// O comando chega sem a barra. Um nome desconhecido responde como na origem
// ("Comando invalido."), mas aqui tambem lista o que existe.
func (h *Handler) RunCommand(ctx context.Context, cc *CommandContext, nome string) string {
	nome = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(nome, "/", "")))
	if nome == "" {
		return "Comando invalido."
	}
	if nome == "help" || nome == "ajuda" {
		return "Comandos: " + strings.Join(CommandNames(), ", ")
	}
	c, ok := comandos[nome]
	if !ok {
		return "Comando invalido."
	}
	// Do chat, o nivel da conta manda. Do console, o token ja foi conferido
	// por quem aceitou a conexao.
	if cc.Origem == OrigemChat {
		if cc.Caller == nil {
			return "Comando invalido."
		}
		if CommandLevel(cc.Caller.Account.Type) < c.Level {
			return "Sem permissao."
		}
	}
	return c.Run(h, ctx, cc)
}

// alvo resolve o jogador sobre quem o comando age.
//
// Na origem cada comando repetia o mesmo bloco: "if Data.Count = 2 then
// GetPlayerByName(Data[0]) else aplica em si mesmo". Do console nao havia
// "si mesmo" coerente — o comando rodava como um Admin online qualquer,
// escolhido pela varredura do vetor de conexoes, e um /addgold sem nome dava
// ouro a esse Admin sorteado.
func (h *Handler) alvo(cc *CommandContext, nome string) (*netsrv.Conn, *Session, string) {
	if nome != "" {
		id, ok := h.Chan.FindByCharacter(nome)
		if !ok {
			return nil, nil, "Jogador nao encontrado."
		}
		s, conn, ok := h.Chan.Lookup(id)
		if !ok || s.Status < Playing {
			return nil, nil, "Jogador nao encontrado."
		}
		return conn, s, ""
	}
	if cc.Origem == OrigemConsole {
		return nil, nil, "Pelo console o nome do jogador e obrigatorio."
	}
	if cc.Caller == nil || cc.Caller.Status < Playing {
		return nil, nil, "Jogador nao encontrado."
	}
	return cc.CallerConn, cc.Caller, ""
}

// Itens que a origem recusava em /additem.
var itensBanidos = map[int64]bool{20038: true, 20039: true, 20040: true, 20041: true}

func init() {
	registrar(&Command{
		Name:  "additem",
		Level: auth.AccGameMaster,
		Usage: "/additem [jogador] <item> [quantidade]",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			// A origem aceitava "<item> [qtd]" e "<jogador> <item> <qtd>",
			// decidindo pela contagem: com tres parametros o primeiro era o
			// nome. Mantido.
			nome, pItem, pQtd := "", 0, 1
			if len(cc.Args) >= 3 {
				nome, pItem, pQtd = cc.Arg(0), 1, 2
			}
			item, ok := cc.Int(pItem)
			if !ok {
				return "Uso: " + "/additem [jogador] <item> [quantidade]"
			}
			if itensBanidos[item] {
				return "Itens banidos."
			}
			if item <= 0 || item > 65535 {
				return "Item fora da faixa."
			}
			qtd := int64(1)
			if len(cc.Args) > pQtd {
				if v, ok := cc.Int(pQtd); ok {
					qtd = v
				}
			}
			if qtd <= 0 {
				qtd = 1
			}
			if qtd > itemfn.MaxSlotAmount {
				return fmt.Sprintf("Quantidade acima de %d.", itemfn.MaxSlotAmount)
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			slot, ok := h.freeInventorySlot(s)
			if !ok {
				return "Inventario cheio."
			}
			it := model.Item{Index: uint16(item)}
			itemfn.SetAmount(h.data(), &it, uint16(qtd))
			s.Inventory.Slots[slot] = it
			h.sendRefreshItem(conn, s, store.SlotTypeInventory, uint16(slot), it)
			return fmt.Sprintf("Item %d x%d para %s no slot %d.",
				item, qtd, s.Avatar.Name, slot)
		},
	})

	registrar(&Command{
		Name:  "addgold",
		Level: auth.AccGameMaster,
		Usage: "/addgold [jogador] <ouro>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			ouro, ok := cc.Int(p)
			if !ok {
				return "Uso: /addgold [jogador] <ouro>"
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			// A origem lia o valor como UInt64 e somava num campo de 32 bits
			// sem olhar o teto: passar do limite dava a volta e o jogador
			// ficava com quase nada.
			antes := s.Avatar.Gold
			if ouro >= 0 {
				// A origem fazia "Inc(Gold, Value)" com Value lido como
				// Int64 do parametro: dois /addgold com o maximo do Int64
				// davam a volta e deixavam o jogador com ouro negativo.
				if ouro > MaxGold-antes {
					s.Avatar.Gold = MaxGold
				} else {
					s.Avatar.Gold = antes + ouro
				}
			} else {
				s.Avatar.Gold = subGold(antes, -ouro)
			}
			h.sendRefreshMoney(conn, s)
			return fmt.Sprintf("%s: ouro %d -> %d.", s.Avatar.Name, antes, s.Avatar.Gold)
		},
	})

	registrar(&Command{
		Name:  "addexp",
		Level: auth.AccGameMaster,
		Usage: "/addexp [jogador] <exp>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			exp, ok := cc.Int(p)
			if !ok || exp < 0 {
				return "Uso: /addexp [jogador] <exp>"
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			ganho := h.addExp(conn, s, exp, ExpTypeMob)
			return fmt.Sprintf("%s: +%d de exp (nivel %d).",
				s.Avatar.Name, ganho, s.Avatar.Level)
		},
	})

	registrar(&Command{
		Name:  "addlevel",
		Level: auth.AccGameMaster,
		Usage: "/addlevel [jogador] <niveis>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			niveis, ok := cc.Int(p)
			if !ok {
				return "Uso: /addlevel [jogador] <niveis>"
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			// A origem calculava a exp alvo com
			//
			//   LevelExp := ExpList[Player.Level + (Level - 1)] + 1
			//
			// lendo o nivel de QUEM DIGITOU mesmo quando o alvo era outro
			// jogador, e indexando ExpList sem checagem. Level e WORD, entao
			// "/addlevel 0" dava 65535 no indice, caia no except, e o
			// fallback era "High(ExpList)" — o ultimo INDICE (151), nao o
			// ultimo valor. Em seguida "AddExp := LevelExp - Exp" num UInt64
			// dava a volta e o personagem recebia 1.8e19 de exp.
			if niveis <= 0 {
				return "Numero de niveis tem de ser positivo."
			}
			destino := int64(s.Avatar.Level) + niveis
			if destino < 0 || destino > 65535 || h.Exp == nil {
				return "Nivel fora da tabela de experiencia."
			}
			alvoExp, ok := h.Exp.ExpForLevel(uint16(destino))
			if !ok {
				return fmt.Sprintf("Nivel %d fora da tabela de experiencia.", destino)
			}
			atual := s.Avatar.Exp
			if int64(alvoExp) <= atual {
				return fmt.Sprintf("%s ja tem exp de nivel %d.", s.Avatar.Name, destino)
			}
			ganho := h.addExp(conn, s, int64(alvoExp)-atual, ExpTypeMob)
			return fmt.Sprintf("%s: +%d de exp, nivel %d.",
				s.Avatar.Name, ganho, s.Avatar.Level)
		},
	})

	registrar(&Command{
		Name:  "move",
		Level: auth.AccGameMaster,
		Usage: "/move [jogador] <x> <y>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, px := "", 0
			if len(cc.Args) >= 3 {
				nome, px = cc.Arg(0), 1
			}
			x, ok1 := cc.Int(px)
			y, ok2 := cc.Int(px + 1)
			if !ok1 || !ok2 {
				return "Uso: /move [jogador] <x> <y>"
			}
			pos := model.Position{X: float32(x), Y: float32(y)}
			if !ValidPosition(pos) {
				return "Posicao fora do mapa."
			}
			_, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			h.teleportTo(s, pos)
			return fmt.Sprintf("%s movido para (%d, %d).", s.Avatar.Name, x, y)
		},
	})

	registrar(&Command{
		Name:  "dc",
		Level: auth.AccGameMaster,
		Usage: "/dc <jogador>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			// A origem passava "CMD[0]" sem conferir a contagem: "/dc" sem
			// nome levantava excecao na lista vazia.
			nome := cc.Arg(0)
			if nome == "" {
				return "Uso: /dc <jogador>"
			}
			id, ok := h.Chan.FindByCharacter(nome)
			if !ok {
				return "Jogador nao encontrado."
			}
			_, conn, ok := h.Chan.Lookup(id)
			if !ok || conn == nil {
				return "Jogador nao encontrado."
			}
			conn.Close()
			return fmt.Sprintf("%s desconectado.", nome)
		},
	})

	registrar(&Command{
		Name:  "dmg",
		Level: auth.AccGameMaster,
		Usage: "/dmg [jogador] <dano>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			dano, ok := cc.Int(p)
			if !ok || dano < 0 {
				// A origem usava "finally" em vez de "except": sem parametro
				// o dano era aplicado com valor zero e a excecao subia.
				return "Uso: /dmg [jogador] <dano>"
			}
			_, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			// A origem so sabia bater em quem digitou.
			h.applyDamage(s, int(min(dano, 1<<31-1)))
			return fmt.Sprintf("%s: %d de dano (HP %d).",
				s.Avatar.Name, dano, s.Avatar.CurHP)
		},
	})

	registrar(&Command{
		Name:  "addbuff",
		Level: auth.AccGameMaster,
		Usage: "/addbuff [jogador] <indice>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			idx, ok := cc.Int(p)
			if !ok || idx <= 0 || idx > 65535 {
				return "Uso: /addbuff [jogador] <indice>"
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			segundos := h.durations().BuffSeconds(uint16(idx))
			if segundos <= 0 {
				return fmt.Sprintf("Buff %d nao tem duracao na tabela.", idx)
			}
			h.applyBuff(s, uint16(idx))
			h.sendStatus(conn, s)
			return fmt.Sprintf("Buff %d em %s por %ds.", idx, s.Avatar.Name, segundos)
		},
	})

	registrar(&Command{
		Name:  "removebuff",
		Level: auth.AccGameMaster,
		Usage: "/removebuff [jogador] <indice>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			nome, p := "", 0
			if len(cc.Args) >= 2 {
				nome, p = cc.Arg(0), 1
			}
			idx, ok := cc.Int(p)
			if !ok || idx <= 0 || idx > 65535 {
				return "Uso: /removebuff [jogador] <indice>"
			}
			conn, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			if s.Base.Buffs == nil || !h.removeBuffAndNotify(s, uint16(idx)) {
				return fmt.Sprintf("%s nao tem o buff %d.", s.Avatar.Name, idx)
			}
			_ = conn
			return fmt.Sprintf("Buff %d removido de %s.", idx, s.Avatar.Name)
		},
	})

	registrar(&Command{
		Name:  "setrank",
		Level: auth.AccAdmin,
		Usage: "/setrank <jogador> <0..5>",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			// A origem aceitava "/setrank <n>" aplicando em si mesmo, e nao
			// limitava o valor: "TAccountType(Rank)" com Rank acima de 5 dava
			// um enum fora da faixa, e a comparacao ">= GameMaster" passava a
			// valer para qualquer coisa. Aqui o nome e obrigatorio, o valor e
			// conferido, e so Admin usa.
			nome := cc.Arg(0)
			rank, ok := cc.Int(1)
			if nome == "" || !ok {
				return "Uso: /setrank <jogador> <0..5>"
			}
			if rank < 0 || rank > auth.AccAdmin {
				return fmt.Sprintf("Rank fora da faixa (0..%d).", auth.AccAdmin)
			}
			_, s, erro := h.alvo(cc, nome)
			if erro != "" {
				return erro
			}
			s.Account.Type = uint8(rank)
			return fmt.Sprintf("%s agora e rank %d (so nesta sessao: "+
				"a origem tambem nao gravava no banco).", s.Avatar.Name, rank)
		},
	})

	registrar(&Command{
		Name:  "who",
		Level: auth.AccModerator,
		Usage: "/who",
		Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
			// Nao existe na origem. O console precisa de alguma forma de
			// saber quem esta online para mandar comandos com nome, que agora
			// sao obrigatorios.
			ids := h.Chan.PlayingIDs()
			if len(ids) == 0 {
				return "Ninguem em jogo."
			}
			var nomes []string
			for _, id := range ids {
				if s, _, ok := h.Chan.Lookup(id); ok {
					nomes = append(nomes, fmt.Sprintf("%s(%d)", s.Avatar.Name, id))
				}
			}
			sort.Strings(nomes)
			return fmt.Sprintf("%d em jogo: %s", len(nomes), strings.Join(nomes, ", "))
		},
	})

	// Comandos da origem que dependem de sistema ainda nao portado. Respondem
	// dizendo isso, em vez de nao fazer nada em silencio — que e o que
	// /addmob e /equip faziam (corpo vazio e "Ainda nao setado.").
	for _, naoPortado := range []struct{ nome, falta string }{
		{"addcash", "cash de conta"},
		{"addeff", "efeito visual"},
		{"removeeff", "efeito visual"},
		{"addguildexp", "experiencia de guilda"},
		{"addmob", "spawn avulso de mob (corpo vazio tambem na origem)"},
		{"addtitle", "titulos"},
		{"updatetitle", "titulos"},
		{"removetitle", "titulos"},
		{"equip", "equipar por comando (\"Ainda nao setado.\" na origem)"},
		{"moveitem", "mover item por comando"},
		{"pix", "transferencia entre jogadores"},
		{"pixcobrar", "transferencia entre jogadores"},
		{"pixaceitar", "transferencia entre jogadores"},
		{"pixrecusar", "transferencia entre jogadores"},
		{"setarchon", "cargos de nacao"},
		{"setarchon1", "cargos de nacao"},
		{"setarchon2", "cargos de nacao"},
		{"setarchon3", "cargos de nacao"},
		{"setmarechal", "cargos de nacao"},
		{"setdntype", "tipo de dano global"},
		{"startevent", "eventos"},
		{"stopevent", "eventos"},
		{"resetevent", "eventos"},
		{"walkto", "caminho por waypoint"},
		{"npc", "NPC (nome na lista da origem sem ramo no case)"},
	} {
		falta := naoPortado.falta
		registrar(&Command{
			Name:  naoPortado.nome,
			Level: auth.AccGameMaster,
			Usage: "/" + naoPortado.nome,
			Run: func(h *Handler, ctx context.Context, cc *CommandContext) string {
				return "Ainda nao portado: " + falta + "."
			},
		})
	}
}

// RunConsole implementa console.Runner: roda o comando sem jogador nenhum
// por tras.
//
// Na origem o console procurava um Admin online e rodava o comando como ele
// (ConsoleSocket.pas:159-186). Duas consequencias: sem Admin online nada
// funcionava, e com Admin online um comando sem nome de jogador agia sobre
// esse Admin sorteado — "/addgold 1000000" dava o ouro a ele.
func (h *Handler) RunConsole(ctx context.Context, nome string, args []string) string {
	cc := &CommandContext{Origem: OrigemConsole, Args: args}
	return h.RunCommand(ctx, cc, nome)
}
