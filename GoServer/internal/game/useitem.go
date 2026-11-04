package game

import (
	"context"
	"fmt"
	"strings"

	"aika/internal/auth"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/skillfn"
	"aika/internal/store"
)

// Codigos de usar item.
const (
	// CodeUseItem e o $31D: cliente -> servidor, usar o item de um slot.
	CodeUseItem = 0x31D
	// CodeUseBuffItem e o $21B: cliente -> servidor, usar item de buff.
	CodeUseBuffItem = 0x21B
)

// Tipos de item que o uso distingue, as constantes ITEM_TYPE_* de
// GlobalDefs.pas:852.
const (
	ItemTypeHPMPLagrimas    = 29
	ItemTypeBagInv          = 217
	ItemTypeBagStorage      = 218
	ItemTypeBagPran         = 219
	ItemTypeShopOpen        = 224
	ItemTypeStorageOpen     = 226
	ItemTypeUseGoldCoin     = 234
	ItemTypeUseCashCoin     = 239
	ItemTypeCityScroll      = 202
	ItemTypeLocScroll       = 204
	ItemTypeRecipe          = 205
	ItemTypeScrollPortal    = 208
	ItemTypePranDigest      = 230
	ItemTypeSetNation       = 300
	ItemTypeScrollLakia     = 303
	ItemTypeScrollLeopold   = 304
	ItemTypeAddExpPerc      = 404
	ItemTypeHPPotion        = 700
	ItemTypeMPPotion        = 701
	ItemTypePotionBuff      = 702
	ItemTypeUseToUpLevel    = 704
	ItemTypeRandomChest     = 705
	ItemTypeRepairWeapon    = 708
	ItemTypeRepairArmor     = 709
	ItemTypeChest           = 714
	ItemTypeBuff            = 715
	ItemTypeBuff2           = 716
	ItemTypeUseRichGoldCoin = 717
	ItemTypeHPMPPotion      = 800
	ItemTypePranFood        = 26
)

// StorageBagFirstSlot e StorageBagSlots sao os slots de expansao do armazem.
//
// A origem percorria "for i := 1 to 3" sobre "Storage.Itens[80 + i]": o slot
// 80 nunca era considerado, entao a terceira expansao nunca entrava e a
// primeira ficava inalcancavel.
const (
	StorageBagFirstSlot = 80
	StorageBagSlots     = 4
)

// InventoryBagLastSlot e o slot 63, onde o "Simbolo do Viajante" entra.
const InventoryBagLastSlot = 63

// GoldCoinRichValue e o valor fixo do ITEM_TYPE_USE_RICH_GOLD_COIN, chumbado
// na origem ("Player.AddGold(950000000)").
const GoldCoinRichValue int64 = 950_000_000

// ExpPercDivisor e o divisor do ITEM_TYPE_ADD_EXP_PERC: a origem fazia
// "Perc := Value / 1000", entao o UseEffect e por mil.
const ExpPercDivisor = 1000

// soupPrefix e o prefixo de nome que decide se o buff e de sopa.
//
// A origem fazia "Copy(String(ItemList[Item.Index].Name), 0, 4) = 'Sopa'". O
// indice 0 em Copy e tolerado pelo Delphi e vale 1, entao a comparacao e com
// os quatro primeiros caracteres do nome. E fragil (depende do nome na tabela
// de itens), mas e o que decide a categoria em jogo, entao foi mantido.
const soupPrefix = "Sopa"

// buffConflicts sao os buffs que nao se combinam, por indice de skill.
//
// A origem repetia o mesmo bloco de cinco ramos quase identicos para as
// pocoes de valor de batalha (493 a 497), mais ramos soltos para 251, 285,
// 280/281, 298 e 305. Aqui a regra e uma tabela.
var buffConflicts = map[uint32][]uint32{
	251: {251},
	285: {285},
	305: {305},
	280: {281},
	281: {280},
	298: {176},
	493: {494, 495, 496, 497},
	494: {493, 495, 496, 497},
	495: {493, 494, 496, 497},
	496: {493, 494, 495, 497},
	497: {493, 494, 495, 496},
}

