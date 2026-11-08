package game

import (
	"context"
	"math/rand/v2"
	"time"

	"aika/internal/combat"
	"aika/internal/entity"
	"aika/internal/mobdata"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/stats"
	"aika/internal/store"
)

// Codigos de combate.
const (
	CodeAttack     = 0x302 // cliente -> servidor: atacar
	CodeRevive     = 0x303 // cliente -> servidor: reviver
	CodeRecvDamage = 0x102 // servidor -> cliente: resultado do golpe
	CodeSendEffect = 0x117 // servidor -> cliente: efeito visual
)

// MinDelayAttack e o MIN_DELAY_ATTACK de GlobalDefs.pas.
const MinDelayAttack = 300 * time.Millisecond

// WeaponSlot e o slot da arma: a origem exigia
// "if not(Equip[6].Index > 0) then Exit".
const WeaponSlot = 6

// AttackRange e a distancia maxima de um golpe corpo a corpo.
//
// A origem nao checava distancia nenhuma no ataque basico entre jogadores:
// bastava o ClientID do alvo para acerta-lo de qualquer ponto do mapa. Aqui o
// alvo precisa estar a vista.
const AttackRange = 10

// AttacksPerDurabilityLoss e o "if (AttacksAccumulated >= 48)" da origem.
const AttacksPerDurabilityLoss = 48

// attack e o porte da parte de TPacketHandlers.AttackTarget ($302) que trata
// o ataque basico entre jogadores.
//
// Skill, dungeon, pran e mascote dependem de sistemas que ainda nao estao
// portados; ver GoServer/PORTE.md.
func (h *Handler) attack(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.SendAtkPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	if s.Dead {
		// A origem zerava o HP e reenviava, para o cliente parar de pedir.
		s.Avatar.CurHP = 0
		h.sendCurrentHPMP(c, s)
		h.sendEffect(s, 0)
		return nil
	}

	// Skill ainda nao esta portada: atacar com skill nao pode cair no ataque
	// basico, senao a skill sairia de graca com o dano do golpe simples.
	if pkt.Skill != 0 {
		h.message(c, "Habilidades ainda nao estao disponiveis neste servidor.")
		return nil
	}

	// Sem arma no slot 6 nao ha golpe, como na origem.
	if s.Equip.Get(WeaponSlot).IsEmpty() {
		return nil
	}

	// Ritmo minimo entre golpes. A origem mandava "Outros programas
	// detectados." e, se a mensagem repetisse dentro de 5 s, marcava
	// SocketClosed. Aqui o golpe rapido demais e so descartado: o limite nao
	// foi calibrado contra lag de verdade, e derrubar quem tem conexao ruim
	// seria pior que o problema.
	agora := h.now()
	if !s.LastAttack.IsZero() && agora.Sub(s.LastAttack) <= MinDelayAttack {
		return nil
	}
	s.LastAttack = agora
	s.AtkAnim = uint8(pkt.Anim)
	s.AtkSkill = 0

	// Dentro de uma dungeon o alvo e um mob da instancia, cujo id vem do
	// MobsPosition e cai na faixa reservada a NPC (2048..3047), nao na dos
	// mobs do mundo. A origem tratava esse caso primeiro e, por nao ter um
	// Exit no fim do bloco, caia depois no ramo de jogador e batia duas vezes
	// (PacketHandlers.pas:5753).
	if h.attackDungeonMob(ctx, c, s, pkt.Index) {
		return nil
	}

	// Mob e jogador vem no mesmo campo Index, separados pela faixa do id.
	if IsMobTarget(pkt.Index) {
		return h.hitMob(c, s, pkt.Index)
	}

	alvo, err := h.resolveAttackTarget(s, pkt.Index)
	if err != nil {
		return nil
	}

	h.resolveHit(c, s, alvo)
	return nil
}

