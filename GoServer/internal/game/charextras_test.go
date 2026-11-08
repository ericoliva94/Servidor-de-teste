package game

import (
	"context"
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

type fakeItemBars struct {
	mu    sync.Mutex
	barra [store.ItemBarSlots]uint32
	saves int
}

func (f *fakeItemBars) Load(_ context.Context, _ uint32) ([store.ItemBarSlots]uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.barra, nil
}

func (f *fakeItemBars) Save(_ context.Context, _ uint32, b [store.ItemBarSlots]uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.barra, f.saves = b, f.saves+1
	return nil
}

type fakeTeleports struct {
	mu    sync.Mutex
	texto string
	saves int
}

func (f *fakeTeleports) TeleportPositions(context.Context, uint32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.texto, nil
}

func (f *fakeTeleports) SaveTeleportPositions(_ context.Context, _, _ uint32, t string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texto, f.saves = t, f.saves+1
	return nil
}

func prepExtras(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, s, e := prepMisc(t)
	s.Account.ID = 7
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: 42, Name: "Alfa"}
	return h, s, e
}

func barra(t *testing.T, h *Handler, s *Session, e *espiao, slot, tipo, idx uint32) {
	t.Helper()
	if err := h.changeItemBar(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeChangeItemBar, packets.ChangeItemBarPacket{DestSlot: slot, SrcType: tipo, SrcIndex: idx}))); err != nil {
		t.Fatal(err)
	}
}

// ---- barra de itens

func TestBarraDeItensSkillItemELimpar(t *testing.T) {
	h, s, e := prepExtras(t)
	s.Inventory.Slots[0] = model.Item{Index: 300, Refi: 2}

	barra(t, h, s, e, 3, barSkill, 25)
	if s.ItemBar[3] != 25*16+2 {
		t.Errorf("skill = %d, queria %d", s.ItemBar[3], 25*16+2)
	}
	r := decodifica[packets.ChangeItemBarPacket](t, proximoDe(t, e, CodeChangeItemBar))
	if r.DestSlot != 3 || r.SrcType != barSkill || r.SrcIndex != 25 || r.Header.Index != WorldIndex {
		t.Errorf("resposta = %+v", r)
	}

	barra(t, h, s, e, 4, barItem, 300)
	if s.ItemBar[4] != 300 {
		t.Errorf("item = %d, queria 300", s.ItemBar[4])
	}

	barra(t, h, s, e, 3, barClear, 0)
	if s.ItemBar[3] != 0 {
		t.Errorf("slot limpo = %d", s.ItemBar[3])
	}
}

func TestBarraDeItensRecusas(t *testing.T) {
	h, s, e := prepExtras(t)
	barra(t, h, s, e, 24, barSkill, 5)         // slot fora da barra
	barra(t, h, s, e, 0xFFFFFFFF, barClear, 0) // slot enorme
	barra(t, h, s, e, 1, barSkill, 12000)      // skill fora da tabela
	barra(t, h, s, e, 1, barSkill, 0xFFFFFFFF) // estouraria o x16
	barra(t, h, s, e, 2, barItem, 999)         // item que o jogador nao tem
	barra(t, h, s, e, 2, barItem, 0x10000)     // fora do WORD
	barra(t, h, s, e, 2, 9, 1)                 // tipo desconhecido
	if s.ItemBar != [store.ItemBarSlots]uint32{} {
		t.Errorf("barra alterada por pedido invalido: %v", s.ItemBar)
	}
	barra(t, h, s, e, 0, barPran, 1)
	if got := mensagemDe(t, e); !strings.Contains(got, "Pran") {
		t.Errorf("mensagem = %q", got)
	}
}

func TestBarraDeItensGravaNaSaidaECarregaNaEntrada(t *testing.T) {
	h, s, e := prepExtras(t)
	f := &fakeItemBars{}
	h.ItemBarDB = f
	s.ItemBar[5] = 77
	h.OnDisconnect(e.conn)
	if f.saves != 1 || f.barra[5] != 77 {
		t.Errorf("saves=%d barra[5]=%d", f.saves, f.barra[5])
	}
	if got := h.loadItemBar(context.Background(), 42); got[5] != 77 {
		t.Errorf("carga = %v", got)
	}
	var sem Handler
	if got := sem.loadItemBar(context.Background(), 42); got != [store.ItemBarSlots]uint32{} {
		t.Error("sem banco a barra deveria vir vazia")
	}
}

