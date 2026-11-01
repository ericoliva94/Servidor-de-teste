unit GuildData;
interface
uses
  Windows, SysUtils, MiscData, System.Classes, DateUtils, AnsiStrings,
  PartyData;

  procedure LoadGuildMembersFromSQL(GuildIndex, Slot: Integer);


{$OLDTYPELAYOUT ON}
const
  MaxGuildPlayersOfLevel: ARRAY [0 .. 9] OF BYTE = (32, 48, 64, 80, 96, 112,
    128, 128, 128, 128);
type
  PPlayerFromGuild = ^TPlayerFromGuild;
  TPlayerFromGuild = packed record
    CharIndex: DWORD;
    Name: ARRAY [0 .. 19] OF AnsiChar;
    Rank: BYTE;
    ClassInfo: BYTE;
    Level: BYTE;
    Logged: Boolean;
    LastLogin: DWORD;
    Unk: ARRAY [0 .. 31] OF BYTE;
  end;
type
  TGuildNotice = packed record
    Null: WORD;
    Text: ARRAY [0 .. 33] OF AnsiChar;
  end;
type
  TGuildAllyItem = packed record
    Index: WORD;
    Name: ARRAY [0 .. 17] OF AnsiChar;
  end;
type
  TGuildAlly = packed record
    Leader: WORD;
    Guilds: ARRAY [0 .. 3] OF TGuildAllyItem;
  end;
type
  TGuildChest = packed record
    Items: ARRAY [0 .. 49] OF TITEM;
    Gold: UInt64;
  end;
type
  TGuildRankConfig = packed record
    Invite, Kick, UseGuildSkill, EditNotices, OpenGWH, UseGWH: Boolean;
  end;
type
  PGuild = ^TGuild;
  TGuild = record

  private type
    TCloseChestThread = class(TThread)
    public
      Guild: PGuild;
    protected
      procedure Execute; override;
    end;
  public
    Index, Slot, Nation, Exp, Level, TotalMembers: DWORD;
    BravurePoints, SkillPoints: DWORD;
    Promote: Boolean;
    GuildLeaderCharIndex: Integer;
    Name: ARRAY [0 .. 18] OF AnsiChar;
    Notices: ARRAY [0 .. 2] OF TGuildNotice;
    RanksConfig: ARRAY [0 .. 4] OF BYTE;
    Site: ARRAY [0 .. 37] OF AnsiChar;
    Ally: TGuildAlly;
    Members: ARRAY [0 .. 127] OF TPlayerFromGuild;
    Chest: TGuildChest;
    MemberInChest: Integer;
    LastChestActionDate: TDateTime;
    CloseChestThread: TCloseChestThread;
    function CreateGuild(GuildName: ARRAY OF AnsiChar;
      GuildNation, ChannelId: Integer; Party: PParty; SelfPlayer: pointer): Boolean;
    function AddMember(CharIndex, ChannelId: DWORD; Rank: BYTE;
      RecruterCharIndex: DWORD): Boolean;
    function RemoveMember(CharIndex: DWORD; Expulsion: Boolean): Boolean;
    function ChangeRank(CharIndex, Rank: DWORD): Boolean;
    function UpdateLevel(CharIndex, Level: DWORD): Boolean;
    function GetFreeMemberId: Integer;
    function GetAllyGuildCount: Integer;
    function FindMemberFromCharIndex(CharIndex: DWORD): Integer;
    function UpdateRanksConfig(RanksConfig: ARRAY OF BYTE): Boolean;
    function UpdateNotices(Notice1, Notice2,
      Notice3: ARRAY OF AnsiChar): Boolean;
    function UpdateSite(FSite: ARRAY OF AnsiChar): Boolean;
    function OpenChest(CharIndex: DWORD; ChannelId: DWORD): Boolean;
    function CloseChest: Boolean;
    function SendChatMessage(const Packet; Size: Integer): Boolean;
    function SendChatAllyMessage(const Packet; Size: Integer): Boolean;
    function SendMemberLogin(CharIndex: DWORD): Boolean;
    function SendMemberLogout(CharIndex: DWORD): Boolean;
    function GetRankConfig(Rank: Integer): TGuildRankConfig;

  end;
{$OLDTYPELAYOUT OFF}
implementation
uses
  Functions, GlobalDefs, Player, Log, SQL;

