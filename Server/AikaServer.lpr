program AikaServer;
{$APPTYPE CONSOLE}
{$R *.res}
uses
  Interfaces,
  SysUtils,
  DateUtils,
  Windows,
  Forms,
  Dialogs,
  StrUtils,
  Classes,
  Winsock2,
  Log in 'Src\Functions\Log.pas',
  GlobalDefs in 'Src\Data\GlobalDefs.pas',
  ServerSocket in 'Src\Connections\ServerSocket.pas',
  Load in 'Src\Functions\Load.pas',
  NPC in 'Src\Mob\NPC.pas',
  BaseMob in 'Src\Mob\BaseMob.pas',
  Player in 'Src\Mob\Player.pas',
  CpuUsage in 'Src\Functions\CpuUsage.pas',
  Functions in 'Src\Functions\Functions.pas',
  ItemFunctions in 'Src\Functions\ItemFunctions.pas',
  SkillFunctions in 'Src\Functions\SkillFunctions.pas',
  Util in 'Src\Functions\Util.pas',
  ConnectionsThread in 'Src\Threads\ConnectionsThread.pas',
  PlayerThread in 'Src\Threads\PlayerThread.pas',
  UpdateThreads in 'Src\Threads\UpdateThreads.pas',
  FilesData in 'Src\Data\FilesData.pas',
  MiscData in 'Src\Data\MiscData.pas',
  Packets in 'Src\Data\Packets.pas',
  PlayerData in 'Src\Data\PlayerData.pas',
  PartyData in 'Src\Party\PartyData.pas',
  CharacterMail in 'Src\Mail\CharacterMail.pas',
  MailFunctions in 'Src\Mail\MailFunctions.pas',
  EncDec in 'Src\Connections\EncDec.pas',
  NPCHandlers in 'Src\PacketHandlers\NPCHandlers.pas',
  PacketHandlers in 'Src\PacketHandlers\PacketHandlers.pas',
  LoginSocket in 'Src\Connections\LoginSocket.pas',
  AuthHandlers in 'Src\PacketHandlers\AuthHandlers.pas',
  TokenSocket in 'Src\Connections\TokenSocket.pas',
  CommandHandlers in 'Src\PacketHandlers\CommandHandlers.pas',
  GuildData in 'Src\Guild\GuildData.pas',
  SQL in 'Src\Connections\SQL.pas',
  MOB in 'Src\Mob\MOB.pas',
  PET in 'Src\Mob\PET.pas',
  R_Painel in 'Src\Functions\R_Painel.pas',
  EntityMail in 'Src\Data\Entity\EntityMail.pas',
  EntityFriend in 'Src\Data\Entity\EntityFriend.pas',
  SendPacketForm in 'Src\Forms\SendPacketForm.pas' {frmSendPacket},
  Dungeon in 'Src\Dungeons\Dungeon.pas',
  Nation in 'Src\Nation\Nation.pas',
  PingBackForm in 'Src\Forms\PingBackForm.pas' {frmPingback},
  AuctionFunctions in 'Src\Auction\AuctionFunctions.pas',
  CharacterAutcion in 'Src\Auction\CharacterAutcion.pas',
  Objects in 'Src\Mob\Objects.pas',
  CastleSiege in 'Src\Nation\CastleSiege.pas',
  StoreQueue in 'Src\Functions\StoreQueue.pas',
  PlayerFunctions in 'Src\PacketHandlers\PlayerFunctions.pas',
  ConsoleSocket in 'Src\Connections\ConsoleSocket.pas',

  AltarEvent in 'Src\Events\AltarEvent.pas',
  GlobalEvents in 'Src\Events\GlobalEvents.pas',
  EventPackets in 'Src\Events\EventPackets.pas',
  GuardianStones in 'Src\Events\GuardianStones.pas',
  AltarSystem in 'Src\Events\AltarSystem.pas',

  RankedHandlers in 'Src\Rankeds\RankedHandlers.pas';





function ConsoleHandler(dwCtrlType: DWORD): BOOL; stdcall;
var
  i: BYTE;
begin
  Result := False;
  if (dwCtrlType = CTRL_CLOSE_EVENT) or (dwCtrlType = CTRL_LOGOFF_EVENT) or
     (dwCtrlType = CTRL_SHUTDOWN_EVENT) then
  begin
    if not(ServerHasClosed) then
    begin
      TFunctions.SaveGuilds;
      for i := Low(Nations) to High(Nations) do
        Nations[i].SaveNation;

      for i := Low(Servers) to High(Servers) do
        Servers[i].CloseServer;

      Logger.Write('Server Closed Succesfully!', TLogType.ServerStatus);
    end;
    Result := True;
  end;
end;

