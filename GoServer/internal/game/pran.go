package game

import (
	"context"
	"sync"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/pran"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeRenamePran e o $3E02, o batismo de uma pran.
const CodeRenamePran = 0x3E02

// PranStore guarda os nomes das prans da conta.
type PranStore interface {
	ByAccount(ctx context.Context, accountID uint32) ([]store.PranName, error)
	NameTaken(ctx context.Context, nome string) (bool, error)
	SetName(ctx context.Context, id uint32, nome string) (bool, error)
	LoadAccount(ctx context.Context, accountID uint32) ([]*pran.Pran, error)
	Save(ctx context.Context, p *pran.Pran) error
}

// pranNameMu serializa conferir e gravar o nome: dois jogadores pedindo o
// mesmo nome ao mesmo tempo nao passam os dois pela conferencia.
var pranNameMu sync.Mutex

// pranNameValid aceita 1 a 16 letras ou digitos ASCII. A origem so testava os
// caracteres (IsLetter devolve True para texto vazio), entao nome vazio passava.
func pranNameValid(nome string) bool {
	if nome == "" || len(nome) > 16 {
		return false
	}
	for i := 0; i < len(nome); i++ {
		b := nome[i]
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z') {
			return false
		}
	}
	return true
}

// renamePran e o porte de TPacketHandlers.RenamePran ($3E02): da o nome a
// primeira pran da conta que ainda esta sem nome e devolve o pacote com o id da
// conta, mais os slots 84 e 85 do armazem.
func (h *Handler) renamePran(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.RenamePranPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := netsrv.CString(pkt.Name[:])
	if !pranNameValid(nome) {
		h.message(c, "O nome não pode conter esses caracteres.")
		return nil
	}
	if h.PranDB == nil {
		h.message(c, "Pran indisponivel.")
		return nil
	}

	pranNameMu.Lock()
	defer pranNameMu.Unlock()
	usado, err := h.PranDB.NameTaken(ctx, nome)
	if err != nil {
		h.log().Error("conferir nome de pran", "usuario", s.Username, "erro", err)
		h.message(c, "Nao foi possivel nomear a pran agora.")
		return nil
	}
	if usado {
		h.message(c, "O nome ja está em uso.")
		return nil
	}
	prans, err := h.PranDB.ByAccount(ctx, s.Account.ID)
	if err != nil {
		h.log().Error("buscar prans", "usuario", s.Username, "erro", err)
		h.message(c, "Nao foi possivel nomear a pran agora.")
		return nil
	}
	alvo := -1
	for i, pr := range prans {
		if pr.Name == "" {
			alvo = i
			break
		}
	}
	if alvo < 0 {
		h.message(c, "Todas suas prans já possuem nome.")
		return nil
	}
	ok, err := h.PranDB.SetName(ctx, prans[alvo].ID, nome)
	if err != nil || !ok {
		if err != nil {
			h.log().Error("gravar nome de pran", "usuario", s.Username, "erro", err)
		}
		h.message(c, "Nao foi possivel nomear a pran agora.")
		return nil
	}

	for _, cached := range s.Prans {
		if cached != nil && cached.DBID == prans[alvo].ID {
			cached.Name = nome // senao a gravacao periodica devolveria o nome vazio
		}
	}
	pkt.AccountID = s.Account.ID
	for slot := StorageFirstPranSlot; slot <= StorageLastPranSlot; slot++ {
		h.sendRefreshItem(c, s, store.SlotTypeStorage, uint16(slot), s.Storage.Get(slot))
	}
	return c.SendStruct(CodeRenamePran, pkt)
}

// pranEquipSlot e o slot do Equip do personagem que guarda o item da pran.
const pranEquipSlot = 10

// Codigos de pran.
const (
	CodePranToWorld = 0x907 // servidor -> cliente: dados completos da pran
	CodePranSpawn   = 0x349 // servidor -> cliente: a pran aparece no mundo
)

// loadPrans carrega as prans da conta no login. Sem banco fica vazio.
func (h *Handler) loadPrans(ctx context.Context, s *Session) {
	s.SpawnedPran = -1
	s.Prans = [2]*pran.Pran{}
	if h.PranDB == nil || s.Account.ID == 0 {
		return
	}
	lista, err := h.PranDB.LoadAccount(ctx, s.Account.ID)
	if err != nil {
		h.log().Error("carregar prans", "usuario", s.Username, "erro", err)
	}
	for i := 0; i < len(lista) && i < len(s.Prans); i++ {
		s.Prans[i] = lista[i]
	}
}

// enterPran e o bloco de entrada da pran de TPlayer.SendToWorld: se o item da
// pran esta no Equip[10] do personagem, manda o spawn e os dados dela.
func (h *Handler) enterPran(c *netsrv.Conn, s *Session) {
	id := s.Equip.Get(pranEquipSlot).Identific
	if id <= 0 {
		return
	}
	for slot, p := range s.Prans {
		if p == nil || int32(p.ItemID) != id {
			continue
		}
		h.sendPranSpawn(c, s, slot)
		h.sendPranToWorld(c, s, slot)
		s.SpawnedPran = slot
		p.Spawned = true
		h.applyPranEffects(s, p)
		return
	}
}

// sendPranToWorld e o porte de TPlayer.SendPranToWorld ($907).
func (h *Handler) sendPranToWorld(c *netsrv.Conn, s *Session, slot int) {
	p := s.Prans[slot]
	if p == nil || c == nil {
		return
	}
	var pkt packets.SendPranToWorld
	pkt.Header.Index = shopHeaderIndex // $7535 como a origem
	netsrv.PutCString(pkt.Name[:], p.Name)
	pkt.PranClass = p.Class
	pkt.Food = p.Food
	pkt.Personality = p.Personality()
	pkt.Unk = p.PackSkills()
	for i, v := range p.ItemBar {
		pkt.PranSkillBar[i] = uint8(v)
	}
	pkt.Devotion = uint32(p.Devotion)
	pkt.MaxHP, pkt.CurHP = p.MaxHP, p.CurHP
	pkt.MaxMP, pkt.CurMP = p.MaxMP, p.CurMP
	pkt.Exp = p.Exp
	pkt.DefFis, pkt.DefMag = p.DefFis, p.DefMag
	pkt.Equips = p.Equip
	pkt.Inventory = p.Inventory
	if err := c.SendStruct(CodePranToWorld, pkt); err != nil {
		h.log().Debug("enviar pran", "usuario", s.Username, "erro", err)
	}
}

// IsFairyClass diz se a classe esta na forma de fada.
func IsFairyClass(class uint8) bool { return pran.IsFairy(class) }

// savePrans grava as prans carregadas; a origem as gravava junto do personagem.
func (h *Handler) savePrans(s *Session) {
	if h.PranDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	for _, p := range s.Prans {
		if p == nil || p.DBID == 0 {
			continue
		}
		if err := h.PranDB.Save(ctx, p); err != nil {
			h.log().Error("gravar pran", "usuario", s.Username, "pran", p.DBID, "erro", err)
		}
	}
}

// pranOnDisconnect solta o id de pran do jogador.
func (h *Handler) pranOnDisconnect(s *Session) {
	if h.PranIDs != nil && s.PranClientID != 0 {
		h.PranIDs.Free(s.PranClientID, s.ClientID)
	}
	s.PranClientID = 0
}
