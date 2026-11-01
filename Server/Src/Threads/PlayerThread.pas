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

const
  // maior pacote aceito (mesmo tamanho do buffer que os handlers recebem)
  MAX_RECV_PACKET_SIZE = 22000;
  // bytes recebidos que ainda nao formam um pacote completo
  RECV_PENDING_SIZE = 65536;

type
  PPlayerThread = ^TPlayerThread;
  TPlayerThread = class(TThread)
  private
    { Private declarations }
    FDset: TFDSet;
    Channel: BYTE;
    fCritSect: TCriticalSection;
    DownNetTimes:  Integer;
    // O TCP entrega um fluxo de bytes: um recv() pode trazer varios pacotes
    // ou so parte de um. FPending guarda o que sobrou ate o pacote completar.
    FPending: array [0 .. RECV_PENDING_SIZE - 1] of BYTE;
    FPendingLen: Integer;
    FHelloChecked: Boolean;

    function RecvPackets(): Boolean;
    function DispatchPackets(): Boolean;
    function IsHello(): Boolean;
    function WaitForData(TimeoutMs: Integer): Boolean;
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
var
  Sock: TSocket;
begin
  if not(Self.Term) then
  begin
    Sock := Servers[Channel].Players[Self.ClientID].Socket;
    Servers[Channel].Disconnect(Servers[Channel].Players[Self.ClientID]);

    // Disconnect so fecha o socket de quem chegou a logar; conexoes que
    // caem antes disso (login falho, scanner de porta) vazavam o handle.
    if not(Servers[Channel].Players[Self.ClientID].xdisconnected) and
      (Sock <> 0) and (Sock <> INVALID_SOCKET) then
      closesocket(Sock);

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
    // Antes RecvPackets era chamado duas vezes por volta (o primeiro resultado
    // era descartado) e fCritSect era liberado sem ter sido adquirido.
    // A limpeza da conexao acontece uma unica vez, em Self.Terminate.
    if not(Self.CheckSocket) or MPlayer.SocketClosed then
      break;

    if not(Self.WaitForData(5)) then
      Continue;

    if not(Self.RecvPackets) or MPlayer.SocketClosed then
      break;
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
function TPlayerThread.WaitForData(TimeoutMs: Integer): Boolean;
var
  ReadSet: TFDSet;
  Timeout: TTimeVal;
begin
  // espera ate chegar dado no socket (em vez de girar com Sleep e recv vazio)
  FD_ZERO(ReadSet);
  _FD_SET(Servers[Self.Channel].Players[ClientID].Socket, ReadSet);
  Timeout.tv_sec := 0;
  Timeout.tv_usec := TimeoutMs * 1000;
  // erro tambem retorna True: o recv seguinte detecta e encerra a conexao
  Result := (select(0, @ReadSet, nil, nil, @Timeout) <> 0);
end;

function TPlayerThread.IsHello(): Boolean;
var
  Size0, Size4: Integer;
begin
  // Logo ao conectar o cliente manda 4 bytes de "hello" (?? ?? 11 1F),
  // as vezes junto do primeiro pacote, as vezes sozinho.
  if (FPending[0] = $11) and (FPending[1] = $F3) then
    Exit(True);
  if (FPending[2] = $11) and (FPending[3] = $1F) then
    Exit(True);
  if (FPendingLen = 4) then
    Exit(True);

  // tamanho no offset 0 nao faz sentido, mas um pacote valido comeca no 4
  Size0 := PWord(@FPending[0])^;
  if (Size0 >= SizeOf(TPacketHeader)) and (Size0 <= FPendingLen) then
    Exit(False);
  if (FPendingLen < 4 + SizeOf(TPacketHeader)) then
    Exit(False);
  Size4 := PWord(@FPending[4])^;
  Result := (Size4 >= SizeOf(TPacketHeader)) and (Size4 <= FPendingLen - 4);
end;

function TPlayerThread.RecvPackets(): Boolean;
var
  RecvBytes: Integer;