// useItem e o porte de TPacketHandlers.UseItem ($31D) com a parte de
// TItemFunctions.UseItem cujos sistemas ja estao portados.
//
// Diferencas centrais em relacao a origem:
//
//   - A origem conferia "Packet.Slot > 63" e seguia, SEM checar se o slot
//     tinha item: para um slot vazio ela lia "ItemList[0]" e aplicava o
//     efeito daquela linha. Aqui o slot vazio e recusado.
//
//   - "ItemList[item.Index]" era indexado com um WORD vindo do banco, sem
//     conferir a faixa, em dezenas de pontos. Aqui a tabela devolve nil fora
//     da faixa e o pedido e recusado.
//
//   - O tipo de item desconhecido caia em "else Exit" sem nenhuma mensagem:
//     o cliente ficava esperando. Aqui o jogador recebe aviso e o log registra
//     o tipo, o que torna visivel o que falta portar.
func (h *Handler) useItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	// "if (Player.Base.IsDead) then Exit".
	if s.Dead {
		return nil
	}

	var pkt packets.UseItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Slot >= store.InventorySlots {
		return nil
	}

	switch pkt.TypeSlot {
	case store.SlotTypeInventory:
		return h.useInventoryItem(ctx, c, s, int(pkt.Slot), pkt.Type1)
	case store.SlotTypeItemUse:
		return h.repairWithItem(c, s, int(pkt.Slot), pkt.Type1)
	case store.SlotTypeCash:
		return h.usePremiumItem(ctx, c, s, int(pkt.Slot))
	default:
		// A origem caia no fim do case sem responder.
		h.log().Debug("usar item com tipo de slot desconhecido",
			"usuario", s.Username, "tipo", pkt.TypeSlot, "slot", pkt.Slot)
		return nil
	}
}

// useInventoryItem e o porte de TItemFunctions.UseItem.
func (h *Handler) useInventoryItem(ctx context.Context, c *netsrv.Conn,
	s *Session, slot int, tipo1 uint32) error {

	it := s.Inventory.Get(slot)
	if it.IsEmpty() {
		return nil
	}
	t := h.data()
	if t.Item(int(it.Index)) == nil {
		h.log().Warn("item fora da tabela no uso",
			"usuario", s.Username, "slot", slot, "item", it.Index)
		return nil
	}
	// Item selado nao funciona. A origem conferia isso em UseBuffItem mas
	// nao aqui, entao um item selado dava o efeito e seguia selado.
	if it.IsSealed() {
		h.message(c, "Este item esta selado.")
		return nil
	}
	// "if (not Player.IsGM) and (Level < ItemList[].Level) then Exit".
	if !h.isGM(s) && s.Avatar.Level < itemfn.ItemLevel(t, it.Index) {
		h.message(c, "Seu nivel e baixo para este item.")
		return nil
	}

	efeito := itemfn.UseEffect(t, it.Index)
	consumir := true

	switch itemfn.ItemType(t, it.Index) {
	case ItemTypeHPPotion:
		h.healPlayer(c, s, uint32(efeito), 0)

	case ItemTypeMPPotion:
		h.healPlayer(c, s, 0, uint32(efeito))

	case ItemTypeHPMPPotion, ItemTypeHPMPLagrimas:
		h.healPlayer(c, s, uint32(efeito), uint32(efeito))

	case ItemTypeUseGoldCoin:
		h.giveGold(c, s, int64(itemfn.SellPrice(t, it.Index)))

	case ItemTypeUseRichGoldCoin:
		h.giveGold(c, s, GoldCoinRichValue)

	case ItemTypeAddExpPerc:
		if !h.giveExpPerc(c, s, efeito) {
			return nil
		}

	case ItemTypeUseToUpLevel:
		if !h.giveLevels(c, s, efeito) {
			return nil
		}

	case ItemTypePotionBuff, ItemTypeBuff, ItemTypeBuff2:
		if !h.applyItemBuff(c, s, it.Index, efeito) {
			return nil
		}

	case ItemTypeBagInv:
		if !h.useInventoryBag(c, s, slot) {
			return nil
		}
		// O slot de origem ja foi esvaziado pela troca.
		consumir = false

	case ItemTypeBagStorage:
		if !h.useStorageBag(c, s, slot) {
			return nil
		}
		consumir = false

	case ItemTypeStorageOpen:
		// Porte do "OpennedOption := 7; OpennedNPC := Base.ClientID" da
		// origem: o item abre o armazem sem NPC, e o proprio jogador faz o
		// papel de NPC aberto.
		s.OpenedOption = NPCOptionStorage
		s.OpenedNPC = s.ClientID
		h.sendStorage(c, s, StorageTypePlayer)

	default:
		h.message(c, "Este item ainda nao esta disponivel neste servidor.")
		h.log().Info("tipo de item sem porte no uso",
			"usuario", s.Username, "item", it.Index,
			"tipo", itemfn.ItemType(t, it.Index),
			"nome", itemfn.ItemName(t, it.Index))
		return nil
	}

	if consumir {
		h.consumeItem(c, s, slot, 1)
	}
	return nil
}

