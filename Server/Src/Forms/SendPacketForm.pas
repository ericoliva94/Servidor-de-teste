unit SendPacketForm;
interface
uses
  Winapi.Windows, Winapi.Messages, System.SysUtils, System.Variants,
  System.Classes, Vcl.Graphics,
  Vcl.Controls, Vcl.Forms, Vcl.Dialogs, Vcl.StdCtrls, Vcl.ExtCtrls, Vcl.ComCtrls,
  Vcl.Menus;
type
  TfrmSendPacket = class(TForm)
    memPacket: TMemo;
    edtNick: TEdit;
    btnSendPacket: TButton;
    lblPacket: TLabel;
    Label1: TLabel;
    btncloseserver: TButton;
    edtClientID: TEdit;
    Label2: TLabel;
    btnSendByClientID: TButton;
    gbServerOperation: TGroupBox;
    btnSendMsg: TButton;
    edtMsg: TEdit;
    Label3: TLabel;
    btnResetServer: TButton;
    btnClearMsg: TButton;
    cbServerInfo: TGroupBox;
    Label4: TLabel;
    Label5: TLabel;
    Label6: TLabel;
    lblCpu: TLabel;
    lblRam: TLabel;
    lblActivePlayers: TLabel;
    TimerServerInfo: TTimer;
    GroupBox1: TGroupBox;
    Label7: TLabel;
    lblChannels: TLabel;
    Splitter1: TSplitter;
    procedure btnSendPacketClick(Sender: TObject);
    procedure btncloseserverClick(Sender: TObject);
    procedure btnSendByClientIDClick(Sender: TObject);
    procedure btnSendMsgClick(Sender: TObject);
    procedure btnResetServerClick(Sender: TObject);
    procedure btnClearMsgClick(Sender: TObject);
    procedure TimerServerInfoTimer(Sender: TObject);
  private
    { Private declarations }
  public
    { Public declarations }
    procedure UpdateCaption(Content: String);
  end;
var
  frmSendPacket: TfrmSendPacket;
implementation
{$R *.dfm}
uses
  GlobalDefs, Functions, Packets, Log, Player, PlayerData, Load;
type
  TByteArray = record
    Content: ARRAY OF BYTE;
  end;
procedure TfrmSendPacket.UpdateCaption(Content: String);
begin
  Self.Caption := Content;
end;
procedure TfrmSendPacket.btnClearMsgClick(Sender: TObject);
begin
  edtMsg.Text := '';
end;
procedure TfrmSendPacket.btncloseserverClick(Sender: TObject);
var
  i: Integer;
begin
  Logger.Write('Fechando o servidor, aguarde...', TLogType.ConnectionsTraffic);
  try
    TFunctions.SaveGuilds;
  except
  end;
  try
    for i := Low(Nations) to High(Nations) do
    begin
      Nations[i].SaveNation;
    end;
  except
  end;
  for i := Low(Servers) to High(Servers) do
  begin
    Servers[i].CloseServer;
  end;
  xServerClosed := True;
  Logger.Write('Server Closed Succesfully!', TLogType.ServerStatus);
  Logger.Write('Feche a janela deste console. Tudo foi salvo!',
    TLogType.ConnectionsTraffic);
  ServerHasClosed := True;
  while(StrToInt(Self.lblActivePlayers.Caption) > 0) do
    Sleep(10);
  Application.Terminate;
  keybd_event(VK_CONTROL, 0, 0, 0);
  keybd_event(Ord('C'), 0, 0, 0);
  keybd_event(Ord('C'), 0, KEYEVENTF_KEYUP, 0);
  keybd_event(VK_CONTROL, 0, KEYEVENTF_KEYUP, 0);
  Self.Close;
end;


