package game

import (
	"context"
	"errors"
	"math"

	"aika/internal/entity"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// CodeMovement e o $301, usado nos dois sentidos: o cliente manda o destino e
// o servidor repassa aos visiveis.
const CodeMovement = 0x301

// MoveNormal e o MOVE_NORMAL do original.
const MoveNormal = 0

// VisibilityThreshold e a distancia que dispara a atualizacao da lista de
// visiveis. No original era "Distance(Character.LastPos) >= 3".
const VisibilityThreshold = 3

// MapLimit e o maior valor de coordenada aceito.
//
// O original nao validava o destino: a checagem de IsValid e toda a verificacao
// de velocidade estao dentro de um bloco comentado em MovementCommand, entao o
// cliente podia mandar qualquer coordenada, inclusive infinito ou NaN, e ela
// ia direto para LastPos e dali para as contas de distancia.
const MapLimit = 40000

// Erros de movimento.
var (
	ErrNotPlaying      = errors.New("game: sessao nao esta em jogo")
	ErrInvalidPosition = errors.New("game: posicao invalida")
	ErrDeadCannotMove  = errors.New("game: personagem morto nao se move")
)

// ValidPosition diz se a coordenada pode entrar no estado do jogo.
//
// Recusa infinito e NaN, que e o que TPosition.IsValid testava antes de a
// chamada ser comentada, e tambem valor fora do mapa.
func ValidPosition(p model.Position) bool {
	for _, v := range [2]float32{p.X, p.Y} {
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
		if f < 0 || f > MapLimit {
			return false
		}
	}
	return true
}

// movement e o porte de TPacketHandlers.MovementCommand ($301).
func (h *Handler) movement(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		// O original chamava o handler apenas "if (Player.IsInstantiated)",
		// que nao garante que o jogador chegou a entrar no mundo.
		return ErrNotPlaying
	}
	if s.Dead {
		return nil // "if(Player.Base.IsDead) then Exit"
	}

	var pkt packets.MovementPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	dest := model.Position{X: pkt.Destination.X, Y: pkt.Destination.Y}
	if !ValidPosition(dest) {
		h.log().Warn("movimento com posicao invalida",
			"usuario", s.Username, "x", dest.X, "y", dest.Y, "ip", c.IP())
		// Devolve o jogador para onde ele estava, em vez de aceitar a
		// coordenada: aceitar NaN contamina todas as contas de distancia.
		return h.resync(s)
	}

	// Salto maior que o alcance de visao nao e andar, e teleporte: o destino
	// esta fora do que o cliente podia nem desenhar. O original tinha uma
	// checagem de velocidade bem mais fina, com desconexao, mas ela esta
	// inteira dentro de um bloco comentado em MovementCommand — nunca rodou.
	// Aqui o salto absurdo e barrado e o jogador volta para onde estava, sem
	// desconectar: o limite nao foi calibrado contra lag de verdade, e
	// derrubar quem tem conexao ruim seria pior que o problema.
	if s.Position != (model.Position{}) && s.Position.Distance(dest) > h.forgetDistance() {
		h.log().Warn("salto de posicao recusado",
			"usuario", s.Username, "de_x", s.Position.X, "de_y", s.Position.Y,
			"para_x", dest.X, "para_y", dest.Y, "ip", c.IP())
		return h.resync(s)
	}

	s.Position = dest
	s.LastMoved = h.now()

	// O cabecalho vai com o id do autor, nao com o que veio do cliente.
	//
	// O original repassava o pacote do jeito que chegou
	// ("SendToVisible(Packet, Packet.Header.Size, False)"), e Header.Index e
	// escrito pelo cliente: bastava mandar o id de outro jogador para fazer o
	// boneco dele andar na tela de todos os visiveis.
	pkt.Header.Index = s.ClientID

	// Repassa o movimento aos visiveis sem reenviar ao proprio autor, que ja
	// desenhou o passo localmente.
	raw, err := netsrv.Marshal(CodeMovement, pkt)
	if err != nil {
		return err
	}
	if h.Chan != nil {
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}

	// A lista de visiveis so e recalculada a cada 3 celulas andadas, como no
	// original: refazer a cada passo custaria uma varredura por pacote.
	if s.Position.Distance(s.AnchorPosition) >= VisibilityThreshold {
		s.AnchorPosition = s.Position
		h.refreshVisible(s)
		// Sair de perto fecha o NPC aberto, como no original.
		s.OpenedNPC = 0
		s.OpenedOption = 0
	}
	return nil
}

