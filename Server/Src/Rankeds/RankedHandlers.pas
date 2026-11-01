unit RankedHandlers;

interface

uses
  Windows,
  SysUtils, Math,
  Player, ServerSocket, Log, Packets, MiscData, PacketHandlers;

type
  TRankedMatch = record
    MatchID: Integer;
    Server1: TServerSocket;
    Server2: TServerSocket;
    P1ID: Word;
    P2ID: Word;
    OriginalChannelP1: Integer;
    OriginalChannelP2: Integer;
    DeathCountP1: Integer;
    DeathCountP2: Integer;
  end;

  TRankedSystem = class
  public
    class procedure StartMatch(Server: TServerSocket; P1ID, P2ID, MatchID: Integer);
    class procedure GoMatch(MatchID: Integer);
    class procedure RegisterDeath(Server: TServerSocket; DeadID, KillerID: Integer);
    class procedure FinishMatch(Server: TServerSocket; MatchID: Integer);
    class procedure TeleportBack(Server: TServerSocket; var Player: TPlayer; OriginalChannel: Integer);
  end;

procedure SendChannelChange(var Player: TPlayer; Channel: Integer);

implementation

uses
  GlobalDefs, PlayerData;

var
  RankedMatches: array of TRankedMatch;

// ==================================================================
// PROCURA JOGADOR POR NOME (GLOBAL)
// ==================================================================
function FindPlayerByNameGlobal(const Name: string;
  out Server: TServerSocket; out Player: PPlayer): Boolean;
var
  s, p: Integer;
begin
  Result := False;
  Server := nil;
  Player := nil;

  for s := Low(Servers) to High(Servers) do
  begin
    for p := 1 to MAX_CONNECTIONS do
    begin
      if (Servers[s].Players[p].Status = TPlayerStatus.Playing) then
      begin
        if SameText(Servers[s].Players[p].Character.Base.Name, Name) then
        begin
          Server := Servers[s];
          Player := @Servers[s].Players[p];
          Result := True;
          Exit;
        end;
      end;
    end;
  end;
end;

function FindMatchOfPlayer(Server: TServerSocket; DeadSlot: Integer): Integer;
var
  ClientID: Integer;
  I: Integer;
begin
  Result := -1;

  // Converte DeadSlot → ClientID real
  ClientID := Server.Players[DeadSlot].Base.ClientID;
  if ClientID = 0 then Exit;

  // Procura o match
  for I := 0 to High(RankedMatches) do
    if (RankedMatches[I].P1ID = ClientID) or
       (RankedMatches[I].P2ID = ClientID) then
    begin
      Result := RankedMatches[I].MatchID;
      Exit;
    end;
end;


// PROCURA PLAYER POR CLIENTID (GLOBAL)
function FindPlayerGlobal(ClientID: Integer; out Server: TServerSocket; out Player: PPlayer): Boolean;
var
  s, p: Integer;
begin
  Result := False;
  Server := nil;
  Player := nil;

  for s := Low(Servers) to High(Servers) do
  begin
    for p := 1 to MAX_CONNECTIONS do
    begin
      if Servers[s].Players[p].Status >= TPlayerStatus.Playing then
      begin
        if Servers[s].Players[p].Base.ClientID = ClientID then
        begin
          Server := Servers[s];
          Player := @Servers[s].Players[p];
          Exit(True);
        end;
      end;
    end;
  end;
end;

procedure DeleteMatch(Index: Integer);
var
  I: Integer;
begin
  if (Index < 0) or (Index > High(RankedMatches)) then
    Exit;

  for I := Index to High(RankedMatches) - 1 do
    RankedMatches[I] := RankedMatches[I + 1];

  SetLength(RankedMatches, Length(RankedMatches) - 1);
end;

procedure SendRankedEventToWeb(const Msg: string);
begin
  Logger.Write('[RANKED_WEB] ' + Msg, TLogType.Painel);
end;

// ==================================================================
// ENVIA PACOTE DE TROCA DE CANAL PARA O CLIENTE (VISUAL)
// ==================================================================
procedure SendChannelChange(var Player: TPlayer; Channel: Integer);
var
  P: TChangeChannelPacket; // do Packets.pas (0xF05)
begin
  FillChar(P, SizeOf(P), 0);
  P.Header.Size  := SizeOf(P);
  P.Header.Index := Player.Base.ClientId;
  P.Header.Code  := $F05;
  P.Info1        := 0;
  P.TypeChanel   := Channel;
  Player.SendPacket(P, SizeOf(P));
end;

// ==================================================================
// FORÇA TROCA DE CANAL USANDO O HANDLER ORIGINAL DO AIKA
// ==================================================================
procedure ForceChangeChannel(var Player: TPlayer; NewChannel: Integer);
var
  P: TChangeChannelPacket; // mesmo struct 0xF05 do Packets.pas
  Buffer: array[0..SizeOf(TChangeChannelPacket)-1] of Byte;
begin
  FillChar(P, SizeOf(P), 0);

  P.Header.Size  := SizeOf(P);
  P.Header.Index := Player.Base.ClientID;
  P.Header.Code  := $F05;
  // se quiser, pode preencher Time também:
  // P.Header.Time := GetTickCount;

  P.Info1      := 0;           // normalmente ignorado no handler
  P.TypeChanel := NewChannel;  // canal de destino

  // Copia struct para um buffer de bytes e chama o handler oficial
  Move(P, Buffer[0], SizeOf(P));
  TPacketHandlers.ChangeChannel(Player, Buffer);
end;