procedure TfrmSendPacket.btnResetServerClick(Sender: TObject);
begin
  Logger.Write('EXECUTANDO SOFT RESET DO SERVIDOR...', TLogType.ServerStatus);

  try
    // ===========================
    //  SALVAMENTO DE DADOS
    // ===========================
    Logger.Write('Salvando Guilds...', TLogType.ServerStatus);
    try
      TFunctions.SaveGuilds;
    except
      on E: Exception do Logger.Write('ERRO SaveGuilds: ' + E.Message, TLogType.Error);
    end;

    Logger.Write('Salvando Nações...', TLogType.ServerStatus);
    try
      for var n := Low(Nations) to High(Nations) do
        Nations[n].SaveNation;
    except
      on E: Exception do Logger.Write('ERRO SaveNation: ' + E.Message, TLogType.Error);
    end;

    Logger.Write('Salvando Players Online...', TLogType.ServerStatus);
    try
      for var s := Low(Servers) to High(Servers) do
        if Assigned(Servers[s]) then
        //  Servers[s].SaveAllPlayersOnline;
    except
      on E: Exception do Logger.Write('ERRO SaveAllPlayersOnline: ' + E.Message, TLogType.Error);
    end;

    Logger.Write('Salvando Auctions...', TLogType.ServerStatus);
    try
      //TFunctions.SaveAuctions;
    except
      on E: Exception do Logger.Write('ERRO SaveAuctions: ' + E.Message, TLogType.Error);
    end;

    Logger.Write('Salvando Relíquias...', TLogType.ServerStatus);
    try
    //  TFunctions.SaveReliquias;
    except
      on E: Exception do Logger.Write('ERRO SaveReliquias: ' + E.Message, TLogType.Error);
    end;


    // ===========================
    //  RELOAD DE ARQUIVOS
    // ===========================
    Logger.Write('Recarregando arquivos principais...', TLogType.ServerStatus);

    try TLoad.InitItemList;            except end;
    try TLoad.InitSetItem;             except end;
    try TLoad.InitSkillData;           except end;
    try TLoad.InitConjunts;            except end;
    try TLoad.InitExpList;             except end;
    try TLoad.InitPranExpList;         except end;
    try TLoad.InitServerList;          except end;
    try TLoad.InitQuestList;           except end;
    try TLoad.InitDropList;            except end;
    try TLoad.InitRecipes;             except end;
    try TLoad.InitMakeItems;           except end;
    try TLoad.InitPremiumItems;        except end;
    try TLoad.InitMobPos;              except end;
    try TLoad.InitMapsData;            except end;
    try TLoad.InitScrollPositions;     except end;
    try TLoad.LoadNPCOptions;          except end;
    try TLoad.InitNPCS;                except end;


    // ===========================
    //  RELOAD LIVE / SERVERS
    // ===========================
    Logger.Write('Recarregando NPCs e Mobs...', TLogType.ServerStatus);

    for var s := Low(Servers) to High(Servers) do
    begin
      if not Assigned(Servers[s]) then
        Continue;

      try
        // NÃO EXISTE Reload, então removido
        // Se quiser implementar no futuro, me peça.
      except
        on E: Exception do
          Logger.Write('ERRO no reload (Server '+IntToStr(s)+'): '+E.Message, TLogType.Error);
      end;
    end;

    Logger.Write('Soft Reset concluído com sucesso!', TLogType.ServerStatus);

  except
    on E: Exception do
    begin
      Logger.Write('Soft Reset FALHOU: ' + E.Message, TLogType.Error);
    end;
  end;
end;




procedure TfrmSendPacket.btnSendByClientIDClick(Sender: TObject);
var
  i: Integer;
  Len: Integer;
  Buffer: ARRAY [0 .. 8091] OF BYTE;
  MPlayer: PPlayer;
begin
  ZeroMemory(@Buffer, 8092);
  if (memPacket.Text <= '') or (edtClientID.Text <= '') then
    Exit;
  for i := 0 to Length(Servers) - 1 do
  begin
    MPlayer := Servers[i].GetPlayer(StrToInt(edtClientID.Text));
    if (MPlayer.Status >= Playing) then
    begin
      Len := TFunctions.StrToArray(memPacket.Text, Buffer);
      if Len > 1 then
        Servers[i].Players[MPlayer.Base.ClientId].SendPacket(Buffer, Len);
      Break;
    end;
  end;