var
  InputStr: string;
  Uptime: TDateTime;
  timeinit: Integer;
  CreateSendPacketForm: Boolean = True;
  i, j, k: Integer;
  cmdto: String;
  F: TextFile;
  Path, MobN: String;
  CsvLine: String;
  ConsoleSock: TConsoleSocket;


type
  TConsoleKeepAlive = class(TThread)
  protected
    procedure Execute; override;
  end;

// Logica para o servidor nao morrer, tipo um coracao

procedure TConsoleKeepAlive.Execute;
var
  hIn: THandle;
  InputRecord: TInputRecord;
  eventsWritten: DWORD;
begin
  hIn := GetStdHandle(STD_INPUT_HANDLE);

  while not Terminated do
  begin
    // ======= ENTER =======
    FillChar(InputRecord, SizeOf(InputRecord), 0);
    InputRecord.EventType := KEY_EVENT;
    InputRecord.Event.KeyEvent.bKeyDown := True;
    InputRecord.Event.KeyEvent.wVirtualKeyCode := VK_RETURN;
    WriteConsoleInput(hIn, InputRecord, 1, eventsWritten);
    Sleep(1000); // 1s

    // ======= SPACE =======
    FillChar(InputRecord, SizeOf(InputRecord), 0);
    InputRecord.EventType := KEY_EVENT;
    InputRecord.Event.KeyEvent.bKeyDown := True;
    InputRecord.Event.KeyEvent.wVirtualKeyCode := VK_SPACE;
    WriteConsoleInput(hIn, InputRecord, 1, eventsWritten);
    Sleep(1000); // 1s
  end;
end;

type
  TServerHeartbeat = class(TThread)
  protected
    procedure Execute; override;
  end;

procedure TServerHeartbeat.Execute;
begin
  while not Terminated do
  begin
    Logger.Write('Servidor ativo...', TLogType.ServerStatus);
    Sleep(1000); // 1s
  end;
end;