procedure LoadGuildMembersFromSQL(GuildIndex, Slot: Integer);

var
  Q: TQuery;
  R: Integer;
begin
  Q := TQuery.Create(
        AnsiString(MYSQL_SERVER),
        MYSQL_PORT,
        AnsiString(MYSQL_USERNAME),
        AnsiString(MYSQL_PASSWORD),
        AnsiString(MYSQL_DATABASE)
      );

  try
    Q.SetQuery(Format(
      'SELECT char_index, name, player_rank, classinfo, level, logged, last_login ' +
      'FROM guilds_players WHERE guild_index = %d',
      [GuildIndex]
    ));

    Q.Run(True);

    R := 0;

    Guilds[Slot].TotalMembers := 0;

    while not Q.Query.Eof do
    begin
      ZeroMemory(@Guilds[Slot].Members[R], SizeOf(TPlayerFromGuild));

      Guilds[Slot].Members[R].CharIndex  := Q.Query.FieldByName('char_index').AsInteger;

      Move(
        PAnsiChar(AnsiString(Q.Query.FieldByName('name').AsString))^,
        Guilds[Slot].Members[R].Name,
        Length(Q.Query.FieldByName('name').AsString)
      );

      Guilds[Slot].Members[R].Rank      := Q.Query.FieldByName('player_rank').AsInteger;
      Guilds[Slot].Members[R].ClassInfo := Q.Query.FieldByName('classinfo').AsInteger;
      Guilds[Slot].Members[R].Level     := Q.Query.FieldByName('level').AsInteger;
      Guilds[Slot].Members[R].Logged    := Q.Query.FieldByName('logged').AsInteger = 1;
      Guilds[Slot].Members[R].LastLogin := Q.Query.FieldByName('last_login').AsInteger;

      Inc(Guilds[Slot].TotalMembers);
      Inc(R);

      Q.Query.Next;
    end;

  finally
    Q.Free;
  end;
end;

procedure TGuild.TCloseChestThread.Execute;
var
  I: Integer;
  Player: PPlayer;
begin
  while not(Self.Guild.MemberInChest < 0) and
    not(Self.Guild.MemberInChest > High(Self.Guild.Members)) do
  begin
    if MinutesBetween(Now, Self.Guild.LastChestActionDate) >= 2 then
    begin
      for I := Low(Servers) to High(Servers) do
        if Servers[I].GetPlayerByCharIndex
          (Self.Guild.Members[Self.Guild.MemberInChest].CharIndex, Player) then
        begin
          Player.SendSignal(Player.Character.Base.ClientId, $F2F);
          Break;
        end;
      Break;
    end;
    Sleep(50);
  end;
  Self.Terminate;
end;


function TGuild.CreateGuild(GuildName: ARRAY OF AnsiChar;
  GuildNation, ChannelId: Integer; Party: PParty; SelfPlayer: pointer): Boolean;
var
  PartyMemberClientId: WORD;
  MemberId, GuildCnt: Integer;
  xPlayer, SPlayer: PPlayer;
  I, M: Integer;
  Member: PPlayerFromGuild;
  SQLComp: TQuery;