// hitMob trata o golpe do jogador num mob.
//
// Porte do ramo de mob de AttackTarget com SendDamage. A origem resolvia o
// mob por "MOBS.TMobS[mobid].MobsP[smid]", com os dois indices vindo de
// conversoes sobre o ClientID e sem checagem de faixa nos vetores de 450 e 50
// posicoes.
func (h *Handler) hitMob(c *netsrv.Conn, s *Session, mobID uint16) error {
	if h.Mobs == nil {
		return nil
	}
	m, ok := h.Mobs.Get(mobID)
	if !ok || m.Dead {
		return nil
	}
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil {
		return nil
	}
	// O mob tem de estar a vista, como qualquer alvo.
	if !s.Position.InRange(m.Position, AttackRange) {
		return nil
	}

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
		true, 0, rnd)

	dano := 0
	if tipo != combat.Miss {
		atk := int(s.Avatar.AtkFis)
		def := int(sp.DefFis)
		if pen := int(s.Avatar.FisPenetration); pen > 0 {
			def -= (def / 100) * pen
		}
		dano = atk - (def >> 3)
		if dano <= 0 {
			dano = 1
		}
		// O mesmo ruido do golpe entre jogadores.
		baixo, alto := dano/20, dano/10
		extra := 13
		if alto > baixo {
			extra += baixo + rnd.IntN(alto-baixo)
		} else {
			extra += baixo
		}
		dano = combat.ClampDamage(dano + extra)
	}

	morreu := h.Mobs.Hit(m, dano, s.ClientID, h.now())
	h.broadcastMobDamage(s, m, tipo, dano)

	if morreu {
		h.onMobDeath(c, s, m, sp)
	} else if h.procSetSkill(s, m, rnd) {
		h.onMobDeath(c, s, m, sp)
	}
	h.wearWeapon(c, s)
	return nil
}

// procSetSkill porta Effect5Skill de BaseMob.pas: com o efeito EF_CALLSKILL do
// conjunto vestido, um golpe tem chance de disparar a skill do conjunto (no
// Cometa Ardente, a Estrela Cadente) como um segundo golpe no mesmo alvo.
//
// A chance e RATE_EFFECT5 vezes o tamanho de EFF_5 (3), como na origem.
// Devolve true quando o golpe da skill matou o mob. So skills de alvo unico
// (TargetType 21) sao tratadas, que e o ramo que a origem usa aqui.
func (h *Handler) procSetSkill(s *Session, m *Mob, rnd interface{ IntN(int) int }) bool {
	var lista []uint16
	for _, sk := range s.Avatar.CallSkills {
		if sk != 0 {
			lista = append(lista, sk)
		}
	}
	if len(lista) == 0 || h.SkillTable == nil {
		return false
	}
	taxa := int(h.RateEffect5)
	if taxa == 0 {
		taxa = 10
	}
	if rnd.IntN(100)+1 > taxa*len(s.Avatar.CallSkills) {
		return false
	}
	skill := lista[rnd.IntN(len(lista))]
	d := h.SkillTable.Skill(int(skill))
	if d == nil || d.TargetType != 21 {
		return false
	}

	dano := (int(s.Avatar.AtkFis) + int(s.Avatar.AtkMag)) / 3
	dano += int(d.Damage)
	if d.Adicional > 0 {
		dano *= 2
	}
	if dano > 20000 {
		dano = 20000
	}
	dano += 20 + rnd.IntN(180)

	morreu := h.Mobs.Hit(m, dano, s.ClientID, h.now())

	// A animacao vai no byte baixo, como o "Packet.Animation := SkillData[].Anim"
	// da origem (Anim e DWORD, Animation e Byte).
	antSkill, antAnim := s.AtkSkill, s.AtkAnim
	s.AtkSkill, s.AtkAnim = uint32(skill), uint8(d.Anim)
	h.broadcastMobDamage(s, m, combat.Critical, dano)
	s.AtkSkill, s.AtkAnim = antSkill, antAnim
	return morreu
}