begin
  Result := True;

  // servidor encerrando ou socket já marcado para fechar
  if ServerHasClosed then Exit;
  if Servers[Self.Channel].Players[ClientID].SocketClosed then Exit;

  RecvBytes := Recv(Servers[Self.Channel].Players[ClientID].Socket,
                    FPending[FPendingLen], SizeOf(FPending) - FPendingLen, 0);

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

  Inc(FPendingLen, RecvBytes);
  Result := Self.DispatchPackets;
end;

function TPlayerThread.DispatchPackets(): Boolean;
var
  Packet: array [0 .. MAX_RECV_PACKET_SIZE - 1] of BYTE;
  Offset, PacketSize: Integer;
begin
  Result := True;
  Offset := 0;

  if not(FHelloChecked) then
  begin
    if (FPendingLen < 4) then
      Exit;
    FHelloChecked := True;
    if Self.IsHello then
      Offset := 4;
    Servers[Channel].Players[Self.ClientID].RecvPackets := 1;
  end;

  // Antes só o primeiro pacote de cada recv() era tratado (o resto era
  // descartado) e pacotes partidos eram processados pela metade. Isso
  // dessincronizava cliente e servidor e terminava em disconnect.
  while ((FPendingLen - Offset) >= SizeOf(TPacketHeader)) do
  begin
    PacketSize := PWord(@FPending[Offset])^;

    if (PacketSize < SizeOf(TPacketHeader)) or
      (PacketSize > MAX_RECV_PACKET_SIZE) then
    begin
      Logger.Write('[' + Servers[Channel].Players[Self.ClientID].Ip +
        '] pacote com tamanho invalido (' + IntToStr(PacketSize) +
        '), descartando ' + IntToStr(FPendingLen - Offset) + ' bytes.',
        TLogType.Warnings);
      Offset := FPendingLen;
      Break;
    end;

    // pacote ainda incompleto: espera o resto no próximo recv()
    if ((FPendingLen - Offset) < PacketSize) then
      Break;

    Move(FPending[Offset], Packet[0], PacketSize);
    // handlers leem structs de tamanho fixo: o resto do buffer vai zerado
    if (PacketSize < MAX_RECV_PACKET_SIZE) then
      ZeroMemory(@Packet[PacketSize], SizeOf(Packet) - PacketSize);
    Inc(Offset, PacketSize);

    // descriptografa só os bytes deste pacote (cada um tem sua própria chave)
    TEncDec.Decrypt(Packet, PacketSize);

    // cliente ainda não recebeu ID
    if (Servers[Channel].Players[Self.ClientID].Base.ClientID = 0) then
      Continue;

    // processamento thread-safe
    fCritSect.Enter;
    try
      try
        Servers[Channel].PacketControl(
          Servers[Channel].Players[Self.ClientID],
          PacketSize,
          Packet,
          0
        );
      except
        // uma exceção aqui encerrava a thread sem desconectar o jogador
        on E: Exception do
          Logger.Write('PacketControl [' + E.ClassName + ']: ' + E.Message,
            TLogType.Error);
      end;
    finally
      fCritSect.Leave;
    end;

    if (Servers[Channel].Players[Self.ClientID].SocketClosed) then
    begin
      Result := False;
      Break;
    end;
  end;

  // guarda o pedaço que sobrou no início do buffer
  if (Offset > 0) then
  begin
    if (Offset < FPendingLen) then
      Move(FPending[Offset], FPending[0], FPendingLen - Offset);
    Dec(FPendingLen, Offset);
  end;

  // um pacote válido sempre cabe no buffer; se encheu, o fluxo corrompeu
  if (FPendingLen >= SizeOf(FPending)) then
  begin
    Logger.Write('[' + Servers[Channel].Players[Self.ClientID].Ip +
      '] buffer de recepção cheio sem pacote válido.', TLogType.Warnings);
    FPendingLen := 0;
  end;
end;

function TPlayerThread.CheckSocket: Boolean;
begin
  // só verifica: a desconexão é feita uma única vez em Terminate
  // (antes PlayersThreads era decrementado aqui e de novo no Execute)
  Result := not((Servers[Channel].Sock = INVALID_SOCKET) or
    (Servers[Self.Channel].Players[ClientID].Socket = INVALID_SOCKET));
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
