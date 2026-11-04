package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/dropdata"
	"aika/internal/gamedata"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// tabelaDeDrops monta uma tabela de drop a mao.
func tabelaDeDrops(cat int, porRaridade map[dropdata.Rarity][]uint16) *dropdata.Table {
	t := &dropdata.Table{}
	if cat >= 0 && cat < dropdata.CategoryCount {
		for r, itens := range porRaridade {
			t.Categories[cat].Items[r] = itens
		}
	}
	return t
}

// handlerComDrops devolve um handler com mundo de mob, drops e objetos.
func handlerComDrops(t *testing.T, dropIndex uint16, tab *dropdata.Table) (*Handler, *time.Time) {
	sp := especie("Bicho", 1, 1, 1000, 1000)
	sp.DropIndex = dropIndex
	sp.Exp = 10
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	h.Drops = tab
	h.MapObjs = NewMapObjects()
	itens := novaTabelaDeItens()
	itens.poeItem(100, gamedata.ItemFromList{})
	itens.poeItem(9000, gamedata.ItemFromList{})
	h.Data = itens
	return h, agora
}

func collectPacket(index uint32) []byte {
	var p packets.CollectItem
	p.Index = index
	buf, err := netsrv.Marshal(CodeCollectMapItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- registro de objetos

func TestObjetosIndicesDeUm(t *testing.T) {
	o := NewMapObjects()
	vistos := map[uint16]bool{}
	for range 5 {
		idx, ok := o.Add(&MapObject{ItemID: 100})
		if !ok {
			t.Fatal("deveria ter cabido")
		}
		if idx == 0 {
			t.Error("o indice zero significa 'sem objeto' no protocolo")
		}
		if vistos[idx] {
			t.Errorf("indice %d repetido", idx)
		}
		vistos[idx] = true
	}
	if o.Count() != 5 {
		t.Errorf("%d objetos, queria 5", o.Count())
	}
}

// Tirar do chao libera o indice e devolve o objeto uma vez so: dois jogadores
// clicando no mesmo item nao podem levar os dois.
func TestTakeSoUmaVez(t *testing.T) {
	o := NewMapObjects()
	idx, _ := o.Add(&MapObject{ItemID: 100})

	obj, ok := o.Take(idx)
	if !ok || obj.ItemID != 100 {
		t.Fatalf("primeira coleta = %v/%v", obj, ok)
	}
	if _, ok := o.Take(idx); ok {
		t.Error("a segunda coleta nao deveria achar nada")
	}
	if o.Count() != 0 {
		t.Errorf("%d objetos", o.Count())
	}
}

func TestTakeConcorrente(t *testing.T) {
	o := NewMapObjects()
	idx, _ := o.Add(&MapObject{ItemID: 100})

	const n = 20
	ganhos := make(chan bool, n)
	for range n {
		go func() {
			_, ok := o.Take(idx)
			ganhos <- ok
		}()
	}
	total := 0
	for range n {
		if <-ganhos {
			total++
		}
	}
	if total != 1 {
		t.Errorf("%d goroutines levaram o item, no maximo 1", total)
	}
}

// O vetor cheio recusa, e nao sobrescreve.
func TestObjetosCheio(t *testing.T) {
	o := NewMapObjects()
	for i := 0; i < MaxMapObjects; i++ {
		if _, ok := o.Add(&MapObject{ItemID: 100}); !ok {
			t.Fatalf("falhou no objeto %d", i)
		}
	}
	if _, ok := o.Add(&MapObject{ItemID: 100}); ok {
		t.Error("deveria ter recusado com o vetor cheio")
	}
	if o.Count() != MaxMapObjects {
		t.Errorf("%d objetos", o.Count())
	}
}

// A origem nao apagava o objeto por tempo: com o vetor cheio todo drop novo
// era recusado para sempre.
func TestObjetosExpiram(t *testing.T) {
	o := NewMapObjects()
	t0 := time.Now()
	o.Add(&MapObject{ItemID: 100, CreatedAt: t0})
	o.Add(&MapObject{ItemID: 200, CreatedAt: t0.Add(time.Hour)})

	fora := o.Expired(t0.Add(MapObjectLifetime+time.Second), MapObjectLifetime)
	if len(fora) != 1 || fora[0].ItemID != 100 {
		t.Errorf("expirados = %v", fora)
	}
	if o.Count() != 1 {
		t.Errorf("%d objetos sobraram", o.Count())
	}
}

func TestNear(t *testing.T) {
	o := NewMapObjects()
	o.Add(&MapObject{ItemID: 100, Position: model.Position{X: 1000, Y: 1000}})
	o.Add(&MapObject{ItemID: 200, Position: model.Position{X: 5000, Y: 5000}})

	if n := len(o.Near(model.Position{X: 1000, Y: 1000}, 10)); n != 1 {
		t.Errorf("%d perto, queria 1", n)
	}
	if n := len(o.Near(model.Position{X: 1000, Y: 1000}, 10000)); n != 2 {
		t.Errorf("%d perto, queria 2", n)
	}
}

func TestObjetosNilNaoQuebra(t *testing.T) {
	var o *MapObjects
	if o.Count() != 0 {
		t.Error("nil deveria contar zero")
	}
	if _, ok := o.Get(1); ok {
		t.Error("nil nao deveria achar")
	}
	if _, ok := o.Add(&MapObject{}); ok {
		t.Error("nil nao deveria aceitar")
	}
	if _, ok := o.Take(1); ok {
		t.Error("nil nao deveria tirar")
	}
	if o.Expired(time.Now(), time.Minute) != nil || o.Near(model.Position{}, 10) != nil {
		t.Error("nil deveria responder vazio")
	}
	if _, ok := NewMapObjects().Add(nil); ok {
		t.Error("objeto nil nao deveria entrar")
	}
}

// ---- drop ao matar

func TestMatarMobDeixaItemNoChao(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0,
		map[dropdata.Rarity][]uint16{dropdata.Normal: {100}}))
	h.Rand = randFixo{v: 50}

	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if h.MapObjs.Count() != 1 {
		t.Fatalf("%d objetos no chao, queria 1", h.MapObjs.Count())
	}
	obj, _ := h.MapObjs.Get(1)
	if obj.ItemID != 100 {
		t.Errorf("item = %d, queria 100", obj.ItemID)
	}
	// O drop e de quem matou.
	if obj.Owner != s.ClientID {
		t.Errorf("dono = %d, queria %d", obj.Owner, s.ClientID)
	}
	// E o objeto nasce na posicao do mob.
	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	if obj.Position != m.Position {
		t.Errorf("posicao = %v, queria %v", obj.Position, m.Position)
	}
	// O jogador ve o objeto aparecer.
	codigos := map[uint16]bool{}
	for range 6 {
		select {
		case p := <-e.pacotes:
			codigos[p.code] = true
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !codigos[CodeSpawnMapObject] {
		t.Error("o jogador deveria ver o objeto no chao")
	}
}

// Categoria vazia nao dropa nada e nao trava.
func TestMatarMobComCategoriaVazia(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if h.MapObjs.Count() != 0 {
		t.Errorf("%d objetos, queria 0", h.MapObjs.Count())
	}
}

// DropIndex fora da faixa nao quebra: a origem indexava Drops[DropIndex] sem
// checar.
func TestDropIndexForaDaFaixa(t *testing.T) {
	h, _ := handlerComDrops(t, 99, tabelaDeDrops(0,
		map[dropdata.Rarity][]uint16{dropdata.Normal: {100}}))
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if h.MapObjs.Count() != 0 {
		t.Errorf("%d objetos, queria 0", h.MapObjs.Count())
	}
}

// ---- coleta

func TestColetarItem(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Name = "Alfa"
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	idx, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Amount: 1, Position: s.Position, CreatedAt: h.now()})

	if err := h.collectMapItem(context.Background(), e.conn, s,
		asPacket(t, collectPacket(uint32(idx)))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 100 {
		t.Errorf("inventario = %d, queria 100", s.Inventory.Get(0).Index)
	}
	if h.MapObjs.Count() != 0 {
		t.Error("o objeto deveria ter saido do chao")
	}
	if got := e.espera(t); got != CodeRefreshItem {
		t.Errorf("recebeu %#x, queria %#x", got, CodeRefreshItem)
	}
}

// A origem nao checava distancia: bastava o indice para pegar do outro lado
// do mapa.
func TestColetarLongeERecusado(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	idx, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Position: model.Position{X: 5000, Y: 5000}, CreatedAt: h.now()})

	if err := h.collectMapItem(context.Background(), e.conn, s,
		asPacket(t, collectPacket(uint32(idx)))); err != nil {
		t.Fatal(err)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("nao deveria ter coletado de longe")
	}
	if h.MapObjs.Count() != 1 {
		t.Error("o objeto deveria ter ficado no chao")
	}
}

