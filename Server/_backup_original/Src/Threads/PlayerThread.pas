unit PlayerThread;
{$O+}
interface
uses
  Classes, Windows, SysUtils, Winsock2, DateUtils, SyncObjs;
type
  PSocket = ^TSocket;
type
  PPlayerThreadMobsUpdate = ^TPlayerThreadMobsUpdate;
  TPlayerThreadMobsUpdate = class(TThread)
  private
    Channel: BYTE;
    fCritSect: TCriticalSection;
    function UpdateMobsForMe(): Boolean;
  protected
    procedure Execute; override;
  public
    Term: Boolean;
    ClientID: Integer;
    constructor Create(const ClientID: Integer; const Socket: TSocket;
      Channel: BYTE); virtual;
    destructor Destroy(); override;
    procedure Terminate(); overload;
  end;

type
  PPlayerThread = ^TPlayerThread;
  TPlayerThread = class(TThread)
  private
    { Private declarations }
    FDset: TFDSet;
    Channel: BYTE;
    fCritSect: TCriticalSection;
    DownNetTimes:  Integer;

    function RecvPackets(): Boolean;
    function CheckSocket: Boolean;
    function UpdateMobsForMe(): Boolean;
    // function CheckSocket2: Boolean;
  protected
    procedure Execute; override;
  public
    // Socket: PSocket;
    Term: Boolean;
    ClientID: Integer;

    constructor Create(const ClientID: Integer; const Socket: TSocket;
      Channel: BYTE); virtual;
    destructor Destroy(); override;
    procedure Terminate(); overload;
  end;
implementation
uses GlobalDefs, Log, Packets, Player, EncDec, ItemFunctions, PlayerData,
  EntityMail, BaseMob;
constructor TPlayerThread.Create(const ClientID: Integer; const Socket: TSocket;
  Channel: BYTE);
begin

  fCritSect := TCriticalSection.Create;
  Self.ClientID := ClientID;
  // Self.Socket := @Socket;
  Self.Channel := Channel;
  Self.Term := FALSE;
  Servers[Channel].Players[Self.ClientID].Create(ClientID, Channel);
  Servers[Channel].Players[Self.ClientID].Base.TimeForGoldTime := Now;
  Inc(PlayersThreads);
  Self.FreeOnTerminate := True;

  //Servers[Channel].Players[Self.ClientID].MobsThread :=
    //TPlayerThreadMobsUpdate.Create(ClientID, Socket, Channel);

  inherited Create(FALSE);
  { TTask.Run(procedure
    var
      MPlayer: PPlayer;
    begin
        //sleep(10000);
        TThread.Synchronize(Self,
        procedure
        begin
          MPlayer := @Servers[Channel].Players[Self.ClientID];
       // Inc(PlayersThreads);
          MPlayer^.LastTimeSaved := Now;
          while ((Self.CheckSocket = True) and (Self.RecvPackets = True)) do
          begin
            if (SecondsBetween(Now, MPlayer^.LastTimeSaved) >= 300) then
            begin
              if (MPlayer^.Status >= Playing) then
              begin
                MPlayer^.SaveInGame(MPlayer^.SelectedCharacterIndex);
              end;
              MPlayer^.LastTimeSaved := Now;
              TEntityMail.SendUnreadMails(MPlayer^);
            end;
            Sleep(10);
          end;
          FD_ZERO(FDset);
  // Socket := nil;
  //dec(PlayersThreads);
          shutdown(Servers[Self.Channel].Players[ClientID].Socket, SD_BOTH);
          ZeroMemory(@Servers[Self.Channel].Players[ClientID], sizeof(TPlayer));
        end);
    end);  }
  {TThread.CreateAnonymousThread(
    procedure ()
    var
      MPlayer: PPlayer;
    begin
      MPlayer := @Servers[Channel].Players[Self.ClientID];
       // Inc(PlayersThreads);
      MPlayer^.LastTimeSaved := Now;
      while ((Self.CheckSocket = True) and (Self.RecvPackets = True)) do
      begin
        if (SecondsBetween(Now, MPlayer^.LastTimeSaved) >= 300) then
        begin
          if (MPlayer^.Status >= Playing) then
          begin
            MPlayer^.SaveInGame(MPlayer^.SelectedCharacterIndex);
          end;
          MPlayer^.LastTimeSaved := Now;
          TEntityMail.SendUnreadMails(MPlayer^);
        end;
        PThr.Sleep(10);
      end;
    end); }
  //Self.FreeOnTerminate := True;
  // Self.Resume;
  // MPlayer^.LastTimeSaved := Now;
  //
