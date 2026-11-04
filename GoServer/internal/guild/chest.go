package guild

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"aika/internal/model"
)

// ChestSlots e o tamanho de TGuildChest.Items: ARRAY [0..49] OF TITEM.
const ChestSlots = 50

// ChestTimeout e o tempo sem acao que fecha o bau. Porte do
// "MinutesBetween(Now, LastChestActionDate) >= 2" de TCloseChestThread.
const ChestTimeout = 2 * time.Minute

// MaxChestGold e o teto do ouro do bau.
//
// A origem punha teto de 2.000.000.000 so na RETIRADA (e comparando com o
// ouro da bolsa, nao com o do bau) e nenhum no DEPOSITO: o campo e UInt64 e
// crescia sem limite ate dar a volta em 1,8e19, momento em que o bau da
// guilda ficava com ouro quase zero. Aqui o mesmo teto vale nos dois lados.
const MaxChestGold uint64 = 2_000_000_000

// Erros do bau.
var (
	ErrChestBusy       = errors.New("guild: bau em uso por outro membro")
	ErrChestNotOpen    = errors.New("guild: bau nao esta aberto para este personagem")
	ErrChestNoGold     = errors.New("guild: ouro insuficiente no bau")
	ErrChestGoldCap    = errors.New("guild: ouro acima do teto do bau")
	ErrChestSlotRange  = errors.New("guild: slot fora do bau")
	ErrChestNotAMember = errors.New("guild: personagem nao e membro desta guilda")
)

// Chest e o bau da guilda, com o estado de quem o tem aberto.
//
// Diferencas centrais em relacao a TGuildChest + TGuild.OpenChest/CloseChest:
//
//   - A origem guardava quem estava no bau em "MemberInChest: Integer", um
//     INDICE no vetor Members, e o comparava com
//     "FindMemberFromCharIndex(...)". OpenChest atribuia esse indice SEM
//     checar o retorno: quando o personagem nao era membro da guilda,
//     FindMemberFromCharIndex devolvia -1 e MemberInChest ficava -1. Dali em
//     diante qualquer nao-membro passava no teste
//     "MemberInChest <> FindMemberFromCharIndex(...)" (-1 <> -1 e falso) e
//     mexia no bau. Pior: o mesmo -1 ia direto para "Members[-1].Rank", uma
//     leitura antes do inicio do vetor. Aqui o dono e o CharIndex, nao um
//     indice, e abrir exige ser membro.
//
//   - A origem criava uma THREAD por abertura (TCloseChestThread), com
//     "Sleep(50)" em laco, sem FreeOnTerminate e sem guardar a anterior:
//     abrir o bau duas vezes deixava a primeira thread vazando. Aqui o prazo
//     e conferido pelo laco do servidor (Registry.SweepChests).
//
//   - Quando o prazo vencia, a thread da origem mandava o sinal $F2F para o
//     cliente e saia — SEM chamar CloseChest. Se o cliente nao respondesse,
//     ou se o jogador caisse, MemberInChest ficava preso e o bau da guilda
//     ficava trancado para todos ATE O SERVIDOR REINICIAR. Nada no caminho de
//     desconexao chamava CloseChest. Aqui o prazo fecha de fato, e a saida do
//     jogador tambem.
//
//   - Items e Gold eram mexidos direto, de qualquer conexao, sem nenhuma
//     protecao: dois membros arrastando o mesmo slot ao mesmo tempo
//     duplicavam o item. Aqui todo acesso passa pelo mutex.
type Chest struct {
	mu    sync.Mutex
	items [ChestSlots]model.Item
	gold  uint64

	// holder e o personagem com o bau aberto, zero quando fechado.
	holder uint32
	// holderName e o nome dele, para a mensagem "'%s' esta usando o armazem".
	holderName string
	// lastAction e o instante da ultima acao, que renova o prazo.
	lastAction time.Time

	// loaded diz se os itens ja vieram do banco. A origem lia o bau de TODAS
	// as guildas no boot (Load.pas:200), o que obrigava a ter cada bau de
	// cada guilda em memoria desde o inicio; aqui a leitura e na primeira
	// abertura. Fica aqui dentro, sob o mesmo mutex, para nao haver dois
	// lugares a sincronizar.
	loaded bool
}

