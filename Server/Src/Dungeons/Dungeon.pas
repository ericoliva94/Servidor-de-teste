unit Dungeon;

interface

uses
  Windows, SysUtils, MOB, MiscData, PartyData, NPC, Classes, BaseMob, Math, System.SyncObjs;
{$OLDTYPELAYOUT ON}

{$REGION 'Dungeon Thread'}

type
  TDungeonMainThread = class(TThread)
  private
    FDelay: Integer;
    ChannelId: BYTE;
    InstanceID: BYTE;
  protected
    procedure Execute; override;
  public
    constructor Create(SleepTime: Integer; ChannelId: BYTE; InstanceID: BYTE);
  end;
{$ENDREGION}
{$REGION 'Dungeons Data'}

type
  TDungeonMobDrop = packed record
    SemCoroa: Array [0 .. 41] of WORD; // 12 {1=ursula normal}{2=coroacinza}
    CoroaPrata: Array [0 .. 41] of WORD;
    CoroaDourada: Array [0 .. 41] of WORD;
  end;

type
  TDungeonMobDropData = packed record
    Drops: Array [0 .. 41] of WORD;
  end;

type
  TDungeon = record
    Index: DWORD;
    Dificult: BYTE;
    LevelMin: BYTE;
    EntranceNPCID: WORD;
    EntrancePosition: TPosition; // pra calcular se todos os players estão lá
    SpawnInDungeonPosition: TPosition; // posição que vai spawnar la dentro
    MOBS: TMobDungeonStruct; // até 45 tipos de mobs diferentes
    MobsDrop: TDungeonMobDrop;
    NPCS: Array [2600 .. 2609] of TNpc; // até 10 npcs dentro da dg
  end;

type
  PMobsStructDungeonInstance = ^TMobsStructDungeonInstance;

  TMobsStructDungeonInstance = packed record
    IntName: WORD;
    Equip: Array [0 .. 12] of WORD; // mob equips [face, weapon]
    Base: TBaseMob;
    MaxHP, CurrentHP: DWORD;
    Position: TPosition;
    MobLevel: BYTE;
    MobExp: DWORD;
    MobType: WORD;
    MobElevation, Cabeca, Perna: BYTE; // mob "altura"
    FisAtk, MagAtk, FisDef, MagDef: DWORD; // mob attributes

    DeadTime: TDateTime;
    LastMoviment: TDateTime;
    MovedTo: TypeMobLocation;
    MovimentsTime: BYTE;

    XPositionDif: Integer;
    YPositionDif: Integer;
    XPositionsToMove: Integer;
    YPositionsToMove: Integer;

    IsAttacked: Boolean;
    AttackerID: WORD;
    FirstPlayerAttacker: Integer;

    LastMyAttack: TDateTime;
    OrigPos: TPosition;

    NeighborIndex: Integer;

  public
    procedure AttackPlayer(DanoInicial: Integer; ChannelIndex: integer; ClientID: integer);
    procedure MobMove(Position: TPosition; DungeonInstance: BYTE;
      Speed: BYTE = 25; MoveType: BYTE = 0);
  end;

type
  PDungeonInstance = ^TDungeonInstance;

  TDungeonInstance = record
    Index: WORD;
    Party: PParty; // pointer da party que está entrando
    CreateTime: TDateTime;
    DungeonID: BYTE;
    Dificult: BYTE;
    MOBS: Array [1 .. 450] of TMobsStructDungeonInstance;
    MobsDrop: Array [1 .. 450] of TDungeonMobDropData;
    MainThread: TDungeonMainThread;
    InstanceOnline: Boolean;
  end;

{$ENDREGION}

{$OLDTYPELAYOUT OFF}
implementation

uses
  GlobalDefs, Packets, Log, DateUtils, PlayerData, Player;

{$REGION 'Mob Dungeon Functions'}

procedure TMobsStructDungeonInstance.AttackPlayer(DanoInicial: Integer; ChannelIndex: integer; ClientID: integer);
var
  Packet: TRecvDamagePacket;
  dnType: TDamageType;
  SkillIDa: DWORD;
  AddBuff: boolean;
  Deftarget, DANO, DamageReduction, PlayerHP: INTEGER;