end;
destructor TPlayerThread.Destroy();
begin
  fCritSect.Free;
  inherited;
end;
procedure TPlayerThread.Terminate();
begin
  if not(Self.Term) then
  begin
    Servers[Channel].Disconnect(Servers[Channel].Players[Self.ClientID]);

    Servers[Channel].Players[Self.ClientID].PlayerThreadActive := FALSE;

    Dec(PlayersThreads);
    Self.Term := True;
  end;
  ZeroMemory(@Servers[Self.Channel].Players[ClientID], sizeof(TPlayer));
  Servers[Self.Channel].Players[ClientID].xdisconnected := True;
  inherited Terminate;
end;

function TPlayerThread.UpdateMobsForMe(): Boolean;
var
  i, j: Integer;
  MyBase: PBaseMob;
begin
  Result := False;

  MyBase := @Servers[Self.Channel].Players[Self.ClientID].Base;

  for i := 0 to 449 do // 0..193
  begin
    for j := 1 to 49 do
    begin
      if (Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index = 0) then
        continue;

      if((Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index >= 3340) and
        (Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index <= 3369)) then
        Continue;

      try
        Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].UpdateSpawnToPlayers(i, j, Self.ClientID);
      except
        continue;
      end;

      try
        if(Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].CurrentPos.InRange(
          MyBase.PlayerCharacter.LastPos, 30)) then
        begin
          Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].MobMoviment(Self.ClientID);
          Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].MobHandler(Self.ClientID);
        end;
      except
        continue;
      end;
    end;
  end;



  {for i := Low(MyBase.VisibleTargets) to High(MyBase.VisibleTargets) do
  begin
    if(MyBase.VisibleTargets[i].TargetType = 0) then
      Continue;
    try
      Servers[Self.Channel].MOBS.TMobS[PBaseMob(MyBase.VisibleTargets[i].Mob).Mobid]
        .MobsP[PBaseMob(MyBase.VisibleTargets[i].Mob).SecondIndex].MobMoviment(Self.ClientID);
    except
      continue;

    end;

    try
      Servers[Self.Channel].MOBS.TMobS[PBaseMob(MyBase.VisibleTargets[i].Mob).Mobid]
        .MobsP[PBaseMob(MyBase.VisibleTargets[i].Mob).SecondIndex].MobHandler(Self.ClientID);
    except
      continue;
    end;
  end;}
end;

procedure TPlayerThread.Execute;
var
  MPlayer: PPlayer;
  Sk, Rp: Boolean;