begin
  Result := False;
  SPlayer := SelfPlayer;

  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  if not(SQLComp.Query.Connection.Connected) then
  begin
    Logger.Write('Falha de conexão individual com mysql.[CreateGuild]',
      TlogType.Warnings);
    Logger.Write('PERSONAL MYSQL FAILED LOAD.[CreateGuild]', TlogType.Error);
    SQLComp.Destroy;
    Exit;
  end;

  try
    // Pega próximo id (só pra ter um index temporário dentro do servidor)
    SQLComp.Query.SQL.Clear;
    SQLComp.Query.SQL.Add('SELECT coalesce(MAX(id), 0) FROM guilds');
    SQLComp.Run(True);
    if (SQLComp.Query.RecordCount > 0) then
      Self.Index := SQLComp.Query.Fields[0].AsInteger + 1
    else
      Self.Index := 1;

    ZeroMemory(@Self.Members, SizeOf(Self.Members));
    Self.TotalMembers := 0;

    Move(GuildName, Self.Name[0], Length(GuildName));
    Self.Ally.Leader := Self.Index;
    Self.Ally.Guilds[0].Index := Self.Index;
    Move(GuildName, Self.Ally.Guilds[0].Name, SizeOf(Self.Ally.Guilds[0].Name));
    Self.RanksConfig[4] := $FF;
    Self.Nation := GuildNation;
    Self.MemberInChest := $FF;
    Self.Exp := 1;
    Self.Level := 1;
    Self.BravurePoints := 0;
    Self.SkillPoints := 0;
    Self.Promote := False;
    Self.Chest.Gold := 0;

    // Monta Members[] a partir da Party
    for PartyMemberClientId in Party.Members do
    begin
      if PartyMemberClientId = 0 then
        Continue;

      xPlayer := @Servers[ChannelId].Players[PartyMemberClientId];
      if xPlayer.Base.Character.CharIndex = 0 then
        Continue;

      MemberId := Self.GetFreeMemberId;
      if MemberId < 0 then
        Break;

      ZeroMemory(@Self.Members[MemberId], SizeOf(Self.Members[MemberId]));
      Self.Members[MemberId].CharIndex := xPlayer.Base.Character.CharIndex;
      Self.Members[MemberId].ClassInfo := xPlayer.Character.Base.ClassInfo;
      Self.Members[MemberId].Level := xPlayer.Character.Base.Level;
      Self.Members[MemberId].Logged := True;
      Self.Members[MemberId].LastLogin := DateTimeToUnix(Now);
      AnsiStrings.StrCopy(Self.Members[MemberId].Name, xPlayer.Character.Base.Name);

      if PartyMemberClientId = Party.Leader then
      begin
        Self.Members[MemberId].Rank := 4;
        Self.GuildLeaderCharIndex := xPlayer.Character.Base.CharIndex;
      end
      else
        Self.Members[MemberId].Rank := 0;

      xPlayer.Character.Base.GuildIndex := Self.Index;
      xPlayer.Character.GuildSlot := Self.Slot;

      Inc(Self.TotalMembers);
    end;

    // Salva guild na tabela GUILDS
    SQLComp.SetQuery(
      Format('INSERT INTO guilds (slot, name, nation, experience,' +
      ' level, totalmembers, bravurepoints, skillpoints, promote, notice1, notice2,' +
      ' notice3, site, rank1, rank2, rank3, rank4, rank5, ally_leader,' +
      ' guild_ally1_index, guild_ally1_name, guild_ally2_index, guild_ally2_name,' +
      ' guild_ally3_index, guild_ally3_name, guild_ally4_index, guild_ally4_name,' +
      ' storage_gold, leader_char_index) VALUES (%d, %s, %d, %d,' +
      ' %d, %d, %d, %d, %d, %s, %s,' +
      ' %s, %s, %d, %d, %d, %d, %d, %d,' +
      ' %d, %s, %d, %s,' +
      ' %d, %s, %d, %s,' +
      ' %d, %d)',
      [Self.Slot, QuotedStr(String(Self.Name)),
       Self.Nation, Self.Exp, Self.Level, Self.TotalMembers,
       Self.BravurePoints, Self.SkillPoints, Self.Promote.ToInteger,
       QuotedStr(String(Self.Notices[0].Text)),
       QuotedStr(String(Self.Notices[1].Text)),
       QuotedStr(String(Self.Notices[2].Text)),
       QuotedStr(String(Self.Site)), Self.RanksConfig[0],
       Self.RanksConfig[1], Self.RanksConfig[2], Self.RanksConfig[3],
       Self.RanksConfig[4], Self.Ally.Leader,
       Self.Ally.Guilds[0].Index, QuotedStr(String(Self.Ally.Guilds[0].Name)),
       Self.Ally.Guilds[1].Index, QuotedStr(String(Self.Ally.Guilds[1].Name)),
       Self.Ally.Guilds[2].Index, QuotedStr(String(Self.Ally.Guilds[2].Name)),
       Self.Ally.Guilds[3].Index, QuotedStr(String(Self.Ally.Guilds[3].Name)),
       Self.Chest.Gold, Self.GuildLeaderCharIndex]));
    SQLComp.Run(False);

    // Pega o ID real (auto_increment)
    SQLComp.Query.SQL.Clear;
    SQLComp.Query.SQL.Add(
      Format('SELECT id FROM guilds WHERE leader_char_index=%d AND name=%s',
      [Self.GuildLeaderCharIndex, QuotedStr(String(Self.Name))]));
    SQLComp.Run(True);
    if not SQLComp.Query.IsEmpty then
      Self.Index := SQLComp.Query.Fields[0].AsInteger;

    // Salva membros em guilds_players + characters.guildindex
    for M := Low(Self.Members) to High(Self.Members) do
    begin
      if (Self.Members[M].CharIndex = 0) then
        Continue;

      Member := @Self.Members[M];

      // Garante consistência: remove lixo anterior desse char
      SQLComp.SetQuery(
        Format('DELETE FROM guilds_players WHERE char_index=%d',
        [Member^.CharIndex]));
      SQLComp.Run(False);

      SQLComp.SetQuery(
        Format('INSERT INTO guilds_players (guild_index, char_index, name, player_rank,' +
        ' classinfo, level, logged, last_login) VALUES (%d, %d, %s, %d,' +
        ' %d, %d, %d, %d)',
        [Self.Index, Member^.CharIndex,
         QuotedStr(String(Member^.Name)), Member^.Rank, Member^.ClassInfo,
         Member^.Level, Member^.Logged.ToInteger, Member^.LastLogin]));
      SQLComp.Run(False);

      SQLComp.SetQuery(
        Format('UPDATE characters SET guildindex=%d WHERE id=%d',
        [Self.Index, Member^.CharIndex]));
      SQLComp.Run(False);

      // Atualiza players online
      if Servers[ChannelId].GetPlayerByCharIndex(Member^.CharIndex, xPlayer) then
      begin
        xPlayer.SendGuildInfo;
        xPlayer.RefreshPlayerInfos;
        xPlayer.SendP152;
        xPlayer.SendGuildPlayers;
      end;
    end;

    Result := True;
  except
    on E: Exception do
    begin
      Logger.Write('guild_save_create_error: ' + E.Message, TLogType.Error);
    end;
  end;

  SQLComp.Destroy;