begin
  DANO := 0;



  try
  if Self.AttackerID = 0 then
    Exit;

  if Self.base.IsDead then
      Exit;

      SkillIDa := 0;
      ZeroMemory(@Packet, sizeof(Packet));

      Packet.Header.size := sizeof(Packet);
      Packet.Header.Index := Self.Base.ClientID;
      Packet.Header.Code := $102;
      Packet.SkillID := SkillIDa;
      Packet.AttackerPos := Self.Position;
      Packet.AttackerID := Self.Base.ClientID;
      Packet.Animation := 06;
      Packet.AttackerHP := Self.CurrentHP;
      Packet.TargetID := Self.AttackerID;
      Packet.MobAnimation := 26;

    if (Self.AttackerID > MAX_CONNECTIONS) then
      Exit;

    var AttackerIDs: PPlayer := @Servers[Self.Base.ChannelId].Players[Self.AttackerID];
    var PlayerBase: PBaseMob := @AttackerIDs.Base;
    PlayerHP := PlayerBase.PlayerCharacter.Base.CurrentScore.CurHP;

    if Attackerids.Base.IsDead then
    Exit;

    try
      if Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.GetMobAbility(EF_IMMUNITY) > 0 then
      begin
        dnType := TDamageType.Immune;
        Packet.DNType := dnType;
        PlayerBase.LastReceivedAttack := Now;
        Packet.MobCurrHP := PlayerHP;
        PlayerBase.SendToVisible(Packet, Packet.Header.size, True);
        Exit;
      end

      else if PlayerBase.BuffExistsByIndex(19) then
      begin
        PlayerBase.RemoveBuffByIndex(19);
        dnType := TDamageType.Block;
        Packet.DNType := dnType;
        PlayerBase.LastReceivedAttack := Now;
        Packet.MobCurrHP := PlayerHP;
        PlayerBase.SendToVisible(Packet, Packet.Header.size, True);
        Exit;
      end

      else if Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.BuffExistsByIndex(91) then
      begin
        PlayerBase.RemoveBuffByIndex(91);
        dnType := TDamageType.Miss2;
        Packet.DNType := dnType;
        PlayerBase.LastReceivedAttack := Now;
        Packet.MobCurrHP := PlayerHP;
        PlayerBase.SendToVisible(Packet, Packet.Header.size, True);
        Exit;
      end;

    except
          on E: Exception do
          Logger.Write('Dungeon: erro ao atacar player ' +
            E.Message + '  ' + DateTimeToStr(now) +
            '.', TLogType.Error);
      end;


    Deftarget := AttackerIDs.Base.PlayerCharacter.Base.CurrentScore.DefFis;
    var Dificuldade := AttackerIDs.DungeonLobbyDificult;
    var DungeonIDLobby :=AttackerIDs.DungeonLobbyIndex;

    case Dificuldade of
      0..3: // Dificuldades de 0 a 3
        begin
          var multiplicador := Dificuldade + 1;
          var divisorDefesa := DungeonIDLobby + 2;

          case DungeonIDLobby of
            1: DANO := ((DanoInicial + (25 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            2: DANO := ((DanoInicial + (50 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            3: DANO := ((DanoInicial + (75 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            4: DANO := ((DanoInicial + (100 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            5: DANO := ((DanoInicial + (125 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            6: DANO := ((DanoInicial + (150 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            7: DANO := ((DanoInicial + (175 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
            8: DANO := ((DanoInicial + (200 * multiplicador)) div 2) - (Deftarget div divisorDefesa);
          end;

        end;

    end;

    DamageReduction := (DANO div 100) * (AttackerIDs.Base.GetEquipedItensDamageReduce div 10);
    Dec(DANO, DamageReduction);
    Inc(DANO, (RandomRange(10, 39) + 7));
    if (DANO < 0) then
      DANO := 1;

    var AttackResultType := RandomRange(1, 101);
      case AttackResultType of
        2 .. 15: dnType := TDamageType.Miss;
        16 .. 30: dnType := TDamageType.Critical;
        31 .. 100: dnType := TDamageType.Normal;
      else
        dnType := TDamageType.Normal;
    end;

    if (SecondsBetween(Now, PlayerBase.RevivedTime) <= 7) then
      Exit;

    Self.Base.AttackParseForMobs(0, 0, PlayerBase, DANO, dnType, AddBuff, Packet.MobAnimation);

    Packet.dnType := dnType;
    Packet.DANO := DANO;
    AttackerIDs.Base.LastReceivedAttack := Now;



         try
         if Self.AttackerID = 0 then
         Exit;

            if (Packet.DANO >= Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.CurHP) and not (AttackerIDS.base.IsDead) then
              begin

                // Atualização do HP e status do jogador
                Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.CurHP := 0;
                Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.IsDead := True;

                // Enviar a animação de morte primeiro
                Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendEffect($0);
                Packet.MobAnimation := 30; // Define a animação de morte para o mob
                Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible(Packet, Packet.Header.size, True);
                        if(Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.Index > 0) then
                          begin
                                if(Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.Members.Count = 1) then
                                begin
                                  // deve tá removendo o player depois que morre automatico, sla
                                  var InstanceToClose := AttackerIDs.DungeonInstanceID;
                                  AttackerIDS.base.VisibleMobs.Clear;
                                  AttackerIDS.base.VisibleNPCS.Clear;
                                  AttackerIDS.base.VisiblePlayers.Clear;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex := 0;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult := 0;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonID := 0;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonIDDificult := 0;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID := 0;
                                  Servers[Self.Base.ChannelId].Players[Self.AttackerID].InDungeon := False;


                                        if (InstanceToClose >= Low(DungeonInstances)) and
                                           (InstanceToClose <= High(DungeonInstances)) then
                                        begin
                                              DungeonInstances[InstanceToClose].InstanceOnline := False;
                                              DungeonInstances[InstanceToClose].Index := 0;
                                              DungeonInstances[InstanceToClose].CreateTime := Now;
                                              DungeonInstances[InstanceToClose].Party := nil;
                                              DungeonInstances[InstanceToClose].DungeonID := 0;
                                              DungeonInstances[InstanceToClose].Dificult := 0;
                                              ZeroMemory(@DungeonInstances[InstanceToClose].MOBS,
                                              sizeof(DungeonInstances[InstanceToClose].MOBS));
                                        end;
                                   Servers[Self.Base.ChannelId].Players[Self.AttackerID].IsInsideDungeon := true;

                                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].Teleport(TPosition.Create(3399, 564));
                                end
                                else
                                begin
                                Servers[Self.Base.ChannelId].Players[Self.AttackerID].IsInsideDungeon := true;
                                end;
                      end;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendEffect($0);
                    Packet.MobAnimation := 30;

                    Self.IsAttacked := FALSE;
                    Self.LastMoviment := Now;
                    Self.MovimentsTime := 0;
                    Self.MovedTo := Init;
              end
               else
               begin
               if Servers[Self.Base.ChannelId].Players[Self.AttackerID]
                   .Base.IsDead then
                   Exit;

                 Servers[Self.Base.ChannelId].Players[Self.AttackerID]
                   .Base.Character.CurrentScore.CurHP :=
                   (Servers[Self.Base.ChannelId].Players[Self.AttackerID]
                   .Base.Character.CurrentScore.CurHP - Packet.DANO);
               end;

            Servers[Self.Base.ChannelId].Players[Self.AttackerID]
              .Base.LastReceivedAttack := Now;

            Packet.MobCurrHP := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
              .Base.Character.CurrentScore.CurHP;

            Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible
              (Packet, Packet.Header.size, True);
           except
            on E: Exception do
              Logger.Write('Error at mob Attack Player: ' + DateTimeToStr(Now), TlogType.Error);
          end;

  except
    on E: Exception do
      Logger.Write('Error at mob Attack Player: ' + DateTimeToStr(Now), TlogType.Error);
  end;
end;


procedure TMobsStructDungeonInstance.MobMove(Position: TPosition;
  DungeonInstance: BYTE; Speed: BYTE; MoveType: BYTE);
var
  Packet: TMobMovimentPacket;
  i: Integer;
begin
  ZeroMemory(@Packet, sizeof(TMobMovimentPacket));

  Packet.Header.size := sizeof(TMobMovimentPacket);
  Packet.Header.Index := Self.Base.ClientID;
  Packet.Header.Code := $301;
  Packet.Destination := Position;
  Packet.MoveType := MoveType;

  // Atribui o valor de Packet.Speed diretamente
  if (Speed = 25) then
    Packet.Speed := 25
  else
    Packet.Speed := Speed;

  Self.Base.PlayerCharacter.LastPos := Position;

  // A otimização ocorre aqui, evitando acessar "Self.Base.VisibleMobs" repetidamente
  for i in Self.Base.VisibleMobs do
  begin
    if (i >= 1) and (i <= MAX_CONNECTIONS) then
    begin
      with Servers[Self.Base.ChannelId].Players[i] do
      begin
        // Verifica o status do jogador diretamente
        if (Status < TPlayerStatus.Playing) then
          continue;

        SendPacket(Packet, Packet.Header.size);
      end;
    end;
  end;
end;


{$ENDREGION}
{$REGION 'TDungeonMainThread'}

constructor TDungeonMainThread.Create(SleepTime: Integer; ChannelId: BYTE;
  InstanceID: BYTE);
begin
  Self.FDelay := SleepTime;
  Self.FreeOnTerminate := True;
  Self.ChannelId := ChannelId;
  Self.InstanceID := InstanceID;

  inherited Create(FALSE);
end;

procedure TDungeonMainThread.Execute;
var
  i, j: WORD;
  Cid1, Cid2: WORD;
  SelfParty: PParty;
  Packet: TSendRemoveMobPacket;
  InstanceIDD, Dificult: BYTE;
  UpdatedBuffs: Integer;
  Rand: Integer;
begin
  SelfParty := DungeonInstances[Self.InstanceID].Party;
  if (SelfParty = nil) then
    Exit;



  while (DungeonInstances[Self.InstanceID].InstanceOnline) do
  begin
{$REGION 'Atualização do spawn dentro da dungeon'}
    if (SelfParty.Members.Count > 1) then
    begin
      for i in SelfParty.Members.ToArray do
      begin
        Cid1 := i;

        for j in SelfParty.Members.ToArray do
        begin
          Cid2 := j;

          if (Cid1 = Cid2) then
            continue;

          if (Servers[Self.ChannelId].Players[Cid1]
            .Base.PlayerCharacter.LastPos.InRange(Servers[Self.ChannelId]
            .Players[Cid2].Base.PlayerCharacter.LastPos, DISTANCE_TO_WATCH))
          then
          begin
            if (Servers[Self.ChannelId].Players[Cid1]
              .Base.VisiblePlayers.Contains(Servers[Self.ChannelId].Players
              [Cid2].Base.ClientID)) then
              continue;

            Servers[Self.ChannelId].Players[Cid1].Base.AddVisible
              (Servers[Self.ChannelId].Players[Cid2].Base);

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranSpawn(0, Cid1, 0);
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranSpawn(1, Cid1, 0);
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranSpawn(0, Cid2, 0);
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranSpawn(1, Cid2, 0);
            end;
          end
          else
          begin
            if not(Servers[Self.ChannelId].Players[Cid1]
              .Base.VisiblePlayers.Contains(Servers[Self.ChannelId].Players
              [Cid2].Base.ClientID)) then
              continue;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranUnspawn(0, Cid2);
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranUnspawn(1, Cid2);
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranUnspawn(0, Cid1);
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranUnspawn(1, Cid1);
            end;

            Servers[Self.ChannelId].Players[Cid1].Base.RemoveVisible
              (Servers[Self.ChannelId].Players[Cid2].Base);

            if (Servers[Self.ChannelId].Players[Cid2].Base.IsActive = FALSE)
            then
            begin
              ZeroMemory(@Packet, sizeof(Packet));

              Packet.Header.size := sizeof(Packet);
              Packet.Header.Index := $7535;
              Packet.Header.Code := $101;

              Packet.Index := Cid2;

              Servers[Self.ChannelId].Players[Cid1].Base.SendPacket(Packet,
                Packet.Header.size);
            end;
          end;
        end;

        Dificult := Servers[Self.ChannelId].Players[Cid1].DungeonIDDificult;
        InstanceIDD := Servers[Self.ChannelId].Players[Cid1].DungeonInstanceID;

        for j := Low(DungeonInstances[InstanceIDD].MOBS)
          to High(DungeonInstances[InstanceIDD].MOBS) do
        begin
          if (DungeonInstances[InstanceIDD].MOBS[j].IntName = 0) or
            (DungeonInstances[InstanceIDD].MOBS[j].Base.IsDead) then
            continue;

          if (Servers[Self.ChannelId].Players[Cid1]
            .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
            .MOBS[j].Position, DISTANCE_TO_WATCH)) then
          begin
            if not(DungeonInstances[InstanceIDD].MOBS[j].Base.IsDead) then
            begin
              if(DungeonInstances[InstanceIDD].MOBS[j].AttackerID = 0) then
              begin
                 if (Servers[Self.ChannelId].Players[Cid1]
                  .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
                    .MOBS[j].Position, 7)) then
                 begin
                   DungeonInstances[InstanceIDD].MOBS[j].OrigPos :=
                    DungeonInstances[InstanceIDD].MOBS[j].Position;
                   DungeonInstances[InstanceIDD].MOBS[j].AttackerID :=
                     Servers[Self.ChannelId].Players[Cid1].Base.ClientID;
                   DungeonInstances[InstanceIDD].MOBS[j].LastMyAttack := Now;
                 end;
              end
              else
              begin
                if not(DungeonInstances[InstanceIDD]
                    .MOBS[j].Position.InRange(DungeonInstances[InstanceIDD]
                    .MOBS[j].OrigPos, 20)) then
                 begin
                   DungeonInstances[InstanceIDD].MOBS[j].Position :=
                   DungeonInstances[InstanceIDD].MOBS[j].OrigPos;

                   DungeonInstances[Self.InstanceID].MOBS[j]
                    .MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,
                   Self.InstanceID, 35);

                  DungeonInstances[InstanceIDD].MOBS[j].AttackerID := 0;
                 end;
              end;
            end;

            if (Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Contains
              (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID)) then
              continue;

            Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Add
              (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID);
            DungeonInstances[InstanceIDD].MOBS[j].Base.VisibleMobs.Add(Cid1);

            if (Servers[Self.ChannelId].Players[Cid1].Base.AddTargetToList
              (@DungeonInstances[InstanceIDD].MOBS[j].Base)) then  ///////
              Servers[Self.ChannelId].Players[Cid1].SendSpawnMobDungeon
                (@DungeonInstances[InstanceIDD].MOBS[j]);
          end
          else
          begin
            if not(Servers[Self.ChannelId].Players[Cid1]
              .Base.VisibleMobs.Contains(DungeonInstances[InstanceIDD].MOBS[j]
              .Base.ClientID)) then
              continue;

            DungeonInstances[InstanceIDD].MOBS[j].AttackerID := 0;

            Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Remove
              (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID);
            DungeonInstances[InstanceIDD].MOBS[j].Base.VisibleMobs.Remove(Cid1);

            if (Servers[Self.ChannelId].Players[Cid1].Base.RemoveTargetFromList
              (@DungeonInstances[InstanceIDD].MOBS[j].Base)) then
            Servers[Self.ChannelId].Players[Cid1].SendRemoveMobDungeon
              (@DungeonInstances[InstanceIDD].MOBS[j]);
          end;
        end;
      end;
    end
    else
    begin
    {$ENDREGION}
      Cid1 := DungeonInstances[InstanceID].Party.Members.First;

      Dificult := Servers[Self.ChannelId].Players[Cid1].DungeonIDDificult;
      InstanceIDD := Servers[Self.ChannelId].Players[Cid1].DungeonInstanceID;

      for j := Low(DungeonInstances[InstanceIDD].MOBS)
        to High(DungeonInstances[InstanceIDD].MOBS) do
      begin
        if (DungeonInstances[InstanceIDD].MOBS[j].IntName = 0) or
          (DungeonInstances[InstanceIDD].MOBS[j].Base.IsDead) then
          continue;

        if (Servers[Self.ChannelId].Players[Cid1].Base.PlayerCharacter.LastPos.
          InRange(DungeonInstances[InstanceIDD].MOBS[j].Position,
          DISTANCE_TO_WATCH)) then
        begin
          if(DungeonInstances[InstanceIDD].MOBS[j].AttackerID = 0) then
          begin
             if (Servers[Self.ChannelId].Players[Cid1]
              .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
                .MOBS[j].Position, 35)) then
             begin
               DungeonInstances[InstanceIDD].MOBS[j].OrigPos :=
                DungeonInstances[InstanceIDD].MOBS[j].Position;
               DungeonInstances[InstanceIDD].MOBS[j].AttackerID :=
                 Servers[Self.ChannelId].Players[Cid1].Base.ClientID;
               DungeonInstances[InstanceIDD].MOBS[j].LastMyAttack := Now;
             end;
          end
          else
          begin
            if not(DungeonInstances[InstanceIDD]
                .MOBS[j].Position.InRange(DungeonInstances[InstanceIDD]
                .MOBS[j].OrigPos, 20)) then
             begin
               DungeonInstances[InstanceIDD].MOBS[j].Position :=
               DungeonInstances[InstanceIDD].MOBS[j].OrigPos;

               DungeonInstances[Self.InstanceID].MOBS[j]
                .MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,
               Self.InstanceID, 35);

               DungeonInstances[InstanceIDD].MOBS[j].AttackerID := 0;
             end;
          end;

          if (Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Contains
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID)) then
            continue;

          Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Add
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID);
          DungeonInstances[InstanceIDD].MOBS[j].Base.VisibleMobs.Add(Cid1);

          if (Servers[Self.ChannelId].Players[Cid1].Base.AddTargetToList
              (@DungeonInstances[InstanceIDD].MOBS[j].Base)) then
          Servers[Self.ChannelId].Players[Cid1].SendSpawnMobDungeon
            (@DungeonInstances[InstanceIDD].MOBS[j]);
        end
        else
        begin
          if not(Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Contains
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID)) then
            continue;

          Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Remove
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID);
          DungeonInstances[InstanceIDD].MOBS[j].Base.VisibleMobs.Remove(Cid1);

          if (Servers[Self.ChannelId].Players[Cid1].Base.RemoveTargetFromList
              (@DungeonInstances[InstanceIDD].MOBS[j].Base)) then
          Servers[Self.ChannelId].Players[Cid1].SendRemoveMobDungeon
            (@DungeonInstances[InstanceIDD].MOBS[j]);

//            WriteLn('removendo mob ' + @DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID.ToString);
        end;
      end;
    end;
{$ENDREGION}
{$REGION 'Mob Handler dungeon'}
    for i := Low(DungeonInstances[Self.InstanceID].MOBS) to High(DungeonInstances[Self.InstanceID].MOBS) do
    begin
      with DungeonInstances[Self.InstanceID].MOBS[i] do
      begin
        if (AttackerID = 0) or (AttackerID > MAX_CONNECTIONS) then
        begin
    //    AttackerID:= DungeonInstances[Self.InstanceID].Party.Leader;
    //    moblocalplayer := Servers[Self.ChannelId].Players[DungeonInstances[Self.InstanceID].Party.Leader];
        Continue;
        end;

        var moblocalplayer: PPlayer := @Servers[Self.ChannelId].Players[AttackerID];

        if (Base.ClientID = 0) then
          continue;

        if not(Base.IsDead) then
        begin

{$REGION 'Buffs'}
      UpdatedBuffs := Base.RefreshBuffs;
      if (UpdatedBuffs > 0) then
        Base.SendRefreshBuffs;
{$ENDREGION}

{$REGION 'Attack'}
      if IsAttacked then
      begin
        if (Position.Distance(moblocalplayer.Base.PlayerCharacter.LastPos) <= 5) then
        begin
          if (SecondsBetween(Now, LastMyAttack) >= 2) then
          begin
            if not(moblocalplayer.Base.IsDead) then
            begin
              if (Base.GetDebuffCount = 0) then
              begin
                LastMyAttack := Now;
                AttackPlayer(FisAtk,Self.ChannelId,Servers[Self.ChannelId].Players[AttackerID].Base.ClientID); // 100 é o valor do dano
                moblocalplayer.Base.LastReceivedAttack := Now;
              end;
            end;
          end;
        end
        else
        begin
          Randomize;
          Rand := Random(7);
          Position := moblocalplayer.Base.Neighbors[Rand].pos;
          MobMove(Position, Self.InstanceID, 35);
        end;

        if not(Position.InRange(OrigPos, 15)) then
        begin
          Position := OrigPos;
          MobMove(Position, Self.InstanceID, 35);
          AttackerID := 0;
          IsAttacked := False;
        end;
      end
      else
      begin
        for j in DungeonInstances[Self.InstanceID].Party.Members.ToArray do
        begin
          with Servers[Self.ChannelId].Players[j] do
            if Base.PlayerCharacter.LastPos.InRange(Position, 15) then
            begin
              OrigPos := Position;
              IsAttacked := True;
              AttackerID := Base.ClientID;
              LastMyAttack := Now;
              break;
            end;
        end;
      end;

{$ENDREGION}
    end;
  end;
end;

{$ENDREGION}
    Sleep(FDelay);
  end;
end;

{$ENDREGION}
end.
