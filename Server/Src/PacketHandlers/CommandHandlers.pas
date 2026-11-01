unit CommandHandlers;

interface

uses
  Player, Classes, StrUtils, SysUtils, PlayerFunctions, Log, AltarEvent, Objects;



type
  TPendingPixRequest = record
    FromName: string;   // nome do personagem remetente
    Value: Integer;     // valor enviado
    Active: Boolean;
  end;

    TCommandHandlers = class(TObject)

  public
    class procedure ProcessCommands(var Player: TPlayer; Command: string;
      Parameter: string);
  private
    class procedure AddItem(var Player: TPlayer; Data: TStringList);
    class procedure EquipItem(var Player: TPlayer; Data: TStringList);
    class procedure MoveItem(var Player: TPlayer; Data: TStringList);

    class procedure AddLevel(var Player: TPlayer; Data: TStringList);
    class procedure AddGold(var Player: TPlayer; Data: TStringList);
    class procedure AddExp(var Player: TPlayer; Data: TStringList);
    class procedure AddCash(var Player: TPlayer; Data: TStringList);

    class procedure AddBuff(var Player: TPlayer; Data: TStringList);
    class procedure RemoveBuff(var Player: TPlayer; Data: TStringList);

    class procedure SetRank(var Player: TPlayer; Data: TStringList);

    class procedure MovePlayer(var Player: TPlayer; Data: TStringList);
    class procedure WalkTo(var Player: TPlayer; Data: TStringList);

    class procedure Disconnect(var Player: TPlayer; Parameter: string);
    class procedure SetDnType(Parameter: string);

    class procedure Demage(var Player: TPlayer; Data: TStringList);

    class procedure AddMob(var Player: TPlayer; Data: TStringList);

    class procedure AddEff(var Player: TPlayer; Data: TStringList);
    class procedure RemoveEff(var Player: TPlayer; Data: TStringList);

    class procedure AddGuildExp(var Player: TPlayer; Data: TStringList);

    class procedure AddTitle(var Player: TPlayer; Data: TStringList);
    class procedure UpdateTitle(var Player: TPlayer; Data: TStringList);
    class procedure RemoveTitle(var Player: TPlayer; Data: TStringList);

    class procedure PixEnviar(var Player: TPlayer; Data: TStringList);
    class procedure PixCobrar(var Player: TPlayer; Data: TStringList);
    class procedure PixAceitar(var Player: TPlayer; Data: TStringList);
    class procedure PixRecusar(var Player: TPlayer; Data: TStringList);

    class procedure ProcessPix(var Player: TPlayer; Data: TStringList);

    class procedure SetMarechal(var Player: TPlayer; Data: TStringList);
    class procedure SetArchon(var Player: TPlayer; Data: TStringList; ArchonType: Integer);

    class procedure StartAltarEvent();
    class procedure ResetAltarEvent();
    class procedure StopAltarEvent();


  end;

implementation

uses
  GlobalDefs, ItemFunctions, MiscData, PlayerData, BaseMob, Windows;


class procedure TCommandHandlers.StartAltarEvent;
begin
  InitAltarEvent();
  Logger.Write('[ALTAR] Evento iniciado manualmente pelo GM.', TLogType.ServerStatus);
end;

class procedure TCommandHandlers.ResetAltarEvent;
begin
  InitAltarEvent();
  Logger.Write('[ALTAR] Evento resetado manualmente pelo GM.', TLogType.ServerStatus);
end;

class procedure TCommandHandlers.StopAltarEvent;
begin
  Altar.Active := False;  // se o seu evento usa Altar.Active
  Logger.Write('[ALTAR] Evento parado manualmente pelo GM.', TLogType.ServerStatus);
end;


class procedure TCommandHandlers.ProcessCommands(var Player: TPlayer;
  Command: string; Parameter: string);
var
  CMD: TStringList;
  i: Integer;