end;


function TGuild.AddMember(CharIndex: DWORD; ChannelId: DWORD; Rank: BYTE;
  RecruterCharIndex: DWORD): Boolean;
var
  I, P: Integer;
  MemberId: Integer;
  Player: PPlayer;
  AddBuff: BYTE;
  SQLComp: TQuery;
begin
  AddBuff := 0;
  Result := False;

  // Conferência do recrutador
  if not Servers[ChannelId].GetPlayerByCharIndex(RecruterCharIndex, Player) then
    Exit;

  // Conferência do player adicionado
  if not Servers[ChannelId].GetPlayerByCharIndex(CharIndex, Player) then
    Exit;

  MemberId := Self.GetFreeMemberId;
  if MemberId < 0 then
  begin
    Player.SendClientMessage('A guild não tem mais espaço.');
    Exit;
  end;

  if (Player.IsGradeMarshal) then
    AddBuff := 1
  else if (Player.IsGradeArchon) then
    AddBuff := 2;

  ZeroMemory(@Members[MemberId], SizeOf(Members[MemberId]));
  Self.Members[MemberId].CharIndex   := Player.Base.Character.CharIndex;
  Move(Player.Character.Base.Name, Self.Members[MemberId].Name, 16);
  Self.Members[MemberId].Rank        := Rank;
  Self.Members[MemberId].ClassInfo   := Player.Character.Base.ClassInfo;
  Self.Members[MemberId].Level       := Player.Character.Base.Level;
  Self.Members[MemberId].Logged      := True;
  Self.Members[MemberId].LastLogin   := DateTimeToUnix(Now);

  Player.Character.Base.GuildIndex := Self.Index;
  Player.Character.GuildSlot       := Self.Slot;

  Player.SendGuildInfo;
  Player.RefreshPlayerInfos;
  Player.SendP152;
  Player.SendGuildPlayers;

  case AddBuff of
    1: Player.Base.AddBuffWhenEntering(CAVALEIROS_MARECHAL, Now);
    2: Player.Base.AddBuffWhenEntering(CAVALEIROS_ARCHON, Now);
  end;

  // Notificar restante dos membros em memória
  for I := Low(Members) to High(Members) do
    if (Members[I].CharIndex > 0) and
       (Members[I].CharIndex <> Self.Members[MemberId].CharIndex) then
      for P := Low(Servers) to High(Servers) do
        if Servers[P].GetPlayerByCharIndex(Members[I].CharIndex, Player) then
        begin
          Player.AddPlayerToGuild(Self.Members[MemberId]);
          Break;
        end;

  // Atualiza contador
  Inc(Self.TotalMembers);

  // === SINCRONIZAÇÃO COM MYSQL ===
  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  try
    if not(SQLComp.Query.Connection.Connected) then
    begin
      Logger.Write('Falha MYSQL em AddMember', TlogType.Warnings);
      Exit;
    end;

    // Remove lixo anterior desse char (caso ele já tenha pertencido a outra guild)
    SQLComp.SetQuery(
      Format('DELETE FROM guilds_players WHERE char_index=%d',
      [Self.Members[MemberId].CharIndex]));
    SQLComp.Run(False);

    // Insere o registro correto
    SQLComp.SetQuery(
      Format('INSERT INTO guilds_players (guild_index, char_index, name, player_rank,' +
      ' classinfo, level, logged, last_login) VALUES (%d, %d, %s, %d,' +
      ' %d, %d, %d, %d)',
      [Self.Index,
       Self.Members[MemberId].CharIndex,
       QuotedStr(String(Self.Members[MemberId].Name)),
       Self.Members[MemberId].Rank,
       Self.Members[MemberId].ClassInfo,
       Self.Members[MemberId].Level,
       1,
       Self.Members[MemberId].LastLogin]));
    SQLComp.Run(False);

    // Atualiza totalmembers da guild
    SQLComp.SetQuery(
      Format('UPDATE guilds SET totalmembers=%d WHERE id=%d',
      [Self.TotalMembers, Self.Index]));
    SQLComp.Run(False);

    // Atualiza guildindex do personagem
    SQLComp.SetQuery(
      Format('UPDATE characters SET guildindex=%d WHERE id=%d',
      [Self.Index, Self.Members[MemberId].CharIndex]));
    SQLComp.Run(False);
  except
    on E: Exception do
      Logger.Write('Erro AddMember MYSQL: ' + E.Message, TLogType.Error);
  end;
  SQLComp.Destroy;

  Result := True;
