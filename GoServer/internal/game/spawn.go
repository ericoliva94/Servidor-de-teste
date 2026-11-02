package game

import (
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
)

// Codigos de spawn e remocao.
const (
	CodeCreateMob = 0x349 // servidor -> cliente: desenha uma entidade
	CodeRemoveMob = 0x101 // servidor -> cliente: apaga uma entidade
)

// Tipos de spawn. SPAWN_NORMAL da origem.
const (
	SpawnNormal = 0
	// SpawnPlayerKillFlag e o $80 que a origem somava ao SpawnType de quem
	// esta com o nome vermelho.
	SpawnPlayerKillFlag = 0x80
)

// EffectTypeShop e o valor que a origem punha em EffectType quando o
// personagem esta com loja pessoal aberta.
const EffectTypeShop = 2

// nationShift e o multiplicador de GuildIndexAndNation: a origem gravava
// "Nation * 4096", isto e, a nacao nos 4 bits altos e a guilda nos 12 baixos.
const nationShift = 4096

// guildIndexMask e o maior indice de guilda que cabe nos 12 bits baixos.
//
// A origem montava o campo com StrToInt('$' + IntToStr(Nation) +
// IntToHex(GuildIndex, 3)): texto hexadecimal concatenado. Com GuildIndex
// acima de $FFF o IntToHex devolvia 4 digitos, o numero montado passava de 16
// bits e o StrToInt estourava a faixa do WORD. Aqui o indice e mascarado.
const guildIndexMask = 0x0FFF

// buildCreateMob monta o $349 de uma sessao. Porte de TBaseMob.GetCreateMob.
func (h *Handler) buildCreateMob(s *Session, spawnType uint8) packets.SendCreateMobPacket {
	var pkt packets.SendCreateMobPacket
	a := &s.Avatar

	pkt.Header.Index = s.ClientID
	netsrv.PutCString(pkt.Name[:], a.Name)
	pkt.Equip = a.VisibleEquip()
	pkt.ItemEff[7] = a.RefineSlot6
	pkt.Position.X = s.Position.X
	pkt.Position.Y = s.Position.Y
	pkt.Rotation = a.Rotation

	// O jogador manda o maximo nos campos MaxHP/MaxMP e o atual nos CurHP/
	// CurMP. A origem preenchia MaxMP com MaxHP no ramo de mob/NPC — erro de
	// copia que aqui nao se repete.
	pkt.MaxHP = a.MaxHP
	pkt.MaxMP = a.MaxMP
	pkt.CurHP = min(a.CurHP, a.MaxHP)
	pkt.CurMP = min(a.CurMP, a.MaxMP)

	pkt.Unk0 = 0x0A // valor fixo do ramo de jogador da origem
	pkt.Effects[1] = 0x1D
	// SpeedMove e BYTE no pacote e WORD no personagem.
	pkt.SpeedMove = uint8(min(a.SpeedMove, 255))

	if a.PlayerKill {
		// A origem fazia Inc(SpawnType, $80) num WORD e depois gravava num
		// BYTE; o or deixa claro que e um bit de bandeira.
		spawnType |= SpawnPlayerKillFlag
	}
	pkt.SpawnType = spawnType

	pkt.Altura = a.Altura
	pkt.Tronco = a.Tronco
	pkt.Perna = a.Perna
	pkt.Corpo = a.Corpo

	pkt.GuildIndexAndNation = a.Nation * nationShift
	if a.GuildIndex > 0 {
		netsrv.PutCString(pkt.Title[:], a.GuildName)
		pkt.GuildIndexAndNation = a.Nation*nationShift | (a.GuildIndex & guildIndexMask)
	}

	pkt.TitleId = a.TitleID
	// A origem fazia "ActiveTitle.Level - 1" num BYTE: com titulo de nivel 0
	// (sem titulo) o campo ia como 255. Aqui o piso e zero.
	if a.TitleLevel > 0 {
		pkt.TitleLevel = a.TitleLevel - 1
	}

	if a.ShopName != "" {
		netsrv.PutCString(pkt.Title[:], a.ShopName)
		pkt.Corpo = 3
		pkt.EffectType = EffectTypeShop
	}

	// Buffs ativos e o instante em que cada um termina.
	//
	// A origem gravava "DateTimeToUnix(IncSecond(_buffs[Index], Duration))":
	// instante inicial mais a duracao da skill.
	if s.Base.Buffs != nil {
		dur := h.durations()
		i := 0
		for _, index := range s.Base.Buffs.Indexes() {
			if i >= len(pkt.Buffs) {
				// A origem nao tinha este limite: o laco
				// "for Index in Self._buffs.Keys" escrevia em Packet.Buffs[i]
				// com i crescendo sem teto, e a partir do 60o buff passava do
				// fim do vetor, por cima do resto do pacote.
				break
			}
			inicio, ok := s.Base.Buffs.StartedAt(index)
			if !ok {
				continue
			}
			fim := inicio.Add(time.Duration(dur.BuffSeconds(index)) * time.Second)
			pkt.Buffs[i] = index
			pkt.Time[i] = uint32(fim.Unix())
			i++
		}
	}
	return pkt
}

