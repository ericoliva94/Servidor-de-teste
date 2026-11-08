package game

import (
	"context"
	"fmt"

	"aika/internal/combat"
	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/mobdata"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/skillfn"
	"aika/internal/store"
)

// Codigos de skill.
const (
	CodeLearnSkill   = 0x31C // cliente -> servidor: aprender ou subir nivel
	CodeUseSkill     = 0x320 // cliente -> servidor: lancar
	CodeResetSkills  = 0x32A // cliente -> servidor: zerar a arvore
	CodeCancelSkill  = 0x327 // cliente -> servidor: cancelar o lancamento
	CodePlayerSkills = 0x106 // servidor -> cliente: indices das skills
	CodeSkillLevels  = 0x107 // servidor -> cliente: pontos e niveis
)

// BulletSlot e o slot 15, onde ficam as balas das classes 2 e 3.
const BulletSlot = 15

// ItemTypeInfiniteBullet e o ItemType 52, a bala que nao se gasta.
const ItemTypeInfiniteBullet = 52

// learnSkill e o porte de TPacketHandlers.LearnSkill ($31C).
//
// A origem lia "SkillData[Packet.SkillIndex]" cinco vezes antes de qualquer
// validacao, com o indice vindo do cliente como DWORD num ARRAY [0..11998]:
// um indice qualquer lia memoria fora da tabela, e o que viesse de lixo
// decidia nivel minimo, pontos, custo e classe.
func (h *Handler) learnSkill(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.LearnSkillPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	d := skillfn.Data(h.SkillTable, pkt.SkillIndex)
	if d == nil {
		h.log().Warn("skill fora da tabela",
			"indice", pkt.SkillIndex, "usuario", s.Username, "ip", c.IP())
		return nil
	}
	if d.Index == skillfn.BlockedSkillIndex {
		h.message(c, "Esta habilidade nao esta disponivel.")
		return nil
	}

	if s.Avatar.Level < uint16(min(d.MinLevel, 65535)) {
		h.message(c, fmt.Sprintf("Nao possui o nivel necessario (%d).", d.MinLevel))
		return nil
	}
	if uint32(s.Avatar.SkillPoint) < d.SkillPoints {
		h.message(c, "Nao possui pontos de habilidade suficientes.")
		return nil
	}
	if int64(d.LearnCosts) > s.Avatar.Gold {
		h.message(c, "Nao possui ouro suficiente.")
		return nil
	}
	if !skillfn.MatchClass(s.Avatar.ClassInfo, d.Classe) {
		h.message(c, "Esta habilidade nao pertence a sua classe.")
		return nil
	}

	base, ok := skillfn.IncLevel(&s.Skills, pkt.SkillIndex)
	if !ok {
		if base != 0 {
			h.message(c, "Esta habilidade ja esta no nivel maximo.")
		}
		return nil
	}

	// O desconto vem depois de o nivel subir, como na origem.
	//
	// Ali o ramo "if (SkillPoint = 1) then CurrentScore.Status := 0" zerava os
	// pontos de ATRIBUTO do jogador junto com o ultimo ponto de habilidade:
	// aprender uma skill com exatamente 1 ponto sobrando apagava a reserva de
	// status inteira. E o unico caso em que o jogador perde dado por usar o
	// jogo normalmente, e nao foi portado.
	s.Avatar.SkillPoint = subPoints(s.Avatar.SkillPoint, int(d.SkillPoints))
	s.Avatar.Gold = subGold(s.Avatar.Gold, int64(d.LearnCosts))

	h.sendSkillList(c, s)
	h.sendRefreshPoint(c, s)
	h.log().Info("skill aprendida",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"skill", pkt.SkillIndex, "grupo", base)
	return nil
}

// subPoints desconta sem dar a volta no WORD.
func subPoints(x uint16, v int) uint16 {
	if v <= 0 {
		return x
	}
	if int(x) <= v {
		return 0
	}
	return x - uint16(v)
}

// subGold desconta sem ficar negativo. A origem fazia "Dec(Gold, LearnCosts)"
// num Int64 sem piso, e o ramo de GM pulava a checagem de ouro: um GM
// aprendendo skill deixava o personagem com ouro negativo.
func subGold(x int64, v int64) int64 {
	if v <= 0 || x <= v {
		if x <= v {
			return 0
		}
	}
	return x - v
}

