unit GuardianStones;

interface

uses
  SysUtils,
  GlobalDefs,
  MiscData,   // TPosition
  Log;        // Logger.Write

type
  TGuardianStoneState = (gsAlive, gsDestroyed);

  TGuardianStone = record
    ID: Integer;
    Color: Byte;
    PosX: WORD;
    PosY: WORD;
    MaxHP: Integer;
    HP: Integer;
    State: TGuardianStoneState;
    RespawnTime: TDateTime;
  end;

var
  AltarGuardianStones: array[1..3] of TGuardianStone;

procedure InitGuardianStones();
procedure DamageStone(StoneID: Integer; Damage: Integer; const AttackerName: string);
procedure RespawnStonesIfNeeded();
function AllStonesDestroyed: Boolean;

implementation

uses
  DateUtils;

const
  GUARDIAN_STONE_MAP_ID = 9;                 // Mapa Verband
  GUARDIAN_STONE_BASE_HP = 500000;           // HP das pedras
  GUARDIAN_STONE_RESPAWN_MINUTES = 60;       // Respawn

// ---------------------------------------------------------
// Inicialização
// ---------------------------------------------------------
procedure InitGuardianStones();
begin
  // Pedra Azul – 3655, 918
  AltarGuardianStones[1].ID    := 1;
  AltarGuardianStones[1].Color := 1; // azul
  AltarGuardianStones[1].PosX  := 3655;
  AltarGuardianStones[1].PosY  := 918;
  AltarGuardianStones[1].MaxHP := GUARDIAN_STONE_BASE_HP;
  AltarGuardianStones[1].HP    := AltarGuardianStones[1].MaxHP;
  AltarGuardianStones[1].State := gsAlive;
  AltarGuardianStones[1].RespawnTime := 0;

  // Pedra Amarela – 3231, 1004
  AltarGuardianStones[2].ID    := 2;
  AltarGuardianStones[2].Color := 3; // amarela
  AltarGuardianStones[2].PosX  := 3231;
  AltarGuardianStones[2].PosY  := 1004;
  AltarGuardianStones[2].MaxHP := GUARDIAN_STONE_BASE_HP;
  AltarGuardianStones[2].HP    := AltarGuardianStones[2].MaxHP;
  AltarGuardianStones[2].State := gsAlive;
  AltarGuardianStones[2].RespawnTime := 0;

  // Pedra Vermelha – 3425, 1363
  AltarGuardianStones[3].ID    := 3;
  AltarGuardianStones[3].Color := 2; // vermelha
  AltarGuardianStones[3].PosX  := 3425;
  AltarGuardianStones[3].PosY  := 1363;
  AltarGuardianStones[3].MaxHP := GUARDIAN_STONE_BASE_HP;
  AltarGuardianStones[3].HP    := AltarGuardianStones[3].MaxHP;
  AltarGuardianStones[3].State := gsAlive;
  AltarGuardianStones[3].RespawnTime := 0;

  Logger.Write('[ALTAR] Pedras Protetoras inicializadas.',
               TLogType.ServerStatus);
end;



// ---------------------------------------------------------
// Todas destruídas?
// ---------------------------------------------------------
function AllStonesDestroyed: Boolean;
var
  i: Integer;
begin
  Result := True;
  for i := Low(AltarGuardianStones) to High(AltarGuardianStones) do
  begin
    if AltarGuardianStones[i].State <> gsDestroyed then
    begin
      Result := False;
      Exit;
    end;
  end;
end;


// ---------------------------------------------------------
// Dano nas pedras
// ---------------------------------------------------------
procedure DamageStone(StoneID: Integer; Damage: Integer; const AttackerName: string);
var
  Stone: ^TGuardianStone;
begin
  if (StoneID < Low(AltarGuardianStones)) or (StoneID > High(AltarGuardianStones)) then
    Exit;

  Stone := @AltarGuardianStones[StoneID];

  if Stone.State <> gsAlive then
    Exit;

  Dec(Stone.HP, Damage);
  if Stone.HP < 0 then
    Stone.HP := 0;

  Logger.Write(
    Format('[ALTAR] Pedra %d tomou %d de dano. HP atual: %d',
           [StoneID, Damage, Stone.HP]),
    TLogType.Painel
  );

  if Stone.HP <= 0 then
  begin
    Stone.State := gsDestroyed;
    Stone.RespawnTime := IncMinute(Now, GUARDIAN_STONE_RESPAWN_MINUTES);

    Logger.Write(
      Format('[ALTAR] Pedra Protetora %d destruída por %s.',
             [StoneID, AttackerName]),
      TLogType.ServerStatus
    );
  end;
end;


// ---------------------------------------------------------
// Respawn automático
// ---------------------------------------------------------
procedure RespawnStonesIfNeeded();
var
  i: Integer;
begin
  for i := Low(AltarGuardianStones) to High(AltarGuardianStones) do
  begin
    if (AltarGuardianStones[i].State = gsDestroyed) and
       (AltarGuardianStones[i].RespawnTime > 0) and
       (Now >= AltarGuardianStones[i].RespawnTime) then
    begin
      AltarGuardianStones[i].State := gsAlive;
      AltarGuardianStones[i].HP    := AltarGuardianStones[i].MaxHP;
      AltarGuardianStones[i].RespawnTime := 0;

      Logger.Write(
        Format('[ALTAR] Pedra Protetora %d foi respawnada.',
               [AltarGuardianStones[i].ID]),
        TLogType.ServerStatus
      );
    end;
  end;
end;

end.

