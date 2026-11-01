unit AltarSystem;

interface

uses
  SysUtils, Windows, DateUtils, BaseMob, Player, GlobalDefs, Log;

type
  TAltarState = record
    Active: Boolean;
    Capturing: Boolean;
    CapturedBy: string[16];
    CaptureStart: TDateTime;
    MobID: WORD;
    Channel: Byte;
    Nation: Byte;
    CountdownSent: Integer;
  end;

type
  TAltarData = record
    IsActive: Boolean;
    IsCapturing: Boolean;
    CapturingPlayer: WORD;
    CapturingNation: Byte;
    CaptureStart: TDateTime;
    LastWinnerName: array[0..15] of AnsiChar;
  end;

var
  Altar: TAltarState;

procedure InitAltar(MobID: WORD; Channel: Byte);
procedure StartCapture(var Player: TPlayer);
procedure ProcessAltarTimer();
procedure FinishCapture(var Player: TPlayer);

implementation

procedure InitAltar(MobID: WORD; Channel: Byte);
begin
  Altar.Active := True;
  Altar.Capturing := False;
  Altar.CapturedBy := '';
  Altar.MobID := MobID;
  Altar.Channel := Channel;
  Altar.CaptureStart := 0;
  Altar.CountdownSent := 0;

  Logger.Write('ALTAR: Sistema inicializado. Mob=' +
    IntToStr(MobID) + ' Canal=' + IntToStr(Channel), TLogType.ServerStatus);
end;

procedure StartCapture(var Player: TPlayer);
begin
  if not Altar.Active then
  begin
    Player.SendClientMessage('O altar não está ativo.');
    Exit;
  end;

  if Altar.Capturing then
  begin
    Player.SendClientMessage('O altar já está sendo capturado.');
    Exit;
  end;

  Altar.Capturing := True;
  Altar.CapturedBy := Player.Base.Character.Name;
  Altar.CaptureStart := Now;
  Altar.Nation := Player.Base.Character.Nation;
  Altar.CountdownSent := 0;

  // Mensagem apenas para a Nação do jogador
 { Servers[Player.ChannelIndex].SendNationMsg(
    Player.Base.Character.Nation,
    '[ALTAR] ' + Player.Base.Character.Name +
    ' iniciou a captura! 10 segundos restantes!',
    255, 200, 0
  );}

  Player.SendClientMessage('Captura iniciada. Aguarde 10 segundos...');
end;

procedure FinishCapture(var Player: TPlayer);
begin
  Altar.Capturing := False;

  // Mensagem apenas para a nação dele
 { Servers[Player.ChannelIndex].SendNationMsg(
    Player.Base.Character.Nation,
    '[ALTAR] ' + Player.Base.Character.Name +
    ' capturou o Altar da Aika!',
    0, 255, 180
  );}

  // Mensagem GLOBAL
  Servers[Player.ChannelIndex].SendServerMsg(
    '[ALTAR] O altar foi capturado por ' + Player.Base.Character.Name + '!',
    0, 200, 255
  );

  // Recompensa (exemplo)
  //TItemFunctions.PutItem(Player, 4358, 1);



  Altar.CapturedBy := Player.Base.Character.Name;
end;

procedure ProcessAltarTimer();
var
  p: PPlayer;
  i: Integer;
  elapsed: Integer;
begin
  if not Altar.Active then Exit;
  if not Altar.Capturing then Exit;

  elapsed := SecondsBetween(Now, Altar.CaptureStart);

  // Verifica cancelamento
  for i := 1 to MAX_CONNECTIONS do
  begin
    p := @Servers[Altar.Channel].Players[i];

    if (not p.SocketClosed) and
       (p.Base.Character.Name = Altar.CapturedBy) then
    begin
      if p.Base.IsDead then
      begin
   {     Servers[Altar.Channel].SendNationMsg(
          Altar.Nation,
          '[ALTAR] Captura cancelada! ' + Altar.CapturedBy + ' morreu.',
          255, 80, 80
        );}
        Altar.Capturing := False;
        Exit;
      end;
    end;
  end;

  // Enviar contagem para a nação
  if elapsed > Altar.CountdownSent then
  begin
    Altar.CountdownSent := elapsed;

    if elapsed < 10 then
    begin
    {  Servers[Altar.Channel].SendNationMsg(
        Altar.Nation,
        '[ALTAR] ' + Altar.CapturedBy + ' capturando... ' +
        IntToStr(10 - elapsed) + ' segundos restantes.',
        255, 230, 60
      );               }
    end;
  end;

  // Finaliza aos 10s
  if elapsed >= 10 then
  begin
    for i := 1 to MAX_CONNECTIONS do
    begin
      p := @Servers[Altar.Channel].Players[i];

      if (not p.SocketClosed) and
         (p.Base.Character.Name = Altar.CapturedBy) and
         (not p.Base.IsDead) then
      begin
        FinishCapture(p^);
        Exit;
      end;
    end;
  end;
end;

end.