end;


function TGuild.RemoveMember(CharIndex: DWORD; Expulsion: Boolean): Boolean;
var
  MemberId: Integer;
  I, P: Integer;
  Player: PPlayer;
  RemoveBuff: BYTE;
  SQLComp: TQuery;
  OldGuildIndex: Integer;
begin
  Result := False;
  RemoveBuff := 0;
  MemberId := Self.FindMemberFromCharIndex(CharIndex);
  if MemberId < 0 then
    Exit;

  OldGuildIndex := Self.Index;

  for I := Low(Servers) to High(Servers) do
    if Servers[I].GetPlayerByCharIndex(CharIndex, Player) then
    begin
      Player.GetOutGuild(Expulsion);
      if (Player.IsGradeMarshal) then
        RemoveBuff := 1
      else if (Player.IsGradeArchon) then
        RemoveBuff := 2;
      Break;
    end;

  // Notifica restantes
  for I := Low(Members) to High(Members) do
    if (Members[I].CharIndex > 0) and (Members[I].CharIndex <> CharIndex) then
      for P := Low(Servers) to High(Servers) do
        if Servers[P].GetPlayerByCharIndex(Members[I].CharIndex, Player) then
        begin
          Player.SendData(Player.Base.ClientId, $F1B, CharIndex);
          Break;
        end;

  if Members[MemberId].Rank = 4 then
  begin
    // Líder saiu: guild será destruída
    for I := Low(Members) to High(Members) do
      if (Members[I].CharIndex > 0) and (Members[I].CharIndex <> CharIndex) then
        Self.RemoveMember(Members[I].CharIndex, True);

   // ZeroMemory(@Self, SizeOf(Self));
  end
  else
  begin
    ZeroMemory(@Self.Members[MemberId], SizeOf(Self.Members[MemberId]));
    if Self.TotalMembers > 0 then
      Dec(Self.TotalMembers);
  end;

  case RemoveBuff of
    1: Player.Base.RemoveBuff(CAVALEIROS_MARECHAL);
    2: Player.Base.RemoveBuff(CAVALEIROS_ARCHON);
  end;

  // ==== MYSQL SYNC ====
  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  try
    if not(SQLComp.Query.Connection.Connected) then
    begin
      Logger.Write('Falha MYSQL em RemoveMember', TlogType.Warnings);
      Exit;
    end;

    // Remove o registro do membro na guild
    SQLComp.SetQuery(
      Format('DELETE FROM guilds_players WHERE guild_index=%d AND char_index=%d',
      [OldGuildIndex, CharIndex]));
    SQLComp.Run(False);

    // Remove vínculo de guild do personagem
    SQLComp.SetQuery(
      Format('UPDATE characters SET guildindex=0 WHERE id=%d',
      [CharIndex]));
    SQLComp.Run(False);

    // Se a guild ainda existe (não foi zerada totalmente), atualiza totalmembers
    if Self.Index = OldGuildIndex then
    begin
      SQLComp.SetQuery(
        Format('UPDATE guilds SET totalmembers=%d WHERE id=%d',
        [Self.TotalMembers, OldGuildIndex]));
      SQLComp.Run(False);
    end
    else
    begin
      // Guild foi destruída (ZeroMemory): limpa tudo do SQL referente a ela
      SQLComp.SetQuery(
        Format('DELETE FROM guilds_players WHERE guild_index=%d', [OldGuildIndex]));
      SQLComp.Run(False);

      SQLComp.SetQuery(
        Format('DELETE FROM guilds WHERE id=%d', [OldGuildIndex]));
      SQLComp.Run(False);
    end;
  except
    on E: Exception do
      Logger.Write('Erro RemoveMember MYSQL: ' + E.Message, TLogType.Error);
  end;
  SQLComp.Destroy;

  Result := True;