// ==================================================================
// STARTMATCH
// ==================================================================
class procedure TRankedSystem.StartMatch(Server: TServerSocket; P1ID, P2ID, MatchID: Integer);
var
  M: TRankedMatch;
  Pl1, Pl2: PPlayer;
  Server1, Server2: TServerSocket;
begin
  // Buscar jogadores em qualquer servidor
  if not FindPlayerGlobal(P1ID, Server1, Pl1) then
  begin
    Logger.Write('[RANKED] P1 não encontrado.', TLogType.Painel);
    Exit;
  end;

  if not FindPlayerGlobal(P2ID, Server2, Pl2) then
  begin
    Logger.Write('[RANKED] P2 não encontrado.', TLogType.Painel);
    Exit;
  end;

  // Registrar Match
  M.MatchID := MatchID;
  M.Server1 := Server1;
  M.Server2 := Server2;
  M.P1ID := Pl1.Base.ClientID;
  M.P2ID := Pl2.Base.ClientID;
  M.OriginalChannelP1 := Pl1^.ChannelIndex;
  M.OriginalChannelP2 := Pl2^.ChannelIndex;
  M.DeathCountP1 := 0;
  M.DeathCountP2 := 0;

  SetLength(RankedMatches, Length(RankedMatches) + 1);
  RankedMatches[High(RankedMatches)] := M;

  // *** ALTERAR CANAL PARA 1 USANDO O SISTEMA OFICIAL ***
  if Pl1^.ChannelIndex <> 1 then
    ForceChangeChannel(Pl1^, 1);

  if Pl2^.ChannelIndex <> 1 then
    ForceChangeChannel(Pl2^, 1);

  // TELEPORTAR PARA A ARENA (no mesmo canal atual do player)
  Pl1^.Teleport(TPosition.Create(2435, 3600));
  Pl2^.Teleport(TPosition.Create(2435, 3825));

  SendRankedEventToWeb(Format('RANKED_EVENT %d %d TP_DONE', [MatchID, M.P1ID]));
  SendRankedEventToWeb(Format('RANKED_EVENT %d %d TP_DONE', [MatchID, M.P2ID]));
end;

class procedure TRankedSystem.GoMatch(MatchID: Integer);
begin
  SendRankedEventToWeb(Format('RANKED_EVENT %d START', [MatchID]));
end;

class procedure TRankedSystem.RegisterDeath(Server: TServerSocket; DeadID, KillerID: Integer);
var
  MatchID, I: Integer;
  M: TRankedMatch;
  DeadPlayer: PPlayer;
begin
  MatchID := FindMatchOfPlayer(Server, DeadID);
  if MatchID = -1 then Exit;

  for I := 0 to High(RankedMatches) do
    if RankedMatches[I].MatchID = MatchID then
    begin
      M := RankedMatches[I];
      DeadPlayer := @Server.Players[DeadID];
      if DeadPlayer.Base.ClientID = M.P1ID then Inc(M.DeathCountP1)
      else if DeadPlayer.Base.ClientID = M.P2ID then Inc(M.DeathCountP2);


      RankedMatches[I] := M;

      if (DeadID = M.P1ID) and (M.DeathCountP1 = 1) then
      begin
        DeadPlayer^.Teleport(TPosition.Create(2435, 3600));
        Exit;
      end;

      if (DeadID = M.P2ID) and (M.DeathCountP2 = 1) then
      begin
        DeadPlayer^.Teleport(TPosition.Create(2435, 3825));
        Exit;
      end;

      if (DeadID = M.P1ID) and (M.DeathCountP1 >= 2) then
      begin
        SendRankedEventToWeb(Format('RANKED_EVENT %d %d LOSE', [MatchID, DeadID]));
        SendRankedEventToWeb(Format('RANKED_EVENT %d %d WIN', [MatchID, M.P2ID]));
        FinishMatch(Server, MatchID);
        Exit;
      end;

      if (DeadID = M.P2ID) and (M.DeathCountP2 >= 2) then
      begin
        SendRankedEventToWeb(Format('RANKED_EVENT %d %d LOSE', [MatchID, DeadID]));
        SendRankedEventToWeb(Format('RANKED_EVENT %d %d WIN', [MatchID, M.P1ID]));
        FinishMatch(Server, MatchID);
        Exit;
      end;

      Exit;
    end;
end;

class procedure TRankedSystem.FinishMatch(Server: TServerSocket; MatchID: Integer);
var
  I: Integer;
  M: TRankedMatch;
  Pl1, Pl2: PPlayer;
begin
  for I := 0 to High(RankedMatches) do
    if RankedMatches[I].MatchID = MatchID then
    begin
      M := RankedMatches[I];

      // Jogador 1
      if FindPlayerGlobal(M.P1ID, M.Server1, Pl1) then
      begin
        TeleportBack(M.Server1, Pl1^, M.OriginalChannelP1);
      end;

      // Jogador 2
      if FindPlayerGlobal(M.P2ID, M.Server2, Pl2) then
      begin
        TeleportBack(M.Server2, Pl2^, M.OriginalChannelP2);
      end;

      SendRankedEventToWeb(Format('RANKED_EVENT %d END', [MatchID]));
      DeleteMatch(I);
      Exit;
    end;
end;

class procedure TRankedSystem.TeleportBack(Server: TServerSocket; var Player: TPlayer; OriginalChannel: Integer);
begin
  if Player.ChannelIndex <> OriginalChannel then
    SendChannelChange(Player, OriginalChannel);

  // volta para área inicial da nação
  Player.Teleport(TPosition.Create(3507, 543));
end;

end.

