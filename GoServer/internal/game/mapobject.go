package game

import (
	"context"
	"sync"
	"time"

	"aika/internal/dropdata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de objeto no mapa.
const (
	CodeSpawnMapObject  = 0x33B // servidor -> cliente: objeto no chao
	CodeRemoveMapObject = 0x33C // servidor -> cliente: objeto sumiu
	CodeCollectMapItem  = 0x336 // cliente -> servidor: coletar
	CodeCancelCollect   = 0x33A // cliente -> servidor: cancelar a coleta
)

// Tipos de conteudo, as constantes OBJECT_* de GlobalDefs.pas.
const (
	ObjectReliquare = 1
	ObjectBoxItem   = 2
	ObjectBoxGold   = 3
	ObjectBoxEvent  = 4
)

// Aparencia e nome do bau de item que o drop de mob cria, do ramo 321 de
// CreateMapObject.
const (
	dropFace        = 325
	dropNameID      = 915
	dropCollectTime = 10
)

// MaxMapObjects e o tamanho do vetor OBJ do original.
const MaxMapObjects = 1000

// MapObjectLifetime e quanto tempo o objeto fica no chao.
//
// A origem nao apagava o objeto de drop por tempo: ele ficava no vetor OBJ
// ate alguem coletar, e com o vetor cheio CreateMapObject passava a recusar
// todo drop novo com "Erro ao criar o objeto no mapa". Aqui o objeto expira.
const MapObjectLifetime = 3 * time.Minute

// CollectRange e a distancia maxima para coletar.
//
// A origem nao checava distancia na coleta: bastava o indice do objeto, que
// vem do cliente, para pegar um item do outro lado do mapa.
const CollectRange = 5

// MapObject e um objeto no chao. Porte de TOBJ.
type MapObject struct {
	Index       uint16
	Position    model.Position
	ContentType uint8
	ItemID      uint16
	Amount      uint16
	CollectTime uint16
	Face        uint16
	NameID      uint16
	CreatedAt   time.Time
	// Owner e quem tem direito ao item; zero quando qualquer um pode pegar.
	Owner uint16
}

// MapObjects guarda os objetos no chao de um canal.
//
// Substitui o vetor "OBJ: ARRAY [..]" do original, indexado pelo indice que
// vem do cliente no pacote de coleta sem checagem de faixa.
type MapObjects struct {
	mu       sync.Mutex
	objs     map[uint16]*MapObject
	nextHint uint16
	max      uint16
}

// NewMapObjects cria o registro.
func NewMapObjects() *MapObjects {
	return &MapObjects{
		objs:     make(map[uint16]*MapObject, 64),
		nextHint: 1,
		max:      MaxMapObjects,
	}
}

// Count conta os objetos no chao.
func (o *MapObjects) Count() int {
	if o == nil {
		return 0
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.objs)
}

// Get devolve o objeto de um indice.
func (o *MapObjects) Get(index uint16) (*MapObject, bool) {
	if o == nil {
		return nil, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	obj, ok := o.objs[index]
	return obj, ok
}

// Add poe um objeto no chao e devolve o indice alocado.
func (o *MapObjects) Add(obj *MapObject) (uint16, bool) {
	if o == nil || obj == nil {
		return 0, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	idx, ok := o.freeIndexLocked()
	if !ok {
		return 0, false
	}
	obj.Index = idx
	o.objs[idx] = obj
	return idx, true
}

func (o *MapObjects) freeIndexLocked() (uint16, bool) {
	if len(o.objs) >= int(o.max) {
		return 0, false
	}
	start := o.nextHint
	if start < 1 || start > o.max {
		start = 1
	}
	for i := uint16(0); i < o.max; i++ {
		idx := start + i
		if idx > o.max {
			idx -= o.max
			if idx < 1 {
				idx = 1
			}
		}
		if _, usado := o.objs[idx]; !usado {
			o.nextHint = idx + 1
			return idx, true
		}
	}
	return 0, false
}

// Take tira o objeto do chao, devolvendo-o. Serve para a coleta: dois
// jogadores clicando no mesmo item ao mesmo tempo, so um leva.
func (o *MapObjects) Take(index uint16) (*MapObject, bool) {
	if o == nil {
		return nil, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	obj, ok := o.objs[index]
	if !ok {
		return nil, false
	}
	delete(o.objs, index)
	return obj, true
}

// Expired tira do chao os objetos que passaram do tempo e devolve quais.
func (o *MapObjects) Expired(agora time.Time, vida time.Duration) []*MapObject {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	var fora []*MapObject
	for idx, obj := range o.objs {
		if agora.Sub(obj.CreatedAt) >= vida {
			delete(o.objs, idx)
			fora = append(fora, obj)
		}
	}
	return fora
}

// Near lista os objetos a no maximo dist celulas.
func (o *MapObjects) Near(pos model.Position, dist uint16) []*MapObject {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []*MapObject
	for _, obj := range o.objs {
		if pos.InRange(obj.Position, dist) {
			out = append(out, obj)
		}
	}
	return out
}

// ---- drop

// dropFromMob sorteia e poe no chao o item que o mob deixou.
func (h *Handler) dropFromMob(c *netsrv.Conn, s *Session, m *Mob, dropIndex uint16) {
	if h.Drops == nil || h.MapObjs == nil {
		return
	}
	cat := h.Drops.Category(int(dropIndex))
	if cat == nil {
		h.log().Debug("categoria de drop fora da faixa",
			"indice", dropIndex, "mob", m.ClientID)
		return
	}

	rnd := h.rand()
	item, raridade, ok := cat.Roll(rnd.IntN(dropdata.RollMax)+1, int(h.DropRateBonus), rnd)
	if !ok {
		return
	}

	obj := &MapObject{
		Position:    m.Position,
		ContentType: ObjectBoxItem,
		ItemID:      item,
		Amount:      1,
		CollectTime: dropCollectTime,
		Face:        dropFace,
		NameID:      dropNameID,
		CreatedAt:   h.now(),
		Owner:       s.ClientID,
	}
	idx, ok := h.MapObjs.Add(obj)
	if !ok {
		// A origem avisava "Erro ao criar o objeto no mapa. Abra um ticket no
		// suporte." e seguia. Aqui o drop e so registrado: o vetor cheio e
		// problema do servidor, nao do jogador.
		h.log().Warn("sem espaco para objeto no mapa",
			"objetos", h.MapObjs.Count(), "item", item)
		return
	}

	h.broadcastMapObject(obj)
	h.log().Debug("drop no chao",
		"item", item, "raridade", raridade.String(),
		"indice", idx, "mob", m.ClientID, "para", s.Avatar.Name)
}

// broadcastMapObject desenha o objeto para quem esta perto.
func (h *Handler) broadcastMapObject(obj *MapObject) {
	if h.Chan == nil {
		return
	}
	raw, err := netsrv.Marshal(CodeSpawnMapObject, buildMapObject(obj))
	if err != nil {
		h.log().Error("montar objeto do mapa", "indice", obj.Index, "erro", err)
		return
	}
	dist := h.watchDistance()
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if ok && outro.Position.InRange(obj.Position, dist) {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// buildMapObject monta o pacote de objeto no mapa.
func buildMapObject(obj *MapObject) packets.SpawnMapObjectPacket {
	var pkt packets.SpawnMapObjectPacket
	pkt.Header.Index = WorldIndex
	pkt.ObjID = obj.Index
	// As coordenadas vao como WORD; negativo nao existe no mapa.
	pkt.X = uint16(min(max(obj.Position.X, 0), 65535))
	pkt.Y = uint16(min(max(obj.Position.Y, 0), 65535))
	pkt.Face = obj.Face
	pkt.ContentType = obj.ContentType
	pkt.ContentAmount = obj.Amount
	pkt.CollectTime = obj.CollectTime
	pkt.NameID = obj.NameID
	return pkt
}

// sendMapObjectsNear desenha os objetos perto de um jogador que acabou de
// chegar ou de andar.
func (h *Handler) sendMapObjectsNear(s *Session) {
	if h.MapObjs == nil || h.Chan == nil {
		return
	}
	for _, obj := range h.MapObjs.Near(s.Position, h.watchDistance()) {
		raw, err := netsrv.Marshal(CodeSpawnMapObject, buildMapObject(obj))
		if err != nil {
			continue
		}
		_ = h.Chan.SendTo(s.ClientID, raw)
	}
}

// removeMapObject apaga o objeto da tela de quem esta perto.
func (h *Handler) removeMapObject(obj *MapObject) {
	if h.Chan == nil {
		return
	}
	var pkt packets.CancelCollectItem
	pkt.Header.Index = WorldIndex
	pkt.Index = uint32(obj.Index)
	raw, err := netsrv.Marshal(CodeRemoveMapObject, pkt)
	if err != nil {
		return
	}
	dist := h.watchDistance()
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if ok && outro.Position.InRange(obj.Position, dist) {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// ---- coleta

// collectMapItem e o porte de TPacketHandlers.CollectMapItem ($336).
//
// A origem indexava "Servers[].OBJ[Packet.Index]" com o indice vindo do
// cliente e sem checar a faixa do vetor, e nao checava distancia: bastava o
// indice para pegar um item do outro lado do mapa.
func (h *Handler) collectMapItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.MapObjs == nil {
		return nil
	}

	var pkt packets.CollectItem
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Index == 0 || pkt.Index > MaxMapObjects {
		return nil
	}
	idx := uint16(pkt.Index)

	obj, ok := h.MapObjs.Get(idx)
	if !ok {
		return nil
	}
	if !s.Position.InRange(obj.Position, CollectRange) {
		h.log().Debug("coleta longe demais",
			"usuario", s.Username, "objeto", idx)
		return nil
	}
	// O drop e de quem matou o mob; zero libera para todos.
	if obj.Owner != 0 && obj.Owner != s.ClientID {
		h.message(c, "Este item pertence a outro jogador.")
		return nil
	}

	slot, ok := h.freeInventorySlot(s)
	if !ok {
		h.message(c, "Seu inventario esta cheio.")
		return nil
	}

	// Tirar do chao antes de dar: dois jogadores clicando no mesmo item ao
	// mesmo tempo nao podem levar os dois. Na origem a coleta mexia no vetor
	// OBJ sem trava, com uma thread por jogador.
	obj, ok = h.MapObjs.Take(idx)
	if !ok {
		return nil
	}

	s.Inventory.Slots[slot] = model.Item{Index: obj.ItemID, Refi: obj.Amount}
	// Item empilhavel carrega a quantidade em Refi; o resto nao usa o campo.
	if !itemfn.CanAgroup(h.data(), obj.ItemID) {
		s.Inventory.Slots[slot].Refi = 0
	}

	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot), s.Inventory.Slots[slot])
	h.removeMapObject(obj)
	h.log().Info("item coletado",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"item", obj.ItemID, "slot", slot)
	return nil
}

// cancelCollect e o porte de TPacketHandlers.CancelCollectMapItem ($33A).
func (h *Handler) cancelCollect(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	// Nao ha estado de coleta em curso neste porte: a coleta e atomica.
	return nil
}

// freeInventorySlot acha o primeiro slot livre do inventario, respeitando as
// bolsas.
func (h *Handler) freeInventorySlot(s *Session) (int, bool) {
	for slot := 0; slot < MovableInventorySlots; slot++ {
		bolsa := inventoryBagSlot(slot)
		if bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			// Sem a bolsa o grupo de 15 slots nao existe para o jogador.
			continue
		}
		if s.Inventory.Get(slot).IsEmpty() {
			return slot, true
		}
	}
	return 0, false
}

// MapObjectTick tira do chao os objetos vencidos.
func (h *Handler) MapObjectTick() int {
	if h.MapObjs == nil {
		return 0
	}
	fora := h.MapObjs.Expired(h.now(), MapObjectLifetime)
	for _, obj := range fora {
		h.removeMapObject(obj)
	}
	return len(fora)
}