begin
  Command := LowerCase(ReplaceStr(Command, '/', ''));

  CMD := TStringList.Create;
  CMD.Delimiter := ' ';
  CMD.StrictDelimiter := True;
  CMD.DelimitedText := Trim(Parameter);

  // ============================
  // /setarchon <guildName>
  // ============================
  if Command = 'setarchon' then
  begin
    if CMD.Count < 1 then
    begin
      Player.SendClientMessage('Uso correto: /setarchon <nome_da_guild>');
      Exit;
    end;
    //SetArchon(Player, CMD[0]);
    Exit;
  end;

  // ======================================================
  // EVENTO ALTAR — START / RESET / STOP
  // ======================================================
  if Command = 'startevent' then
  begin
    if (CMD.Count > 0) and (LowerCase(CMD[0]) = 'altar') then
    begin
      StartAltarEvent();
      Player.SendClientMessage('Evento Altar iniciado.');
    end;
    Exit;
  end;

  if Command = 'resetevent' then
  begin
    if (CMD.Count > 0) and (LowerCase(CMD[0]) = 'altar') then
    begin
      ResetAltarEvent();
      Player.SendClientMessage('Evento Altar reiniciado.');
    end;
    Exit;
  end;

  if Command = 'stopevent' then
  begin
    if (CMD.Count > 0) and (LowerCase(CMD[0]) = 'altar') then
    begin
      StopAltarEvent();
      Player.SendClientMessage('Evento Altar parado.');
    end;
    Exit;
  end;



  // ============================
  // resto do seu ProcessCommands ORIGINAL
  // ============================

  {$REGION 'Mensagem de GM'}
  if (Command = 'msg') then
  begin
    Servers[Player.ChannelIndex].SendServerMsg
      (AnsiString('[' + Player.Base.Character.Name + '] ' +
      ShortString(Parameter)), 32, 16);
    Exit;
  end;

  if (Command = 'msgall') then
  begin
    for i := Low(Servers) to High(Servers) do
    begin
      if not(Servers[i].IsActive) then
        Continue;

      Servers[i].SendServerMsg
        (AnsiString('[' + Player.Base.Character.Name + '] ' +
        ShortString(Parameter)), 32, 16);
    end;

    Exit;
  end;
{$ENDREGION}


{$ENDREGION}
{$REGION 'Addtitle Remove Update'}
    // ---------- Titles ----------
  if Command = 'addtitle' then
  begin
    AddTitle(Player, CMD);
    Exit;
  end;

  if Command = 'updatetitle' then
  begin
    UpdateTitle(Player, CMD);
    Exit;
  end;


  if Command = 'removetitle' then
  begin
    RemoveTitle(Player, CMD);
    Exit;
  end;

{$ENDREGION}
{$REGION 'Outros Commandos'}
  case AnsiIndexStr(Command, [
    'additem', 'equip', 'addexp', 'addgold', 'addcash', 'addbuff', 'addlevel',
    'move', 'walkto', 'dc', 'setdntype', 'setrank', 'removebuff', 'dmg',
    'addmob', 'addeff', 'removeeff', 'moveitem', 'addguildexp', 'addtitle',
    'updatetitle', 'removetitle',

    // PIX
    'pix',          // envia
    'pixcobrar',    // cobra
    'pixaceitar',   // aceita
    'pixrecusar',    // recusa

     'setmarechal',
     'setarchon1',
     'setarchon2',
     'setarchon3',
     'npc'
]) of


    0:
      Self.AddItem(Player, CMD);
    1:
      Self.EquipItem(Player, CMD);
    2:
      Self.AddExp(Player, CMD);
    3:
      Self.AddGold(Player, CMD);
    4:
      Self.AddCash(Player, CMD);
    5:
      Self.AddBuff(Player, CMD);
    6:
      Self.AddLevel(Player, CMD);
    7:
      Self.MovePlayer(Player, CMD);
    8:
      Self.WalkTo(Player, CMD);
    9:
      Self.Disconnect(Player, CMD[0]);
    10:
      Self.SetDnType(CMD[0]);
    11:
      Self.SetRank(Player, CMD);
    12:
      Self.RemoveBuff(Player, CMD);
    13:
      Self.Demage(Player, CMD);
    14:
      Self.AddMob(Player, CMD);
    15:
      Self.AddEff(Player, CMD);
    16:
      Self.RemoveEff(Player, CMD);
    17:
      Self.MoveItem(Player, CMD);
    18:
      Self.AddGuildExp(Player, CMD);

    19:
      Self.AddTitle(Player, CMD);
    20:
      Self.UpdateTitle(Player, CMD);
    21:
      Self.RemoveTitle(Player, CMD);
        // comando 'pix' = enviar
    22: Self.PixEnviar(Player, CMD);

    // comando 'pixcobrar'
    23: Self.PixCobrar(Player, CMD);

    // comando 'pagarpix' = aceitar
    24: Self.PixAceitar(Player, CMD);

    // comando 'recusarpix'
    25: Self.PixRecusar(Player, CMD);

    26: Self.SetMarechal(Player, CMD);

    27: Self.SetArchon(Player, CMD, 1); // tactician

    28: Self.SetArchon(Player, CMD, 2); // judge

    29: Self.SetArchon(Player, CMD, 3);  // treasurer



  else
    begin
      Player.SendClientMessage('Comando inválido.');
    end;

  end;
