package game

import (
	"context"
	"errors"

	"aika/internal/charcreate"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeCreateCharacter e o $3E04.
const CodeCreateCharacter = 0x3E04

// CharacterCreator grava um personagem novo.
type CharacterCreator interface {
	Create(ctx context.Context, accountID uint32, n store.NewCharacter) (uint32, error)
	EnsureStorageBag(ctx context.Context, accountID uint32, itemID uint16, slot int) error
}

// createCharacter e o porte de TPacketHandlers.CreateCharacter ($3E04).
//
// O personagem sai do modelo da classe (Server/Bin/Data/BaseAccs, so a parte
// conferida) com face e cabelo do pedido, a bolsa inicial, a municao das
// classes de arma de fogo, as skills iniciais pela regra do codigo e a barra
// de atalhos inicial. Vai ao banco o personagem, os itens, as skills e a barra;
// se qualquer etapa depois do INSERT falha, o personagem e apagado e o jogador
// ve o erro (a origem deixava o personagem pela metade no banco).
//
// Defeitos da origem: nome vazio passava (IsLetter devolve True para texto
// vazio); o slot pedido podia ja estar ocupado e ganhava uma segunda linha; a
// conferencia do nome e a gravacao eram consultas separadas. O bonus de
// indicacao (item de evento 4357 x500) nao esta portado: depende do sistema de
// eventos.
func (h *Handler) createCharacter(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if !canManageChars(s) {
		return ErrNotAuthenticated
	}
	var pkt packets.CreateCharacterRequestPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	refuse := func(msg string) error {
		_ = h.sendCharList(c, s)
		h.message(c, msg)
		return nil
	}

	name := netsrv.CString(pkt.Name[:])
	switch err := charcreate.ValidName(name); {
	case errors.Is(err, charcreate.ErrNameChars), errors.Is(err, charcreate.ErrNameEmpty):
		return refuse("Voce so pode usar caracteres alfanumericos.")
	case errors.Is(err, charcreate.ErrNameLong):
		return refuse("Limitado a 14 caracteres apenas.")
	}
	if pkt.SlotIndex > charcreate.MaxSlot {
		return refuse("SLOT_ERROR")
	}
	grp, err := charcreate.ClassGroup(pkt.ClassIndex)
	if err != nil {
		return refuse("class_id error, try to create your toon again.")
	}
	if charcreate.ValidHair(pkt.Cabelo) != nil {
		return refuse("Cabelo invalido.")
	}
	if h.CharCreate == nil {
		return refuse("Criacao de personagem indisponivel.")
	}
	novo, err := charcreate.Build(h.Templates[grp], pkt.ClassIndex, pkt.Cabelo, pkt.Local)
	if err != nil {
		h.log().Error("modelo de personagem", "classe", grp, "erro", err)
		return refuse("Criacao de personagem indisponivel.")
	}

	id, err := h.CharCreate.Create(ctx, s.Account.ID, store.NewCharacter{
		Name: name, Slot: int(pkt.SlotIndex), ClassInfo: novo.ClassInfo,
		Str: novo.Str, Agi: novo.Agi, Int: novo.Int, Cons: novo.Cons, Luck: novo.Luck, Status: novo.Status,
		Altura: novo.Sizes.Altura, Tronco: novo.Sizes.Tronco, Perna: novo.Sizes.Perna, Corpo: novo.Sizes.Corpo,
		Exp: novo.Exp, Level: novo.Level, Gold: 0, PosX: novo.PosX, PosY: novo.PosY,
	})
	switch {
	case errors.Is(err, store.ErrCharLimit):
		return refuse("Voce ja tem 3 personagens.")
	case errors.Is(err, store.ErrNameTaken):
		return refuse("Ja existe um personagem com esse nome.")
	case errors.Is(err, store.ErrSlotTaken):
		return refuse("Esse slot ja esta ocupado.")
	case err != nil:
		h.log().Error("criar personagem", "usuario", s.Username, "nome", name, "erro", err)
		return refuse("Nao foi possivel criar o personagem.")
	}

	if err := h.saveNewCharacter(ctx, s, id, novo); err != nil {
		h.log().Error("gravar personagem novo", "usuario", s.Username, "personagem", id, "erro", err)
		if h.CharDelete != nil {
			if derr := h.CharDelete.Delete(ctx, s.Account.ID, id); derr != nil {
				h.log().Error("desfazer personagem incompleto", "personagem", id, "erro", derr)
			}
		}
		return refuse("Nao foi possivel criar o personagem.")
	}
	if err := h.CharCreate.EnsureStorageBag(ctx, s.Account.ID, charcreate.ItemStorageBag, charcreate.StorageBagSlot); err != nil {
		h.log().Warn("expansao inicial do armazem", "usuario", s.Username, "erro", err)
	}

	h.log().Info("personagem criado", "usuario", s.Username, "personagem", id, "nome", name, "classe", grp)
	if h.Chars != nil {
		chars, avisos, err := h.Chars.ByAccount(ctx, s.Account.ID)
		if err != nil {
			h.log().Error("recarregar personagens", "usuario", s.Username, "erro", err)
		} else {
			s.Characters = chars
			for _, a := range avisos {
				h.log().Warn("lista de personagens", "aviso", a)
			}
		}
	}
	return h.sendCharList(c, s)
}

// saveNewCharacter grava itens, skills e a barra do personagem novo.
func (h *Handler) saveNewCharacter(ctx context.Context, s *Session, id uint32, n charcreate.New) error {
	if h.Items != nil {
		eq := store.ItemSet{Slots: append([]model.Item(nil), n.Equip[:]...)}
		inv := store.ItemSet{Slots: append([]model.Item(nil), n.Inventory[:]...)}
		if err := h.Items.SaveEquip(ctx, id, eq); err != nil {
			return err
		}
		if err := h.Items.SaveInventory(ctx, id, inv); err != nil {
			return err
		}
	}
	if h.SkillDB != nil {
		if err := h.SkillDB.Save(ctx, id, n.Skills); err != nil {
			return err
		}
	}
	if h.ItemBarDB != nil {
		if err := h.ItemBarDB.Save(ctx, id, n.ItemBar); err != nil {
			return err
		}
	}
	return nil
}