// buildRemoveMob monta o $101, que apaga uma entidade da tela.
func buildRemoveMob(clientID uint16) packets.SendRemoveMobPacket {
	var pkt packets.SendRemoveMobPacket
	pkt.Header.Index = WorldIndex
	pkt.Index = uint32(clientID)
	return pkt
}

// spawnPair poe duas sessoes na lista de visiveis uma da outra e manda a cada
// uma o pacote de criacao da outra.
//
// Porte de TBaseMob.AddToVisible. A origem chegava ao mesmo resultado por
// recursao mutua (Self.AddToVisible chamava mob.AddToVisible, que chamava
// Self.AddToVisible de volta e parava porque o primeiro passo ja havia
// inserido na lista); aqui os dois lados sao tratados de frente.
func (h *Handler) spawnPair(a, b *Session) {
	if a == nil || b == nil || a.ClientID == b.ClientID {
		return
	}
	a.Base.VisiblePlayers.Add(b.ClientID)
	b.Base.VisiblePlayers.Add(a.ClientID)
	h.sendSpawn(b, a.ClientID) // b aparece para a
	h.sendSpawn(a, b.ClientID) // a aparece para b
}

// despawnPair desfaz o par. Porte de TBaseMob.RemoveFromVisible.
func (h *Handler) despawnPair(a, b *Session) {
	if a == nil || b == nil || a.ClientID == b.ClientID {
		return
	}
	a.Base.VisiblePlayers.Remove(b.ClientID)
	b.Base.VisiblePlayers.Remove(a.ClientID)
	if a.Base.Targets != nil {
		a.Base.Targets.Remove(b.ClientID)
	}
	if b.Base.Targets != nil {
		b.Base.Targets.Remove(a.ClientID)
	}
	h.sendRemove(b.ClientID, a.ClientID)
	h.sendRemove(a.ClientID, b.ClientID)
}

// sendSpawn manda o pacote de criacao de quem para quem.
func (h *Handler) sendSpawn(quem *Session, paraID uint16) {
	if h.Chan == nil {
		return
	}
	pkt := h.buildCreateMob(quem, SpawnNormal)
	raw, err := netsrv.Marshal(CodeCreateMob, pkt)
	if err != nil {
		h.log().Error("montar spawn", "cliente", quem.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(paraID, raw)
}

// sendRemove manda o pacote que apaga quemID da tela de paraID.
func (h *Handler) sendRemove(quemID, paraID uint16) {
	if h.Chan == nil {
		return
	}
	raw, err := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(quemID))
	if err != nil {
		h.log().Error("montar remocao", "cliente", quemID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(paraID, raw)
}

// removeFromEveryone apaga a sessao da tela e da lista de todo mundo que a
// via. Usado na saida do jogador.
//
// E o que faltava na origem: a limpeza dependia de Connection.Destroy ser
// chamado, e quando nao era o jogador desconectado continuava na lista de
// visiveis dos outros, que viam um boneco parado para sempre.
func (h *Handler) removeFromEveryone(s *Session) {
	if h.Chan == nil || s.ClientID == 0 {
		return
	}
	ids := s.Base.VisiblePlayers.IDs()
	raw, err := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(s.ClientID))
	if err != nil {
		h.log().Error("montar remocao", "cliente", s.ClientID, "erro", err)
		return
	}
	for _, id := range ids {
		outro, _, ok := h.Chan.Lookup(id)
		if ok {
			outro.Base.VisiblePlayers.Remove(s.ClientID)
			if outro.Base.Targets != nil {
				outro.Base.Targets.Remove(s.ClientID)
			}
		}
		_ = h.Chan.SendTo(id, raw)
	}
	s.Base.VisiblePlayers.Clear()
}