// resync manda o cliente de volta a ultima posicao valida.
func (h *Handler) resync(s *Session) error {
	if h.Chan == nil {
		return nil
	}
	var pkt packets.MovementPacket
	pkt.Header.Index = s.ClientID
	pkt.Destination.X = s.Position.X
	pkt.Destination.Y = s.Position.Y
	pkt.MoveType = MoveNormal
	// Speed e BYTE no pacote; SpeedMove e WORD no personagem.
	pkt.Speed = uint8(min(s.SpeedMove, 255))
	raw, err := netsrv.Marshal(CodeMovement, pkt)
	if err != nil {
		return err
	}
	return h.Chan.SendTo(s.ClientID, raw)
}

// refreshVisible recalcula quem esta a vista e manda os pacotes de spawn e
// de remocao que a mudanca exige.
//
// Porte da parte de TBaseMob.UpdateVisibleList que trata jogadores. O original
// varria o vetor de 1000 posicoes a cada chamada; aqui a varredura e sobre
// quem esta em jogo.
//
// Diferenca deliberada: entra em visivel a DISTANCE_TO_WATCH e so sai acima
// de DISTANCE_TO_FORGET. O original tinha as duas constantes e usava so a
// primeira nos dois sentidos, porque o trecho que usava a segunda esta
// comentado; com um limite so, quem andava em cima da borda entrava e saia da
// lista a cada passo e levava um spawn e um remove por passo. Os dois limites
// dao a folga que evita isso, que e para o que a segunda constante existe.
func (h *Handler) refreshVisible(s *Session) {
	if h.Chan == nil {
		return
	}
	ver := h.watchDistance()
	esquecer := h.forgetDistance()

	for _, id := range h.Chan.PlayingIDs() {
		if id == s.ClientID {
			continue
		}
		outro, _, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		dist := s.Position.Distance(outro.Position)
		jaVisivel := s.Base.VisiblePlayers.Contains(id)

		switch {
		case !jaVisivel && dist <= ver:
			h.spawnPair(s, outro)
		case jaVisivel && dist > esquecer:
			h.despawnPair(s, outro)
		}
	}
}

// watchDistance e o DISTANCE_TO_WATCH do .ini, com o padrao do original.
func (h *Handler) watchDistance() uint16 {
	if h.WatchDistance > 0 {
		return h.WatchDistance
	}
	return DefaultWatchDistance
}

// forgetDistance e o DISTANCE_TO_FORGET do .ini. Nunca menor que o de ver:
// com forget abaixo de watch a entidade sairia da lista no mesmo passo em que
// entrasse, e o cliente receberia spawn e remove sem parar.
func (h *Handler) forgetDistance() uint16 {
	d := h.ForgetDistance
	if d == 0 {
		d = DefaultForgetDistance
	}
	if v := h.watchDistance(); d < v {
		return v
	}
	return d
}

// Distancias padrao, os valores de GlobalDefs.pas.
const (
	DefaultWatchDistance  = 50 // DISTANCE_TO_WATCH
	DefaultForgetDistance = 60 // DISTANCE_TO_FORGET
)

// assertPlayerID garante que um id vindo de lista cabe na faixa de jogador
// antes de ser usado. Existe para o porte nao repetir o
// "if (i <= MAX_CONNECTIONS)" sem limite de baixo do original.
func assertPlayerID(id uint16, maxConns uint16) bool {
	return entity.IsPlayerID(id, maxConns)
}