// consumeItem tira uma unidade do slot e avisa o cliente.
//
// Porte de TItemFunctions.DecreaseAmount seguido de SendRefreshItemSlot. A
// origem usava Refi como quantidade, o mesmo campo que guarda o refino de
// equipamento; mantido, porque e o que o cliente le.
func (h *Handler) consumeItem(c *netsrv.Conn, s *Session, slot int, quant uint16) {
	if slot < 0 || slot >= len(s.Inventory.Slots) {
		return
	}
	it := &s.Inventory.Slots[slot]
	if it.Refi > quant {
		it.Refi -= quant
	} else {
		*it = model.Item{}
	}
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot), *it)
}

// healPlayer soma HP e MP respeitando o maximo.
//
// A origem fazia "Inc(CurrentScore.CurHP, UseEffect)" sem teto nenhum: beber
// pocao com a vida cheia empurrava CurHP acima de MaxHP, e o personagem era
// gravado assim. O envio ao cliente limitava o valor na hora de montar o
// pacote, entao o excesso ficava invisivel e preso no registro.
func (h *Handler) healPlayer(c *netsrv.Conn, s *Session, hp, mp uint32) {
	if hp > 0 {
		s.Avatar.CurHP = min(s.Avatar.CurHP+hp, s.Avatar.MaxHP)
	}
	if mp > 0 {
		s.Avatar.CurMP = min(s.Avatar.CurMP+mp, s.Avatar.MaxMP)
	}
	h.sendCurrentHPMP(c, s)
}

// giveGold soma ouro respeitando o teto.
//
// A origem chamava Player.AddGold, que faz "Inc(Gold, Value)" sem teto.
func (h *Handler) giveGold(c *netsrv.Conn, s *Session, valor int64) {
	if valor <= 0 {
		return
	}
	if valor > MaxGold-s.Avatar.Gold {
		s.Avatar.Gold = MaxGold
	} else {
		s.Avatar.Gold += valor
	}
	h.sendRefreshMoney(c, s)
	h.message(c, fmt.Sprintf("Voce recebeu %d de ouro.", valor))
}

// giveExpPerc e o porte de TPlayer.AddExpPerc.
//
// Diferencas: a origem fazia "ExpList[Level] - ExpList[Level - 1]" com Level
// num vetor dinamico e sem conferir a faixa — nivel 0 lia ExpList[-1] — e
// tinha um "if Level = 90 then Exit" chumbado que barrava o item exatamente
// no nivel 90 e deixava passar no 91.
func (h *Handler) giveExpPerc(c *netsrv.Conn, s *Session, porMil uint16) bool {
	if h.Exp == nil || porMil == 0 {
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}
	nivel := s.Avatar.Level
	if nivel == 0 {
		return false
	}
	atual, ok1 := h.Exp.ExpForLevel(nivel)
	anterior, ok2 := h.Exp.ExpForLevel(nivel - 1)
	if !ok1 || !ok2 || atual <= anterior {
		h.message(c, "Seu nivel esta fora da tabela de experiencia.")
		return false
	}
	faixa := int64(atual - anterior)
	ganho := faixa * int64(porMil) / ExpPercDivisor
	if ganho <= 0 {
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}
	h.addExp(c, s, ganho, ExpTypeMob)
	return true
}