end;

{$ENDREGION}


class procedure TCommandHandlers.AddItem(var Player: TPlayer;
  Data: TStringList);
var
  Index: WORD;
  Quanty: WORD;
  OtherPlayer: PPlayer;
begin
  if (Data.Count = 3) then
  begin
    try
      if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
      then
      begin
        Player.SendClientMessage('Jogador não encontrado.');
        Exit;
      end;

      try
        Index := Data[1].ToInteger;

        case Index of
          20038, 20039, 20040, 20041:
            begin
              Player.SendClientMessage('Itens banidos.');
              Exit;
            end;
        end;

        if (Data.Count = 2) then
        begin
          Quanty := 1;
        end
        else if (Data.Count = 3) then
        begin
          Quanty := Data[2].ToInteger;
        end;
      except
        Exit;
      end;
    finally
      TItemFunctions.PutItem(OtherPlayer^, Index, Quanty);
    end;

    Exit;
  end;

  try
    Index := Data[0].ToInteger;

    case Index of
      20038, 20039, 20040, 20041:
        begin
          Player.SendClientMessage('Itens banidos.');
          Exit;
        end;
    end;

    if (Data.Count = 1) then
    begin
      Quanty := 1;
    end
    else if (Data.Count = 2) then
    begin
      Quanty := Data[1].ToInteger;
    end
    else
      Exit;
  except
    Exit;
  end;

  TItemFunctions.PutItem(Player, Index, Quanty);
end;


class procedure TCommandHandlers.EquipItem(var Player: TPlayer;
  Data: TStringList);
begin
  Player.SendClientMessage('Ainda não setado.');
end;
class procedure TCommandHandlers.MoveItem(var Player: TPlayer;
  Data: TStringList);
var
  SourceSlot, DestSlot: BYTE;
  aux: TItem;
begin
  try
    if (Data.Count < 2) then
    begin
      Player.SendClientMessage('2 parametros necessários');
      Exit;
    end;

    SourceSlot := Data[0].ToInteger;
    DestSlot := Data[1].ToInteger;

    aux := Player.Character.Base.Inventory[DestSlot];
    Player.Character.Base.Inventory[DestSlot] := Player.Character.Base.Inventory
      [SourceSlot];
    Player.Character.Base.Inventory[SourceSlot] := aux;

    Player.Base.SendRefreshItemSlot(SourceSlot, false);
    Player.Base.SendRefreshItemSlot(DestSlot, false);
  except
    Exit;
  end;
end;
class procedure TCommandHandlers.AddLevel(var Player: TPlayer;
  Data: TStringList);
var
  Level: WORD;
  LevelExp: UInt64;
  AddExp: UInt64;
  OtherPlayer: PPlayer;
  Helper: Integer;