end;

function TGuild.ChangeRank(CharIndex, Rank: DWORD): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
  SQLComp: TQuery;
  MemberIdx: Integer;
begin
  Result := False;
  MemberIdx := Self.FindMemberFromCharIndex(CharIndex);
  if MemberIdx < 0 then Exit;

  Self.Members[MemberIdx].Rank := Rank;
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player) then
          begin
            Player.UpdateGuildMemberRank(CharIndex, Rank);
            Break;
          end;
  except
    Exit;
  end;

  // MYSQL
  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  try
    if SQLComp.Query.Connection.Connected then
    begin
      SQLComp.SetQuery(
        Format('UPDATE guilds_players SET player_rank=%d WHERE guild_index=%d AND char_index=%d',
        [Rank, Self.Index, CharIndex]));
      SQLComp.Run(False);
    end;
  finally
    SQLComp.Destroy;
  end;

  Result := True;
end;

function TGuild.UpdateLevel(CharIndex, Level: DWORD): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
  SQLComp: TQuery;
  MemberIdx: Integer;
begin
  Result := False;
  MemberIdx := Self.FindMemberFromCharIndex(CharIndex);
  if MemberIdx < 0 then Exit;

  Self.Members[MemberIdx].Level := Level;
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player) then
          begin
            Player.UpdateGuildMemberLevel(CharIndex, Level);
            Break;
          end;
  except
    Exit;
  end;

  // MYSQL
  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  try
    if SQLComp.Query.Connection.Connected then
    begin
      SQLComp.SetQuery(
        Format('UPDATE guilds_players SET level=%d WHERE guild_index=%d AND char_index=%d',
        [Level, Self.Index, CharIndex]));
      SQLComp.Run(False);
    end;
  finally
    SQLComp.Destroy;
  end;

  Result := True;
end;

function TGuild.GetFreeMemberId: Integer;
var
  I: Integer;