// NewChest cria um bau vazio.
func NewChest() *Chest { return &Chest{} }

// Gold devolve o ouro do bau.
func (c *Chest) Gold() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gold
}

// SetGold grava o ouro do bau, respeitando o teto, e diz se teve de cortar.
// Usado pela carga do banco.
func (c *Chest) SetGold(v uint64) (cortou bool) {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gold = min(v, MaxChestGold)
	return v > MaxChestGold
}

// MoveGold soma delta ao ouro do bau e devolve o novo valor.
//
// delta positivo e deposito, negativo e retirada, igual ao Packet.Value da
// origem. A conta e feita em int64 e so depois volta para uint64: a origem
// fazia "Chest.Gold := Chest.Gold + UInt64(Packet.Value)", e para um valor
// negativo o UInt64() virava um numero gigante que, por dar a volta, acertava
// a subtracao por acidente — e errava feio quando o bau tinha menos ouro que
// o pedido, porque a guarda comparava um UInt64 com um Int64.
func (c *Chest) MoveGold(delta int64) (uint64, error) {
	if c == nil {
		return 0, ErrChestNotOpen
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if delta < 0 {
		// -delta nao transborda porque o chamador ja recusou math.MinInt64.
		if uint64(-delta) > c.gold {
			return c.gold, ErrChestNoGold
		}
		c.gold -= uint64(-delta)
		return c.gold, nil
	}
	if c.gold+uint64(delta) > MaxChestGold {
		return c.gold, ErrChestGoldCap
	}
	c.gold += uint64(delta)
	return c.gold, nil
}

// Do roda f com o vetor de itens do bau travado.
//
// E o unico caminho que mexe nos itens: assim uma troca entre o bau e o
// inventario acontece inteira com o bau travado, e dois membros nao levam o
// mesmo item.
func (c *Chest) Do(f func(itens []model.Item)) {
	if c == nil || f == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f(c.items[:])
}

// At devolve uma copia do item de um slot.
func (c *Chest) At(slot int) (model.Item, error) {
	if c == nil {
		return model.Item{}, ErrChestSlotRange
	}
	if slot < 0 || slot >= ChestSlots {
		return model.Item{}, ErrChestSlotRange
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items[slot], nil
}

// Snapshot devolve o bau no formato do pacote $D58.
func (c *Chest) Snapshot() model.GuildChest {
	var out model.GuildChest
	if c == nil {
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copy(out.Items[:], c.items[:])
	out.Gold = c.gold
	return out
}

// IsLoaded diz se os itens do bau ja vieram do banco.
func (c *Chest) IsLoaded() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loaded
}

// LoadItems enche os itens do bau com o que veio do banco, marca o bau como
// carregado e devolve os avisos.
//
// itens vem indexado pelo slot. A origem fazia
// "ItemSlot := Byte(FieldByName('slot').AsInteger)" e escrevia em
// "Guilds[Slot].Chest.Items[ItemSlot]" com Items sendo ARRAY [0..49]: um slot
// de 50 a 255 no banco escrevia DEPOIS do fim do vetor, por cima de
// Chest.Gold, de MemberInChest, do ponteiro CloseChestThread e do inicio da
// guilda seguinte (Load.pas:212). A faixa do slot ja e conferida na leitura
// (store.Items.ByOwner); o corte aqui e a segunda linha de defesa.
func (c *Chest) LoadItems(itens []model.Item) []string {
	if c == nil {
		return nil
	}
	var avisos []string
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = [ChestSlots]model.Item{}
	for slot, it := range itens {
		if slot >= ChestSlots {
			if !it.IsEmpty() {
				avisos = append(avisos, fmt.Sprintf(
					"item %d do bau no slot %d, fora de 0..%d, ignorado",
					it.Index, slot, ChestSlots-1))
			}
			continue
		}
		c.items[slot] = it
	}
	c.loaded = true
	return avisos
}

// Holder devolve o personagem com o bau aberto, zero quando fechado.
func (c *Chest) Holder() uint32 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.holder
}

// HolderName devolve o nome de quem tem o bau aberto.
func (c *Chest) HolderName() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.holderName
}