// Indice fora da faixa nao quebra.
func TestColetarIndiceInvalido(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)

	for _, i := range []uint32{0, 1, MaxMapObjects + 1, 4000000000} {
		if err := h.collectMapItem(context.Background(), e.conn, s,
			asPacket(t, collectPacket(i))); err != nil {
			t.Fatalf("indice %d: %v", i, err)
		}
	}
	e.nada(t, 150*time.Millisecond)
}

// O item e de quem matou o mob.
func TestColetarItemDeOutroJogador(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	idx, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Position: s.Position, CreatedAt: h.now(), Owner: 999})

	if err := h.collectMapItem(context.Background(), e.conn, s,
		asPacket(t, collectPacket(uint32(idx)))); err != nil {
		t.Fatal(err)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("nao deveria ter coletado item de outro")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// Dono zero libera para qualquer um.
func TestItemSemDonoQualquerUmPega(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	idx, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Position: s.Position, CreatedAt: h.now(), Owner: 0})

	if err := h.collectMapItem(context.Background(), e.conn, s,
		asPacket(t, collectPacket(uint32(idx)))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 100 {
		t.Error("deveria ter coletado")
	}
}

// Inventario cheio avisa e o item fica no chao.
func TestColetarComInventarioCheio(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: 9000}
	}
	h.Chan.Join(e.conn, s)

	idx, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Position: s.Position, CreatedAt: h.now()})

	if err := h.collectMapItem(context.Background(), e.conn, s,
		asPacket(t, collectPacket(uint32(idx)))); err != nil {
		t.Fatal(err)
	}
	if h.MapObjs.Count() != 1 {
		t.Error("o item deveria ter ficado no chao")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// O slot livre respeita a bolsa: sem a bolsa o grupo nao existe.
func TestSlotLivreRespeitaABolsa(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	s := sessaoComInventario()
	// Tira a primeira bolsa: os slots 0..14 saem de jogo.
	s.Inventory.Slots[60] = model.Item{}

	slot, ok := h.freeInventorySlot(s)
	if !ok {
		t.Fatal("deveria achar slot")
	}
	if slot < 15 {
		t.Errorf("slot = %d, os 0..14 nao deveriam contar sem a bolsa", slot)
	}
}

// Sem bolsa nenhuma nao ha slot.
func TestSemBolsaNaoHaSlot(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	s := sessaoComInventario()
	for _, b := range []int{60, 61, 62, 63} {
		s.Inventory.Slots[b] = model.Item{}
	}
	if _, ok := h.freeInventorySlot(s); ok {
		t.Error("sem bolsa nao deveria haver slot")
	}
}

// Item empilhavel carrega a quantidade; o resto nao usa o campo.
func TestQuantidadeSoParaEmpilhavel(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	itens := novaTabelaDeItens()
	itens.poeItem(100, gamedata.ItemFromList{CanAgroup: 1})
	itens.poeItem(200, gamedata.ItemFromList{CanAgroup: 0})
	itens.poeItem(9000, gamedata.ItemFromList{})
	h.Data = itens

	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	i1, _ := h.MapObjs.Add(&MapObject{
		ItemID: 100, Amount: 7, Position: s.Position, CreatedAt: h.now()})
	i2, _ := h.MapObjs.Add(&MapObject{
		ItemID: 200, Amount: 7, Position: s.Position, CreatedAt: h.now()})

	h.collectMapItem(context.Background(), e.conn, s, asPacket(t, collectPacket(uint32(i1))))
	h.collectMapItem(context.Background(), e.conn, s, asPacket(t, collectPacket(uint32(i2))))

	var empilhavel, normal model.Item
	for i := 0; i < MovableInventorySlots; i++ {
		switch s.Inventory.Get(i).Index {
		case 100:
			empilhavel = s.Inventory.Get(i)
		case 200:
			normal = s.Inventory.Get(i)
		}
	}
	if empilhavel.Refi != 7 {
		t.Errorf("empilhavel com %d, queria 7", empilhavel.Refi)
	}
	if normal.Refi != 0 {
		t.Errorf("nao empilhavel com %d em Refi, queria 0", normal.Refi)
	}
}

// Coletar sem estar em jogo e recusado.
func TestColetarExigeEstarEmJogo(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	s := sessaoAutenticada()
	if err := h.collectMapItem(context.Background(), connDescartavel(t), s,
		asPacket(t, collectPacket(1))); err != ErrNotPlaying {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

// ---- tick

func TestMapObjectTickTiraOsVencidos(t *testing.T) {
	h, agora := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)

	h.MapObjs.Add(&MapObject{ItemID: 100, Position: s.Position, CreatedAt: h.now()})
	if n := h.MapObjectTick(); n != 0 {
		t.Errorf("%d expiraram antes do tempo", n)
	}
	*agora = agora.Add(MapObjectLifetime + time.Second)
	if n := h.MapObjectTick(); n != 1 {
		t.Errorf("%d expiraram, queria 1", n)
	}
	if h.MapObjs.Count() != 0 {
		t.Error("o chao deveria estar limpo")
	}
}

func TestMapObjectTickSemRegistro(t *testing.T) {
	h := testHandler()
	if n := h.MapObjectTick(); n != 0 {
		t.Errorf("= %d", n)
	}
}

// Entrar no mundo e andar desenham os objetos perto.
func TestObjetosAparecemAoAndar(t *testing.T) {
	h, _ := handlerComDrops(t, 0, tabelaDeDrops(0, nil))
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Position = model.Position{X: 1000, Y: 1000}
	h.Chan.Join(e.conn, s)
	h.MapObjs.Add(&MapObject{ItemID: 100, Position: s.Position, CreatedAt: h.now()})

	h.sendMapObjectsNear(s)
	if got := e.espera(t); got != CodeSpawnMapObject {
		t.Errorf("recebeu %#x, queria %#x", got, CodeSpawnMapObject)
	}
}

// O pacote leva o indice, a posicao e o conteudo.
func TestPacoteDoObjeto(t *testing.T) {
	obj := &MapObject{
		Index: 7, Position: model.Position{X: 1234, Y: 4321},
		ContentType: ObjectBoxItem, Amount: 3, CollectTime: 10,
		Face: dropFace, NameID: dropNameID,
	}
	pkt := buildMapObject(obj)
	if pkt.ObjID != 7 || pkt.X != 1234 || pkt.Y != 4321 {
		t.Errorf("= %+v", pkt)
	}
	if pkt.ContentType != ObjectBoxItem || pkt.ContentAmount != 3 {
		t.Errorf("conteudo = %d/%d", pkt.ContentType, pkt.ContentAmount)
	}
	if pkt.Face != dropFace || pkt.NameID != dropNameID {
		t.Errorf("aparencia = %d/%d", pkt.Face, pkt.NameID)
	}
	// Coordenada negativa nao da a volta no WORD.
	neg := &MapObject{Index: 1, Position: model.Position{X: -10, Y: -10}}
	p2 := buildMapObject(neg)
	if p2.X != 0 || p2.Y != 0 {
		t.Errorf("coordenada negativa = %d/%d, queria 0/0", p2.X, p2.Y)
	}
}