// fillAnimation preenche no $102 o que o cliente usa para tocar o golpe.
//
// Porte de TBaseMob.SendDamage: SkillID e Animation vinham do pedido do
// cliente e MobAnimation de SkillData[Skill].TargetAnimation. O Go deixava os
// tres em zero, e o cliente nao animava o ataque.
func (h *Handler) fillAnimation(pkt *packets.RecvDamagePacket, s *Session) {
	pkt.SkillID = s.AtkSkill
	pkt.Animation = s.AtkAnim
	if h.SkillTable == nil {
		return
	}
	if d := h.SkillTable.Skill(int(s.AtkSkill)); d != nil {
		pkt.MobAnimation = uint8(d.TargetAnimation)
	}
}

// broadcastMobDamage manda o $102 do golpe num mob a quem ve o mob.
func (h *Handler) broadcastMobDamage(s *Session, m *Mob, tipo combat.DamageType, dano int) {
	if h.Chan == nil {
		return
	}
	var pkt packets.RecvDamagePacket
	pkt.Header.Index = s.ClientID
	pkt.AttackerID = s.ClientID
	pkt.AttackerHP = s.Avatar.CurHP
	pkt.AttackerPos.X = s.Position.X
	pkt.AttackerPos.Y = s.Position.Y
	pkt.TargetID = m.ClientID
	h.fillAnimation(&pkt, s)
	pkt.DNType = uint8(tipo)
	pkt.DANO = uint64(max(dano, 0))
	pkt.MobCurrHP = m.CurHP
	if m.Dead {
		pkt.DeathPos.X = m.Position.X
		pkt.DeathPos.Y = m.Position.Y
	}

	raw, err := netsrv.Marshal(CodeRecvDamage, pkt)
	if err != nil {
		h.log().Error("montar dano em mob", "mob", m.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	// Quem tambem ve o mob precisa ver a barra cair.
	for _, id := range h.Chan.PlayingIDs() {
		if id == s.ClientID {
			continue
		}
		outro, _, ok := h.Chan.Lookup(id)
		if ok && outro.Base.VisibleMobs.Contains(m.ClientID) {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// onMobDeath da a experiencia e apaga o mob da tela de quem o via.
//
// Na origem a experiencia ia para quem deu o ultimo golpe, e o drop para
// FirstPlayerAttacker. Mantido: a experiencia vai para quem matou.
func (h *Handler) onMobDeath(c *netsrv.Conn, s *Session, m *Mob, sp *mobdata.Species) {
	ganho := h.addExp(c, s, int64(sp.Exp), ExpTypeMob)
	h.awardPranExp(c, s, ganho)
	h.dropFromMob(c, s, m, sp.DropIndex)
	h.log().Debug("mob morto",
		"mob", sp.Name, "id", m.ClientID,
		"por", s.Avatar.Name, "exp", ganho)
	h.broadcastMobRemove(m)
}

// RespawnTick devolve ao mundo os mobs cujo tempo passou e os desenha para
// quem esta perto.
//
// Na origem isso acontecia dentro de UpdateSpawnToPlayers, isto e, o mob so
// voltava quando algum jogador passava perto o bastante para a rotina rodar
// com ele; sem jogador por perto o mob ficava morto indefinidamente, e o
// contador de respawn nao corria. Aqui o laco e do servidor.
func (h *Handler) RespawnTick() int {
	if h.Mobs == nil {
		return 0
	}
	voltaram := h.Mobs.Respawn(h.now())
	for _, m := range voltaram {
		h.broadcastMobSpawn(m)
	}
	return len(voltaram)
}

// resolveAttackTarget acha o alvo de um golpe, validando o id do cliente.
//
// A origem indexava "Servers[ChannelIndex].Players[Packet.Index]" em varios
// pontos. O bloco de checagem de guilda e duelo testava a faixa, mas o bloco
// que resolve o alvo testava so "Packet.Index <= MAX_CONNECTIONS": zero
// caia no proprio jogador, o que e proposital, mas um indice vazio no vetor
// dava um registro zerado tratado como jogador vivo.
func (h *Handler) resolveAttackTarget(s *Session, index uint16) (*Session, error) {
	if h.Chan == nil {
		return nil, ErrSlotInvalido
	}
	if index == 0 || index == s.ClientID {
		// Golpe em si mesmo: a origem apontava OtherMob para o proprio
		// jogador. Sem skill de cura portada isso nao faz nada util, e
		// aplicar o dano no autor seria pior.
		return nil, ErrSlotInvalido
	}
	if !entity.IsPlayerID(index, h.Chan.MaxConns()) {
		return nil, ErrSlotInvalido
	}
	alvo, _, ok := h.Chan.Lookup(index)
	if !ok || alvo.Status < Playing || alvo.Dead {
		return nil, ErrSlotInvalido
	}
	// Em duelo so o oponente pode ser atacado, e so depois da espera de 15 s.
	if s.Duel.active && (index != s.Duel.with || h.duelWaiting(s)) {
		return nil, ErrSlotInvalido
	}
	// O alvo tem de estar a vista: a origem aceitava qualquer ClientID.
	if !s.Position.InRange(alvo.Position, AttackRange) {
		return nil, ErrSlotInvalido
	}
	return alvo, nil
}

// resolveHit calcula e aplica um golpe. Porte de TBaseMob.SendDamage com
// TBaseMob.GetDamage e o pipeline de AttackParseForMobs.
func (h *Handler) resolveHit(c *netsrv.Conn, s *Session, alvo *Session) {
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
		true, 0, rnd)

	dano := 0
	if tipo != combat.Miss {
		dano = h.baseDamage(s, alvo)
	}

	if dano > 0 {
		alvoAlvo := &sessionTarget{s: alvo, h: h}
		autorAlvo := &sessionTarget{s: s, h: h}
		res := combat.ApplyToTarget(dano, tipo, alvoAlvo, autorAlvo, &alvo.DefesaPoints)
		dano, tipo = res.Damage, res.Type
		if res.Reflected > 0 {
			h.applyDamage(s, res.Reflected)
		}
		dano = h.divineUnion(alvo, dano)
		dano = h.reactiveHit(s, alvo, dano, tipo, tipo != combat.Miss)
	}

	if dano > 0 {
		// A origem somava um ruido ao dano final:
		// "Inc(xDano, RandomRange(xDano div 20, xDano div 10) + 13)".
		baixo, alto := dano/20, dano/10
		extra := 13
		if alto > baixo {
			extra += baixo + rnd.IntN(alto-baixo)
		} else {
			extra += baixo
		}
		dano += extra

		// Bonus por classe do ataque basico, como na origem: templario soma
		// 140% e arqueiro divide por 1.69.
		switch stats_MobClassOf(s.Avatar.ClassInfo) {
		case classTemplar:
			dano += int(float64(dano) * 1.4)
		case classArcher:
			dano = int(float64(dano) / 1.69)
		}
		dano = combat.ClampDamage(dano)
	}

	h.applyDamage(alvo, dano)
	h.broadcastDamage(s, alvo, tipo, dano)

	if alvo.Dead {
		h.onDeath(s, alvo)
	}

	h.wearWeapon(c, s)
}

// baseDamage e a conta de TBaseMob.GetDamage sem as habilidades de efeito.
func (h *Handler) baseDamage(s, alvo *Session) int {
	atk := int(s.Avatar.AtkFis)
	def := int(alvo.Avatar.DefFis)
	if pen := int(s.Avatar.FisPenetration); pen > 0 {
		def -= (def / 100) * pen
	}
	// "ResultDamage - (MobDef shr 3)": a defesa entra dividida por 8.
	dano := atk - (def >> 3)
	if dano <= 0 {
		// A origem punha 1 quando a conta zerava ou ficava negativa.
		dano = 1
	}
	return dano
}

// applyDamage desconta vida e marca a morte.
func (h *Handler) applyDamage(s *Session, dano int) {
	if dano <= 0 {
		return
	}
	// Em duelo ninguem morre: o golpe que mataria deixa 10 de vida e o duelo acaba.
	if s.Duel.active && uint32(dano) >= s.Avatar.CurHP {
		s.Avatar.CurHP = duelEndHP
		return
	}
	if uint32(dano) >= s.Avatar.CurHP {
		s.Avatar.CurHP = 0
		s.Dead = true
		return
	}
	s.Avatar.CurHP -= uint32(dano)
}

// broadcastDamage manda o $102 ao autor, ao alvo e a quem ve os dois.
func (h *Handler) broadcastDamage(s, alvo *Session, tipo combat.DamageType, dano int) {
	var pkt packets.RecvDamagePacket
	pkt.Header.Index = s.ClientID
	pkt.AttackerID = s.ClientID
	pkt.AttackerHP = s.Avatar.CurHP
	pkt.AttackerPos.X = s.Position.X
	pkt.AttackerPos.Y = s.Position.Y
	pkt.TargetID = alvo.ClientID
	h.fillAnimation(&pkt, s)
	pkt.DNType = uint8(tipo)
	pkt.DANO = uint64(max(dano, 0))
	pkt.MobCurrHP = alvo.Avatar.CurHP
	if alvo.Dead {
		pkt.DeathPos.X = alvo.Position.X
		pkt.DeathPos.Y = alvo.Position.Y
	}

	raw, err := netsrv.Marshal(CodeRecvDamage, pkt)
	if err != nil {
		h.log().Error("montar dano", "atacante", s.ClientID, "erro", err)
		return
	}
	if h.Chan == nil {
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	_ = h.Chan.SendTo(alvo.ClientID, raw)
	h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
}

// onDeath trata a morte do alvo.
func (h *Handler) onDeath(autor, morto *Session) {
	morto.Dead = true
	morto.Avatar.CurHP = 0
	h.log().Info("jogador morto",
		"morto", morto.Avatar.Name, "por", autor.Avatar.Name,
		"canal", h.Channel)
}

// wearWeapon desgasta a arma a cada 48 golpes. Porte do bloco
// "if (AttacksAccumulated >= 48)".
//
// A origem fazia "Dec(Equip[6].MIN, 1)" num BYTE sem piso: arma com
// durabilidade zero dava a volta para 255 e voltava a funcionar sozinha. Aqui
// o piso e zero.
func (h *Handler) wearWeapon(c *netsrv.Conn, s *Session) {
	s.AttacksAccumulated++
	if s.AttacksAccumulated < AttacksPerDurabilityLoss {
		return
	}
	s.AttacksAccumulated = 0

	arma := &s.Equip.Slots[WeaponSlot]
	if arma.IsEmpty() {
		return
	}
	// O desgaste cresce com o refino, como na origem.
	var perda uint8
	switch {
	case arma.Refi >= 161 && arma.Refi <= 240:
		perda = 3
	case arma.Refi >= 81:
		perda = 2
	case arma.Refi >= 1:
		perda = 1
	default:
		return
	}
	if arma.Min <= perda {
		arma.Min = 0
	} else {
		arma.Min -= perda
	}
	h.sendRefreshItem(c, s, store.SlotTypeEquip, WeaponSlot, *arma)
}

// sendEffect manda o $117 a quem ve o jogador. Porte de TPlayer.SendEffect.
func (h *Handler) sendEffect(s *Session, efeito uint32) {
	if h.Chan == nil {
		return
	}
	var pkt packets.SendClientIndexPacket
	pkt.Header.Index = s.ClientID
	pkt.Index = uint32(s.ClientID)
	pkt.Effect = efeito
	raw, err := netsrv.Marshal(CodeSendEffect, pkt)
	if err != nil {
		h.log().Error("montar efeito", "cliente", s.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
}

// revive e o porte de TPacketHandlers.RevivePlayer ($303).
func (h *Handler) revive(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if !s.Dead {
		// Reviver quem nao morreu era aceito na origem e enchia o HP de
		// graca: bastava mandar o $303.
		return nil
	}

	s.Dead = false
	s.Position = StartPosition
	s.AnchorPosition = s.Position
	// A origem devolvia o jogador com a vida cheia.
	s.Avatar.CurHP = s.Avatar.MaxHP
	s.Avatar.CurMP = s.Avatar.MaxMP
	h.clearBuffs(s)

	if err := h.resync(s); err != nil {
		return err
	}
	h.sendStatus(c, s)
	h.refreshVisible(s)
	h.refreshVisibleMobs(s)
	h.respawnToVisible(s)

	h.log().Info("jogador reviveu",
		"usuario", s.Username, "personagem", s.Avatar.Name)
	return nil
}

// rand devolve a fonte de sorteio da sessao.
//
// A origem chamava Randomize dentro de GetDamage e de SendDamage, isto e,
// reiniciava o gerador global a cada golpe, com a hora como semente: dois
// golpes no mesmo instante saiam com a mesma sequencia. Aqui o gerador da
// biblioteca e usado como esta, sem reiniciar.
func (h *Handler) rand() combat.Rand {
	if h.Rand != nil {
		return h.Rand
	}
	return globalRand{}
}

// globalRand usa o gerador padrao de math/rand/v2, que e seguro para uso
// concorrente.
type globalRand struct{}

func (globalRand) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.IntN(n)
}

// sessionTarget adapta a sessao ao alvo do pipeline de dano.
//
// Na origem as mensagens de absorcao saiam por
// Servers[ChannelId].Players[mob.ClientID].SendClientMessage, com Players
// sendo ARRAY [1..1000] e mob.ClientID podendo ser id de mob: indice fora do
// vetor. Aqui quem entrega a mensagem e esta casca, que sabe que o alvo e
// jogador.
type sessionTarget struct {
	s *Session
	h *Handler
}

func (t *sessionTarget) HasBuff(index uint16) bool {
	return t.s.Base.Buffs.Has(index)
}

func (t *sessionTarget) RemoveBuff(index uint16) {
	t.h.removeBuffAndNotify(t.s, index)
}

// Ability devolve zero: o acumulador de efeitos (GetMobAbility) depende de
// equipamento, buff e skill, que ainda nao estao portados.
func (t *sessionTarget) Ability(eff int) int { return t.s.Avatar.Ability.get(eff) }

func (t *sessionTarget) DecAbility(eff, v int) { t.s.Avatar.Ability.decrease(eff, v) }

func (t *sessionTarget) Notify(msg string) {
	if t.h == nil || t.h.Chan == nil {
		return
	}
	_, c, ok := t.h.Chan.Lookup(t.s.ClientID)
	if ok && c != nil {
		t.h.message(c, msg)
	}
}

func (t *sessionTarget) AddHP(value int) {
	if value >= 0 {
		novo := uint64(t.s.Avatar.CurHP) + uint64(value)
		if novo > uint64(t.s.Avatar.MaxHP) {
			novo = uint64(t.s.Avatar.MaxHP)
		}
		t.s.Avatar.CurHP = uint32(novo)
		return
	}
	t.h.applyDamage(t.s, -value)
}

// Polimorfo depende de buff, que ainda nao esta portado.
func (t *sessionTarget) IsPolymorphed() bool { return false }

func (t *sessionTarget) ClearPolymorph() {}

// IsPlayer e sempre true: esta casca so envolve sessao de jogador. Na origem
// o teste era "mob.ClientID <= MAX_CONNECTIONS", sem limite de baixo, e o id
// 0 passava como jogador.
func (t *sessionTarget) IsPlayer() bool { return true }

func (t *sessionTarget) Respawn() {
	if t.h != nil {
		t.h.respawnToVisible(t.s)
	}
}

func (t *sessionTarget) CurrentMP() (uint32, bool) {
	return t.s.Avatar.CurMP, true
}

func (t *sessionTarget) RemoveMP(value int) {
	if value <= 0 {
		return
	}
	if uint32(value) >= t.s.Avatar.CurMP {
		t.s.Avatar.CurMP = 0
		return
	}
	t.s.Avatar.CurMP -= uint32(value)
}

// Classes do bonus de dano do ataque basico.
const (
	classTemplar = 1
	classArcher  = 2
)

// stats_MobClassOf e um atalho para a classe do personagem.
func stats_MobClassOf(classInfo uint16) int { return stats.MobClass(classInfo) }