begin
  if (Data.Count = 2) then
  begin
    try
      Level := Data[1].ToInteger;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Level := Data[0].ToInteger;
    except
      Exit;
    end;
  end;

  try
    LevelExp := ExpList[Player.Character.Base.Level + (Level - 1)] + 1;
  except
    LevelExp := High(ExpList);
  end;

  AddExp := LevelExp - UInt64(Player.Character.Base.Exp);

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.AddExp(AddExp, Helper);
    OtherPlayer.Base.SendRefreshLevel;
    Exit;
  end;

  Player.AddExp(AddExp, Helper);
  Player.Base.SendRefreshLevel;
end;

class procedure TCommandHandlers.AddGold(var Player: TPlayer;
  Data: TStringList);
var
  Gold: UInt64;
  OtherPlayer: PPlayer;
begin
  if (Data.Count = 2) then
  begin
    try
      Gold := Data[1].ToInt64;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Gold := Data[0].ToInt64;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.AddGold(Gold);
    Exit;
  end;

  Player.AddGold(Gold);
end;
class procedure TCommandHandlers.AddExp(var Player: TPlayer; Data: TStringList);
var
  Exp: UInt64;
  OtherPlayer: PPlayer;
  Helper: Integer;
begin
  if (Data.Count = 2) then
  begin
    try
      Exp := Data[1].ToInt64;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Exp := Data[0].ToInt64;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.AddExp(Exp, Helper);
    Exit;
  end;

  Player.AddExp(Exp, Helper);
end;
class procedure TCommandHandlers.AddCash(var Player: TPlayer;
  Data: TStringList);
var
  Cash: Cardinal;
  OtherPlayer: PPlayer;
begin
  if (Data.Count = 2) then
  begin
    try
      Cash := Data[1].ToInt64;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Cash := Data[0].ToInt64;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.AddCash(Cash);
    Exit;
  end;

  Player.AddCash(Cash);
end;
class procedure TCommandHandlers.AddBuff(var Player: TPlayer;
  Data: TStringList);
var
  Buff: Cardinal;
  OtherPlayer: PPlayer;
begin
  //Player.SendClientMessage('Comando temporariamente desativado.');
  //Exit;

  if (Data.Count = 2) then
  begin
    try
      Buff := Data[1].ToInt64;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Buff := Data[0].ToInt64;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.Base.AddBuff(Buff);
    Exit;
  end;

  Player.Base.AddBuff(Buff);
end;
class procedure TCommandHandlers.RemoveBuff(var Player: TPlayer;
  Data: TStringList);
var
  Buff: Cardinal;
  OtherPlayer: PPlayer;
begin
  if (Data.Count = 2) then
  begin
    try
      Buff := Data[1].ToInt64;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Buff := Data[0].ToInt64;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.Base.RemoveBuff(Buff);
    Exit;
  end;

  Player.Base.RemoveBuff(Buff);
end;
class procedure TCommandHandlers.SetRank(var Player: TPlayer;
  Data: TStringList);
var
  Rank: BYTE;
  OtherPlayer: PPlayer;
begin
  if not(Player.Account.Header.AccountType = TAccountType.Admin) then
    Exit;

  if (Data.Count = 2) then
  begin
    try
      Rank := Data[1].ToInteger;
    except
      Exit;
    end;
  end
  else
  begin
    try
      Rank := Data[0].ToInteger;
    except
      Exit;
    end;
  end;

  if (Data.Count = 2) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    OtherPlayer.Account.Header.AccountType := TAccountType(Rank);
    Exit;
  end;

  Player.Account.Header.AccountType := TAccountType(Rank);
end;
class procedure TCommandHandlers.MovePlayer(var Player: TPlayer;
  Data: TStringList);
var
  Pos: TPosition;
  OtherPlayer: PPlayer;
begin
  if (Data.Count = 1) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    Player.Teleport(OtherPlayer.Base.PlayerCharacter.LastPos);

    Exit;
  end
  else if (Data.Count = 3) then
  begin
    if not(Servers[Player.ChannelIndex].GetPlayerByName(Data[0], OtherPlayer))
    then
    begin
      Player.SendClientMessage('Jogador não encontrado.');
      Exit;
    end;

    try
      Pos.Create(Data[1].ToSingle, Data[2].ToSingle);
    except
      Pos.Create(50, 50);
    end;

    OtherPlayer.Teleport(Pos);

    Exit;
  end;

  try
    Pos.Create(Data[0].ToSingle, Data[1].ToSingle);
  except
    Pos.Create(50, 50);
  end;

  Player.Teleport(Pos);
