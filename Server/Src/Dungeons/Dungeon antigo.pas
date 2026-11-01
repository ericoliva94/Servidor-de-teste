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
    procedure AttackPlayer(DanoInicial: Integer);
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
  GlobalDefs, Packets, Log, DateUtils, PlayerData;

{$REGION 'Mob Dungeon Functions'}

procedure TMobsStructDungeonInstance.AttackPlayer(DanoInicial: Integer);
var
  Packet: TRecvDamagePacket;
  dnType: TDamageType;
  SkillIDa: DWORD;
  AddBuff: Boolean;
  Deftarget: integer;
  DANO, DamageReduction: INTEGER;

begin
  try

    SkillIDa := 0;
//    Dano := 0;


     {if (Self.PosGot.Distance(Servers[Self.Base.ChannelId].Players[Self.AttackerID]
      .Base.PlayerCharacter.LastPos) > 5) then
      begin
      Self.PosGot := Self.CurrentPos;
      Exit;
      end;}
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

    if (Servers[Self.Base.ChannelId].Players[Self.AttackerID]
      .Base.GetMobAbility(EF_IMMUNITY) > 0) then
    begin
      DnType := TDamageType.Immune;

      Packet.DNType := dnType;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.LastReceivedAttack := Now;

      Packet.MobCurrHP := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.Character.CurrentScore.CurHP;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible
        (Packet, Packet.Header.size, True);
    end;

    if (Servers[Self.Base.ChannelId].Players[Self.AttackerID]
      .Base.BuffExistsByIndex(19)) then
    begin

      Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.RemoveBuffByIndex(19);

      DnType := TDamageType.Block;

      Packet.DNType := dnType;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.LastReceivedAttack := Now;

      Packet.MobCurrHP := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.Character.CurrentScore.CurHP;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible
        (Packet, Packet.Header.size, True);
      Exit;

    end;

    if (Servers[Self.Base.ChannelId].Players[Self.AttackerID]
      .Base.BuffExistsByIndex(91)) then
    begin
      Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.RemoveBuffByIndex(91);
      DnType := TDamageType.Miss2;

      Packet.DNType := dnType;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.LastReceivedAttack := Now;

      Packet.MobCurrHP := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.Character.CurrentScore.CurHP;

      Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible
        (Packet, Packet.Header.size, True);
      Exit;
    end;



    { case Servers[Self.Base.ChannelId].Players[Self.AttackerID]
      .Base.Character.Level of
      0 .. 20:
      begin
      Packet.DANO := 10;
      end;
      21 .. 40:
      begin
      Packet.DANO := 50;
      end;
      41 .. 60:
      begin
      Packet.DANO := 200;
      end;
      end; }
                DefTarget := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
            .Base.PlayerCharacter.Base.CurrentScore.DefFis;