begin
  //Priority := tpHighest;
  DownNetTimes := 0;
  MPlayer := @Servers[Channel].Players[Self.ClientID];
  MPlayer^.LastTimeSaved := Now;
  MPlayer^.xdisconnected := False;
  while (not(Self.Term) and not(MPlayer.xdisconnected)) do
  begin
    //fCritSect.Enter;
    Sleep(5);
    if(not(CheckSocket) or not(Self.RecvPackets) or(MPlayer.SocketClosed)) then
    fCritSect.Enter;

    if(not(Self.RecvPackets) or (MPlayer.SocketClosed)) then
    begin
      Servers[Channel].Disconnect(MPlayer^);
      Servers[Channel].Players[Self.ClientID].PlayerThreadActive := FALSE;
      Dec(PlayersThreads);
      Self.Term := True;
      fCritSect.Release;
      //Self.Sleep(1);
      break;
    end;

    //fCritSect.Release;
    //Self.Sleep(1);
    fCritSect.Release;
    Self.Sleep(1);
    {if not(MPlayer^.Authenticated) then
    begin
      if(SecondsBetween(Now, Mplayer^.ConnectionedTime) >= 10) then
      begin
        //Servers[Channel].Disconnect(MPlayer^);
        closesocket(MPlayer.Socket);
        Self.Term := True;
        Dec(PlayersThreads);
        //fCritSect.Release;
        break;
      end;
    end; }
    //fCritSect.Release;

   { try
      Rp := Self.RecvPackets;
    finally
      if(Rp) then
      begin
        if (SecondsBetween(Now, MPlayer^.LastTimeSaved) >= 300) then
        begin
          if (MPlayer^.Status >= Playing) then
          begin
            try
              MPlayer^.SaveInGame(MPlayer^.SelectedCharacterIndex);
            finally
              MPlayer^.LastTimeSaved := Now;
            end;
            TEntityMail.SendUnreadMails(MPlayer^);
          end;
        end;
      end;
    end;
    }

   {
    try
      try
        if (not(Self.CheckSocket) or not(Self.RecvPackets)) then
        begin
          Self.Term := True;
          break;
          //Inc(DownNetTimes);
          //Sleep(25);
          //if(DownNetTimes >= 4) then
          //begin
            //fCritSect.Leave;
            //break;
          //end;
        end
        else
        begin
          if not(MPlayer^.xdisconnected) then
          begin
            if(Assigned(MPlayer^.PlayerSQL.Query.Connection)) then
            begin
              if (SecondsBetween(Now, MPlayer^.LastTimeSaved) >= 300) then
              begin
                if (MPlayer^.Status >= Playing) then
                begin
                  try
                    MPlayer^.SaveInGame(MPlayer^.SelectedCharacterIndex);
                  finally
                    MPlayer^.LastTimeSaved := Now;
                  end;
                  TEntityMail.SendUnreadMails(MPlayer^);
                end;
              end;
            end;
          end;
          //DownNetTimes := 0;
          //MPlayer^.Base.LureMobsInRange;
        end;
      except
        sleep(1000);
        Continue;
      end;
    finally
    end;
    }
  end;
  Self.Terminate;
end;
function TPlayerThread.RecvPackets(): Boolean;
var
  RecvBuffer : array [0..21999] of BYTE;
  RecvBuffer2: array [0..21999] of BYTE;
  initialOffset: Integer;
  HeaderSize, RecvBytes: Integer;
  Header2: TPacketHeader;
begin
  Result := True;

  // servidor encerrando ou socket já marcado para fechar
  if ServerHasClosed then Exit;
  if Servers[Self.Channel].Players[ClientID].SocketClosed then Exit;

  RecvBytes := Recv(Servers[Self.Channel].Players[ClientID].Socket,
                    RecvBuffer, SizeOf(RecvBuffer), 0);

  // se Recv falhar com EWOULDBLOCK, não há dados por enquanto
  if (RecvBytes = SOCKET_ERROR) and (WSAGetLastError = WSAEWOULDBLOCK) then
    Exit;

  // desconexão/erro real
  if (RecvBytes <= 0) then
  begin
    Servers[Channel].Players[Self.ClientID].SocketClosed := True;
    Result := False;
    Exit;
  end;

  // pacote válido
  if (RecvBytes >= SizeOf(TPacketHeader)) and (RecvBytes <= SizeOf(RecvBuffer)) then
  begin
    initialOffset := 0;

    // primeiro pacote do cliente (ajuste legacy de 4 bytes no topo)
    if (Servers[Channel].Players[Self.ClientID].RecvPackets = 0) then
    begin
      if (RecvBytes > 60) then
      begin
        initialOffset := 4;
        Move(RecvBuffer[initialOffset], RecvBuffer, SizeOf(RecvBuffer));
        Servers[Channel].Players[Self.ClientID].RecvPackets := 1;
      end;
    end;

    // backup do buffer (mantido como no seu código)
    Move(RecvBuffer, RecvBuffer2, SizeOf(RecvBuffer2));

    // descriptografar antes de ler o cabeçalho
    TEncDec.Decrypt(RecvBuffer, SizeOf(RecvBuffer));

    ZeroMemory(@Header2, SizeOf(Header2));
    Move(RecvBuffer, Header2, SizeOf(Header2));
    HeaderSize := Header2.Size;

    // cliente ainda não recebeu ID
    if (Servers[Channel].Players[Self.ClientID].Base.ClientID = 0) then
      Exit;

    // processamento thread-safe
    fCritSect.Enter;
    try
      Servers[Channel].PacketControl(
        Servers[Channel].Players[Self.ClientID],
        HeaderSize,
        RecvBuffer,
        initialOffset
      );
    finally
      fCritSect.Leave;
    end;

    {  // (opcional) processamento de pacote encadeado no mesmo recv()
    if (HeaderSize < RecvBytes) then
    begin
      Move(RecvBuffer2[HeaderSize], RecvBuffer2, RecvBytes - HeaderSize);
      TEncDec.Decrypt(RecvBuffer2, RecvBytes - HeaderSize);
      ZeroMemory(@Header2, SizeOf(Header2));
      Move(RecvBuffer2, Header2, SizeOf(Header2));
      HeaderSize := Header2.Size;

      fCritSect.Enter;
      try
        Servers[Channel].PacketControl(
          Servers[Channel].Players[Self.ClientID],
          HeaderSize,
          RecvBuffer2,
          0
        );
      finally
        fCritSect.Leave;
      end;
    end;
    }
  end
  else
  begin
    // pacote fora do formato esperado: normalizar estado
    Servers[Channel].Players[Self.ClientID].RecvPackets := 1;
  end;