begin
  Result := -1;
  if TotalMembers >= MaxGuildPlayersOfLevel[Self.Level] then
    Exit;
  for I := Low(Members) to High(Members) do
    if Members[I].CharIndex = 0 then
    begin
      Result := I;
      Break;
    end;
end;


function TGuild.GetAllyGuildCount: Integer;
var
  I: Integer;
begin
  Result := 0;

  for I := 0 to 3 do
  begin
    if(Self.Ally.Guilds[i].Index <> 0) then
      inc(Result);
  end;
end;

function TGuild.FindMemberFromCharIndex(CharIndex: DWORD): Integer;
var
  I: Integer;
begin
  Result := -1;

  // Evita procurar índices inválidos
  if CharIndex = 0 then
    Exit;

  for I := Low(Self.Members) to High(Self.Members) do
  begin
    // Proteção extra contra lixo de memória
    if Self.Members[I].CharIndex = 0 then
      Continue;

    // Encontrou o membro corretamente
    if Self.Members[I].CharIndex = CharIndex then
    begin
      Result := I;
      Exit;
    end;
  end;
end;



function TGuild.UpdateRanksConfig(RanksConfig: ARRAY OF BYTE): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
begin
  Result := False;
  Move(RanksConfig, Self.RanksConfig, 4);
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player)
          then
          begin
            Player.UpdateGuildRanksConfig;
            Break;
          end;
  except
    Exit;
  end;
  Result := True;
end;


function TGuild.UpdateNotices(Notice1, Notice2,
  Notice3: ARRAY OF AnsiChar): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
begin
  Result := False;
  AnsiStrings.StrCopy(Self.Notices[0].Text, Notice1);
  AnsiStrings.StrCopy(Self.Notices[1].Text, Notice2);
  AnsiStrings.StrCopy(Self.Notices[2].Text, Notice3);
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player)
          then
          begin
            Player.UpdateGuildNotices;
            Break;
          end;
  except
    Exit;
  end;
  Result := True;
end;
function TGuild.UpdateSite(FSite: ARRAY OF AnsiChar): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
begin
  AnsiStrings.StrCopy(Self.Site, FSite);
  Result := False;
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player)
          then
          begin
            Player.UpdateGuildSite;
            Break;
          end;
  except
    Exit;
  end;
  Result := True;
end;
function TGuild.OpenChest(CharIndex: DWORD; ChannelId: DWORD): Boolean;
var
  Player: PPlayer;