//    case Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//      .Base.GetMobClass of
//      0 .. 3:
//        begin
//          DefTarget := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//            .Base.PlayerCharacter.Base.CurrentScore.DefFis;
//        end;
//    else
//      DefTarget := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//        .Base.PlayerCharacter.Base.CurrentScore.DefMag;
//    end;
       //colocar por modo, a divisao pra cada mob vai ser diferente



        case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult of





        //      Writeln(DungeonInstances[Self].MOBS[i].Base);
           {$REGION Dungeon normal, URSULA1, EVGENIA2, EVGENIA 23, MINA14, MINA25, JARDIM6}
               0: // Dificuldade Normal
               begin
                   case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
                       1: //ursula
                       DANO := ((DanoInicial+ (25*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       2: //evgenia 1
                       DANO := ((DanoInicial+ (50*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       3: //evgenia 2
                       DANO := ((DanoInicial+ (75*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       4: //mina 1
                       DANO := ((DanoInicial+ (100*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       5: //mina 2
                       DANO := ((DanoInicial+ (125*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       6: //jardim
                       DANO := ((DanoInicial+ (150*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       7: //prisao
                       DANO := ((DanoInicial+ (175*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       8: //lycan
                       DANO := ((DanoInicial+ (200*1)) div 2) - (DefTarget div 2); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                   end;
               end;
               1: // Dificuldade Normal
               begin
                   case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
                       1: //ursula
                       DANO := ((DanoInicial+ (25*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       2: //evgenia 1
                       DANO := ((DanoInicial+ (50*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       3: //evgenia 2
                       DANO := ((DanoInicial+ (75*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       4: //mina 1
                       DANO := ((DanoInicial+ (100*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       5: //mina 2
                       DANO := ((DanoInicial+ (125*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       6: //jardim
                       DANO := ((DanoInicial+ (150*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       7: //prisao
                       DANO := ((DanoInicial+ (175*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       8: //lycan
                       DANO := ((DanoInicial+ (200*2)) div 2) - (DefTarget div 3); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                   end;
               end;
                              2: // Dificuldade Normal
               begin
                   case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
                       1: //ursula
                       DANO := ((DanoInicial+ (25*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       2: //evgenia 1
                       DANO := ((DanoInicial+ (50*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       3: //evgenia 2
                       DANO := ((DanoInicial+ (75*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       4: //mina 1
                       DANO := ((DanoInicial+ (100*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       5: //mina 2
                       DANO := ((DanoInicial+ (125*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       6: //jardim
                       DANO := ((DanoInicial+ (150*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       7: //prisao
                       DANO := ((DanoInicial+ (175*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       8: //lycan
                       DANO := ((DanoInicial+ (200*3)) div 2) - (DefTarget div 4); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                   end;
               end;
                              3: // Dificuldade Normal
               begin
                   case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
                       1: //ursula
                       DANO := ((DanoInicial+ (25*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       2: //evgenia 1
                       DANO := ((DanoInicial+ (50*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       3: //evgenia 2
                       DANO := ((DanoInicial+ (75*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       4: //mina 1
                       DANO := ((DanoInicial+ (100*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       5: //mina 2
                       DANO := ((DanoInicial+ (125*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       6: //jardim
                       DANO := ((DanoInicial+ (150*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       7: //prisao
                       DANO := ((DanoInicial+ (175*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                       8: //lycan
                       DANO := ((DanoInicial+ (200*4)) div 2) - (DefTarget div 8); //dano do mob dividido por 2 - defesa do personagem dividida por algo
                   end;
               end;

        end;

                         DamageReduction := (DANO div 100) * (Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.GetEquipedItensDamageReduce div 10);
                       Dec(DANO, DamageReduction);
                       inc(DANO, (RandomRange(10, 39) + 7));

//    writeln(Servers[Self.Base.ChannelId].Players[Self.Base.ClientID].);
//   writeln(packet.TargetID.ToString);
//   writeln(packet.AttackerID.ToString);
//   writeln(Servers[Self.Base.ChannelId].mo[packet.AttackerID].Character.Name);
//   writeln(Servers[Self.Base.ChannelId].NPCS[packet.TargetID].Character.Name);
//   writeln(Servers[Self.Base.ChannelId].mobs.TMobS[].Base.Character.Name);
//   writeln(Servers[Self.Base.ChannelId].Players[packet.TargetID].Base.Character.Name);


//writeln(    Servers[Self.Base.ChannelId].Players[Self.Base.ClientID].Base.Character.Name);
//    writeln('teste 1' + Servers[Self.Base.ChannelId].Players[Self.Base.ClientID].Base.Character.CurrentScore.DNFis.ToString);
//    writeln('teste 2' +Servers[Self.Base.ChannelId].Players[Self.Base.ClientID].Base.Character.Name);
//    writeln('teste 3' +Servers[Self.Base.ChannelId].MOBS.TMobS[Self.Base.ClientID].FisAtk.ToString);
//    writeln('teste 4' +Servers[Self.Base.ChannelId].MOBS.TMobS[Self.Base.ClientID].Name);
//                writeln('Atk basico1: ' + DungeonInstances[Self.InstanceID].MOBS[i].Base.PlayerCharacter.Base.CurrentScore.DNFis.ToString);
//            writeln('Atk basico2: '+DungeonInstances[Self.InstanceID].MOBS[i].FisAtk.ToString);


//     Writeln('Ataque do mob fisico ' + Self.Base.);
//    Dano := Self.Base.PlayerCharacter.Base.CurrentScore.DNFis;


//     Writeln('Ataque do mob fisico ' + Self.Base.PlayerCharacter.Base.CurrentScore.DNFis.ToString);
//     Writeln('Ataque do mob magico ' + Self.Base.PlayerCharacter.Base.CurrentScore.DNMag.ToString);
//    if (SkillIDa > 0) then
//      DANO := (Self.Base.PlayerCharacter.Base.CurrentScore.DNFis div 2) +
//        SkillData[SkillIDa].Damage
//    else
//      DANO := Self.Base.PlayerCharacter.Base.CurrentScore.DNFis div 2;
//    case Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//      .Base.GetMobClass of
//      0 .. 3:
//        begin
//          DefTarget := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//            .Base.PlayerCharacter.Base.CurrentScore.DefFis;
////                  WriteLn('Minha defesa fisica' + DefTarget.ToString);
//        end;
//    else
//      DefTarget := Servers[Self.Base.ChannelId].Players[Self.AttackerID]
//        .Base.PlayerCharacter.Base.CurrentScore.DefMag;
////      WriteLn('Minha defesa magica' + DefTarget.ToString);
//    end;





    var AttackResultType: integer;
    var chancededano: string;
    AttackResultType := RandomRange(1, 101);
    case AttackResultType of
      2 .. 15:
        begin
          dnType := TDamageType.Miss;
          chancededano := 'miss';
        end;
      16 .. 30:
        begin
          dnType := TDamageType.Critical;
          chancededano := 'crit';
        end;
      31 .. 100:
        begin
          dnType := TDamageType.Normal;
          chancededano := 'normal';
        end;
    else
      dnType := TDamageType.Normal;
    end;

{
1 2
2 0
3 2
4 0
5 1

1 2  dg evgenia
2 2
3 2
4 2
5 1

1 1  = dg ursula
2 2
3 1
4 2
5 1

          WriteLn('1 ' + Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex.ToString);
          WriteLn('2 ' + Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult.ToString);
          WriteLn('3 ' + Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonID.ToString);
          WriteLn('4 ' + Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonIDDificult.ToString);
          WriteLn('5 ' + Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID.ToString);

          }











          //aqui funciona

//case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult of
//
//
//
//
//
////      Writeln(DungeonInstances[Self].MOBS[i].Base);
//   {$REGION Dungeon normal, URSULA1, EVGENIA2, EVGENIA 23, MINA14, MINA25, JARDIM6}
//       0: // Dificuldade Normal
//       begin
//           case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
//               1: //ursula
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               2: //evgenia 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               3: //evgenia 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               4: //mina 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               5: //mina 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               6: //jardim
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               7: //prisao
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//               8: //lycan
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.02) + RandomRange(1,99));
//           end;
//       end;
//              1: // Dificuldade Dificil
//       begin
//           case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
//               1: //ursula
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               2: //evgenia 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               3: //evgenia 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               4: //mina 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               5: //mina 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               6: //jardim
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               7: //prisao
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//               8: //lycan
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.04) + RandomRange(1,99));
//           end;
//       end;
//              2: // Dificuldade Elite
//       begin
//           case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
//               1: //ursula
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               2: //evgenia 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               3: //evgenia 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               4: //mina 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               5: //mina 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               6: //jardim
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               7: //prisao
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//               8: //lycan
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.06) + RandomRange(1,99));
//           end;
//       end;
//              3: // Dificuldade Infernal
//       begin
//           case Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex of
//               1: //ursula
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               2: //evgenia 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               3: //evgenia 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               4: //mina 1
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               5: //mina 2
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               6: //jardim
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               7: //prisao
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//               8: //lycan
//                   Dano := Trunc((Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.MaxHp * 0.15) + RandomRange(1,99));
//           end;
//       end;
//
//
//   end;
















   if(SecondsBetween(Now, Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.RevivedTime) <= 7) then
  begin
    Exit;
  end;
//   Servers[Self.Base.ChannelId].Players[Self.AttackerID].SendClientMessage(chancededano);
//   Servers[Self.Base.ChannelId].Players[Self.AttackerID].SendClientMessage(Dano.ToString);
// Aplicar o dano ao mob com base na dificuldade
   Self.Base.AttackParseForMobs(0, 0,@Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base, Dano, dnType, AddBuff, Packet.MobAnimation);

    Packet.dnType := dnType;
    Packet.DANO := Dano;

   if (Packet.DANO >= Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.CurHP) then
begin
  // Atualização do HP e status do jogador
  Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.Character.CurrentScore.CurHP := 0;
  Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.IsDead := True;

  // Enviar a animação de morte primeiro
  Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendEffect($0);
  Packet.MobAnimation := 30; // Define a animação de morte para o mob
  Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendToVisible(Packet, Packet.Header.size, True);
//      if(Dungeon_Modificada = 0) then
//      begin
          if(Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.Index > 0) then
            begin
                  if(Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.Members.Count > 1) then
                  begin
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.RemoveMember(Self.AttackerID);
                    // deve tá removendo o player depois que morre automatico, sla
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonID := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonIDDificult := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].InDungeon := False;
                    //Servers[Self.Base.ChannelId].Players[Self.AttackerID].Teleport(TPosition.Create(3399, 564));
                  end
                  else
                  begin
                    DungeonInstances[Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID].InstanceOnline := False;
                    // deve tá removendo o player depois que morre automatico, sla
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonID := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonIDDificult := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID := 0;
                    Servers[Self.Base.ChannelId].Players[Self.AttackerID].InDungeon := False;
                    //Servers[Self.Base.ChannelId].Players[Self.AttackerID].Teleport(TPosition.Create(3399, 564));
                  end;
             end;
//        end
//          else if(Servers[Self.Base.ChannelId].Players[Self.AttackerID].Party.Members.Count = 0) and (Dungeon_Modificada = 1) then
//          begin
//                              DungeonInstances[Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID].InstanceOnline := False;
//                      // deve tá removendo o player depois que morre automatico, sla
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyIndex := 0;
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonLobbyDificult := 0;
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonID := 0;
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonIDDificult := 0;
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].DungeonInstanceID := 0;
//                      Servers[Self.Base.ChannelId].Players[Self.AttackerID].InDungeon := False;
//          end;


      Servers[Self.Base.ChannelId].Players[Self.AttackerID].Base.SendEffect($0);
      Packet.MobAnimation := 30;

      Self.IsAttacked := FALSE;
      Self.LastMoviment := Now;
      Self.MovimentsTime := 0;
      Self.MovedTo := Init;
    end
    else
    begin
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
      Logger.Write('Error at mob Attack Player [' + E.Message + '] b.e: ' +
        E.BaseException.Message + ' user: ' +
        String(Servers[Self.Base.ChannelId].Players[Self.AttackerID]
        .Base.Character.Name) + ' t:' + DateTimeToStr(Now), TlogType.Error);

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

  if (Speed = 25) then
    Packet.Speed := 25
  else
    Packet.Speed := Speed;

  Self.Base.PlayerCharacter.LastPos := Position;

  for i in Self.Base.VisibleMobs do
  begin
    if (i <= MAX_CONNECTIONS) then
    begin
      if (Servers[Self.Base.ChannelId].Players[i].Status < TPlayerStatus.Playing)
      then
        continue;

      Servers[Self.Base.ChannelId].Players[i].SendPacket(Packet,
        Packet.Header.size);
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
//  WriteLn('Instance ID ' + InstanceIDD.ToString);
//  WriteLn('Dificuldade ' + Dificult.ToString);
//  WriteLn('Player 1 ' + Cid1.ToString);
//  WriteLn('Player 2 ' + Cid2.ToString);
//  WriteLn('Channel ID ' + Self.ChannelId.ToString);
//  WriteLn('Instance ID ' + Self.InstanceID.ToString);
//  WriteLn('Members Count ' + SelfParty.Members.Count.ToString);
//  WriteLn('1 ' + DungeonInstances[Self.InstanceID].Index.ToString);
//  WriteLn('2 ' + DungeonInstances[Self.InstanceID].Party.Index.ToString);
//  WriteLn('3 ' + DungeonInstances[Self.InstanceID].Party.Leader.ToString);
//  WriteLn('4 ' + DungeonInstances[Self.InstanceID].Party.Members[0].ToString);
//  WriteLn('5 ' + DungeonInstances[Self.InstanceID].Party.Members[1].ToString);
//  WriteLn('6 ' + DungeonInstances[Self.InstanceID].Party.Members[2].ToString);
//  WriteLn('7 ' + DungeonInstances[Self.InstanceID].Party.Members[3].ToString);
//  WriteLn('8 ' + DungeonInstances[Self.InstanceID].Party.Members[4].ToString);
//  WriteLn('9 ' + DungeonInstances[Self.InstanceID].Party.Members[5].ToString);
//  WriteLn('10 ' + DungeonInstances[Self.InstanceID].Party.RequestId.ToString);
//  WriteLn('11 ' + DungeonInstances[Self.InstanceID].CreateTime.ToString);
//  WriteLn('12 ' + DungeonInstances[Self.InstanceID].DungeonID.ToString);
//  WriteLn('13 ' + DungeonInstances[Self.InstanceID].Dificult.ToString);
//  WriteLn('14 ' + DungeonInstances[Self.InstanceID].InstanceOnline.ToString);



  while (DungeonInstances[Self.InstanceID].InstanceOnline) do
  begin
{$REGION 'Atualização do spawn dentro da dungeon'}
    if (SelfParty.Members.Count > 1) then
    begin
  //  WriteLn('correto ----- aqui iniciou a dg com mais de 1 player');
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

         //    WriteLn('Cid1 = Cid2');
            Servers[Self.ChannelId].Players[Cid1].Base.AddVisible
              (Servers[Self.ChannelId].Players[Cid2].Base);
        //      WriteLn('aqui foi adicionado como visivel o player 2');

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranSpawn(0, Cid1, 0);
   //         WriteLn('Enviando pran dos outros 1');
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranSpawn(1, Cid1, 0);
      //        WriteLn('Enviando pran dos outros 2');
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranSpawn(0, Cid2, 0);
     //         WriteLn('Enviando pran dos outros 3');
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranSpawn(1, Cid2, 0);
     //         WriteLn('Enviando a pran dos outros 4');
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
         //                 WriteLn('deleta pran do cid 1');
            end;

            if (Servers[Self.ChannelId].Players[Cid1]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid1].SendPranUnspawn(1, Cid2);
         //     WriteLn('deleta pran do cid 2');
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran1.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranUnspawn(0, Cid1);
        //      WriteLn('deleta pran do cid 3');
            end;

            if (Servers[Self.ChannelId].Players[Cid2]
              .Account.Header.Pran2.IsSpawned) then
            begin
              Servers[Self.ChannelId].Players[Cid2].SendPranUnspawn(1, Cid1);
          //    WriteLn('deleta pran do cid 4');
            end;

            Servers[Self.ChannelId].Players[Cid1].Base.RemoveVisible
              (Servers[Self.ChannelId].Players[Cid2].Base);
         //     WriteLn('aqui remove o cid 2 de visivel');

            if (Servers[Self.ChannelId].Players[Cid2].Base.IsActive = FALSE)
            then
            begin
      //      WriteLn('se o player cid2 estiver inativo = executa aqui');
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
      //  WriteLn('Dificuldade ' + Dificult.ToString);
     //   WriteLn('InstanceIDD ' + InstanceIDD.ToString);

        for j := Low(DungeonInstances[InstanceIDD].MOBS)
          to High(DungeonInstances[InstanceIDD].MOBS) do
        begin
          if (DungeonInstances[InstanceIDD].MOBS[j].IntName = 0) or
            (DungeonInstances[InstanceIDD].MOBS[j].Base.IsDead) then
            //WriteLn('Aqui é se, o intname = 0 do mob, ou isdead mob'); aqui tá verificando direto se o mob ta vivo ou morto
            continue;

          if (Servers[Self.ChannelId].Players[Cid1]
            .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
            .MOBS[j].Position, DISTANCE_TO_WATCH)) then
          begin
       //   WriteLn('Se cid 1 = last post in range de mob position distance to watch prossegue');
            if not(DungeonInstances[InstanceIDD].MOBS[j].Base.IsDead) then
            begin
                //      WriteLn('Se o mob nao estiver morto prossegue');
              if(DungeonInstances[InstanceIDD].MOBS[j].AttackerID = 0) then
              begin
               //         WriteLn('Se o mob attacker id = 0');
                 if (Servers[Self.ChannelId].Players[Cid1]
                  .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
                    .MOBS[j].Position, 7)) then
                 begin
              //             WriteLn('Se aqui vai mover o mob pra direcao do atacante (player)');
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
                  // WriteLn('aqui é a prox logica pra sair do range do player pra remover aggro');
               DungeonInstances[InstanceIDD].MOBS[j].Position :=
               DungeonInstances[InstanceIDD].MOBS[j].OrigPos;
//               DungeonInstances[Self.InstanceID].MOBS[j].Base.Teleport(DungeonInstances[Self.InstanceID].MOBS[j].OrigPos.X.ToString,DungeonInstances[Self.InstanceID].MOBS[j].OrigPos.Y.ToString);
               //MOVE_TELEPORT
               //DungeonInstances[Self.InstanceID].MOBS[j].Base.SendEffect(70);
                DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 70, MOVE_NORMAL);
                DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, MOVE_TELEPORT);
//                Servers[Self.ChannelId].Players[Cid1].Base.UpdateVisibleList;
//                Servers[Self.ChannelId].Players[Cid2].Base.UpdateVisibleList;
                DungeonInstances[Self.InstanceID].MOBS[j].Base.UpdateVisibleList;
//               var
//                  l: Integer;
//                begin
//                  // Loop de 0 a 23 para acessar os membros da party e chamar UpdateVisibleList
//                  for l := 0 to 23 do
//                  begin
//                    // Acessa o jogador pelo índice i e chama o método UpdateVisibleList
//                    Servers[Self.ChannelId].Players[DungeonInstances[InstanceID].Party.Members[i]].Base.UpdateVisibleList;
//                  end;
//                end;
             //

//               DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 100);
//               DungeonInstances[Self.InstanceID].MOBS[j].
//               DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 100);
                   //       WriteLn('setando o atacante = 0 e voltando pra posicao de origem com 35 de movimento');
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
             // WriteLn('Aqui verifica o last range ');
          if(DungeonInstances[InstanceIDD].MOBS[j].AttackerID = 0) then
          begin
               // WriteLn('se o atacante do mob for 0 faz algo');
             if (Servers[Self.ChannelId].Players[Cid1]
              .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
                .MOBS[j].Position, 7)) then
             begin
                 //  WriteLn('aqui ele verificou se o atacante do mob = 0 e vai provavelmente levar o mob até a loc do player se nao desce pra prox log');
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
                  // WriteLn('aqui é a prox logica pra sair do range do player pra remover aggro');
               DungeonInstances[InstanceIDD].MOBS[j].Position :=
               DungeonInstances[InstanceIDD].MOBS[j].OrigPos;
//               DungeonInstances[Self.InstanceID].MOBS[j].Base.Teleport(DungeonInstances[Self.InstanceID].MOBS[j].OrigPos.X.ToString,DungeonInstances[Self.InstanceID].MOBS[j].OrigPos.Y.ToString);
               //MOVE_TELEPORT
                DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 70, MOVE_NORMAL);
                DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, MOVE_TELEPORT);
//                Servers[Self.ChannelId].Players[DungeonInstances[InstanceID].Party.Leader].Base.UpdateVisibleList;

//                Servers[Self.ChannelId].Players[Cid2].Base.UpdateVisibleList;
                DungeonInstances[Self.InstanceID].MOBS[j].Base.UpdateVisibleList;
             //

//               DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 100);
//               DungeonInstances[Self.InstanceID].MOBS[j].
//               DungeonInstances[Self.InstanceID].MOBS[j].MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,Self.InstanceID, 100);
                   //       WriteLn('setando o atacante = 0 e voltando pra posicao de origem com 35 de movimento');
              DungeonInstances[InstanceIDD].MOBS[j].AttackerID := 0;
             end;
          end;

          if (Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Contains
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID)) then
                //  WriteLn('se conter mobs na visibilidade de client id cid1 provavelmente player, prossegue ');
            continue;

          Servers[Self.ChannelId].Players[Cid1].Base.VisibleMobs.Add
            (DungeonInstances[InstanceIDD].MOBS[j].Base.ClientID);
          DungeonInstances[InstanceIDD].MOBS[j].Base.VisibleMobs.Add(Cid1);
               // WriteLn('adiciona o mob visivel pro player e o mob visivel pro cid 1');

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
  //              WriteLn('aqui removeu o mob da visibilidade do player vice e versa');

          if (Servers[Self.ChannelId].Players[Cid1].Base.RemoveTargetFromList
              (@DungeonInstances[InstanceIDD].MOBS[j].Base)) then
          Servers[Self.ChannelId].Players[Cid1].SendRemoveMobDungeon
            (@DungeonInstances[InstanceIDD].MOBS[j]);
        end;
      end;
    end;
{$ENDREGION}
{$REGION 'Mob Handler dungeon'}
    for i := Low(DungeonInstances[Self.InstanceID].MOBS)
      to High(DungeonInstances[Self.InstanceID].MOBS) do
    begin
      if (DungeonInstances[Self.InstanceID].MOBS[i].Base.ClientID = 0) then
        continue;

      if (DungeonInstances[Self.InstanceID].MOBS[i].Base.IsDead = FALSE) then
      // attack and IA verifications
      begin
{$REGION 'Buffs'}
        UpdatedBuffs := DungeonInstances[Self.InstanceID].MOBS[i]
          .Base.RefreshBuffs;

        if (UpdatedBuffs > 0) then
        begin
          DungeonInstances[Self.InstanceID].MOBS[i].Base.SendRefreshBuffs;
        end;

{$ENDREGION}
{$REGION 'Attack'}
        if (DungeonInstances[Self.InstanceID].MOBS[i].IsAttacked) then
        // se o mob foi atacado, entao...
        begin
          if (DungeonInstances[Self.InstanceID].MOBS[i].Position.Distance
            (Servers[Self.ChannelId].Players[DungeonInstances[Self.InstanceID]
            .MOBS[i].AttackerID].Base.PlayerCharacter.LastPos) <= 5) then
          // atacar se tiver no range
          begin
            if (SecondsBetween(Now, DungeonInstances[Self.InstanceID].MOBS[i]
              .LastMyAttack) >= 2) then
            // se tiver 2 segundos entre
            // um ataque e outro, ele ataca...
            begin
//            writeln('Atk basico1: ' + DungeonInstances[Self.InstanceID].MOBS[i].Base.PlayerCharacter.Base.CurrentScore.DNFis.ToString);
//            writeln('Atk basico2: '+DungeonInstances[Self.InstanceID].MOBS[i].FisAtk.ToString);


              if not(Servers[Self.ChannelId].Players
                [DungeonInstances[Self.InstanceID].MOBS[i].AttackerID]
                .Base.IsDead) then
              // begin
              // Self.IsAttacked := FALSE;
              // Self.AttackerID := 0;
              // end
              // else
              begin
                if (DungeonInstances[Self.InstanceID]
                  .MOBS[i].Base.GetDebuffCount = 0) then
                 begin
                   DungeonInstances[Self.InstanceID].MOBS[i].LastMyAttack := Now;
                   DungeonInstances[Self.InstanceID].MOBS[i].AttackPlayer(DungeonInstances[Self.InstanceID].MOBS[i].FisAtk); // 100 é o valor do dano.
                   Servers[Self.ChannelId].Players[DungeonInstances[Self.InstanceID].MOBS[i].AttackerID].Base.LastReceivedAttack:= now;
                 end;
              end;
            end;
          end
          else
          begin
            Randomize;

            Rand := Random(7);

            DungeonInstances[Self.InstanceID].MOBS[i].Position :=
              Servers[Self.ChannelId].Players
              [DungeonInstances[Self.InstanceID].MOBS[i].AttackerID]
              .Base.Neighbors[Rand].pos;

            DungeonInstances[Self.InstanceID].MOBS[i]
              .MobMove(DungeonInstances[Self.InstanceID].MOBS[i].Position,
              Self.InstanceID, 35);
          end;

          if not(DungeonInstances[InstanceIDD]
              .MOBS[i].Position.InRange(DungeonInstances[InstanceIDD]
              .MOBS[i].OrigPos, 15)) then
           begin
             DungeonInstances[InstanceIDD].MOBS[j].Position :=
             DungeonInstances[InstanceIDD].MOBS[j].OrigPos;

             DungeonInstances[Self.InstanceID].MOBS[j]
              .MobMove(DungeonInstances[Self.InstanceID].MOBS[j].Position,
             Self.InstanceID, 35);

            DungeonInstances[InstanceIDD].MOBS[j].AttackerID := 0;
            DungeonInstances[InstanceIDD].MOBS[i].IsAttacked := False;
           end;
        end
        else
        begin
          for j in DungeonInstances[InstanceIDD].Party.Members.ToArray do
          begin
            if (Servers[Self.ChannelId].Players[j]
              .Base.PlayerCharacter.LastPos.InRange(DungeonInstances[InstanceIDD]
                .MOBS[i].Position, 15)) then
             begin
               DungeonInstances[InstanceIDD].MOBS[i].OrigPos :=
                DungeonInstances[InstanceIDD].MOBS[i].Position;
               DungeonInstances[InstanceIDD].MOBS[i].IsAttacked := True;
               DungeonInstances[InstanceIDD].MOBS[i].AttackerID :=
                 Servers[Self.ChannelId].Players[j].Base.ClientID;
               DungeonInstances[InstanceIDD].MOBS[i].LastMyAttack := Now;
               break;
             end;
          end;
        end;
{$ENDREGION}
      end;
    end;
{$ENDREGION}
    Sleep(FDelay);
  end;
end;

{$ENDREGION}

end.

