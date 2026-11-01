unit AltarEvent;

interface

uses
  SysUtils, DateUtils, MiscData, Log;

type
  TAltarState = (
    altarLocked,       // Antes das pedras serem destruídas
    altarUnlocked,     // Pedras destruídas → altar clicável
    altarCapturing,    // Jogador está carregando barra de 10s
    altarCaptured      // Capturado e entregue
  );

  TAltarEvent = record
    Position: TPosition;   // posição do altar em Verband
    Active: Boolean;   // se o evento está ativo ou não
    State: TAltarState;
    CaptureStart: TDateTime;
    CaptureDuration: Integer; // segundos
    CapturingPlayer: string;  // Nome do jogador capturando
    LastCapturedBy: string;   // Histórico
  end;

procedure InitAltarEvent();
procedure TickAltarEvent();
procedure TryStartCapture(PlayerName: string);
procedure ForceResetAltar();

function IsAltarUnlocked: Boolean;

var
  Altar: TAltarEvent;

implementation

uses
  GuardianStones;

procedure InitAltarEvent();
begin
  // Posição real do altar (ajuste se usar outra)
  Altar.Position := TPosition.Create(3190, 1800);

  Altar.State := altarLocked;
  Altar.CaptureDuration := 10; // 10s oficiais do Aika
  Altar.CapturingPlayer := '';
  Altar.LastCapturedBy := '';
  Altar.Active := False;  // <--- AQUI

 // Logger.Write('[ALTAR] Evento do Altar inicializado.', TLogType.Error);
end;

function IsAltarUnlocked: Boolean;
begin
  Result := (Altar.State <> altarLocked);
end;

procedure TryStartCapture(PlayerName: string);
begin
  if Altar.State <> altarUnlocked then
  begin
  //  Logger.Write(Format('[ALTAR] %s tentou capturar o altar, mas ele não está liberado.',
    //  [PlayerName]), TLogType.Info);

    Exit;
  end;

  Altar.State := altarCapturing;
  Altar.CaptureStart := Now;
  Altar.CapturingPlayer := PlayerName;

  //Logger.Write(Format('[ALTAR] %s começou a capturar o altar.', [PlayerName]),
   // TLogType.ServerStatus);
end;

procedure FinishCapture();
begin
  Altar.State := altarCaptured;
  Altar.LastCapturedBy := Altar.CapturingPlayer;

//  Logger.Write(Format('[ALTAR] O altar foi capturado por %s!',
    //[Altar.CapturingPlayer]), TLogType.ServerStatus);

  // Aqui depois:
  // - Entregar recompensa
  // - Broadcast para o mapa inteiro
  // - Aplicar título exclusivo
end;

procedure CheckCaptureTimer();
begin
  if Altar.State <> altarCapturing then
    Exit;

  if SecondsBetween(Now, Altar.CaptureStart) >= Altar.CaptureDuration then
    FinishCapture();
end;

procedure CheckStonesState();
begin
  // Se as 3 pedras ainda estão vivas → altar sempre travado
  if not AllStonesDestroyed() then
  begin
    if Altar.State <> altarLocked then
    begin
    //  Logger.Write('[ALTAR] Altar resetado — pedras respawnaram.',
     //   TLogType.ServerStatus);
      ForceResetAltar();
    end;
    Exit;
  end;

  // Se todas as pedras caíram e o altar ainda não foi liberado
  if (Altar.State = altarLocked) then
  begin
    Altar.State := altarUnlocked;
 //   Logger.Write('[ALTAR] Todas as pedras foram destruídas! O altar está LIBERADO!',
  //    TLogType.ServerStatus);
  end;
end;

procedure ForceResetAltar();
begin
  Altar.State := altarLocked;
  Altar.CapturingPlayer := '';
  Altar.CaptureStart := 0;
end;

procedure TickAltarEvent();
begin
  if not Altar.Active then
    Exit;

  CheckStonesState();
  CheckCaptureTimer();
end;


end.