begin
  Result := False;
  if Servers[ChannelId].GetPlayerByCharIndex(CharIndex, Player) = False then
    Exit;
  if Self.GetRankConfig(Self.Members[Self.FindMemberFromCharIndex(CharIndex)
    ].Rank).OpenGWH = False then
  begin
    Player.SendClientMessage('Voc� n�o tem permiss�o para isso.');
    Exit;
  end;
  if (Self.MemberInChest >= 0) and (Self.MemberInChest <= High(Self.Members))
  then
  begin
    Player.SendClientMessage(AnsiString(Format('''%s'' est� usando o armaz�m.',
      [Self.Members[Self.MemberInChest].Name])));
    Exit;
  end;
  Self.MemberInChest := Self.FindMemberFromCharIndex(CharIndex);
  Self.LastChestActionDate := Now;
  Self.CloseChestThread := TCloseChestThread.Create(True);
  Self.CloseChestThread.Guild := @Self;
  Self.CloseChestThread.Start;
  Player.SendGuildChestPermission;
  Player.SendGuildChest;
  Player.RefreshGuildChestGold;
  Result := True;
end;
function TGuild.CloseChest: Boolean;
begin
  Result := False;
  try
    Self.MemberInChest := $FF;
    Self.LastChestActionDate := 0;
  except
    Exit;
  end;
  Result := True;
end;
function TGuild.SendChatMessage(const Packet; Size: Integer): Boolean;
var
  I, P: Integer;
  Player: PPlayer;
begin
  Result := False;
  try
    for I := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[I].CharIndex > 0) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[I].CharIndex, Player)
          then
          begin
            Player.SendPacket(Packet, Size);
            Break;
          end;
  except
    Exit;
  end;
  Result := True;
end;
function TGuild.SendChatAllyMessage(const Packet; Size: Integer): Boolean;
var
  I, P, K: Integer;
  Player: PPlayer;
  xGuild: PGuild;
begin
  Result := False;
  for k := 0 to 3 do
  begin
    if(Self.Ally.Guilds[k].Index <> 0) then
    begin
      xGuild := @Guilds[Servers[0].GetGuildSlotByID(Self.Ally.Guilds[k].Index)];

      if(xGuild.Index > 0) then
      begin
        for I := Low(xGuild.Members) to High(xGuild.Members) do
          if (xGuild.Members[I].CharIndex > 0) then
            for P := Low(Servers) to High(Servers) do
              if Servers[P].GetPlayerByCharIndex(xGuild.Members[I].CharIndex, Player)
              then
              begin
                Player.SendPacket(Packet, Size);
                Break;
              end;
      end;
    end;
  end;
end;
function TGuild.SendMemberLogin(CharIndex: DWORD): Boolean;
var
  i, P, idx: Integer;
  Player: PPlayer;
  SQLComp: TQuery;
begin
  Result := False;

  idx := Self.FindMemberFromCharIndex(CharIndex);
  if idx < 0 then
    Exit;

  Self.Members[idx].Logged := True;
  Self.Members[idx].LastLogin := DateTimeToUnix(Now);

  try
    for i := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[i].CharIndex > 0) and (i <> idx) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[i].CharIndex, Player) then
          begin
            Player.GuildMemberLogin(idx);
            Break;
          end;
  except
    Exit;
  end;

  // MYSQL
  SQLComp := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
    AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
    AnsiString(MYSQL_DATABASE));
  try
    if SQLComp.Query.Connection.Connected then
    begin
      SQLComp.SetQuery(
        Format('UPDATE guilds_players SET logged=1, last_login=%d WHERE guild_index=%d AND char_index=%d',
        [Self.Members[idx].LastLogin, Self.Index, CharIndex]));
      SQLComp.Run(False);
    end;
  finally
    SQLComp.Destroy;
  end;

  Result := True;
end;

function TGuild.SendMemberLogout(CharIndex: DWORD): Boolean;
var
  i, P, idx: Integer;
  Player: PPlayer;
  SQLComp: TQuery;
begin
  Result := False;

  idx := Self.FindMemberFromCharIndex(CharIndex);
  if idx < 0 then
    Exit;

  Self.Members[idx].Logged := False;
  Self.Members[idx].LastLogin := DateTimeToUnix(Now);

  try
    for i := Low(Self.Members) to High(Self.Members) do
      if (Self.Members[i].CharIndex > 0) and (i <> idx) then
        for P := Low(Servers) to High(Servers) do
          if Servers[P].GetPlayerByCharIndex(Self.Members[i].CharIndex, Player) then
          begin
            Player.GuildMemberLogout(idx);
            Break;
          end;
  except
    Exit;
  end;

  // MYSQL
    SQLComp := TQuery.Create(
      AnsiString(MYSQL_SERVER),
      MYSQL_PORT,
      AnsiString(MYSQL_USERNAME),
      AnsiString(MYSQL_PASSWORD),
      AnsiString(MYSQL_DATABASE)
    );

  try
    if SQLComp.Query.Connection.Connected then
    begin
      SQLComp.SetQuery(
        Format('UPDATE guilds_players SET logged=0, last_login=%d WHERE guild_index=%d AND char_index=%d',
        [Self.Members[idx].LastLogin, Self.Index, CharIndex]));
      SQLComp.Run(False);
    end;
  finally
    SQLComp.Destroy;
  end;

  Result := True;
end;

function TGuild.GetRankConfig(Rank: Integer): TGuildRankConfig;
var
  Value: Integer;
begin
  Value := Self.RanksConfig[Rank];

  Result.Invite := (Value and $01) <> 0;
  Result.Kick := (Value and $02) <> 0;
  Result.UseGuildSkill := (Value and $04) <> 0;
  Result.EditNotices := (Value and $10) <> 0;
  Result.OpenGWH := (Value and $40) <> 0;
  Result.UseGWH := (Value and $80) <> 0;
end;
end.