end;

function TPlayerThread.CheckSocket: Boolean;
begin
  Result := True;

  // verificação simples de sockets inválidos
  if (Servers[Channel].Sock = INVALID_SOCKET) or
     (Servers[Self.Channel].Players[ClientID].Socket = INVALID_SOCKET) then
  begin
    if not Servers[Channel].Players[Self.ClientID].xdisconnected then
    begin
      Dec(PlayersThreads);
      Servers[Channel].Disconnect(Servers[Channel].Players[Self.ClientID]);
    end;
    Result := False;
    Exit;
  end;

  // Se quiser reativar o select, deixe seu bloco comentado aqui.
  // FD_ZERO(FDset);
  // _FD_SET(Servers[Self.Channel].Players[ClientID].Socket, FDset);
  // _FD_SET(Servers[Channel].Sock, FDset);
  // if not FD_ISSET(Servers[Self.Channel].Players[ClientID].Socket, FDset) then
  // begin
  //   ...
  // end;
end;

{
  function TPlayerThread.CheckSocket2: Boolean;
  var
  BytesRead: Integer;
  timeout: TTimeVal;
  begin
  Result := True;
  FD_ZERO(FDset);
  _FD_SET(Self.Socket, FDset);
  _FD_SET(Servers[Channel].Sock, FDset);
  timeout.tv_usec := 10;
  // a cada 10ms ele verifica na pilha de pacotes pra ver se tem
  BytesRead := select(0, @FDset, @FDset, nil, nil);
  if (BytesRead < 0) then
  begin // ocorreu um erro na conexão / socket (closesocket)
  Servers[Channel].Disconnect(Servers[Channel].Players[Self.ClientID]);
  Result := FALSE;
  Exit;
  end;
  Self.RecvPackets;
  end;
}
{
  procedure TPlayerThread.UpdateTime;
  var
  Player: PPlayer;
  begin
  if not(Servers[Channel].Players[Self.ClientID].Base.IsActive) then
  Exit;
  Player := @Servers[Channel].Players[Self.ClientID];
  Player.Character.LoggedTime := MinutesBetween(Now, Self.LastUpdate);
  if (Player.Character.LoggedTime >= 10) then
  begin
  TItemFunctions.PutItem(Player^, 11286, 1);
  Self.LastUpdate := Now;
  end;
  end;
  {
  procedure TPlayerThread.EventListener;
  var
  Player: PPlayer;
  EvUpdate: Integer;
  Laps: Integer;
  begin
  if not(Servers[Channel].Players[Self.ClientID].Base.IsActive) then
  Exit;
  Player := @Servers[Channel].Players[Self.ClientID];
  EvUpdate := SecondsBetween(Now, Self.EventUpdate);
  if(EvUpdate >= 1) then
  begin
  Self.EventUpdate := Now;
  if not(Player.Base.EventListener) then
  Exit;
  case Player.Base.EventAction of
  1:
  begin
  Laps := SkillData[Player.Base.LaminaID].Duration;
  if(Cycles > Laps) then
  begin
  Player.Base.EventListener := False;
  Player.Base.EventAction := 0;
  Player.Base.LaminaPoints := 0;
  Player.Base.LaminaID := 0;
  Exit;
  end;
  Player.Base.AreaSkill(Player.Base.LaminaPoints, 0, @Player.Base,
  Player.Base.PlayerCharacter.LastPos);
  Inc(Cycles);
  end;
  end;
  end;
  end;
}
{ TPlayerThreadMobsUpdate }

constructor TPlayerThreadMobsUpdate.Create(const ClientID: Integer;
  const Socket: TSocket; Channel: BYTE);
begin
  fCritSect := TCriticalSection.Create;
  Self.ClientID := ClientID;
  // Self.Socket := @Socket;
  Self.Channel := Channel;
  Self.Term := FALSE;
  Self.FreeOnTerminate := True;

  inherited Create(FALSE);
end;

destructor TPlayerThreadMobsUpdate.Destroy;
begin
  fCritSect.Free;
  inherited;
end;

procedure TPlayerThreadMobsUpdate.Execute;
var
  MPlayer: PPlayer;
begin
  MPlayer := @Servers[Channel].Players[Self.ClientID];

  while not(Servers[Channel].Players[Self.ClientID].SocketClosed) do
  begin
    sleep(600);
    if(Servers[Self.Channel].Players[Self.ClientID].SocketClosed) then
        break;
    if(Servers[Self.Channel].Players[Self.ClientID].IsInstantiated) then
    begin
      //fCritSect.Enter;
      Self.UpdateMobsForMe;
      //fCritSect.Release;
    end;

  end;

end;

procedure TPlayerThreadMobsUpdate.Terminate;
begin
  inherited;
end;

function TPlayerThreadMobsUpdate.UpdateMobsForMe: Boolean;
var
  i, j: Integer;
  MyBase: PBaseMob;
begin
  Result := False;

  MyBase := @Servers[Self.Channel].Players[Self.ClientID].Base;

  for i := 0 to 449 do // 0..193
  begin
    if(Servers[Self.Channel].MOBS.TMobS[i].IntName = 0) then
      Continue;
    if(Servers[Self.Channel].Players[Self.ClientID].SocketClosed) then
        Continue;

    for j := 1 to 49 do
    begin
      if (Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index = 0) then
        continue;

      if((Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index >= 3340) and
        (Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].Index <= 3369)) then
        Continue;

      if not(Servers[Self.Channel].Players[Self.ClientID].IsInstantiated) then
        Continue;
      try
        Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].UpdateSpawnToPlayers(i, j, Self.ClientID);
      except
        continue;
      end;

      if( not(Servers[Self.Channel].Players[Self.ClientID].Status < Playing) and not(MyBase.BuffExistsByIndex(77)) and
        not MyBase.BuffExistsByIndex(53) and not MyBase.BuffExistsByIndex(153)) then
      begin
        if not(MyBase.IsDead) then
        begin
          try
            MyBase.LureMobsInRange;
          except
            Continue;
          end;
        end;
      end;

      try
        if(Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].CurrentPos.InRange(
          MyBase.PlayerCharacter.LastPos, 30)) then
        begin
          //Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].MobMoviment(Self.ClientID);
          Servers[Self.Channel].MOBS.TMobS[i].MobsP[j].MobHandler(Self.ClientID);
        end;
      except
        continue;
      end;
    end;
  end;

  if (SecondsBetween(Now, MyBase.LastTimeGarbaged) >= 10) then
  begin
    MyBase.LastTimeGarbaged := Now;
    MyBase.TargetGarbageService(); //to update the list, handle memory and update position
  end;

end;

end.
