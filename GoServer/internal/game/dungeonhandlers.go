package game

import (
	"context"
	"fmt"
	"time"

	"aika/internal/combat"
	"aika/internal/dungeon"
	"aika/internal/dungeondata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de dungeon.
const (
	// CodeDungeonDialog e o $119 de TNPCHandlers.ShowDungeonDialog: servidor
	// -> cliente, a dungeon que aquele NPC abre e o nivel minimo dela.
	//
	// Uma versao anterior deste porte anotou $348 aqui; $348 e o
	// CloseNPCOption (ServerSocket.pas:3171), nao o dialogo.
	CodeDungeonDialog = 0x119
	// CodeDungeonLobby e o $145: servidor -> cliente, a tela de confirmacao.
	CodeDungeonLobby = 0x145
	// CodeDungeonEnter e o $334: cliente -> servidor pede a dificuldade e,
	// na volta, servidor -> cliente avisa que entrou (com o id da instancia).
	CodeDungeonEnter = 0x334
	// CodeDungeonConfirm e o $355: cliente -> servidor confirma ou recusa.
	CodeDungeonConfirm = 0x355
)

// DungeonNPCOption e o "Player.OpennedOption = 26" que a origem exigia para
// aceitar o pedido de entrada.
const DungeonNPCOption = 26

// DungeonTable e a tabela de dungeons, para o handler nao depender do loader.
type DungeonTable interface {
	Get(dungeonID, dificult uint32) (*dungeondata.Definition, bool)
	ByEntranceNPC(npcID uint16, dificult uint32) (*dungeondata.Definition, bool)
}

// requestEnterDungeon e o porte de TPacketHandlers.RequestEnterDungeon
// ($334).
//
// A origem:
//
//   - rodava sem "if (Player.IsInstantiated)", ao contrario dos vizinhos no
//     mesmo case (ServerSocket.pas:3080): quem nem tinha entrado no jogo
//     podia disparar a entrada;
//
//   - indexava "DGMines1[Packet.Dificult]" com um DWORD do cliente num vetor
//     "Array [0..0]" (PacketHandlers.pas:9489). Qualquer dificuldade diferente
//     de 0 lia uma ficha de dungeon inteira fora do vetor e seguia com ela:
//     nivel minimo, posicao de entrada e tabela de mobs saiam de memoria
//     alheia;
//
//   - no caso do NPC do Kinary conferia o nivel com
//     "for I := Low(Party.Members.ToArray) to High(...)" e usava I como
//     indice de CONEXAO (PacketHandlers.pas:9557): conferia o nivel dos
//     jogadores das conexoes 0, 1, 2..., nao dos membros do grupo;
//
//   - repetia o mesmo bloco cinco vezes, um por NPC, com a lista de dungeon
//     chumbada. Aqui o NPC vem da tabela.
func (h *Handler) requestEnterDungeon(ctx context.Context, c *netsrv.Conn,
	s *Session, p protocol.Packet) error {
	if s.Status != Playing {
		return ErrNotPlaying
	}
	if h.Dungeons == nil || h.DungeonTable == nil {
		return nil
	}
	var pkt packets.SelectDungeonToEnter
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	// A origem exigia a janela do NPC aberta; sem isso o cliente mandava o
	// pacote de qualquer lugar do mapa.
	if s.OpenedOption != DungeonNPCOption {
		return nil
	}
	def, ok := h.DungeonTable.ByEntranceNPC(s.OpenedNPC, pkt.Dificult)
	if !ok {
		h.message(c, "Essa dungeon nao esta disponivel.")
		return nil
	}
	if _, ja := h.Dungeons.Of(s.CharID()); ja {
		h.message(c, "Voce ja esta numa dungeon.")
		return nil
	}

	grupo := h.Parties.Of(s.ClientID)
	if grupo == nil {
		// Sozinho: so o proprio nivel conta.
		if s.Avatar.Level < uint16(def.LevelMin) {
			h.message(c, fmt.Sprintf(
				"A dungeon selecionada requer nivel [%d].", def.LevelMin))
			return nil
		}
		lobby := dungeon.NewLobby(def.Index, pkt.Dificult, 0, s.CharID(),
			[]uint32{s.CharID()}, h.now())
		h.DungeonLobbies.Open(lobby)
		return h.sendDungeonLobby(c, s, def, pkt.Dificult, []uint16{s.ClientID})
	}

	if grupo.Leader != s.ClientID {
		h.message(c, "Voce nao e o lider do grupo.")
		return nil
	}
	membros := h.Parties.MembersOf(grupo)
	var charIDs []uint32
	for _, id := range membros {
		outro, _, ok := h.Chan.Lookup(id)
		if !ok || outro.Status != Playing {
			h.message(c, "Um membro do grupo nao esta em jogo.")
			return nil
		}
		if outro.Avatar.Level < uint16(def.LevelMin) {
			h.message(c, fmt.Sprintf(
				"A dungeon selecionada requer nivel [%d].", def.LevelMin))
			return nil
		}
		if _, ja := h.Dungeons.Of(outro.CharID()); ja {
			h.message(c, "Um membro do grupo ja esta numa dungeon.")
			return nil
		}
		charIDs = append(charIDs, outro.CharID())
	}
	lobby := dungeon.NewLobby(def.Index, pkt.Dificult, grupo.Index, s.CharID(),
		charIDs, h.now())
	h.DungeonLobbies.Open(lobby)

	// A origem punha DungeonLobbyIndex e DungeonLobbyDificult em cada membro
	// e mandava o $145 para o grupo.
	for _, id := range membros {
		outro, conn, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		if err := h.sendDungeonLobby(conn, outro, def, pkt.Dificult, membros); err != nil {
			h.Log.Warn("saguao de dungeon", "erro", err, "clientid", id)
		}
	}
	return nil
}

// sendDungeonLobby e o porte de TPlayer.SendDungeonLobby ($145).
//
// A origem escrevia "Packet.PlayersParty1[Cnt]" com Cnt crescendo por membro
// num vetor "Array [0..5]" (Player.pas:11101): um grupo com mais de 6 membros
// transbordava para PlayersParty2 e, em UnkPlayersRecord, para fora do pacote.
func (h *Handler) sendDungeonLobby(c *netsrv.Conn, s *Session,
	def *dungeondata.Definition, dificult uint32, membros []uint16) error {
	var pkt packets.SendPlayersLobbyDungeon
	pkt.Dungeon = uint16(def.Index)
	pkt.Dificult = uint16(dificult)
	for i, id := range membros {
		if i >= len(pkt.PlayersParty1) {
			break
		}
		pkt.PlayersParty1[i] = uint32(id)
		pkt.UnkPlayersRecord[i] = uint32(i)
	}
	buf, err := netsrv.Marshal(CodeDungeonLobby, pkt)
	if err != nil {
		return err
	}
	return h.sendRaw(c, s, buf)
}

// confirmDungeon e o porte de TPacketHandlers.DungeonLobbyConfirm ($355).
//
// A origem creditava a confirmacao a "Packet.Header.Index", que vem do
// cliente (PacketHandlers.pas:9699): um jogador mandando um pacote por membro
// confirmava por todos e arrastava o grupo para dentro. Aqui a confirmacao e
// de quem mandou.
func (h *Handler) confirmDungeon(ctx context.Context, c *netsrv.Conn,
	s *Session, p protocol.Packet) error {
	if s.Status != Playing {
		return ErrNotPlaying
	}
	if h.Dungeons == nil || h.DungeonTable == nil {
		return nil
	}
	var pkt packets.ConfirmDungeonEnter
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	lobby, ok := h.DungeonLobbies.Of(s.CharID())
	if !ok {
		return nil
	}
	if lobby.Expired(h.now()) {
		h.DungeonLobbies.Drop(lobby)
		h.message(c, "O tempo de confirmacao acabou.")
		return nil
	}

	// Index = 0 e a recusa: fecha para todos, como na origem.
	if pkt.Index == 0 {
		lobby.Cancel(s.CharID())
		h.avisarSaguao(lobby, fmt.Sprintf("[%s] se recusou a entrar na dungeon.",
			s.Avatar.Name), pkt.Index)
		h.DungeonLobbies.Drop(lobby)
		return nil
	}

	completo, aceito := lobby.Confirm(s.CharID())
	if !aceito {
		return nil
	}
	// Repassa a confirmacao aos outros, como "Party.SendToParty".
	h.repassarConfirmacao(lobby, s)
	if !completo {
		return nil
	}
	return h.openDungeon(ctx, lobby)
}

// repassarConfirmacao manda o $355 aos outros membros do saguao.
func (h *Handler) repassarConfirmacao(lobby *dungeon.Lobby, s *Session) {
	for _, charID := range lobby.Members() {
		if charID == s.CharID() {
			continue
		}
		id, conn, outro, ok := h.byCharID(charID)
		if !ok {
			continue
		}
		var pkt packets.ConfirmDungeonEnter
		pkt.Index = uint32(s.ClientID)
		buf, err := netsrv.Marshal(CodeDungeonConfirm, pkt)
		if err != nil {
			continue
		}
		_ = id
		if err := h.sendRaw(conn, outro, buf); err != nil {
			h.Log.Warn("confirmacao de dungeon", "erro", err, "charid", charID)
		}
	}
}

// openDungeon cria a instancia e leva todos para dentro.
//
// A origem fazia o contrario: teleportava e marcava InDungeon ANTES de pedir
// a instancia, e quando GetFreeDungeonInstance devolvia 255 (sempre que outro
// grupo estivesse dentro, porque o vetor tinha um elemento) ela so dava
// "Exit": o grupo ficava num mapa sem mob, sem instancia, e todo acesso
// seguinte usava DungeonInstanceID = 0, fora do vetor [1..1]
// (Player.pas:11158).
func (h *Handler) openDungeon(ctx context.Context, lobby *dungeon.Lobby) error {
	h.DungeonLobbies.Drop(lobby)

	def, ok := h.DungeonTable.Get(lobby.DungeonID, lobby.Dificult)
	if !ok {
		h.avisarSaguao(lobby, "Essa dungeon nao esta disponivel.", 0)
		return nil
	}
	// Quem caiu no meio da confirmacao nao entra.
	var dentro []uint32
	for _, charID := range lobby.Members() {
		if _, _, s, ok := h.byCharID(charID); ok && s.Status == Playing {
			dentro = append(dentro, charID)
		}
	}
	if len(dentro) == 0 {
		return nil
	}

	in, err := h.Dungeons.Open(def, lobby.PartyIndex, dentro, h.now())
	if err != nil {
		h.avisarSaguao(lobby, "Nao foi possivel abrir a dungeon.", 0)
		return nil
	}
	// A instancia existe: agora sim o grupo entra.
	for _, charID := range dentro {
		id, conn, s, ok := h.byCharID(charID)
		if !ok {
			continue
		}
		h.enterDungeon(ctx, conn, s, in, def)
		var pkt packets.SelectDungeonToEnter
		pkt.Dificult = in.ID
		if buf, err := netsrv.Marshal(CodeDungeonEnter, pkt); err == nil {
			if err := h.sendRaw(conn, s, buf); err != nil {
				h.Log.Warn("entrada na dungeon", "erro", err, "clientid", id)
			}
		}
	}
	h.Log.Info("dungeon aberta", "instancia", in.ID, "dungeon", def.Index,
		"dificuldade", def.Dificult, "membros", len(dentro),
		"mobs", in.MobCount())
	return nil
}

// enterDungeon teleporta o jogador para dentro e limpa o que ele via.
func (h *Handler) enterDungeon(ctx context.Context, c *netsrv.Conn, s *Session,
	in *dungeon.Instance, def *dungeondata.Definition) {
	s.DungeonInstance = in.ID
	s.DungeonID = def.Index
	s.DungeonDificult = uint32(def.Dificult)
	s.ReturnPosition = s.Position
	h.teleportTo(s, def.SpawnInDungeonPosition)
}

// leaveDungeon tira o jogador da instancia e o traz de volta. A origem
// repetia este bloco em sete lugares, cada um zerando um subconjunto
// diferente dos campos, e um deles ainda punha "IsInsideDungeon := True" na
// saida (Dungeon.pas:294) — um campo que nada le.
func (h *Handler) leaveDungeon(ctx context.Context, c *netsrv.Conn, s *Session) {
	if h.Dungeons == nil {
		return
	}
	in, fechou := h.Dungeons.Leave(s.CharID())
	if in == nil {
		return
	}
	volta := s.ReturnPosition
	if volta.X == 0 && volta.Y == 0 {
		volta = dungeon.ExitPosition
	}
	s.DungeonInstance = 0
	s.DungeonID = 0
	s.DungeonDificult = 0
	s.ReturnPosition = model.Position{}
	if c != nil {
		h.teleportTo(s, volta)
	}
	if fechou {
		h.Log.Info("dungeon fechada", "instancia", in.ID, "motivo", "vazia")
	}
}

// closeDungeon fecha a instancia e traz todos de volta.
func (h *Handler) closeDungeon(ctx context.Context, in *dungeon.Instance, motivo string) {
	if h.Dungeons == nil || in == nil {
		return
	}
	for _, charID := range h.Dungeons.Close(in) {
		_, conn, s, ok := h.byCharID(charID)
		if !ok {
			continue
		}
		volta := s.ReturnPosition
		if volta.X == 0 && volta.Y == 0 {
			volta = dungeon.ExitPosition
		}
		s.DungeonInstance = 0
		s.DungeonID = 0
		s.DungeonDificult = 0
		s.ReturnPosition = model.Position{}
		h.message(conn, "A dungeon foi encerrada.")
		h.teleportTo(s, volta)
	}
	h.Log.Info("dungeon fechada", "instancia", in.ID, "motivo", motivo)
}

// avisarSaguao manda uma mensagem a todos os membros do saguao.
func (h *Handler) avisarSaguao(lobby *dungeon.Lobby, texto string, index uint32) {
	for _, charID := range lobby.Members() {
		_, conn, _, ok := h.byCharID(charID)
		if !ok {
			continue
		}
		h.message(conn, texto)
		if index == 0 {
			continue
		}
	}
}

// byCharID acha a sessao em jogo de um personagem.
func (h *Handler) byCharID(charID uint32) (uint16, *netsrv.Conn, *Session, bool) {
	if charID == 0 {
		return 0, nil, nil, false
	}
	for _, id := range h.Chan.PlayingIDs() {
		s, conn, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		if s.CharID() == charID {
			return id, conn, s, true
		}
	}
	return 0, nil, nil, false
}

// DungeonTick e o porte da TDungeonMainThread, uma por instancia na origem.
// Fecha o que venceu e roda a IA dos mobs.
func (h *Handler) DungeonTick(ctx context.Context) int {
	if h.Dungeons == nil {
		return 0
	}
	agora := h.now()
	fechadas := 0

	for _, lobby := range h.DungeonLobbies.Expired(agora) {
		h.avisarSaguao(lobby, "O tempo de confirmacao acabou.", 0)
		h.DungeonLobbies.Drop(lobby)
	}

	for _, in := range h.Dungeons.All() {
		switch {
		case in.Expired(agora):
			h.closeDungeon(ctx, in, "prazo")
			fechadas++
			continue
		case in.Cleared():
			h.closeDungeon(ctx, in, "limpa")
			fechadas++
			continue
		}
		h.dungeonMobTick(ctx, in, agora)
	}
	return fechadas
}

// dungeonMobTick e o miolo do laco da TDungeonMainThread: escolhe alvo,
// persegue, bate e volta ao ponto de origem.
func (h *Handler) dungeonMobTick(ctx context.Context, in *dungeon.Instance, agora time.Time) {
	membros := in.MemberList()
	if len(membros) == 0 {
		return
	}
	alcance := uint16(dungeon.AggroRangeParty)
	if len(membros) == 1 {
		alcance = uint16(dungeon.AggroRangeSolo)
	}

	type jogador struct {
		id   uint16
		conn *netsrv.Conn
		s    *Session
	}
	var vivos []jogador
	for _, charID := range membros {
		id, conn, s, ok := h.byCharID(charID)
		if !ok || s.Dead {
			continue
		}
		vivos = append(vivos, jogador{id, conn, s})
	}
	if len(vivos) == 0 {
		return
	}

	for _, m := range in.Mobs() {
		if !m.Alive() {
			continue
		}
		if m.AttackerCharIndex == 0 {
			// Sem alvo: pega o primeiro membro no alcance.
			for _, j := range vivos {
				if m.Pos.InRange(j.s.Position, alcance) {
					in.SetTarget(m, j.s.CharID(), agora)
					break
				}
			}
			continue
		}
		// Com alvo: acha a sessao dele.
		var alvo *jogador
		for i := range vivos {
			if vivos[i].s.CharID() == m.AttackerCharIndex {
				alvo = &vivos[i]
				break
			}
		}
		if alvo == nil {
			// O alvo saiu ou morreu: volta ao ponto de origem.
			in.SetTarget(m, 0, agora)
			in.MoveMob(m, m.OrigPos, agora)
			continue
		}
		// Longe do ponto de origem: desiste e volta.
		if !m.Pos.InRange(m.OrigPos, dungeon.LeashRange) {
			in.SetTarget(m, 0, agora)
			in.MoveMob(m, m.OrigPos, agora)
			continue
		}
		if m.Pos.InRange(alvo.s.Position, dungeon.AttackRange) {
			if in.MarkAttack(m, agora) {
				h.dungeonMobAttack(ctx, in, m, alvo.conn, alvo.s)
			}
			continue
		}
		// Persegue: um passo na direcao do alvo.
		in.MoveMob(m, passoNaDirecao(m.Pos, alvo.s.Position), agora)
	}
}

// passoNaDirecao da um passo do mob na direcao do alvo. A origem sorteava um
// dos oito vizinhos do jogador com "Rand := Random(7)" depois de chamar
// Randomize a cada tick (Dungeon.pas:748), o que teleportava o mob para o
// lado do jogador em um unico passo e nunca sorteava o oitavo vizinho.
func passoNaDirecao(de, para model.Position) model.Position {
	const passo = 1.5
	return stepToward(de, para, passo, passo)
}

// sendRaw manda um pacote JA CIFRADO por netsrv.Marshal para a conexao.
// Conn.Send cifraria de novo.
func (h *Handler) sendRaw(c *netsrv.Conn, s *Session, raw []byte) error {
	if c == nil {
		if h.Chan == nil || s == nil {
			return nil
		}
		return h.Chan.SendTo(s.ClientID, raw)
	}
	return c.SendRaw(raw)
}

// teleportTo poe o jogador numa posicao e refaz o que ele ve. A origem usava
// "Base.WalkTo(pos, 70, MOVE_TELEPORT)" e nao limpava as listas de visiveis
// na entrada da dungeon (so o faziam no caminho da morte, Dungeon.pas:280):
// quem entrava seguia vendo, e sendo visto por, quem ficou do lado de fora.
func (h *Handler) teleportTo(s *Session, pos model.Position) {
	if s == nil || !ValidPosition(pos) {
		return
	}
	// Tudo o que ele via fica para tras.
	for _, id := range s.Base.VisiblePlayers.IDs() {
		if outro, _, ok := h.Chan.Lookup(id); ok {
			h.despawnPair(s, outro)
		}
	}
	s.Base.VisibleMobs.Clear()
	s.Base.VisibleNPCs.Clear()
	if s.Base.Targets != nil {
		s.Base.Targets.Clear()
	}
	s.Position = pos
	s.AnchorPosition = pos
	s.LastMoved = h.now()
	_ = h.resync(s)
	h.refreshVisible(s)
}

// dungeonMobAttack e o porte de TMobsStructDungeonInstance.AttackPlayer
// (Dungeon.pas:112-337).
//
// A conta e a da origem:
//
//	DANO := ((AtkFis + (25 * X * (Dificuldade + 1))) div 2)
//	          - (DefFis div (DungeonID + 2))
//	DamageReduction := (DANO div 100) * (ReducaoDoEquipamento div 10)
//	Dec(DANO, DamageReduction)
//	Inc(DANO, RandomRange(10, 39) + 7)
//	if (DANO < 0) then DANO := 1
//
// com X = DungeonID. Tres coisas da origem mudaram:
//
//   - O sorteio de 2..15 marcava o golpe como "Miss" e o dano era descontado
//     do HP do mesmo jeito: o cliente escrevia Miss e o jogador perdia vida.
//     "Critical" tambem nao mudava o dano. Aqui Miss zera o dano e Critical
//     aumenta, como no golpe de mob do mundo.
//
//   - "case Dificuldade of 0..3" e "case DungeonIDLobby of 1..8" deixavam
//     DANO em zero fora das faixas, e a dungeon so tem dificuldade 0..2 e id
//     1..5. Como o DungeonIDLobby vinha do campo do jogador, que a origem
//     tambem usava para o id do saguao, um id fora de 1..8 dava golpe de dano
//     so aleatorio.
//
//   - A origem lia o HP do alvo de "Base.PlayerCharacter.Base.CurrentScore"
//     para conferir e escrevia em "Base.Character.CurrentScore": leitura e
//     escrita em registros diferentes do mesmo jogador.
func (h *Handler) dungeonMobAttack(ctx context.Context, in *dungeon.Instance,
	m *dungeon.Mob, c *netsrv.Conn, alvo *Session) {
	if m == nil || m.Species == nil || alvo == nil {
		return
	}
	rnd := h.rand()

	dificuldade := int(in.Def.Dificult)
	dungeonID := int(in.Def.Index)
	multiplicador := dificuldade + 1
	divisorDefesa := dungeonID + 2

	dano := (int(m.Species.AtkFis) + 25*dungeonID*multiplicador) / 2
	dano -= int(alvo.Avatar.DefFis) / divisorDefesa

	// A reducao do equipamento, como na origem: percentual do dano.
	if red := h.equipDamageReduce(alvo); red > 0 {
		dano -= (dano / 100) * (red / 10)
	}
	dano += rnd.IntN(29) + 10 + 7 // RandomRange(10, 39) + 7

	tipo := combat.Normal
	switch r := rnd.IntN(100) + 1; {
	case r >= 2 && r <= 15:
		tipo = combat.Miss
	case r >= 16 && r <= 30:
		tipo = combat.Critical
	}
	switch tipo {
	case combat.Miss:
		dano = 0
	case combat.Critical:
		dano = int(float64(dano) * 1.5)
	}
	if dano < 0 {
		dano = 1
	}
	// Os buffs de bloqueio valem aqui tambem: na origem eles eram conferidos
	// antes da conta e saiam com Exit, mas so quando o jogador os tinha.
	if dano > 0 {
		if alvo.Base.Buffs.Has(combat.BuffBlock19) {
			h.removeBuffAndNotify(alvo, combat.BuffBlock19)
			dano, tipo = 0, combat.Block
		} else if alvo.Base.Buffs.Has(combat.BuffBlock91) {
			h.removeBuffAndNotify(alvo, combat.BuffBlock91)
			dano, tipo = 0, combat.Miss2
		}
	}

	h.applyDamage(alvo, dano)
	h.broadcastDungeonAttack(in, m, alvo, tipo, dano)

	if alvo.Dead {
		h.Log.Info("jogador morto em dungeon", "jogador", alvo.Avatar.Name,
			"instancia", in.ID, "mob", m.ClientID)
		in.SetTarget(m, 0, h.now())
		// A origem, no caminho da morte, fechava a instancia inteira quando o
		// grupo tinha um membro so, e nos outros casos nao fazia nada alem de
		// marcar um campo que ninguem le. Aqui quem morre sai da instancia; se
		// era o ultimo, a instancia fecha no mesmo passo.
		h.leaveDungeon(ctx, c, alvo)
	}
}

// equipDamageReduce e o GetEquipedItensDamageReduce da origem: a soma do
// efeito de reducao de dano dos itens equipados. O efeito de item ainda nao
// esta portado, entao a reducao e zero e a conta do golpe fica sem ela, como
// ficava para qualquer jogador sem item com esse efeito.
func (h *Handler) equipDamageReduce(s *Session) int {
	return 0
}

// broadcastDungeonAttack manda o $102 do golpe aos membros da instancia.
func (h *Handler) broadcastDungeonAttack(in *dungeon.Instance, m *dungeon.Mob,
	alvo *Session, tipo combat.DamageType, dano int) {
	var pkt packets.RecvDamagePacket
	pkt.Header.Index = m.ClientID
	pkt.AttackerID = m.ClientID
	pkt.AttackerHP = m.CurHP
	pkt.AttackerPos.X = m.Pos.X
	pkt.AttackerPos.Y = m.Pos.Y
	pkt.TargetID = alvo.ClientID
	pkt.DNType = uint8(tipo)
	pkt.DANO = uint64(max(dano, 0))
	pkt.MobCurrHP = alvo.Avatar.CurHP
	pkt.Animation = MobAttackAnimation
	pkt.MobAnimation = MobHitReaction
	if alvo.Dead {
		pkt.MobAnimation = MobDeathReaction
		pkt.DeathPos.X = alvo.Position.X
		pkt.DeathPos.Y = alvo.Position.Y
	}
	raw, err := netsrv.Marshal(CodeRecvDamage, pkt)
	if err != nil {
		return
	}
	// O golpe so interessa a quem esta na mesma instancia: a origem mandava
	// para a lista de visiveis do jogador, que podia conter gente de fora.
	for _, charID := range in.MemberList() {
		if _, _, s, ok := h.byCharID(charID); ok {
			_ = h.Chan.SendTo(s.ClientID, raw)
		}
	}
}

// attackDungeonMob e a parte de dungeon de TPacketHandlers.AttackCommand
// (PacketHandlers.pas:5712-5753). Devolve false quando o jogador nao esta em
// dungeon, para o handler normal seguir.
//
// A origem:
//
//   - chamava GetMobDgGeralID, que devolve -1 quando nao acha, e em outro
//     caminho usava esse -1 como indice do vetor (MOB.pas:1215);
//
//   - nao tinha "Exit" no fim do bloco de dungeon: depois de bater no mob da
//     instancia, a execucao caia no "if (Packet.Index <= MAX_CONNECTIONS)" e
//     batia de novo, agora num jogador;
//
//   - marcava "AttackerID := Player.Base.ClientId" sem conferir se o mob
//     estava vivo nem a distancia.
func (h *Handler) attackDungeonMob(ctx context.Context, c *netsrv.Conn,
	s *Session, alvoID uint16) (tratou bool) {
	if h.Dungeons == nil {
		return false
	}
	in, ok := h.Dungeons.Of(s.CharID())
	if !ok {
		return false
	}
	// Em dungeon o cliente nao tem outro alvo a oferecer: um id que nao esta
	// na instancia e recusado em silencio, em vez de cair no handler de
	// jogador como na origem.
	m, ok := in.Mob(alvoID)
	if !ok || !m.Alive() {
		return true
	}
	if m.Species == nil {
		return true
	}
	if !s.Position.InRange(m.Pos, AttackRange) {
		return true
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
		def := int(m.Species.DefFis)
		if pen := int(s.Avatar.FisPenetration); pen > 0 {
			def -= (def / 100) * pen
		}
		dano = atk - (def >> 3)
		if dano <= 0 {
			dano = 1
		}
		baixo, alto := dano/20, dano/10
		extra := 13
		if alto > baixo {
			extra += baixo + rnd.IntN(alto-baixo)
		} else {
			extra += baixo
		}
		dano = combat.ClampDamage(dano + extra)
	}

	agora := h.now()
	morreu := in.Hit(m, uint32(max(dano, 0)), s.CharID(), agora)
	h.broadcastDungeonDamage(in, s, m, tipo, dano)
	if morreu {
		h.dungeonMobDied(ctx, in, m)
	}
	h.wearWeapon(c, s)
	return true
}

// broadcastDungeonDamage manda o $102 do golpe aos membros da instancia.
func (h *Handler) broadcastDungeonDamage(in *dungeon.Instance, s *Session,
	m *dungeon.Mob, tipo combat.DamageType, dano int) {
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
	pkt.DNType = uint8(tipo)
	pkt.DANO = uint64(max(dano, 0))
	pkt.MobCurrHP = m.CurHP
	if m.Dead {
		pkt.DeathPos.X = m.Pos.X
		pkt.DeathPos.Y = m.Pos.Y
	}
	raw, err := netsrv.Marshal(CodeRecvDamage, pkt)
	if err != nil {
		return
	}
	for _, charID := range in.MemberList() {
		if _, _, outro, ok := h.byCharID(charID); ok {
			_ = h.Chan.SendTo(outro.ClientID, raw)
		}
	}
}

// dungeonMobDied da a exp e o drop do mob morto.
//
// A origem dividia a exp dentro do laco de membros:
//
//	for i in Party.Members do
//	  MobExp := (MobExp div Party.Members.Count)
//
// (BaseMob.pas:6189). A divisao e refeita a cada volta, sobre o valor ja
// dividido: num grupo de quatro o primeiro membro leva Exp/4, o segundo
// Exp/16, o terceiro Exp/64 e o quarto Exp/256, em vez de Exp/4 para cada um.
//
// O drop de dungeon, por sua vez, esta todo comentado na origem
// (BaseMob.pas:6988-7022): o mob de dungeon caia no drop do mundo, indexado
// por "Mobs.TMobS[mob.Mobid]" com Mobid = slot na instancia (1..450), isto e,
// a lista de drop de uma especie do mundo escolhida pelo numero do slot.
func (h *Handler) dungeonMobDied(ctx context.Context, in *dungeon.Instance, m *dungeon.Mob) {
	if m == nil || m.Species == nil {
		return
	}
	membros := in.MemberList()
	if len(membros) == 0 {
		return
	}
	exp := int(m.Species.Exp)
	// Divisao igual: uma vez, para todos.
	quota := exp / len(membros)

	for _, charID := range membros {
		_, conn, s, ok := h.byCharID(charID)
		if !ok || s.Dead {
			continue
		}
		if quota > 0 {
			h.addExp(conn, s, int64(quota), ExpTypeMob)
		}
	}
	// O drop vai para quem bateu primeiro, da lista da propria dungeon.
	if m.FirstAttacker != 0 {
		if _, conn, s, ok := h.byCharID(m.FirstAttacker); ok {
			h.dropDungeonItem(ctx, conn, s, in, m)
		}
	}
}

// dropDungeonItem sorteia um item da lista da dungeon para o jogador.
func (h *Handler) dropDungeonItem(ctx context.Context, c *netsrv.Conn, s *Session,
	in *dungeon.Instance, m *dungeon.Mob) {
	lista := in.Def.DropsFor(m.Species)
	if len(lista) == 0 {
		return
	}
	rnd := h.rand()
	// "if (DropTax > 50)" da origem: metade das mortes dropa.
	if rnd.IntN(100)+1 <= 50 {
		return
	}
	// A origem sorteava com "RandomRange(1, cnt)", que nunca devolve o
	// indice 0 nem o ultimo, e deixava ItemTax sem valor quando a lista
	// estava vazia.
	item := lista[rnd.IntN(len(lista))]
	if item == 0 {
		return
	}
	if h.MapObjs == nil {
		return
	}
	// A origem punha o item direto no inventario com PutItem/PutEquipament
	// (BaseMob.pas:7013), sem conferir se havia espaco: com o inventario
	// cheio o retorno era descartado e o item sumia. Aqui o item cai no chao,
	// como o drop do mundo, e quem quiser coleta.
	obj := &MapObject{
		Position:    m.Pos,
		ContentType: ObjectBoxItem,
		ItemID:      item,
		Amount:      1,
		CollectTime: dropCollectTime,
		Face:        dropFace,
		NameID:      dropNameID,
		CreatedAt:   h.now(),
		Owner:       s.ClientID,
	}
	if _, ok := h.MapObjs.Add(obj); !ok {
		h.Log.Warn("sem espaco para objeto de dungeon",
			"objetos", h.MapObjs.Count(), "item", item)
		return
	}
	h.broadcastMapObject(obj)
}

// dungeonOnDisconnect solta o jogador da instancia e do saguao quando a
// conexao cai.
//
// A origem limpava os campos de dungeon em Connection.Destroy, mas nao tirava
// o jogador da instancia nem mexia em InstanceOnline: a instancia ficava
// aberta com o membro que caiu na lista, e como havia um unico slot de
// instancia no servidor, nenhum outro grupo conseguia entrar em dungeon
// enquanto o servidor nao reiniciasse.
func (h *Handler) dungeonOnDisconnect(s *Session) {
	if s == nil {
		return
	}
	if h.DungeonLobbies != nil {
		if lobby, ok := h.DungeonLobbies.Of(s.CharID()); ok {
			// Quem cai no saguao cancela para o grupo: ninguem fica esperando
			// uma confirmacao que nao vem mais.
			lobby.Cancel(s.CharID())
			h.DungeonLobbies.Drop(lobby)
			h.avisarSaguao(lobby, fmt.Sprintf(
				"[%s] saiu e a entrada na dungeon foi cancelada.", s.Avatar.Name), 0)
		}
	}
	if h.Dungeons == nil {
		return
	}
	in, fechou := h.Dungeons.Leave(s.CharID())
	if in == nil {
		return
	}
	s.DungeonInstance = 0
	s.DungeonID = 0
	s.DungeonDificult = 0
	// A posicao de volta fica guardada no personagem: quem cai dentro da
	// dungeon volta la fora no proximo login, nao no meio da instancia que
	// nao existe mais.
	if ValidPosition(s.ReturnPosition) &&
		(s.ReturnPosition.X != 0 || s.ReturnPosition.Y != 0) {
		s.Position = s.ReturnPosition
	} else {
		s.Position = dungeon.ExitPosition
	}
	s.ReturnPosition = model.Position{}
	if fechou {
		h.Log.Info("dungeon fechada", "instancia", in.ID, "motivo", "queda")
	}
}

// SendDungeonMobs desenha para o jogador os mobs da instancia que estao ao
// alcance, e apaga os que sairam dele.
//
// Na origem isso era o laco da TDungeonMainThread, que para cada membro
// varria as 450 posicoes do vetor de mobs a cada tick e, quando o grupo tinha
// um membro so, rodava um bloco quase identico copiado logo abaixo
// (Dungeon.pas:597-676).
func (h *Handler) SendDungeonMobs(s *Session) {
	if h.Dungeons == nil || s == nil {
		return
	}
	in, ok := h.Dungeons.Of(s.CharID())
	if !ok {
		return
	}
	ver := h.watchDistance()
	esquecer := h.forgetDistance()
	for _, m := range in.Mobs() {
		dist := s.Position.Distance(m.Pos)
		visivel := s.Base.VisibleMobs.Contains(m.ClientID)
		switch {
		case !visivel && m.Alive() && dist <= ver:
			s.Base.VisibleMobs.Add(m.ClientID)
			h.sendDungeonMobSpawn(s, in, m)
		case visivel && (dist > esquecer || !m.Alive()):
			s.Base.VisibleMobs.Remove(m.ClientID)
			h.sendDungeonMobRemove(s, m)
		}
	}
}

// sendDungeonMobSpawn manda o $35E de um mob da instancia, com os campos
// fixos que a carga da origem punha em todo mob de dungeon: elevacao 7,
// cabeca e perna 119.
func (h *Handler) sendDungeonMobSpawn(s *Session, in *dungeon.Instance, m *dungeon.Mob) {
	if m.Species == nil || h.Chan == nil {
		return
	}
	sp := m.Species
	var pkt packets.SpawnMobPacket
	pkt.Header.Index = m.ClientID
	pkt.Equip[0] = sp.Equip[0]
	pkt.Equip[1] = sp.Equip[1]
	pkt.Equip[6] = sp.Equip[6]
	pkt.Position.X = m.Pos.X
	pkt.Position.Y = m.Pos.Y
	pkt.Rotation = sp.Rotation
	pkt.CurHP = m.CurHP
	pkt.MaxHP = m.MaxHP
	pkt.CurMP = m.MaxHP
	pkt.MaxMP = m.MaxHP
	pkt.Level = sp.Level
	pkt.SpawnType = sp.SpawnType
	pkt.Altura = dungeondata.FixedElevation
	pkt.Tronco = dungeondata.FixedCabeca
	pkt.Perna = dungeondata.FixedPerna
	pkt.MobType = uint8(min(sp.MobType, 255))
	pkt.MobName = sp.IntName
	raw, err := netsrv.Marshal(CodeSpawnMob, pkt)
	if err != nil {
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
}

// sendDungeonMobRemove manda o $101 de remocao.
func (h *Handler) sendDungeonMobRemove(s *Session, m *dungeon.Mob) {
	if h.Chan == nil {
		return
	}
	var pkt packets.SendRemoveMobPacket
	pkt.Header.Index = m.ClientID
	pkt.Index = uint32(m.ClientID)
	raw, err := netsrv.Marshal(CodeRemoveMob, pkt)
	if err != nil {
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
}