begin
  Logger := TLog.Create;
  SetConsoleTitleA('Aika Server');
  //Logger.Write(HourOf(StrToDateTime('29/11/2022 23:58:33')).ToString, TLogType.Packets);

  try
    Uptime := Now;
    WebServerClosed := False;
    xServerClosed := False;
    TLoad.InitCharacters;
    TLoad.InitItemList;
    TLoad.InitSkillData;
    TLoad.InitSetItem;
    TLoad.InitConjunts;
    TLoad.InitReinforce;
    TLoad.InitPremiumItems;
    TLoad.InitExpList;
    TLoad.InitPranExpList;
    TLoad.InitServerConf;
    TLoad.InitServerList;
    TLoad.LoadNPCOptions;
    TLoad.InitMapsData;
    TLoad.InitScrollPositions;
    TLoad.InitQuestList;
    TLoad.InitQuests;
    TLoad.InitTitles;
    TLoad.InitDropList;
    TLoad.InitRecipes;
    TLoad.InitMakeItems;
    Logger.Space; { Space }
    TLoad.InitServers; { Channels Load }
    Logger.Space; { Space }
    TLoad.InitAuthServer;
    ConsoleSock := TConsoleSocket.Create('0.0.0.0', 5555);
    Logger.Space; { Space }
    TLoad.InitNPCS;
    TLoad.InitGuilds;
    for i := Low(Servers) to High(Servers) do
    begin
      Servers[i].ServerHasClosed := False;

      Servers[i].StartThreads;

      if (i <= 2) then
      begin
        Nations[Servers[i].NationID - 1].CreateNation(Servers[i].ChannelID);
        Nations[Servers[i].NationID - 1].LoadNation();
        Servers[i].UpdateReliquareEffects();
      end;

    end;
    TAuctionOffersThread.Create(60000);
    timeinit := MilliSecondsBetween(Now, Uptime);
    Logger.Write('Servidor levou ' + IntToStr(Round(timeinit / 1000)) +
      ' segundos para carregar completamente.', TLogType.ServerStatus);
    Logger.Space;
    ReportMemoryLeaksOnShutdown := True;
    SetConsoleCtrlHandler(@ConsoleHandler, True);
    TConsoleKeepAlive.Create(False);


    if (CreateSendPacketForm) then
    begin
      Application.Initialize;
      Application.CreateForm(TfrmSendPacket, frmSendPacket);
      Application.Run;
    end;

    Path := GetCurrentDir + '\Data\AllMobsInfo.csv';

    AssignFile(F, Path);
    ReWrite(F);

    for I := Low(Servers[1].MOBS.TMobS) to High(Servers[1].MOBS.TMobS) do
    begin
      if (Servers[1].MOBS.TMobS[i].IntName = 0) then
        Continue;

      MobN := String(Servers[1].MOBS.TMobS[i].Name);
      MobN := StringReplace(MobN, '_', ' ', [rfReplaceAll, rfIgnoreCase]);

      CsvLine :=
        Servers[1].MOBS.TMobS[i].IntName.ToString + ',' +
        MobN + ',' +
        Servers[1].MOBS.TMobS[i].Equip[0].ToString + ',' +
        Servers[1].MOBS.TMobS[i].Equip[1].ToString + ',' +
        Servers[1].MOBS.TMobS[i].Equip[6].ToString + ',' +
        Servers[1].MOBS.TMobS[i].InitHP.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Rotation.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MobLevel.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MobElevation.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Cabeca.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Perna.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MobType.ToString + ',' +
        Servers[1].MOBS.TMobS[i].SpawnType.ToString + ',' +
        '0,' +
        Servers[1].MOBS.TMobS[i].FisAtk.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MagAtk.ToString + ',' +
        Servers[1].MOBS.TMobS[i].FisDef.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MagDef.ToString + ',' +
        Servers[1].MOBS.TMobS[i].ReespawnTime.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Skill01.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Skill02.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Skill03.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Skill04.ToString + ',' +
        Servers[1].MOBS.TMobS[i].Skill05.ToString + ',' +
        Servers[1].MOBS.TMobS[i].MobExp.ToString + ',' +
        Servers[1].MOBS.TMobS[i].DropIndex.ToString + ',' +
        IntToStr(Servers[1].MOBS.TMobS[i].IsActiveToSpawn.ToInteger);

      WriteLn(F, CsvLine);


    end;

    CloseFile(F);





    while True do
    begin
      Sleep(30); // Evita CPU 100%

      case AnsiIndexStr(cmdto, [
        'close','savecsvmob','reloadskill','reloaditem','reloadserverconf',
        'reloadmobs','reloaddrops','reloadpremiumshop','reloadquestsserver',
        'reloadquestclient','reloadtitles','reloadrecipes','reloadmakeitem'
      ]) of

        0:
          begin
            ServerHasClosed := True;
            Logger.Write('Fechando o servidor, aguarde...', TLogType.ConnectionsTraffic);

            // Salva Guildas
            TFunctions.SaveGuilds;
            Logger.Write('GUILDAS SALVAS..', TLogType.ConnectionsTraffic);

            // Salva Na��es
            for i := Low(Nations) to High(Nations) do
              Nations[i].SaveNation;
            Logger.Write('NA��ES SALVAS..', TLogType.ConnectionsTraffic);

            // Fecha sockets
            for i := Low(Servers) to High(Servers) do
            begin
              closesocket(Servers[i].Sock);
              Servers[i].Sock := INVALID_SOCKET;
            end;
            Logger.Write('SOCKETS FECHADOS..', TLogType.ConnectionsTraffic);

            // Desconecta jogadores + encerra threads
            for i := Low(Servers) to High(Servers) do
            begin
              Servers[i].ServerHasCLosed := True;
              Servers[i].CloseServer;

              for j := 1 to MAX_CONNECTIONS do
                if (Servers[i].Players[j].Status >= TPlayerStatus.CharList) then
                  Servers[i].Disconnect(Servers[i].Players[j]);
            end;

            xServerClosed := True;
            Logger.Write('Server Closed Succesfully!', TLogType.ServerStatus);
            Logger.Write('Feche a janela deste console. Tudo foi salvo!', TLogType.ConnectionsTraffic);
          end;

        // ========================================
        // RELOAD SKILL
        // ========================================
        2:
          begin
            ZeroMemory(@SkillData, sizeof(SkillData));
            TLoad.InitSkillData;
            Logger.Write('SkillData recarregado.', TLogType.ServerStatus);
          end;

        // ========================================
        // RELOAD ITEM LIST
        // ========================================
        3:
          begin
            ZeroMemory(@ItemList, sizeof(ItemList));
            TLoad.InitItemList;
            Logger.Write('ItemList recarregado.', TLogType.ServerStatus);
          end;

        4: TLoad.InitServerConf;
        5: for i := Low(Servers) to High(Servers) do Servers[i].StartMobs;
        6: TLoad.InitDropList;
        7: TLoad.InitPremiumItems;
        8: TLoad.InitQuests;
        9: TLoad.InitQuestList;
        10: Tload.InitTitles;
        11: TLoad.InitRecipes;
        12: TLoad.InitMakeItems;

        // ========================================
        // SEND CHAT SERVER MESSAGE
        // ========================================
      else
        for i := Low(Servers) to High(Servers) do
          Servers[i].SendServerMsg(AnsiString('[SERVER] ' + cmdto), 32, 16);
      end;
    end;

  except
    on E: Exception do
    begin
      Logger.Write(E.ClassName + ': ' + E.Message, TLogType.error);
      InputStr := '';

    end;
  end;
end.