// resetSkills e o porte de TPacketHandlers.ResetSkills ($32A): zera os niveis
// e devolve os pontos.
func (h *Handler) resetSkills(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	devolvidos := 0
	for i := range s.Skills.Basics {
		devolvidos += int(s.Skills.Basics[i].Level)
		s.Skills.Basics[i].Level = 0
	}
	for i := range s.Skills.Others {
		devolvidos += int(s.Skills.Others[i].Level)
		s.Skills.Others[i].Level = 0
	}
	s.Avatar.SkillPoint = addPoints(s.Avatar.SkillPoint, devolvidos)
	s.Base.Cooldowns.Clear()

	h.sendSkillList(c, s)
	h.sendRefreshPoint(c, s)
	h.log().Info("skills zeradas",
		"usuario", s.Username, "pontos_devolvidos", devolvidos)
	return nil
}

// cancelSkill e o porte de TPacketHandlers.CancelSkillLaunching ($327).
func (h *Handler) cancelSkill(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	s.UsingSkill = 0
	return nil
}

// useSkill e o porte de TPacketHandlers.UseSkill ($320).
//
// A origem repassava o pacote aos visiveis antes de qualquer validacao
// ("Player.Base.SendToVisible(Packet, ...)" na primeira linha): skill
// inexistente, de outra classe, sem MP ou em recarga ainda aparecia na tela de
// todo mundo. Aqui o repasse acontece depois de a skill ser aceita.
func (h *Handler) useSkill(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if s.Dead {
		s.Avatar.CurHP = 0
		h.sendCurrentHPMP(c, s)
		h.sendEffect(s, 0)
		return nil
	}

	var pkt packets.SendSkillUse
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Skill == 0 {
		return nil
	}

	d := skillfn.Data(h.SkillTable, pkt.Skill)
	if d == nil {
		h.log().Warn("skill fora da tabela ao lancar",
			"indice", pkt.Skill, "usuario", s.Username, "ip", c.IP())
		return nil
	}
	s.AtkSkill = pkt.Skill
	s.AtkAnim = 0
	if skillfn.IsPranSkill(d.Classe) {
		h.message(c, "Habilidades de pran ainda nao estao disponiveis.")
		return nil
	}
	// A skill tem de estar aprendida: a origem nao conferia isso, bastava
	// mandar o indice no $320 e ela saia.
	if !skillfn.Knows(s.Skills, pkt.Skill) {
		h.message(c, "Voce nao conhece esta habilidade.")
		return nil
	}
	if s.Avatar.Level < uint16(min(d.MinLevel, 65535)) {
		h.message(c, fmt.Sprintf(
			"Voce nao pode usar essa habilidade. Somente no nivel %d.", d.MinLevel))
		return nil
	}
	if !skillfn.MatchClass(s.Avatar.ClassInfo, d.Classe) {
		// A origem checava a classe so em LearnSkill; no uso a skill de outra
		// classe passava.
		h.message(c, "Esta habilidade nao pertence a sua classe.")
		return nil
	}

	// Classes 2 e 3 precisam de bala no slot 15.
	if !h.hasBullets(c, s) {
		return nil
	}

	// Recarga.
	if !s.Base.Cooldowns.Ready(uint16(pkt.Skill), h.now(), h.cooldowns()) {
		falta := s.Base.Cooldowns.Remaining(uint16(pkt.Skill), h.now(), h.cooldowns())
		h.log().Debug("skill em recarga",
			"usuario", s.Username, "skill", pkt.Skill, "falta", falta)
		return nil
	}

	// Custo de MP, descontado so depois de tudo validado.
	custo := int(d.MP)
	if custo > 0 {
		if s.Avatar.CurMP < uint32(custo) {
			h.message(c, "Voce nao possui o MP necessario para realizar a habilidade.")
			return nil
		}
		s.Avatar.CurMP -= uint32(custo)
	}

	s.Base.Cooldowns.Use(uint16(pkt.Skill), h.now())
	s.UsingSkill = uint16(pkt.Skill)
	s.LongSkillPos = pkt.Pos

	// Agora sim o lancamento vai para a tela de quem ve.
	h.broadcastSkillUse(s, pkt)

	// Despacho de TBaseMob.HandleSkill, pela taxa de sucesso e pelo alcance:
	// 1/0 dano em um alvo, 1/>0 dano em area, 0/0 efeito em si ou no alvo e
	// 0/>0 efeito em area (ou no grupo).
	switch {
	case skillfn.IsSingleTargetDamage(d):
		h.skillSingleTarget(c, s, pkt, d)
	case skillfn.IsAreaSkill(d):
		h.skillArea(c, s, pkt, d)
	case d.SuccessRate == 0 && d.Range > 0:
		h.skillAreaBuff(c, s, pkt, d)
	case d.SuccessRate == 0 && d.Range == 0, skillfn.IsBuff(d):
		h.skillBuff(c, s, pkt, d)
	}

	h.sendCurrentHPMP(c, s)
	return nil
}

