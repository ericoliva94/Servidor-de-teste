unit GlobalEvents;

interface

procedure InitGlobalEvents();
procedure TickGlobalEvents();

implementation

uses
  SysUtils,
  Log,
  GlobalDefs,         // TPosition, Players, Servers[]
  GuardianStones,     // InitGuardianStones, RespawnStonesIfNeeded
  AltarEvent;         // InitAltarEvent, TickAltarEvent

procedure InitGlobalEvents();
begin
  // Inicializar pedras protetoras
  InitGuardianStones();

  // Inicializar sistema do altar
  InitAltarEvent();

  { ---
    SPAWN DO ALTAR (OBJETO ESTÁTICO)
    ATENÇÃO: MOB_ALTAR_ID deve estar definido em GlobalDefs.pas,
    ou então você deve trocar pelo ID numérico diretamente.
  --- }


  Logger.Write('[ALTAR] Sistemas de altar e pedras protetoras inicializados.',
    TLogType.ServerStatus);
end;

procedure TickGlobalEvents();
begin
  // Pedras renascem automaticamente
  RespawnStonesIfNeeded();

  // Lógica contínua do altar
  TickAltarEvent();
end;

end.