// giveLevels e o porte do ITEM_TYPE_USE_TO_UP_LVL.
//
// A origem tinha dois caminhos. Para UseEffect = 1 ela calculava
//
//	Level    := UseEffect * 50;                        // sempre 50
//	LevelExp := ExpList[Player.Level + (Level - 1)] + 1;
//
// indexando um vetor dinamico 49 posicoes a frente do nivel atual. Acima do
// fim da tabela caia no "except", cujo fallback era "High(ExpList)" — o
// ultimo INDICE, nao o ultimo valor —, e em seguida
// "AddExp := LevelExp - UInt64(Exp)" dava a volta no UInt64 e o personagem
// recebia cerca de 1,8e19 de experiencia. E o mesmo defeito do /addlevel.
// Para qualquer outro UseEffect ela chamava AddLevel(UseEffect), que soma
// niveis de uma vez.
//
// Aqui os dois casos viram a mesma conta: o destino e conferido contra a
// tabela e a experiencia vai pelo addExp, que sobe nivel por nivel.
func (h *Handler) giveLevels(c *netsrv.Conn, s *Session, efeito uint16) bool {
	if h.Exp == nil || efeito == 0 {
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}
	niveis := int64(efeito)
	if efeito == 1 {
		// O "UseEffect * 50" da origem, com o -1 que ela aplicava.
		niveis = 49
	}
	destino := int64(s.Avatar.Level) + niveis
	if destino > 65535 {
		h.message(c, "Nivel fora da tabela de experiencia.")
		return false
	}
	alvo, ok := h.Exp.ExpForLevel(uint16(destino))
	if !ok {
		h.message(c, "Nivel fora da tabela de experiencia.")
		return false
	}
	if int64(alvo) <= s.Avatar.Exp {
		h.message(c, "Voce ja passou do nivel deste item.")
		return false
	}
	h.addExp(c, s, int64(alvo)-s.Avatar.Exp, ExpTypeMob)
	return true
}

// applyItemBuff aplica o buff de um item de poção, buquê ou poção especial.
//
// Diferencas centrais em relacao a origem:
//
//   - "SkillData[ItemList[Item.Index].UseEffect]" era indexado com um valor
//     da tabela de itens, sem nenhuma checagem de faixa, e isso aparecia ate
//     seis vezes no mesmo ramo. Aqui skillfn.Data devolve nil fora da faixa.
//
//   - A tabela de conflitos substitui cinco ramos quase identicos.
func (h *Handler) applyItemBuff(c *netsrv.Conn, s *Session, item, efeito uint16) bool {
	if efeito == 0 {
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}
	d := skillfn.Data(h.SkillTable, uint32(efeito))
	if d == nil {
		h.log().Warn("item de buff aponta para skill fora da tabela",
			"usuario", s.Username, "item", item, "efeito", efeito)
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}
	nomeSkill := netsrv.CString(d.Name[:])

	// Sopa: so uma de cada vez, achada pelo prefixo do nome do ITEM.
	if strings.HasPrefix(itemfn.ItemName(h.data(), item), soupPrefix) {
		if h.hasSoupBuff(s) {
			h.message(c, fmt.Sprintf("Nao e combinavel com [%s].", nomeSkill))
			return false
		}
		h.applyBuff(s, efeito)
		h.sendStatus(c, s)
		return true
	}

	for _, conflito := range buffConflicts[d.Index] {
		if h.hasBuffWithSkillIndex(s, conflito) {
			h.message(c, fmt.Sprintf("Nao e combinavel com [%s].", nomeSkill))
			return false
		}
	}

	// A origem nao conferia a duracao: um buff sem linha na tabela de duracao
	// entrava e nunca saia.
	if h.durations().BuffSeconds(efeito) <= 0 {
		h.log().Warn("buff de item sem duracao na tabela",
			"usuario", s.Username, "item", item, "buff", efeito)
		h.message(c, "Este item nao tem efeito configurado.")
		return false
	}

	h.applyBuff(s, efeito)
	h.sendStatus(c, s)
	return true
}

// hasBuffWithSkillIndex e o porte de TBaseMob.BuffExistsByIndex e de
// BuffExistsInArray.
//
// As duas NAO comparam com a chave do buff: elas passam cada chave ativa pela
// tabela ("SkillData[i].Index = j") e comparam o campo Index da linha. Ou
// seja, o conflito e por indice de SKILL, nao por chave de buff, e as duas
// coisas so coincidem quando a linha da tabela tem Index igual a sua posicao.
// A origem fazia esse SkillData[i] sem conferir a faixa da chave.
func (h *Handler) hasBuffWithSkillIndex(s *Session, want uint32) bool {
	if s.Base.Buffs == nil {
		return false
	}
	for _, chave := range s.Base.Buffs.Indexes() {
		d := skillfn.Data(h.SkillTable, uint32(chave))
		if d != nil && d.Index == want {
			return true
		}
	}
	return false
}