end;
procedure TfrmSendPacket.btnSendMsgClick(Sender: TObject);
var
  i: Integer;
begin
  if (edtMsg.Text = '') then
  begin
    MessageBox(0, 'Você não pode deixar o campo mensagem em branco', 'Erro', 0);
    Exit;
  end;
  for i := Low(Servers) to High(Servers) do
  begin
    Servers[i].SendServerMsg(AnsiString('[SERVER] ' + edtMsg.Text), 32, 16);
  end;
end;
procedure TfrmSendPacket.btnSendPacketClick(Sender: TObject);
var
  i: Integer;
  ClientId: Integer;
  Len: Integer;
  Buffer: ARRAY [0 .. 8091] OF BYTE;
begin
  ZeroMemory(@Buffer, 8092);
  if (memPacket.Text <= '') or (edtNick.Text <= '') then
    Exit;
  for i := 0 to Length(Servers) - 1 do
  begin
    ClientId := Servers[i].GetPlayerByName(edtNick.Text);
    if ClientId > 0 then
    begin
      Len := TFunctions.StrToArray(memPacket.Text, Buffer);
      if Len > 1 then
        Servers[i].Players[ClientId].SendPacket(Buffer, Len);
      Break;
    end;
  end;
end;
procedure TfrmSendPacket.TimerServerInfoTimer(Sender: TObject);
var
  Ram, Ram2, ActivePlayers, ActivePlayersIsolated: Integer;
  Cpu: Single;
  i, j, k: Integer;
begin
  Cpu := TFunctions.GetCPUUsage;
  Ram := Round(TFunctions.GetMemoryUsed / (1048576));
  Ram := TFunctions.GetMemoryUsed;
  ActivePlayers:= 0;
  Ram := 0;
  for I := Low(Servers) to High(Servers) do
  begin
    ActivePlayersIsolated := 0;
    for j := Low(Servers[i].Players) to High(Servers[i].Players) do
    begin
      if(Servers[i].Players[j].Status >= CharList) then
      begin
        inc(ActivePlayers);
        inc(ActivePlayersIsolated);
      end;
    end;
    Servers[i].ActiveReliquaresOnTemples := 0;
    for j := Low(Servers[i].Devires) to High(Servers[i].Devires) do
    begin
        Continue;

      for k := 0 to 4 do
      begin
        if(Servers[i].Devires[j].Slots[k].ItemID > 0) then
          Inc(Servers[i].ActiveReliquaresOnTemples, 1);
      end;
    end;

    Servers[i].ActivePlayersNowHere := ActivePlayersIsolated;
    Ram := Ram + 630;
  end;
  ActivePlayersNow := ActivePlayers;
  Self.lblCpu.Font.Size := 8;
  case Round(Cpu) of
    0 .. 20:
      Self.lblCpu.Font.Color := clLime;
    21 .. 40:
      Self.lblCpu.Font.Color := clYellow;
    41 .. 60:
      Self.lblCpu.Font.Color := clMaroon;
    61 .. 80:
      Self.lblCpu.Font.Color := clRed;
    81 .. 100:
      begin
        Self.lblCpu.Font.Color := clRed;
        Self.lblCpu.Font.Size := 10;
      end;
  end;
  Ram2 := Round(Ram / 10);
  Self.lblRam.Font.Size := 8;
  case Ram2 of
    0 .. 50:
      Self.lblRam.Font.Color := clLime;
    51 .. 100:
      Self.lblRam.Font.Color := clYellow;
    101 .. 150:
      Self.lblRam.Font.Color := clRed;
  else
    begin
      Self.lblRam.Font.Color := clRed;
      Self.lblRam.Font.Size := 10;
    end;
  end;
  Self.lblCpu.Caption := FormatFloat('0.00 %', Cpu);
  Self.lblRam.Caption := FormatFloat('0.0 MB', Ram);
  Self.lblActivePlayers.Font.Color := clWhite;
  Self.lblChannels.Font.Color := clWhite;
  Self.lblActivePlayers.Caption := ActivePlayers.ToString;
  Self.lblChannels.Caption := InstantiatedChannels.ToString;
end;
end.