// hasBullets confere a bala do slot 15 para as classes que precisam dela.
func (h *Handler) hasBullets(c *netsrv.Conn, s *Session) bool {
	switch stats_MobClassOf(s.Avatar.ClassInfo) {
	case 2, 3:
		if s.Equip.Get(BulletSlot).IsEmpty() {
			h.message(c, "Voce esta sem balas.")
			return false
		}
	}
	return true
}

// consumeBullets gasta a bala do slot 15: uma para a classe 2 e duas para a
// classe 3, e nenhuma quando a bala e do tipo infinito.
func (h *Handler) consumeBullets(c *netsrv.Conn, s *Session) {
	var quant uint16
	switch stats_MobClassOf(s.Avatar.ClassInfo) {
	case 2:
		quant = 1
	case 3:
		quant = 2
	default:
		return
	}
	bala := &s.Equip.Slots[BulletSlot]
	if bala.IsEmpty() {
		return
	}
	if itemfn.ItemType(h.data(), bala.Index) == ItemTypeInfiniteBullet {
		return
	}
	itemfn.DecreaseAmount(bala, quant)
	h.sendRefreshItem(c, s, store.SlotTypeEquip, BulletSlot, *bala)
}

// broadcastSkillUse repassa o $320 a quem ve o autor, com o id do autor no
// cabecalho.
func (h *Handler) broadcastSkillUse(s *Session, pkt packets.SendSkillUse) {
	if h.Chan == nil {
		return
	}
	pkt.Header.Index = s.ClientID
	raw, err := netsrv.Marshal(CodeUseSkill, pkt)
	if err != nil {
		h.log().Error("montar uso de skill", "cliente", s.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
}

// skillSingleTarget aplica uma skill de dano num alvo.
func (h *Handler) skillSingleTarget(c *netsrv.Conn, s *Session, pkt packets.SendSkillUse, d *gamedata.T_SkillData) {
	alvoID := uint16(min(pkt.Index, 65535))
	if alvoID == 0 || uint32(alvoID) != pkt.Index {
		return
	}
	h.consumeBullets(c, s)

	if IsMobTarget(alvoID) {
		h.skillOnMob(c, s, alvoID, d)
		return
	}
	alvo, err := h.resolveAttackTarget(s, alvoID)
	if err != nil {
		return
	}
	// O alcance da skill vem da tabela; zero cai no alcance corpo a corpo.
	if !s.Position.InRange(alvo.Position, h.skillRange(d)) {
		return
	}
	h.skillHitPlayer(s, alvo, pkt.Skill, d)
}

// skillHitPlayer calcula e aplica o golpe de uma skill num jogador. E a parte
// comum do alvo unico e da area.
func (h *Handler) skillHitPlayer(s, alvo *Session, skill uint32, d *gamedata.T_SkillData) {
	rnd := h.rand()
	tipo := s.Roller.Roll(
		combat.Attacker{
			Critical: s.Avatar.Critical,
			DuploAtk: s.Avatar.DuploAtk,
			Acerto:   uint16(s.Avatar.Acerto),
		},
		combat.Defender{
			CritRes:  alvo.Avatar.CritRes,
			DuploRes: alvo.Avatar.DuploRes,
			Esquiva:  uint16(alvo.Avatar.Esquiva),
		},
		true, skill, rnd)

	dano := 0
	if tipo != combat.Miss {
		dano = h.skillBaseDamage(s, alvo)
		dano += skillfn.SkillDamageBonus(d, s.Avatar.HabAtk)
		dano = clericAdjust(d, dano)
		dano = warriorAdjust(s, d, dano)
		dano, tipo = h.gunnerAdjust(d, dano, tipo, int(alvo.Avatar.DefFis))
		dano = mageAdjust(d, dano, alvo.Avatar.CurMP)
		if zeroDamage(d) {
			dano = 0
		}
		dano = h.danoRuido(dano, rnd)
		h.clericOnHit(alvo, d)
	}
	danoBruto := dano
	if dano > 0 {
		res := combat.ApplyToTarget(dano, tipo,
			&sessionTarget{s: alvo, h: h}, &sessionTarget{s: s, h: h}, &alvo.DefesaPoints)
		dano, tipo = res.Damage, res.Type
		if res.Reflected > 0 {
			h.applyDamage(s, res.Reflected)
		}
		dano = h.divineUnion(alvo, dano)
		// Bolha: o golpe de skill que acerta e anulado, sem efeito de estado.
		if h.bubbleBlocks(s, alvo, d) {
			dano, tipo = 0, combat.None
		}
		dano = h.reactiveHit(s, alvo, dano, tipo, tipo != combat.Miss && tipo != combat.None)
	}
	efeito := tipo != combat.Miss && tipo != combat.None
	if efeito {
		if kind := debuffKind(d); kind != 0 && h.statusResisted(s, alvo, kind) {
			efeito = false
		}
	}

	h.applyDamage(alvo, dano)
	h.broadcastDamage(s, alvo, tipo, dano)
	if alvo.Dead {
		h.onDeath(s, alvo)
	}
	if tipo != combat.Miss && tipo != combat.None {
		h.gunnerOnHit(s, alvo, d, danoBruto, alvo.Dead)
		h.mageOnHit(s, d, danoBruto)
		h.areaOnHit(alvo, d)
		h.markResoluto(alvo, d)
	}
	// Skill com duracao tambem aplica o efeito no alvo, se o golpe valeu e o
	// alvo nao resistiu.
	if skillfn.IsBuff(d) && efeito {
		h.applyHitEffect(s, alvo, uint16(skill), d)
	}
	if efeito && isDash(d) {
		h.dashTo(s, alvo.Position, skill)
	}
}

// skillOnMob aplica a skill num mob.
func (h *Handler) skillOnMob(c *netsrv.Conn, s *Session, mobID uint16, d *gamedata.T_SkillData) {
	if h.Mobs == nil {
		return
	}
	m, ok := h.Mobs.Get(mobID)
	if !ok || m.Dead {
		return
	}
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil {
		return
	}
	if !s.Position.InRange(m.Position, h.skillRange(d)) {
		return
	}
	h.skillHitMob(c, s, m, sp, d)
}

// skillHitMob calcula e aplica o golpe de uma skill num mob. Parte comum do
// alvo unico e da area.
func (h *Handler) skillHitMob(c *netsrv.Conn, s *Session, m *Mob, sp *mobdata.Species, d *gamedata.T_SkillData) {
	rnd := h.rand()
	tipo := s.Roller.Roll(
		combat.Attacker{
			Critical: s.Avatar.Critical,
			DuploAtk: s.Avatar.DuploAtk,
			Acerto:   uint16(s.Avatar.Acerto),
		},
		combat.Defender{
			CritRes:  h.MobCritRes,
			DuploRes: h.MobDuploRes,
			Esquiva:  h.MobEsquiva,
		},
		true, uint32(d.Index), rnd)

	dano := 0
	if tipo != combat.Miss {
		dano = h.skillMobBaseDamage(s, sp)
		dano += skillfn.SkillDamageBonus(d, s.Avatar.HabAtk)
		dano = clericAdjust(d, dano)
		dano = warriorAdjust(s, d, dano)
		dano, tipo = h.gunnerAdjust(d, dano, tipo, int(sp.DefFis))
		if zeroDamage(d) {
			dano = 0
		}
		dano = h.danoRuido(dano, rnd)
		h.gunnerOnHit(s, nil, d, dano, false)
		h.mageOnHit(s, d, dano)
	}

	morreu := h.Mobs.Hit(m, dano, s.ClientID, h.now())
	h.broadcastMobDamage(s, m, tipo, dano)
	if morreu {
		h.onMobDeath(c, s, m, sp)
	} else if tipo != combat.Miss && isDot(d) {
		h.applyMobDot(s, m, d)
	}
	if tipo != combat.Miss && isDash(d) {
		h.dashTo(s, m.Position, uint32(d.Index))
	}
}

// skillBuff aplica uma skill de efeito em si mesmo ou no alvo.
func (h *Handler) skillBuff(c *netsrv.Conn, s *Session, pkt packets.SendSkillUse, d *gamedata.T_SkillData) {
	alvo := s
	// TargetType 1 e "em si mesmo" (HandleSkill: DataSkill.TargetType = 1).
	if d.TargetType != 1 && pkt.Index != 0 && uint32(uint16(pkt.Index)) == pkt.Index {
		id := uint16(pkt.Index)
		if id != s.ClientID && !IsMobTarget(id) {
			if outro, _, ok := h.Chan.Lookup(id); ok && outro.Status >= Playing {
				if s.Position.InRange(outro.Position, h.skillRange(d)) {
					alvo = outro
				}
			}
		}
	}
	h.targetBuffSkill(s, alvo, uint16(pkt.Skill), d, d.TargetType == 1)
	h.log().Debug("buff aplicado",
		"skill", pkt.Skill, "de", s.Avatar.Name, "para", alvo.Avatar.Name)
}

// applyBuff poe o buff na entidade, respeitando o limite de 60 da origem.
func (h *Handler) applyBuff(alvo *Session, skill uint16) {
	if !h.addBuff(alvo, skill) {
		h.log().Debug("buff nao aplicado", "cliente", alvo.ClientID, "skill", skill)
	}
}

// skillRange devolve o alcance da skill, com o corpo a corpo como piso.
func (h *Handler) skillRange(d *gamedata.T_SkillData) uint16 {
	if d == nil || d.DamageRange == 0 {
		return AttackRange
	}
	return uint16(min(d.DamageRange, 65535))
}

// danoRuido soma ao dano o mesmo ruido do golpe basico.
func (h *Handler) danoRuido(dano int, rnd combat.Rand) int {
	if dano <= 0 {
		return 0
	}
	baixo, alto := dano/20, dano/10
	extra := 13
	if alto > baixo {
		extra += baixo + rnd.IntN(alto-baixo)
	} else {
		extra += baixo
	}
	return combat.ClampDamage(dano + extra)
}

// sendSkillList manda os dois pacotes de skill ao cliente.
func (h *Handler) sendSkillList(c *netsrv.Conn, s *Session) {
	h.sendPlayerSkills(c, s, 0)
	h.sendSkillLevels(c, s)
}

// sendPlayerSkills manda o $106 com os indices das skills "Others". Porte de
// TPlayer.SendPlayerSkills.
func (h *Handler) sendPlayerSkills(c *netsrv.Conn, s *Session, npcIndex uint16) {
	var pkt packets.SendSkillsPacket
	pkt.Header.Index = s.ClientID
	pkt.NPCIndex = npcIndex
	if npcIndex > 0 {
		pkt.SendType = 0xB
	}
	for i := range pkt.Skills {
		if i < len(s.Skills.Others) {
			pkt.Skills[i] = s.Skills.Others[i].Index
		}
	}
	if err := c.SendStruct(CodePlayerSkills, pkt); err != nil {
		h.log().Debug("falha ao enviar skills", "usuario", s.Username, "erro", err)
	}
}

// sendSkillLevels manda o $107 com os pontos e a mascara de niveis.
//
// Porte de TPlayer.SendPlayerSkillsLevel, com a ordem corrigida: ali o
// SendPacket vinha ANTES de o vetor Skills ser preenchido, e o Move que o
// preenchia escrevia num registro local que ja havia sido enviado e ia sair
// de escopo. O cliente recebia a mascara sempre zerada — isto e, os niveis de
// skill nunca chegavam.
func (h *Handler) sendSkillLevels(c *netsrv.Conn, s *Session) {
	var pkt packets.SendSkillsLevelPacket
	pkt.Header.Index = s.ClientID
	pkt.SkillPoints = s.Avatar.SkillPoint
	pkt.Unk = 0xCCCC
	h.fillSkillMask(s, pkt.Skills[:])

	if err := c.SendStruct(CodeSkillLevels, pkt); err != nil {
		h.log().Debug("falha ao enviar niveis de skill",
			"usuario", s.Username, "erro", err)
	}
}

// fillSkillMask monta o vetor SkillList. Porte de TPlayer.SetPlayerSkills.
//
// Cada slot de "Others" leva a mascara dos niveis liberados,
// "2^(nivel+1) - 2", que e GetSkillLevel. A origem escrevia esse valor com um
// Move de 2 ou 4 bytes num slot de 2: com nivel 16 ou mais o Move de 4 bytes
// passava para o slot seguinte, sobrescrevendo a mascara da skill vizinha.
// Aqui o valor e cortado em 16 bits, que e o tamanho do campo.
func (h *Handler) fillSkillMask(s *Session, out []uint16) {
	clear(out)
	for i := range s.Skills.Basics {
		if i >= len(out) {
			break
		}
		if s.Skills.Basics[i].Level == 0 {
			continue
		}
		// O valor fixo 2 que a origem punha para as basicas.
		out[i] = 2
	}
	const base = len(s.Skills.Basics)
	for i := range s.Skills.Others {
		j := i + base
		if j >= len(out) {
			break
		}
		nivel := s.Skills.Others[i].Level
		if nivel == 0 {
			continue
		}
		out[j] = skillfn.LevelMask(nivel)
	}
}

// cooldowns devolve a tabela de recarga.
func (h *Handler) cooldowns() skillfn.CooldownMillis {
	return skillfn.CooldownMillis{T: h.SkillTable}
}