// ---- teleporte

func TestTeleportesFormatoIdaEVoltaMantemOsSlots(t *testing.T) {
	var l [MaxTeleportSlots]model.Position
	l[1] = model.Position{X: 1100, Y: 2200}
	l[3] = model.Position{X: 3300, Y: 400}
	txt := formatTeleports(l)
	if txt != "0,0;1100,2200;0,0;3300,400;0,0;" {
		t.Errorf("texto = %q", txt)
	}
	// O formato antigo da origem (so as posicoes validas) ainda le.
	if got := parseTeleports("1100,2200;3300,400;"); got[0].X != 1100 || got[1].X != 3300 {
		t.Errorf("formato antigo = %+v", got)
	}
	// Ida e volta preserva o slot de cada posicao.
	if back := parseTeleports(txt); back[1] != l[1] || back[3] != l[3] || back[0] != (model.Position{}) {
		t.Errorf("ida e volta = %+v", back)
	}
	// Lixo nao quebra.
	_ = parseTeleports(";;,;abc,def;1,2,3;9999999999999,1;;;;;;;;")
}

func setTp(t *testing.T, h *Handler, s *Session, e *espiao, slot uint32) {
	t.Helper()
	if err := h.setTeleport(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeSetTeleport, packets.TeleportFcPacket{Slot: slot}))); err != nil {
		t.Fatal(err)
	}
}

func TestGravarTeleporteDoMagoEcoaEPersiste(t *testing.T) {
	h, s, e := prepExtras(t)
	f := &fakeTeleports{}
	h.TeleportDB = f
	s.Avatar.ClassInfo = 41 // mago
	s.Position = model.Position{X: 1234.6, Y: 987.4}

	setTp(t, h, s, e, 2)

	if s.Teleports[2].X != 1235 || s.Teleports[2].Y != 987 {
		t.Errorf("slot 2 = %+v", s.Teleports[2])
	}
	r := decodifica[packets.TeleportFcPacket](t, proximoDe(t, e, CodeSetTeleport))
	if r.Slot != 2 || r.PosX != 1235 || r.PosY != 987 {
		t.Errorf("resposta = %+v", r)
	}
	if f.saves != 1 || f.texto != "0,0;0,0;1235,987;0,0;0,0;" {
		t.Errorf("gravacao = %d %q", f.saves, f.texto)
	}
}

func TestGravarTeleporteDeOutraClasseNaoPersiste(t *testing.T) {
	h, s, e := prepExtras(t)
	f := &fakeTeleports{}
	h.TeleportDB = f
	s.Position = model.Position{X: 1000, Y: 1000}
	setTp(t, h, s, e, 0)
	if f.saves != 0 {
		t.Error("so o mago grava a lista, como na origem")
	}
	proximoDe(t, e, CodeSetTeleport) // mas o eco existe
}

// Defeito da origem: slot 5 numa lista de 5 posicoes escrevia fora do vetor.
func TestGravarTeleporteSlotForaDaListaERecusado(t *testing.T) {
	h, s, e := prepExtras(t)
	s.Avatar.ClassInfo = 41
	s.Position = model.Position{X: 1000, Y: 1000}
	setTp(t, h, s, e, 5)
	setTp(t, h, s, e, 0xFFFFFFFF)
	e.nada(t, 20*time.Millisecond)
	if s.Teleports != [MaxTeleportSlots]model.Position{} {
		t.Error("lista alterada por slot invalido")
	}
}

