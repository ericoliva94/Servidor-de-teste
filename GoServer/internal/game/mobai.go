package game

import (
	"math"
	"time"

	"aika/internal/combat"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// Cadencias da IA de mob, iguais as dos tempos de thread da origem.
//
// Ali eram nove threads por canal: tres de spawn e tres de ataque a cada
// 1000 ms, e tres de movimento a cada 600 ms, cada uma varrendo um terco do
// vetor de 22.500 posicoes. Aqui sao dois tickers e uma varredura sobre os
// mobs que existem.
const (
	MobAttackTick = 1000 * time.Millisecond
	MobMoveTick   = 600 * time.Millisecond
)

// Parametros do combate e do passo do mob, da origem.
const (
	// MobAttackRange e o "Distance(...) <= 3" de MobHandler.
	MobAttackRange = 3
	// MobAttackCooldown e o "SecondsBetween(Now, LastMyAttack) >= 3". O
	// comentario da origem diz 2 segundos; o codigo diz 3, e o codigo manda.
	MobAttackCooldown = 3 * time.Second
	// MobStep e o 1.5 somado a cada eixo por passo.
	MobStep = 1.5
	// MobChaseStep e o 2 usado na perseguicao.
	MobChaseStep = 2.0
	// MobAxisThreshold e o "if (XPositionDif >= 2)": o eixo so anda quando a
	// diferenca chega a 2 celulas.
	MobAxisThreshold = 2
	// MobChaseThreshold e o ">= 3" da perseguicao.
	MobChaseThreshold = 3
	// MobMovesPerPacket e o "if (MovimentsTime >= 2)": o pacote de movimento
	// sai a cada dois passos.
	MobMovesPerPacket = 2
	// MobPacketSpeed e o 25 que MobMove recebe e troca pela velocidade da
	// especie.
	MobPacketSpeed = 25
	// MobLeashRange e o alcance em que o mob desiste do alvo e volta ao
	// ponto. A origem usava 25 na busca de alvo novo.
	MobLeashRange = 25
)

// MobState e para onde o mob esta indo na patrulha. Porte de
// TypeMobLocation.
type MobState uint8

const (
	// MobAtInit indica que o mob esta indo para DestPos.
	MobAtInit MobState = iota
	// MobAtDest indica que esta voltando para InitPos.
	MobAtDest
)

// MobTick roda um passo da IA de mob.
//
// kind separa o ataque do movimento, que na origem rodavam em threads com
// tempos diferentes.
func (h *Handler) MobTick(agora time.Time) (atacaram, andaram int) {
	if h.Mobs == nil || h.Chan == nil {
		return 0, 0
	}
	// So mexe nos mobs que alguem pode ver: a origem varria o vetor inteiro a
	// cada tick, mesmo com o mapa vazio.
	for _, m := range h.activeMobs() {
		if h.mobAttackStep(m, agora) {
			atacaram++
		}
	}
	return atacaram, andaram
}

// MobMoveTickRun roda um passo de movimento.
func (h *Handler) MobMoveTickRun(agora time.Time) int {
	if h.Mobs == nil || h.Chan == nil {
		return 0
	}
	n := 0
	for _, m := range h.activeMobs() {
		if h.mobMoveStep(m, agora) {
			n++
		}
	}
	return n
}

// activeMobs lista os mobs que estao na lista de visiveis de algum jogador.
//
// Mob que ninguem ve nao precisa andar nem atacar: a origem gastava o tick
// com todos, inclusive num mapa sem jogador nenhum.
func (h *Handler) activeMobs() []*Mob {
	ids := map[uint16]bool{}
	for _, pid := range h.Chan.PlayingIDs() {
		s, _, ok := h.Chan.Lookup(pid)
		if !ok {
			continue
		}
		for _, mid := range s.Base.VisibleMobs.IDs() {
			ids[mid] = true
		}
	}
	out := make([]*Mob, 0, len(ids))
	for id := range ids {
		if m, ok := h.Mobs.Get(id); ok && !m.Dead {
			out = append(out, m)
		}
	}
	return out
}

// mobAttackStep faz o mob bater no alvo, se der. Porte do bloco de ataque de
// TMobSPosition.MobHandler.
func (h *Handler) mobAttackStep(m *Mob, agora time.Time) bool {
	if m.Dead || !m.Attacked || m.AttackerID == 0 {
		return false
	}
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil || sp.IntName == 0 {
		// "if(MOBS.TMobS[Mobid].IntName = 0) then Exit" da origem.
		return false
	}

	alvo, _, ok := h.Chan.Lookup(m.AttackerID)
	if !ok || alvo.Status < Playing {
		// Alvo saiu do canal: o mob esquece e volta a patrulhar. Na origem
		// isso era "if(Players[AttackerID].Status < Playing) then Exit", que
		// so saia do procedimento e deixava o mob preso no alvo que foi
		// embora — ele nunca voltava ao ponto de spawn.
		h.mobForget(m)
		return false
	}
	if alvo.Dead {
		h.mobForget(m)
		return false
	}
	// Alvo longe demais: o mob desiste. A origem nao tinha esse limite, so a
	// perseguicao sem fim.
	if !m.Position.InRange(alvo.Position, MobLeashRange) {
		h.mobForget(m)
		return false
	}
	if !m.Position.InRange(alvo.Position, MobAttackRange) {
		return false
	}
	if !m.LastAttack.IsZero() && agora.Sub(m.LastAttack) < MobAttackCooldown {
		return false
	}
	m.LastAttack = agora

	h.mobHitsPlayer(m, sp, alvo)
	return true
}

// mobForget faz o mob esquecer o alvo e voltar a patrulhar.
func (h *Handler) mobForget(m *Mob) {
	m.Attacked = false
	m.AttackerID = 0
	m.FirstAttackerID = 0
}

// mobHitsPlayer aplica o golpe do mob no jogador. Porte da conta de
// TMobSPosition.AttackPlayer.
func (h *Handler) mobHitsPlayer(m *Mob, sp *mobdata.Species, alvo *Session) {
	rnd := h.rand()

	// "DANO := DNFis div 2", com DNFis vindo da coluna de ataque do CSV.
	dano := int(sp.AtkFis) / 2

	// A defesa usada depende da classe do alvo: fisica para as classes 0 a 3
	// e magica para as demais.
	def := int(alvo.Avatar.DefFis)
	if stats_MobClassOf(alvo.Avatar.ClassInfo) > 3 {
		def = int(alvo.Avatar.DefMag)
	}

	// Sorteio de 1 a 100: 2..15 erra, 16..30 critico, o resto normal. O 1
	// cai no "else" da origem e vale normal.
	tipo := combat.Normal
	switch r := rnd.IntN(100) + 1; {
	case r >= 2 && r <= 15:
		tipo = combat.Miss
	case r >= 16 && r <= 30:
		tipo = combat.Critical
	}

	dano -= def >> 3
	dano += rnd.IntN(29) + 10 + 7 // RandomRange(10, 39) + 7

	if tipo == combat.Miss {
		dano = 0
	} else {
		// Os buffs de bloqueio zeram o golpe e sao consumidos.
		if alvo.Base.Buffs.Has(combat.BuffBlock19) {
			h.removeBuffAndNotify(alvo, combat.BuffBlock19)
			dano = 0
			tipo = combat.Block
		} else if alvo.Base.Buffs.Has(combat.BuffBlock91) {
			h.removeBuffAndNotify(alvo, combat.BuffBlock91)
			dano = 0
			tipo = combat.Miss2
		}
		if tipo == combat.Critical {
			dano = int(math.RoundToEven(float64(dano) * 1.5))
		}
	}
	// "if (DANO < 0) then DANO := 1": zero continua zero, negativo vira 1.
	if dano < 0 {
		dano = 1
	}

	if dano > 0 {
		alvoT := &sessionTarget{s: alvo, h: h}
		mobT := &mobTarget{m: m, w: h.Mobs}
		res := combat.ApplyToTarget(dano, tipo, alvoT, mobT, &alvo.DefesaPoints)
		dano, tipo = res.Damage, res.Type
		if res.Reflected > 0 {
			h.Mobs.Hit(m, res.Reflected, alvo.ClientID, h.now())
		}
		dano = h.divineUnion(alvo, dano)
		dano = h.reactiveHit(nil, alvo, dano, tipo, tipo != combat.Miss)
	}

	h.applyDamage(alvo, dano)
	h.broadcastMobAttack(m, alvo, tipo, dano)

	if alvo.Dead {
		h.log().Info("jogador morto por mob",
			"jogador", alvo.Avatar.Name, "mob", sp.Name, "canal", h.Channel)
		h.mobForget(m)
	}
}

// Animacoes fixas do golpe de mob, de TMobSPosition.AttackPlayer (MOB.pas).
const (
	MobAttackAnimation = 6
	MobHitReaction     = 26
	MobDeathReaction   = 30
)

// broadcastMobAttack manda o $102 do golpe do mob.
func (h *Handler) broadcastMobAttack(m *Mob, alvo *Session, tipo combat.DamageType, dano int) {
	var pkt packets.RecvDamagePacket
	pkt.Header.Index = m.ClientID
	pkt.AttackerID = m.ClientID
	pkt.AttackerHP = m.CurHP
	pkt.AttackerPos.X = m.Position.X
	pkt.AttackerPos.Y = m.Position.Y
	pkt.TargetID = alvo.ClientID
	pkt.DNType = uint8(tipo)
	pkt.DANO = uint64(max(dano, 0))
	pkt.MobCurrHP = alvo.Avatar.CurHP
	// Animacao do golpe (6) e reacao do alvo (26, ou 30 na morte): os valores
	// fixos de TMobSPosition.AttackPlayer. Sem eles o mob bate sem animar.
	pkt.Animation = MobAttackAnimation
	pkt.MobAnimation = MobHitReaction
	if alvo.Dead {
		pkt.MobAnimation = MobDeathReaction
		pkt.DeathPos.X = alvo.Position.X
		pkt.DeathPos.Y = alvo.Position.Y
	}

	raw, err := netsrv.Marshal(CodeRecvDamage, pkt)
	if err != nil {
		h.log().Error("montar golpe de mob", "mob", m.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(alvo.ClientID, raw)
	for _, id := range h.Chan.PlayingIDs() {
		if id == alvo.ClientID {
			continue
		}
		outro, _, ok := h.Chan.Lookup(id)
		if ok && outro.Base.VisibleMobs.Contains(m.ClientID) {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// mobMoveStep anda um passo. Porte de TMobSPosition.MobMoviment.
func (h *Handler) mobMoveStep(m *Mob, agora time.Time) bool {
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil || sp.IntName == 0 {
		return false
	}
	pt := h.mobSpawnPoint(m)
	if pt == nil {
		return false
	}
	// Mutante nao anda.
	if sp.Mutant || pt.InitPos == pt.DestPos {
		return false
	}

	if m.Attacked && m.AttackerID != 0 {
		return h.mobChaseStep(m, sp, agora)
	}
	return h.mobPatrolStep(m, sp, pt, agora)
}

// mobPatrolStep anda entre o ponto inicial e o destino.
func (h *Handler) mobPatrolStep(m *Mob, sp *mobdata.Species, pt *mobdata.Spawn, agora time.Time) bool {
	// A espera entre passos vem do CSV. A origem usava InitMoveWait nos DOIS
	// sentidos, e DestMoveWait — que ela carrega do arquivo — nunca era lido:
	// a coluna existia e nao fazia nada. Aqui cada sentido usa a sua.
	espera := pt.InitMoveWait
	destino := pt.DestPos
	if m.State == MobAtDest {
		espera = pt.DestMoveWait
		destino = pt.InitPos
	}
	if espera < 0 {
		espera = 0
	}
	if !m.LastMove.IsZero() && agora.Sub(m.LastMove) <= time.Duration(espera)*time.Second {
		return false
	}

	antes := m.Position
	m.Position = stepToward(m.Position, destino, MobStep, MobAxisThreshold)

	// Chegou quando nenhum eixo consegue mais andar.
	//
	// A origem testava "if (CurrentPos.Distance(DestPos) <= 0)", e Distance
	// devolve um WORD truncado. Com passo de 1.5 e limite de 2 celulas por
	// eixo, o mob para a 1.5 celula do destino: a distancia truncada vale 1,
	// o teste nunca fecha e o mob fica preso ali para sempre, sem nunca
	// inverter o sentido — e, pior, MovimentsTime continua subindo, entao a
	// cada dois ticks sai um pacote de movimento com a MESMA posicao, para
	// todo mob em patrulha do mapa. Aqui a chegada e o momento em que o passo
	// nao muda nada.
	if m.Position == antes {
		m.Position = destino
		m.Moves = 0
		m.LastMove = agora
		if m.State == MobAtInit {
			m.State = MobAtDest
		} else {
			m.State = MobAtInit
		}
		// O pacote sai uma vez na chegada, para o cliente parar o boneco no
		// lugar certo.
		h.sendMobMove(m, sp, m.Position)
		return true
	}

	m.Moves++
	if m.Moves >= MobMovesPerPacket {
		m.Moves = 0
		h.sendMobMove(m, sp, m.Position)
		return true
	}
	return false
}

// mobChaseStep aproxima o mob do alvo.
func (h *Handler) mobChaseStep(m *Mob, sp *mobdata.Species, agora time.Time) bool {
	alvo, _, ok := h.Chan.Lookup(m.AttackerID)
	if !ok || alvo.Status < Playing || alvo.Dead {
		h.mobForget(m)
		return false
	}
	// Ja esta no alcance de bater: nao precisa andar.
	if m.Position.InRange(alvo.Position, MobAttackRange) {
		return false
	}
	// Alvo longe demais: desiste.
	if !m.Position.InRange(alvo.Position, MobLeashRange) {
		h.mobForget(m)
		return false
	}

	m.Position = stepToward(m.Position, alvo.Position, MobChaseStep, MobChaseThreshold)
	h.sendMobMove(m, sp, m.Position)
	return true
}

// stepToward anda um passo na direcao do destino, por eixo.
//
// Porte do par de blocos "if (XPositionDif >= 2) then ... +/- 1.5" da origem:
// cada eixo anda so quando a diferenca chega ao limite, e o passo e fixo.
func stepToward(de, para model.Position, passo float32, limite float32) model.Position {
	out := de
	if dx := absF(para.X - de.X); dx >= limite {
		if para.X > de.X {
			out.X += passo
		} else {
			out.X -= passo
		}
	}
	if dy := absF(para.Y - de.Y); dy >= limite {
		if para.Y > de.Y {
			out.Y += passo
		} else {
			out.Y -= passo
		}
	}
	// A origem nao validava a posicao do mob em nenhum momento; um destino
	// vindo errado do CSV levava o mob para fora do mapa e dali para as
	// contas de distancia de todo mundo.
	if !ValidPosition(out) {
		return de
	}
	return out
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// mobSpawnPoint devolve o ponto de spawn de um mob.
func (h *Handler) mobSpawnPoint(m *Mob) *mobdata.Spawn {
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil || m.Spawn < 0 || m.Spawn >= len(sp.Spawns) {
		return nil
	}
	return &sp.Spawns[m.Spawn]
}

// sendMobMove manda o $301 do mob a quem o ve. Porte de
// TMobSPosition.MobMove.
func (h *Handler) sendMobMove(m *Mob, sp *mobdata.Species, destino model.Position) {
	var pkt packets.MobMovimentPacket
	pkt.Header.Index = m.ClientID
	pkt.Destination.X = destino.X
	pkt.Destination.Y = destino.Y
	// MobMove recebia 25 e trocava pela velocidade da especie.
	vel := sp.MoveSpeed
	if vel == 0 {
		vel = MobPacketSpeed
	}
	pkt.Speed = uint8(min(vel, 255))

	raw, err := netsrv.Marshal(CodeMovement, pkt)
	if err != nil {
		h.log().Error("montar movimento de mob", "mob", m.ClientID, "erro", err)
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if ok && outro.Base.VisibleMobs.Contains(m.ClientID) {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// mobTarget adapta o mob ao alvo do pipeline de dano, para o reflexo de dano
// do jogador poder bater de volta nele.
type mobTarget struct {
	m *Mob
	w *MobWorld
}

func (t *mobTarget) HasBuff(uint16) bool { return false }

func (t *mobTarget) RemoveBuff(uint16) {}

func (t *mobTarget) Ability(int) int { return 0 }

func (t *mobTarget) DecAbility(int, int) {}

// Notify nao faz nada: mob nao recebe mensagem. Na origem a linha equivalente
// era Players[mob.ClientID].SendClientMessage, com ClientID de mob acima de
// 1000 num vetor de 1000 — indice fora do vetor.
func (t *mobTarget) Notify(string) {}

func (t *mobTarget) AddHP(value int) {
	if value < 0 && t.w != nil {
		t.w.Hit(t.m, -value, 0, time.Now())
	}
}