end;
class procedure TCommandHandlers.WalkTo(var Player: TPlayer; Data: TStringList);

var
  Pos: TPosition;
  mob: PBaseMob;
begin
  Player.SendClientMessage('Comando temporariamente indisponível.');
  Exit;

  if not(Data.Count = 3) then
  begin
    Player.SendClientMessage('Formato do comando: [Index X Y].');
    Exit;
  end;

  if not(TBaseMob.GetMob(Data[0].ToInteger, Player.ChannelIndex, mob)) then
  begin
    Player.SendClientMessage('Não instanciado.');
    Exit;
  end;

  try
    Pos.
    Create(Data[1].ToSingle, Data[2].ToSingle);
  except
    Pos.Create(000, 000);
  end;

  mob.WalkTo(Pos);
end;
class procedure TCommandHandlers.Disconnect(var Player: TPlayer;
  Parameter: string);
var
  OtherPlayer: PPlayer;
begin
  Player.SendClientMessage('Comando temporariamente indisponivel.');
  Exit;

  if not(Servers[Player.ChannelIndex].GetPlayerByName(Parameter, OtherPlayer))
  then
  begin
    Player.SendClientMessage('Jogador não encontrado.');
    Exit;
  end;

  if (OtherPlayer.Account.Header.AccountType > Player.Account.Header.AccountType)
  then
  begin
    Player.SendClientMessage('Você não tem autoridade pra isso.');
    Exit;
  end;

  // OtherPlayer.SendSignal($7535, $313);

  Servers[Player.ChannelIndex].Disconnect(OtherPlayer.Base.ClientId);
end;
class procedure TCommandHandlers.Demage(var Player: TPlayer; Data: TStringList);
var
  Dano: Cardinal;
begin
  //Player.SendClientMessage('Comando temporariamente desativado!');
  //Exit;

  Dano := 0;
  try
    Dano := Data[0].ToInt64;
  finally
    Player.Base.RemoveHP(Dano, true);
  end;
end;
class procedure TCommandHandlers.AddMob(var Player: TPlayer; Data: TStringList);
begin

end;
class procedure TCommandHandlers.SetDnType(Parameter: string);
begin
  try
    DamageType := Parameter.ToInteger;
  except
    DamageType := $1A;
  end;
end;
class procedure TCommandHandlers.AddEff(var Player: TPlayer; Data: TStringList);
begin
  Player.SendEffect(Data[0].ToInteger());
end;
class procedure TCommandHandlers.RemoveEff(var Player: TPlayer;
  Data: TStringList);
begin
  Player.SendEffect(0);
end;
class procedure TCommandHandlers.AddGuildExp(var Player: TPlayer;
  Data: TStringList);
var
  i: Integer;
  GuildLeveled: Boolean;
  OtherPlayer: PPlayer;
begin
  GuildLeveled := false;

  if (DWORD(Guilds[Player.Character.GuildSlot].GuildLeaderCharIndex)
    = Player.Base.PlayerCharacter.Index) then
  begin
    Inc(Guilds[Player.Character.GuildSlot].Exp, Data[0].ToInteger);

    if (Guilds[Player.Character.GuildSlot].Exp > GuildExpList
      [Guilds[Player.Character.GuildSlot].Level + 1]) then
    begin
      Inc(Guilds[Player.Character.GuildSlot].Level);

      GuildLeveled := true;
    end;

    for i := 0 to 127 do
    begin
      if (Guilds[Player.Character.GuildSlot].Members[i].CharIndex = 0) then
        Continue;

      OtherPlayer := Servers[Player.ChannelIndex].GetPlayer
        (String(Guilds[Player.Character.GuildSlot].Members[i].Name));

      if (OtherPlayer.IsInstantiated) then
      begin
        OtherPlayer.SendGuildInfo;

        if (GuildLeveled) then
        begin
          OtherPlayer.SendClientMessage('A sua guild subiu de level!');
        end;
      end;
    end;
  end
  else
  begin
    Player.SendClientMessage('Você não é o lider de sua guild.');
  end;
