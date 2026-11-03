package game

import (
	"context"
	"strings"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de conversa e de informacao.
const (
	CodeClientSay       = 0xF86  // cliente -> servidor: fala
	CodeUpdateRotation  = 0x305  // nos dois sentidos: giro do boneco
	CodeRequestCharInfo = 0x22C  // cliente -> servidor: ver outro jogador
	CodeCharInfo        = 0x19E  // servidor -> cliente: resposta
	CodePingReply       = 0xF93B // servidor -> cliente: resposta do `ping
	CodeRequestTime     = 0x202  // cliente -> servidor: hora do servidor
)

// Tipos de fala, as constantes CHAT_TYPE_* de GlobalDefs.pas.
const (
	ChatNormal   = 0 // ao redor
	ChatWhisper  = 1 // reservado
	ChatParty    = 2
	ChatGuild    = 3
	ChatShout    = 4 // grito
	ChatAlly     = 5
	ChatNation   = 6
	ChatMegafone = 9
)

// ShoutCooldown e o intervalo minimo entre dois gritos. Na origem o teste era
// "SecondsBetween(Now, Player.ShoutTime) < 5".
const ShoutCooldown = 5 * time.Second

// MaxChatRunes limita o texto que o servidor repassa.
//
// O campo Fala e ARRAY [0..127] OF AnsiChar e a origem copiava o que o
// cliente mandasse sem olhar: texto sem terminador fazia a leitura seguir
// pelos campos vizinhos do pacote. Aqui o texto e lido com limite e
// reescrito com terminador antes de ser repassado.
const MaxChatRunes = 127

// clientSay e o porte de TPacketHandlers.SendClientSay ($F86).
func (h *Handler) clientSay(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.ChatPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	fala := strings.TrimRight(netsrv.CString(pkt.Fala[:]), " ")
	if fala == "" {
		return nil
	}

	// O comando `ping nao e fala: a origem respondia o $F93B e saia.
	if fala == "`ping" {
		s.LastPing = h.now()
		var resp packets.SignalData
		resp.Header.Index = s.ClientID
		return c.SendStruct(CodePingReply, resp)
	}

	// O cabecalho e o apelido vao com os dados do autor, nao com o que veio
	// do cliente: a origem repassava o pacote como chegou, e tanto
	// Header.Index quanto Nick sao escritos pelo cliente — dava para falar no
	// nome de outra pessoa.
	pkt.Header.Index = s.ClientID
	netsrv.PutCString(pkt.Nick[:], s.Avatar.Name)
	netsrv.PutCString(pkt.Fala[:], limitarTexto(fala, MaxChatRunes))

	switch pkt.TypeChat {
	case ChatNormal:
		return h.sayToVisible(s, pkt)
	case ChatShout:
		return h.shout(c, s, pkt)
	case ChatParty:
		return h.partyChat(c, s, pkt)
	case ChatGuild:
		return h.guildChat(c, s, pkt)
	case ChatWhisper, ChatAlly, ChatNation, ChatMegafone:
		// Grupo, guilda, alianca e nacao dependem de sistemas que ainda nao
		// estao portados; ver GoServer/PORTE.md. Avisar e melhor que o
		// silencio da origem, em que o jogador digitava e nada acontecia.
		h.message(c, "Este canal de conversa ainda nao esta disponivel.")
		return nil
	default:
		// A origem registrava o tipo desconhecido e saia; mantido.
		h.log().Debug("tipo de fala desconhecido",
			"tipo", pkt.TypeChat, "usuario", s.Username)
		return nil
	}
}

// sayToVisible manda a fala a quem esta a vista, incluindo o autor.
//
// Porte de "Player.Base.SendToVisible(Packet, Size)", cujo sendToSelf e True
// por omissao: o cliente nao desenha a propria fala sozinho.
func (h *Handler) sayToVisible(s *Session, pkt packets.ChatPacket) error {
	raw, err := netsrv.Marshal(CodeClientSay, pkt)
	if err != nil {
		return err
	}
	if h.Chan == nil {
		return nil
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	return nil
}

// shout manda o grito ao canal inteiro, com o limite de tempo da origem.
func (h *Handler) shout(c *netsrv.Conn, s *Session, pkt packets.ChatPacket) error {
	agora := h.now()
	if !s.LastShout.IsZero() && agora.Sub(s.LastShout) < ShoutCooldown {
		h.message(c, "Voce nao pode floodar o grito.")
		return nil
	}
	// O instante e marcado antes do envio: na origem o ShoutTime so era
	// gravado depois do laco de envio, e uma falha no meio dele deixava o
	// jogador sem limite nenhum.
	s.LastShout = agora

	raw, err := netsrv.Marshal(CodeClientSay, pkt)
	if err != nil {
		return err
	}
	if h.Chan == nil {
		return nil
	}

	// A origem mandava o grito so para a mesma nacao quando o autor tem
	// nacao, e para todos quando nao tem. Mantido.
	nacao := s.Avatar.Nation
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		if nacao > 0 && outro.Avatar.Nation > 0 && outro.Avatar.Nation != nacao {
			continue
		}
		_ = h.Chan.SendTo(id, raw)
	}
	h.log().Info("grito", "usuario", s.Username,
		"personagem", s.Avatar.Name, "texto", netsrv.CString(pkt.Fala[:]))
	return nil
}

// updateRotation e o porte de TPacketHandlers.UpdateRotation ($305).
func (h *Handler) updateRotation(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if s.Avatar.Rotation == pkt.Data {
		// Giro repetido nao vira pacote, como na origem.
		return nil
	}
	s.Avatar.Rotation = pkt.Data

	// Mesmo motivo do movimento: o cabecalho vai com o id do autor.
	pkt.Header.Index = s.ClientID
	raw, err := netsrv.Marshal(CodeUpdateRotation, pkt)
	if err != nil {
		return err
	}
	if h.Chan != nil {
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
	return nil
}

// requestCharInfo e o porte de TPacketHandlers.RequestCharInfo ($22C).
//
// A checagem da origem era "if (Packet.Index >= 0) or (Packet.Index <= MAX_CONNECTIONS)":
// com Index sendo DWORD, o primeiro teste e sempre verdadeiro e o "or" deixa
// a condicao sempre verdadeira. Logo abaixo, o ramo do indice fazia
// "@Servers[ChannelIndex].Players[Packet.Index]" com Index ate 4 bilhoes, num
// vetor de 1000 posicoes.
func (h *Handler) requestCharInfo(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil {
		return nil
	}

	var pkt packets.RequestCharInfoPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	var alvoID uint16
	if pkt.Index == 0 {
		nome := strings.TrimSpace(netsrv.CString(pkt.Nick[:]))
		if nome == "" {
			return nil
		}
		id, ok := h.Chan.FindByCharacter(nome)
		if !ok {
			h.message(c, "Alvo nao esta logado.")
			return nil
		}
		alvoID = id
	} else {
		if pkt.Index > uint32(h.Chan.MaxConns()) {
			h.log().Warn("pedido de info com indice fora da faixa",
				"indice", pkt.Index, "usuario", s.Username, "ip", c.IP())
			return nil
		}
		alvoID = uint16(pkt.Index)
	}

	alvo, _, ok := h.Chan.Lookup(alvoID)
	if !ok || alvo.Status < Playing {
		h.message(c, "Alvo nao esta logado.")
		return nil
	}
	return c.SendStruct(CodeCharInfo, h.buildCharInfo(alvo))
}

// buildCharInfo monta o $19E. Porte de TPlayer.CharInfoResponse.
func (h *Handler) buildCharInfo(alvo *Session) packets.CharInfoResponsePacket {
	var pkt packets.CharInfoResponsePacket
	a := &alvo.Avatar

	pkt.Header.Index = alvo.ClientID
	netsrv.PutCString(pkt.Nick[:], a.Name)
	if a.GuildIndex > 0 && a.GuildName != "" {
		netsrv.PutCString(pkt.GuildName[:], a.GuildName)
	} else {
		netsrv.PutCString(pkt.GuildName[:], "Nenhuma")
	}

	pkt.Classe = uint8(min(a.ClassInfo, 255))
	pkt.Nacao = uint8(min(a.Nation, 255))
	pkt.Infamia = uint16(min(a.Infamia, 65535))
	pkt.Honra = a.Honor
	pkt.Pvp = a.KillPoint
	pkt.Str = a.Str
	pkt.AGI = a.Agility
	pkt.Int = a.Int
	pkt.Cons = a.Cons
	pkt.Sorte = a.Luck
	pkt.STATUS = a.StatusPoints
	pkt.Movimento = uint32(a.SpeedMove)
	pkt.Exp = uint64(max(a.Exp, 0))

	// "Level - 1" num WORD: com nivel 0 a origem mandava 65535. Ali havia um
	// "if not(Level = 0)" que evitava isso neste pacote — um dos poucos
	// lugares em que o piso existia.
	if a.Level > 0 {
		pkt.Level = a.Level - 1
	}

	// Os 9 primeiros slots de equipamento vao inteiros, e os slots 11 a 15
	// vao no segundo vetor, como na origem.
	for i := range pkt.Equips {
		pkt.Equips[i] = alvo.Equip.Get(i)
	}
	for i := range pkt.Equips2 {
		pkt.Equips2[i] = alvo.Equip.Get(11 + i)
	}

	// Montaria: a origem copiava campo por campo do slot 9 quando ele tem
	// item, e punha a aparencia igual ao indice.
	if mont := alvo.Equip.Get(9); !mont.IsEmpty() {
		pkt.MountEquip.Index = mont.Index
		pkt.MountEquip.APP = mont.Index
		pkt.MountEquip.Identific = mont.Identific
		pkt.MountEquip.Slot1 = uint16(mont.Effects.Index[0])
		pkt.MountEquip.Slot2 = uint16(mont.Effects.Index[1])
		pkt.MountEquip.Slot3 = uint16(mont.Effects.Index[2])
		pkt.MountEquip.Enc1 = mont.Effects.Value[0]
		pkt.MountEquip.Enc2 = mont.Effects.Value[1]
		pkt.MountEquip.Enc3 = mont.Effects.Value[2]
		pkt.MountEquip.MIN = mont.Min
		pkt.MountEquip.Time = mont.Time
	}
	return pkt
}

// requestServerTime e o porte de TPacketHandlers.RequestServerTime ($202).
func (h *Handler) requestServerTime(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SignalData
	pkt.Header.Index = s.ClientID
	pkt.Data = uint32(h.now().Unix())
	return c.SendStruct(CodeRequestTime, pkt)
}

// limitarTexto corta o texto no numero de runas, sem partir uma no meio.
func limitarTexto(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