// hasSoupBuff e o porte de TBaseMob.BuffExistsSopa: procura entre os buffs
// ativos um cuja skill comece por "Sopa".
func (h *Handler) hasSoupBuff(s *Session) bool {
	if s.Base.Buffs == nil {
		return false
	}
	for _, idx := range s.Base.Buffs.Indexes() {
		d := skillfn.Data(h.SkillTable, uint32(idx))
		if d == nil {
			continue
		}
		if strings.HasPrefix(netsrv.CString(d.Name[:]), soupPrefix) {
			return true
		}
	}
	return false
}

// useInventoryBag e o porte do ITEM_TYPE_BAG_INV, o "Simbolo do Viajante".
//
// A origem copiava o item para "Inventory[63]" SEM conferir se o slot 63 ja
// tinha bolsa: usar um segundo simbolo apagava o primeiro, e com ele os 15
// slots que ele habilitava — os itens que estivessem ali ficavam presos.
func (h *Handler) useInventoryBag(c *netsrv.Conn, s *Session, slot int) bool {
	if slot == InventoryBagLastSlot {
		return false
	}
	if !s.Inventory.Get(InventoryBagLastSlot).IsEmpty() {
		h.message(c, "Limite de expansao atingido.")
		return false
	}
	it := s.Inventory.Slots[slot]
	// A origem chamava SetItemDuration antes de mover: a bolsa passa a valer
	// por tempo. A duracao do item ainda nao esta portada, entao a bolsa vai
	// sem prazo e o fato fica registrado.
	s.Inventory.Slots[InventoryBagLastSlot] = it
	s.Inventory.Slots[slot] = model.Item{}

	h.sendRefreshItem(c, s, store.SlotTypeInventory, InventoryBagLastSlot, it)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot),
		s.Inventory.Get(slot))
	h.message(c, fmt.Sprintf("Selo de [%s] foi removido.",
		itemfn.ItemName(h.data(), it.Index)))
	return true
}

// useStorageBag e o porte do ITEM_TYPE_BAG_STORAGE, o "Simbolo da
// Determinacao".
//
// Tres defeitos da origem:
//
//   - o laco era "for i := 1 to 3" sobre "Storage.Itens[80 + i]", entao o
//     slot 80 NUNCA era considerado: a primeira expansao do armazem era
//     inalcancavel e so tres das quatro funcionavam;
//
//   - nao havia Break, e BagSlot guardava o ULTIMO slot vazio em vez do
//     primeiro;
//
//   - o aviso ao cliente ia como "SendRefreshItemSlot(INV_TYPE, BagSlot, …)"
//     com BagSlot entre 81 e 83 — tipo de slot errado, e um slot que nao
//     existe no inventario (0..63). A expansao so aparecia depois de relogar.
func (h *Handler) useStorageBag(c *netsrv.Conn, s *Session, slot int) bool {
	destino := -1
	for i := range StorageBagSlots {
		alvo := StorageBagFirstSlot + i
		if s.Storage.Get(alvo).IsEmpty() {
			destino = alvo
			break
		}
	}
	if destino < 0 {
		h.message(c, "Limite de expansao atingido.")
		return false
	}
	if destino >= len(s.Storage.Slots) {
		return false
	}
	it := s.Inventory.Slots[slot]
	s.Storage.Slots[destino] = it
	s.Inventory.Slots[slot] = model.Item{}

	h.sendRefreshItem(c, s, store.SlotTypeStorage, uint16(destino), it)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot),
		s.Inventory.Get(slot))
	h.message(c, fmt.Sprintf("Selo de [%s] foi removido.",
		itemfn.ItemName(h.data(), it.Index)))
	return true
}