end;
class procedure TCommandHandlers.AddTitle(var Player: TPlayer; Data: TStringList);
begin
  if Data.Count < 2 then
  begin
    Player.SendClientMessage('Uso correto: /addtitle <id> <level>');
    Exit;
  end;

  Player.AddTitle(StrToIntDef(Data[0], 0), StrToIntDef(Data[1], 1));
end;

class procedure TCommandHandlers.UpdateTitle(var Player: TPlayer; Data: TStringList);
var
  TitleID, Level: Integer;
begin
  // Verifica se existem parâmetros suficientes
  if Data.Count < 2 then
  begin
    Player.SendClientMessage('Uso correto: /updatetitle <id> <level>');
    Exit;
  end;

  // Converte o ID
  if not TryStrToInt(Data[0], TitleID) then
  begin
    Player.SendClientMessage('ID inválido.');
    Exit;
  end;

  // Converte o Level
  if not TryStrToInt(Data[1], Level) then
  begin
    Player.SendClientMessage('Level inválido.');
    Exit;
  end;

  // Chama a função real do jogador
  Player.UpdateTitleLevel(TitleID, Level);
end;

class procedure TCommandHandlers.RemoveTitle(var Player: TPlayer; Data: TStringList);
var
  TitleID: Integer;
begin
  // Verifica se o ID foi passado
  if Data.Count < 1 then
  begin
    Player.SendClientMessage('Uso correto: /removetitle <id>');
    Exit;
  end;

  // Converte o ID
  if not TryStrToInt(Data[0], TitleID) then
  begin
    Player.SendClientMessage('ID inválido.');
    Exit;
  end;

  // Chama a função real do jogador
  Player.RemoveTitle(TitleID);
end;

{$REGION 'Comandos PET'}



{$ENDREGION}
{$REGION 'Area Pix'}

class procedure TCommandHandlers.PixEnviar(var Player: TPlayer; Data: TStringList);
var
  TargetName: string;
  Value: Integer;
  Target: PPlayer;
begin
  if Data.Count < 2 then
  begin
    Player.SendClientMessage('Use: /pix <nick> <valor>');
    Exit;
  end;

  TargetName := Trim(Data[0]);
  Value := StrToIntDef(Data[1], 0);

  if Value <= 0 then
  begin
    Player.SendClientMessage('Valor inválido.');
    Exit;
  end;

  // ouro insuficiente
  if Player.Base.Character.Gold < Value then
  begin
    Player.SendClientMessage('Gold insuficiente.');
    Exit;
  end;

  // localizar destinatário
  Target := TPlayerFunctions.FindBySQLName(TargetName);

  if Target = nil then
  begin
    Player.SendClientMessage('Jogador não encontrado.');
    Exit;
  end;

  // transfere imediatamente
  Player.Base.Character.Gold := Player.Base.Character.Gold - Value;
  Target.Base.Character.Gold := Target.Base.Character.Gold + Value;

  Player.SendClientMessage(
  'PIX enviado para ' + TargetName +
  ' por ' + IntToStr(Value) + ' Gold'
);


  Target.SendClientMessage(
  'Você recebeu ' + IntToStr(Value) +
  ' Gold via PIX de ' + string(Player.Base.Character.Name) + '!'
);


end;

class procedure TCommandHandlers.PixCobrar(var Player: TPlayer; Data: TStringList);
var
  Target: PPlayer;
  Value: Integer;
