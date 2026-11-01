unit ConsoleSocket;

interface

uses
  Windows, Winsock2, SysUtils, Classes,
  Log, GlobalDefs, PlayerData, Player, CommandHandlers;

type
  TConsoleSocket = class(TThread)
  private
    Sock: TSocket;
    FBindIP: string;
    FBindPort: Word;
    function InitSocket: Boolean;
    procedure HandleClient(ClientSock: TSocket);
  protected
    procedure Execute; override;
  public
    constructor Create(const BindIP: string; BindPort: Word);
  end;

implementation

{ TConsoleSocket }

constructor TConsoleSocket.Create(const BindIP: string; BindPort: Word);
begin
  FBindIP   := BindIP;
  FBindPort := BindPort;

  inherited Create(False);
  FreeOnTerminate := False;
end;

function TConsoleSocket.InitSocket: Boolean;
var
  Addr: TSockAddrIn;
  WSA: TWSAData;
  yes: Integer;
begin
  Result := False;

  if WSAStartup(MAKEWORD(2,2), WSA) <> 0 then
  begin
    Logger.Write('ConsoleSocket: Erro WSAStartup.', TLogType.Painel);
    Exit;
  end;

  Sock := socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
  if Sock = INVALID_SOCKET then
  begin
    Logger.Write('ConsoleSocket: Erro ao criar socket.', TLogType.Painel);
    Exit;
  end;

  yes := 1;
  setsockopt(Sock, SOL_SOCKET, SO_REUSEADDR, @yes, SizeOf(yes));

  ZeroMemory(@Addr, SizeOf(Addr));
  Addr.sin_family := AF_INET;
  Addr.sin_port   := htons(FBindPort);

  if FBindIP = '0.0.0.0' then
    Addr.sin_addr.S_addr := INADDR_ANY
  else
    Addr.sin_addr.S_addr := inet_addr(PAnsiChar(AnsiString(FBindIP)));

  if bind(Sock, TSockAddr(Addr), SizeOf(Addr)) = SOCKET_ERROR then
  begin
    Logger.Write('ConsoleSocket: bind() falhou: '+
      IntToStr(WSAGetLastError), TLogType.Painel);
    closesocket(Sock);
    Sock := INVALID_SOCKET;
    Exit;
  end;

  if listen(Sock, SOMAXCONN) = SOCKET_ERROR then
  begin
    Logger.Write('ConsoleSocket: listen() falhou: '+
      IntToStr(WSAGetLastError), TLogType.Painel);
    closesocket(Sock);
    Sock := INVALID_SOCKET;
    Exit;
  end;

  Logger.Write(Format('ConsoleSocket escutando em %s:%d',
    [FBindIP, FBindPort]), TLogType.Painel);

  Result := True;
end;

procedure TConsoleSocket.Execute;
var
  ClientSock: TSocket;
  Addr: TSockAddrIn;
  AddrLen: Integer;
begin
  if not InitSocket() then
    Exit;

  while not Terminated do
  begin
    AddrLen := SizeOf(Addr);
    ClientSock := accept(Sock, @Addr, @AddrLen);

    if ClientSock = INVALID_SOCKET then
    begin
      Sleep(10);
      Continue;
    end;

    try
      HandleClient(ClientSock);
      // protege contra ataques enviando payload gigante

    finally
      closesocket(ClientSock);
    end;
  end;
end;

procedure TConsoleSocket.HandleClient(ClientSock: TSocket);
var
  Buffer: array[0..255] of AnsiChar;
  Received: Integer;
  FullCmd, Command, Params: string;
  SpacePos: Integer;
  s, p: Integer;
  ExecPlayer: PPlayer;
begin
  if Received > 200 then Exit;
  Received := recv(ClientSock, Buffer, SizeOf(Buffer), 0);
  if Received <= 0 then Exit;

  SetString(FullCmd, PAnsiChar(@Buffer[0]), Received);
  FullCmd := Trim(FullCmd);
  if (FullCmd = '') or (FullCmd[1] <> '/') then
  Exit;

  Logger.Write('[CONSOLE] ' + FullCmd, TLogType.Painel);

  // tira barra inicial se tiver
  if (Length(FullCmd) > 0) and (FullCmd[1] = '/') then
    Delete(FullCmd, 1, 1);

  // separa comando + params
  SpacePos := Pos(' ', FullCmd);
  if SpacePos > 0 then
  begin
    Command := Copy(FullCmd, 1, SpacePos - 1);
    Params  := Trim(Copy(FullCmd, SpacePos + 1, MaxInt));
  end
  else
  begin
    Command := FullCmd;
    Params  := '';
  end;

  // procura um GM (Admin) online pra ser o "dono" do comando
  ExecPlayer := nil;
  for s := Low(Servers) to High(Servers) do
  begin
    for p := 1 to MAX_CONNECTIONS do
    begin
      if Servers[s].Players[p].Status >= TPlayerStatus.Playing then
      begin
        if Servers[s].Players[p].Account.Header.AccountType = TAccountType.Admin then
        begin
          ExecPlayer := @Servers[s].Players[p];
          Break;
        end;
      end;
    end;
    if ExecPlayer <> nil then Break;
  end;

  if ExecPlayer = nil then
  begin
    Logger.Write('ConsoleSocket: Nenhum Admin online para processar "'+
      Command+'".', TLogType.Painel);
    Exit;
  end;

  // agora sim, executa o comando como se fosse esse GM
  TCommandHandlers.ProcessCommands(ExecPlayer^, Command, Params);
      // Confirmação limpa para o PAINEL
    send(ClientSock, 'OK'#0, 3, 0);

end;

end.