func TestCarregarTeleportesSoParaMago(t *testing.T) {
	h, s, _ := prepExtras(t)
	h.TeleportDB = &fakeTeleports{texto: "0,0;1100,2200;"}
	h.loadTeleports(context.Background(), s, 42)
	if s.Teleports[1] != (model.Position{}) {
		t.Error("guerreiro nao tem lista de teleporte")
	}
	s.Avatar.ClassInfo = 45
	h.loadTeleports(context.Background(), s, 42)
	if s.Teleports[1].X != 1100 || s.Teleports[1].Y != 2200 {
		t.Errorf("slot 1 = %+v", s.Teleports[1])
	}
}

// ---- atributos

func TestAtributosDetalhadosDoProprioJogador(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.Cons = 100
	s.Avatar.ReduceCooldown, s.Avatar.HabAtk, s.Avatar.CureTax = 7, 8, 9
	s.Avatar.DamageCritical, s.Avatar.CritRes, s.Avatar.DuploRes, s.Avatar.ResDamageCrit = 11, 12, 13, 14
	s.Avatar.FisPenetration, s.Avatar.MagPenetration, s.Avatar.Resistence = 15, 16, 17

	if err := h.requestAllAttributes(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeAllAttributes, packets.RequestAllAttributes{}))); err != nil {
		t.Fatal(err)
	}
	r := decodifica[packets.ResponseAllAttributes](t, proximoDe(t, e, CodeAllAttributesReply))
	if r.Resfriamento != 7 || r.DanoHab != 8 || r.Cura != 9 || r.DanoCrit != 11 || r.ResisCrit != 12 ||
		r.ResisDuplo != 13 || r.DiminDanoCrit != 14 || r.PerFis != 15 || r.PerMag != 16 {
		t.Errorf("resposta = %+v", r)
	}
	if r.RecHP != 300 || r.RecMP != 300 {
		t.Errorf("regeneracao = %d/%d, queria Constituicao x 3 = 300", r.RecHP, r.RecMP)
	}
	if r.ResisLent != 17 || r.ResisMedo != 17 || r.ResisStun != 17 {
		t.Errorf("resistencias = %+v", r)
	}
	if r.Header.Index != s.ClientID {
		t.Errorf("Index = %d", r.Header.Index)
	}
}

func TestAtributosSaturamNoWORD(t *testing.T) {
	var a Avatar
	a.Cons = 40000
	if got := detailedAttributes(&a).RecHP; got != 65535 {
		t.Errorf("RecHP = %d, queria saturado em 65535", got)
	}
}

func pedeAlvo(t *testing.T, h *Handler, s *Session, e *espiao, id uint32) {
	t.Helper()
	if err := h.requestAllAttributesTarget(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeAllAttributesTarget, packets.RequestAllAttributesTarget{TargetID: id}))); err != nil {
		t.Fatal(err)
	}
}

func TestAtributosDoAlvoDaMesmaNacao(t *testing.T) {
	h, s, e := prepMisc(t)
	oe := novoEspiao(t)
	outro := sessaoEmJogo(105, 100)
	outro.Avatar.HabAtk = 321
	h.Chan.Join(oe.conn, outro)

	pedeAlvo(t, h, s, e, uint32(outro.ClientID))
	r := decodifica[packets.ResponseAllAttributesTarget](t, proximoDe(t, e, CodeAllAttributesTargetReply))
	if r.DanoHab != 321 || r.Header.Index != outro.ClientID {
		t.Errorf("resposta = %+v", r)
	}
}

func TestAtributosDoAlvoDeOutraNacaoOuInvalido(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.Nation = 1
	oe := novoEspiao(t)
	outro := sessaoEmJogo(105, 100)
	outro.Avatar.Nation = 2
	h.Chan.Join(oe.conn, outro)

	pedeAlvo(t, h, s, e, uint32(outro.ClientID))
	if got := mensagemDe(t, e); got != "O alvo nao pertence a sua nacao." {
		t.Errorf("mensagem = %q", got)
	}
	for _, id := range []uint32{0, 40, 0xFFFFFFFF, 3000} { // vazio, sem ninguem, enorme, mob
		pedeAlvo(t, h, s, e, id)
	}
	e.nada(t, 20*time.Millisecond)
}

// ---- item no chat