begin
  if Data.Count < 2 then
  begin
    Player.SendClientMessage('Use: /pixcobrar <nome> <valor>');
    Exit;
  end;

  Target := TPlayerFunctions.FindBySQLName(Data[0]);

  if Target = nil then
  begin
    Player.SendClientMessage('Jogador não encontrado.');
    Exit;
  end;

  Value := StrToIntDef(Data[1], 0);
  if Value <= 0 then
  begin
    Player.SendClientMessage('Valor inválido.');
    Exit;
  end;

  Target.PendingPixRequest.FromName := string(Player.Base.Character.Name);
  Target.PendingPixRequest.Value := Value;
  Target.PendingPixRequest.Active := True;

  Player.SendClientMessage('Cobrança enviada.');
  Target.SendClientMessage(
    'Você recebeu uma cobrança PIX de ' +
    string(Player.Base.Character.Name) +
    ' no valor de ' + IntToStr(Value) +
    '. Use /pix aceitar ou /pix recusar.'
  );
end;

class procedure TCommandHandlers.PixAceitar(var Player: TPlayer; Data: TStringList);
var
  Sender: PPlayer;
  Value: Integer;
begin
  if not Player.PendingPixRequest.Active then
  begin
    Player.SendClientMessage('Nenhuma cobrança PIX.');
    Exit;
  end;

  Sender := TPlayerFunctions.FindBySQLName(Player.PendingPixRequest.FromName);

  if Sender = nil then
  begin
    Player.SendClientMessage('Remetente offline.');
    Player.PendingPixRequest.Active := False;
    Exit;
  end;

  Value := Player.PendingPixRequest.Value;

  if Player.Base.Character.Gold < Value then
  begin
    Player.SendClientMessage('Gold insuficiente.');
    Exit;
  end;

  Player.Base.Character.Gold := Player.Base.Character.Gold - Value;
  Sender.Base.Character.Gold := Sender.Base.Character.Gold + Value;

  Player.SendClientMessage('PIX pago com sucesso.');
  Sender.SendClientMessage(string(Player.Base.Character.Name) +
    ' pagou sua cobrança de ' + IntToStr(Value) + '.');

  Player.PendingPixRequest.Active := False;
end;

class procedure TCommandHandlers.PixRecusar(var Player: TPlayer; Data: TStringList);
var
  Sender: PPlayer;
begin
  if not Player.PendingPixRequest.Active then
  begin
    Player.SendClientMessage('Nenhuma cobrança PIX.');
    Exit;
  end;

  Sender := TPlayerFunctions.FindBySQLName(Player.PendingPixRequest.FromName);

  if Sender <> nil then
    Sender.SendClientMessage('Sua cobrança PIX foi recusada por ' +
      string(Player.Base.Character.Name));

  Player.SendClientMessage('PIX recusado.');

  Player.PendingPixRequest.Active := False;
end;

class procedure TCommandHandlers.ProcessPix(var Player: TPlayer; Data: TStringList);
var
  SubCmd: string;
begin
  if Data.Count = 0 then
  begin
    Player.SendClientMessage('Use: /pix <nick> <valor>');
    Player.SendClientMessage('Ou: /pix cobrar <nick> <valor>');
    Player.SendClientMessage('Ou: /pix aceitar | /pix recusar');
    Exit;
  end;

  SubCmd := LowerCase(Data[0]);

  // /pix aceitar
  if SubCmd = 'aceitar' then
  begin
    Self.PixAceitar(Player, Data);
    Exit;
  end;

  // /pix recusar
  if SubCmd = 'recusar' then
  begin
    Self.PixRecusar(Player, Data);
    Exit;
  end;

  // /pix cobrar nick valor
  if SubCmd = 'cobrar' then
  begin
    Data.Delete(0); // remove a palavra "cobrar"
    Self.PixCobrar(Player, Data);
    Exit;
  end;

  // /pix nick valor  (envio)
  Self.PixEnviar(Player, Data);
end;

class procedure TCommandHandlers.SetMarechal(var Player: TPlayer; Data: TStringList);
var
  GuildSlot: Integer;
  GuildID: Integer;
  NationID: Integer;