// repairWithItem e o porte do ramo ITEM_USE_TYPE de TPacketHandlers.UseItem:
// a pedra de reparo (708 para arma, 709 para armadura e escudo).
//
// Diferencas centrais em relacao a origem:
//
//   - Type1 e DWORD e vem do cliente. No ramo 708 ele indexava
//     "Equip[Packet.Type1]" com Equip sendo ARRAY [0..15] e NENHUMA checagem
//     de faixa — e a linha seguinte ESCREVIA ali
//     ("Equip[Type1].Min := Equip[Type1].Max"). O ramo 709 tinha a checagem
//     ("if(Packet.Type1 > 15) then Exit"); o 708 nao.
//
//   - A mensagem "Item reparado com sucesso." era enviada mesmo quando nada
//     foi reparado, porque ficava fora do if que conferia o slot do item.
//
//   - A pedra era consumida em qualquer caso, inclusive quando o alvo nao
//     servia.
func (h *Handler) repairWithItem(c *netsrv.Conn, s *Session, slot int, tipo1 uint32) error {
	pedra := s.Inventory.Get(slot)
	if pedra.IsEmpty() {
		return nil
	}
	t := h.data()
	if t.Item(int(pedra.Index)) == nil {
		return nil
	}

	var aceitos []int
	switch itemfn.ItemType(t, pedra.Index) {
	case ItemTypeRepairWeapon:
		// GetItemEquipSlot = 6 na origem: arma.
		aceitos = []int{WeaponSlot}
	case ItemTypeRepairArmor:
		// "in [2,3,4,5,7]": elmo, armadura, luvas, botas e escudo.
		aceitos = []int{2, 3, 4, 5, 7}
	default:
		h.message(c, "Item ainda nao configurado.")
		return nil
	}

	// "if(Packet.Type1 > 16)" na origem: acima de 16 o alvo esta no
	// inventario, com 16 de deslocamento.
	tipoAlvo := store.SlotTypeEquip
	alvo := int(tipo1)
	if tipo1 > store.EquipSlots {
		tipoAlvo = store.SlotTypeInventory
		alvo = int(tipo1) - store.EquipSlots
	}

	var it *model.Item
	switch tipoAlvo {
	case store.SlotTypeEquip:
		if alvo < 0 || alvo >= store.EquipSlots || alvo >= len(s.Equip.Slots) {
			return nil
		}
		it = &s.Equip.Slots[alvo]
	default:
		if alvo < 0 || alvo >= store.InventorySlots || alvo >= len(s.Inventory.Slots) {
			return nil
		}
		it = &s.Inventory.Slots[alvo]
	}
	if it.IsEmpty() {
		return nil
	}

	destinoOK := false
	for _, a := range aceitos {
		if itemfn.EquipSlot(t, it.Index) == a {
			destinoOK = true
			break
		}
	}
	if !destinoOK {
		h.message(c, "Esta pedra nao serve para este item.")
		return nil
	}
	if it.Min >= it.Max {
		h.message(c, "Este item nao precisa de reparo.")
		return nil
	}

	it.Min = it.Max
	h.sendRefreshItem(c, s, uint8(tipoAlvo), uint16(alvo), *it)
	h.consumeItem(c, s, slot, 1)
	h.message(c, "Item reparado com sucesso.")
	h.log().Info("item reparado",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"tipo_slot", tipoAlvo, "slot", alvo, "item", it.Index)
	return nil
}

// useBuffItem e o porte de TPacketHandlers.UseBuffItem ($21B).
//
// O defeito central: a origem NUNCA consumia o item. Nem DecreaseAmount nem
// SendRefreshItemSlot aparecem na funcao — um item de buff dava buff infinito,
// quantas vezes o jogador clicasse. Aqui o item e consumido como em qualquer
// outro uso.
func (h *Handler) useBuffItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if s.Dead {
		return nil
	}
	var pkt packets.UseBuffItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Slot >= store.InventorySlots {
		return nil
	}
	it := s.Inventory.Get(int(pkt.Slot))
	if it.IsEmpty() || it.IsSealed() {
		return nil
	}
	t := h.data()
	if t.Item(int(it.Index)) == nil {
		return nil
	}

	switch itemfn.ItemType(t, it.Index) {
	case ItemTypeBuff, ItemTypeBuff2, ItemTypePotionBuff:
		if !h.applyItemBuff(c, s, it.Index, itemfn.UseEffect(t, it.Index)) {
			return nil
		}
	default:
		// A origem tinha "0: Result := False" e nenhum ramo padrao: todo
		// outro tipo saia calado.
		h.log().Debug("item de buff com tipo inesperado",
			"usuario", s.Username, "item", it.Index,
			"tipo", itemfn.ItemType(t, it.Index))
		return nil
	}

	h.consumeItem(c, s, int(pkt.Slot), 1)
	return nil
}

// isGM diz se a conta ignora o requisito de nivel do item.
//
// Porte de TPlayer.IsGM, que e "Ord(AccountType) = 5" — exatamente Admin.
// Apesar do nome, um Game Master (4) NAO passa por essa porta na origem;
// mantido assim, porque e o que vale em jogo.
func (h *Handler) isGM(s *Session) bool {
	return s.Account.Type >= auth.AccAdmin
}