func linkItem(t *testing.T, h *Handler, s *Session, e *espiao, link uint8, slot uint8, tipo uint16, fala string) {
	t.Helper()
	p := packets.LinkItemPacket{LinkItem: link, ItemSlot: slot, ChatType: tipo}
	netsrv.PutCString(p.Fala[:], fala)
	if err := h.itemLink(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeItemLink, p))); err != nil {
		t.Fatal(err)
	}
}

func TestItemNoChatNormalChegaAoAutorEAQuemVeComOApelidoCerto(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.Name = "Alfa"
	oe := novoEspiao(t)
	outro := sessaoEmJogo(101, 100)
	h.Chan.Join(oe.conn, outro)
	s.Base.VisiblePlayers.Add(outro.ClientID)
	s.Inventory.Slots[4] = model.Item{Index: 777, Refi: 3, Min: 1, Max: 9}

	linkItem(t, h, s, e, 1, 4, ChatNormal, "olha isso")

	for _, esp := range []*espiao{e, oe} {
		m := decodifica[packets.ChatItemLinkPacket](t, proximoDe(t, esp, CodeChatItemLink))
		if netsrv.CString(m.Nick[:]) != "Alfa" || netsrv.CString(m.Fala[:]) != "olha isso" ||
			m.Item.Index != 777 || m.Item.Refi != 3 || m.Header.Index != s.ClientID {
			t.Errorf("mensagem = %+v", m)
		}
	}
}

func TestItemNoChatRecusas(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Inventory.Slots[4] = model.Item{Index: 777}
	linkItem(t, h, s, e, 0, 4, ChatNormal, "sem link") // LinkItem falso
	linkItem(t, h, s, e, 1, 5, ChatNormal, "vazio")    // slot vazio
	linkItem(t, h, s, e, 1, 200, ChatNormal, "fora")   // slot > 63
	linkItem(t, h, s, e, 1, 61, ChatNormal, "bolsa")   // slot de bolsa
	e.nada(t, 20*time.Millisecond)
}

func TestItemNoChatCanaisSemSistemaOuSemGrupo(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Inventory.Slots[4] = model.Item{Index: 777}

	linkItem(t, h, s, e, 1, 4, ChatParty, "x")
	if got := mensagemDe(t, e); got != "Voce nao esta em um grupo." {
		t.Errorf("grupo: %q", got)
	}
	linkItem(t, h, s, e, 1, 4, ChatGuild, "x")
	if got := mensagemDe(t, e); got != "Voce nao esta em uma guilda." {
		t.Errorf("guilda: %q", got)
	}
	// Megafone mandava a todos sem gastar o item de megafone.
	linkItem(t, h, s, e, 1, 4, ChatMegafone, "x")
	if got := mensagemDe(t, e); got != "Este canal de conversa ainda nao esta disponivel." {
		t.Errorf("megafone: %q", got)
	}
}

func TestItemNoGritoTemOMesmoLimiteDoGrito(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Inventory.Slots[4] = model.Item{Index: 777}
	linkItem(t, h, s, e, 1, 4, ChatShout, "primeiro")
	proximoDe(t, e, CodeChatItemLink)
	linkItem(t, h, s, e, 1, 4, ChatShout, "segundo")
	if got := mensagemDe(t, e); got != "Voce nao pode floodar o grito." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestTamanhosDosPacotesDeExtras(t *testing.T) {
	for nome, par := range map[string][2]int{
		"ChangeItemBarPacket":         {binary.Size(packets.ChangeItemBarPacket{}), 24},
		"TeleportFcPacket":            {binary.Size(packets.TeleportFcPacket{}), 24},
		"ResponseAllAttributes":       {binary.Size(packets.ResponseAllAttributes{}), 50},
		"ResponseAllAttributesTarget": {binary.Size(packets.ResponseAllAttributesTarget{}), 50},
		"ChatItemLinkPacket":          {binary.Size(packets.ChatItemLinkPacket{}), 184},
	} {
		if par[0] != par[1] {
			t.Errorf("%s = %d bytes, o Delphi tem %d", nome, par[0], par[1])
		}
	}
}