begin
  if Data.Count < 1 then
  begin
    Player.SendClientMessage('Uso: /setmarechal <guildSlot>');
    Exit;
  end;

  GuildSlot := StrToIntDef(Data[0], -1);

  if (GuildSlot < Low(Guilds)) or (GuildSlot > High(Guilds)) then
  begin
    Player.SendClientMessage('GuildSlot inválido.');
    Exit;
  end;

  if Guilds[GuildSlot].Index = 0 then
  begin
    Player.SendClientMessage('Não existe guild nesse slot.');
    Exit;
  end;

  NationID := Ord(Player.Account.Header.Nation);
  GuildID := Guilds[GuildSlot].Index;

  Nations[NationID - 1].MarechalGuildID := GuildID;
  Nations[NationID - 1].SaveNation;

  Player.SendClientMessage('A guilda foi definida como Marechal.');

  Servers[Player.ChannelIndex].SendServerMsg(
    AnsiString('[NATION] A guilda ' + String(Guilds[GuildSlot].Name) + ' agora é a Marechal!'),
    32, 16
  );
end;

class procedure TCommandHandlers.SetArchon(var Player: TPlayer; Data: TStringList; ArchonType: Integer);
var
  GuildSlot: Integer;
  GuildID: Integer;
  NationID: Integer;
begin
  if Data.Count < 1 then
  begin
    Player.SendClientMessage('Uso: /setarchon' + ArchonType.ToString + ' <guildSlot>');
    Exit;
  end;

  GuildSlot := StrToIntDef(Data[0], -1);

  if (GuildSlot < Low(Guilds)) or (GuildSlot > High(Guilds)) then
  begin
    Player.SendClientMessage('GuildSlot inválido.');
    Exit;
  end;

  if Guilds[GuildSlot].Index = 0 then
  begin
    Player.SendClientMessage('Não existe guild nesse slot.');
    Exit;
  end;

  NationID := Ord(Player.Account.Header.Nation);
  GuildID := Guilds[GuildSlot].Index;

  case ArchonType of
    1: Nations[NationID - 1].TacticianGuildID := GuildID;
    2: Nations[NationID - 1].JudgeGuildID     := GuildID;
    3: Nations[NationID - 1].TreasurerGuildID := GuildID;
  end;

  Nations[NationID - 1].SaveNation;

  Player.SendClientMessage('Archon atualizado.');

  Servers[Player.ChannelIndex].SendServerMsg(
    AnsiString('[NATION] A guilda ' + String(Guilds[GuildSlot].Name) +
      ' agora é Archon tipo ' + ArchonType.ToString + '!'),
    32, 16
  );
end;

{class procedure TCommandHandlers.SetArchon(
  var Player: TPlayer;
  Data: TStringList
);
var
  GuildSlot: Integer;
  GuildID: Integer;
  NationID: Integer;
begin
  // Verificar permissão
  if Player.Account.Header.AccountType <> TAccountType.Admin then
  begin
    Player.SendClientMessage('Acesso negado.');
    Exit;
  end;

  // Verificar sintaxe
  if Data.Count < 2 then
  begin
    Player.SendClientMessage('Use: /setarchon <slot 1..3> <guildID>');
    Exit;
  end;

  try
    GuildSlot := StrToInt(Data[0]); // 1,2,3 (Tactician, Judge, Treasurer)
    GuildID   := StrToInt(Data[1]);
  except
    Player.SendClientMessage('Valores inválidos.');
    Exit;
  end;

  // Validar slot
  if not (GuildSlot in [1,2,3]) then
  begin
    Player.SendClientMessage('Slot deve ser 1=Tactician, 2=Judge, 3=Treasurer');
    Exit;
  end;

  // Determinar nação do GM
  NationID := Ord(Player.Account.Header.Nation);
  if NationID <= 0 then
  begin
    Player.SendClientMessage('Sua conta não tem nação definida.');
    Exit;
  end;

  // Aplicar no banco de dados interno
  case GuildSlot of
    1: Nations[NationID - 1].TacticianGuildID := GuildID;
    2: Nations[NationID - 1].JudgeGuildID     := GuildID;
    3: Nations[NationID - 1].TreasurerGuildID := GuildID;
  end;

  // Salvar
  Nations[NationID - 1].SaveNation;

  Player.SendClientMessage('Archon atualizado com sucesso!');
end; }



{$ENDREGION}
end.