// IsOpenFor diz se o bau esta aberto para este personagem.
//
// Na origem o teste era "MemberInChest <> FindMemberFromCharIndex(...)", que
// com o bau fechado ($FF) dava verdadeiro para todo mundo so por sorte: $FF
// nao e indice de membro valido. Aqui o bau fechado tem dono zero, e zero nao
// e CharIndex de ninguem.
func (c *Chest) IsOpenFor(charIndex uint32) bool {
	if c == nil || charIndex == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.holder == charIndex
}

// Open reserva o bau para um personagem.
//
// Devolve ErrChestBusy com o nome de quem o tem aberto quando outro membro
// esta usando. Reabrir para o mesmo personagem renova o prazo.
func (c *Chest) Open(charIndex uint32, nome string, agora time.Time) error {
	if c == nil || charIndex == 0 {
		return ErrChestNotAMember
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holder != 0 && c.holder != charIndex {
		// Prazo vencido do dono anterior: assume em vez de recusar para
		// sempre, que e o que a origem fazia quando o cliente nao respondia
		// ao $F2F.
		if agora.Sub(c.lastAction) < ChestTimeout {
			return fmt.Errorf("%w: %s", ErrChestBusy, c.holderName)
		}
	}
	c.holder = charIndex
	c.holderName = nome
	c.lastAction = agora
	return nil
}

// Close solta o bau, se for este personagem que o tem aberto.
func (c *Chest) Close(charIndex uint32) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holder == 0 || c.holder != charIndex {
		return false
	}
	c.holder = 0
	c.holderName = ""
	c.lastAction = time.Time{}
	return true
}

// Touch renova o prazo do bau. Porte de "LastChestActionDate := Now".
func (c *Chest) Touch(agora time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holder != 0 {
		c.lastAction = agora
	}
}

// expire fecha o bau se o prazo venceu e devolve o dono que perdeu o bau.
func (c *Chest) expire(agora time.Time) (uint32, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holder == 0 {
		return 0, false
	}
	if agora.Sub(c.lastAction) < ChestTimeout {
		return 0, false
	}
	dono := c.holder
	c.holder = 0
	c.holderName = ""
	c.lastAction = time.Time{}
	return dono, true
}

// ChestTimeoutNotice e um bau que o prazo fechou.
type ChestTimeoutNotice struct {
	GuildIndex uint16
	CharIndex  uint32
}

// SweepChests fecha os baus sem acao e devolve quem precisa ser avisado.
//
// Substitui a TCloseChestThread: uma thread do sistema operacional por bau
// aberto, em laco de Sleep(50), que no fim mandava o sinal e NAO fechava o
// bau.
func (r *Registry) SweepChests(agora time.Time) []ChestTimeoutNotice {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	baus := make([]*Guild, 0, len(r.byIndex))
	for _, g := range r.byIndex {
		if g != nil && g.Chest != nil {
			baus = append(baus, g)
		}
	}
	r.mu.Unlock()

	var out []ChestTimeoutNotice
	for _, g := range baus {
		if dono, venceu := g.Chest.expire(agora); venceu {
			out = append(out, ChestTimeoutNotice{
				GuildIndex: g.Index,
				CharIndex:  dono,
			})
		}
	}
	return out
}

// CloseChestOf fecha o bau da guilda deste personagem, se ele o tiver aberto.
//
// Chamado na saida do jogador. A origem nao tinha nada assim: quem abria o
// bau e caia trancava o bau da guilda ate o servidor reiniciar.
func (r *Registry) CloseChestOf(charIndex uint32) bool {
	g := r.Of(charIndex)
	if g == nil || g.Chest == nil {
		return false
	}
	return g.Chest.Close(charIndex)
}
