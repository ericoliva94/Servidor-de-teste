unit ConnectionsThread;

interface

uses
  System.Classes, Windows, SysUtils, Winsock2, EncDec, System.SyncObjs,
  Packets;

type
  TConnectionsThread = class(TThread)
  private
    FDelay: Integer;
    Channel: Byte;
    fCritSect: TCriticalSection;
  protected
    procedure Execute; override;
  public
    constructor Create(Delay: Integer; ChannelIndex: Byte);
    destructor Destroy; override;
  end;

type
  TPlayerThreadGarbage = class(TThread)
  private
    FDelay: Integer;
    Channel: BYTE;
  protected
    procedure Execute; override;
  public
    constructor Create(Delay: Integer; ChannelIndex: BYTE);
  end;

implementation

uses
  GlobalDefs, Log, ServerSocket, LoginSocket;

{---------------------------------------------}
{ TConnectionsThread }
{---------------------------------------------}

constructor TConnectionsThread.Create(Delay: Integer; ChannelIndex: Byte);
begin
  FDelay := Delay;
  Channel := ChannelIndex;
  fCritSect := TCriticalSection.Create;
  inherited Create(False);
end;

destructor TConnectionsThread.Destroy;
begin
  fCritSect.Free;
  inherited;
end;

procedure TConnectionsThread.Execute;
var
  i: Integer;
  Conn: ^TConnection;
begin
  while Servers[Channel].IsActive do
  begin
    fCritSect.Enter;
    try
      if not ServerHasClosed then
      begin
        // Aceita nova conexão (non-blocking)
        Servers[Channel].AcceptConnection;

        // Percorre conexões ativas e tenta ler dados
        try
          for i := Low(Servers[Channel].Connections) to High(Servers[Channel].Connections) do
          begin
            Conn := @Servers[Channel].Connections[i];

            if Conn^.IsActive and (Conn^.Socket <> INVALID_SOCKET) then
            begin
              try
                Conn^.ReceiveData;
              except
                on E: Exception do
                  Logger.Write(
                    Format('[Channel %d] Erro em ReceiveData para conexão %d: %s',
                      [Channel, i, E.Message]),
                    TLogType.Error
                  );
              end;
            end;
          end;
        except
          on E: Exception do
            Logger.Write(
              Format('[Channel %d] Erro geral no loop de ReceiveData: %s',
                [Channel, E.Message]),
              TLogType.Error
            );
        end;
      end;
    except
      on E: Exception do
        Logger.Write(
          Format('[Channel %d] Erro em AcceptConnection: %s',
            [Channel, E.Message]),
          TLogType.Error
        );
    end;
    fCritSect.Leave;

    // Evita loop quente
    Sleep(FDelay);
  end;
end;

{---------------------------------------------}
{ TPlayerThreadGarbage }
{---------------------------------------------}

constructor TPlayerThreadGarbage.Create(Delay: Integer; ChannelIndex: BYTE);
begin
  inherited Create(False);
  FDelay := Delay;
  Channel := ChannelIndex;
end;

procedure TPlayerThreadGarbage.Execute;
var
  i: Integer;
begin
  while (Servers[Channel].IsActive) do
  begin
    try
      for i := Low(Servers[Channel].PlayerThreads) to High(Servers[Channel].PlayerThreads) do
      begin
        if (Servers[Channel].FPlayerThreads[i] = nil) then
          Continue;

        if (Servers[Channel].PlayerThreads[i].Term = True) then
        begin
          Servers[Channel].PlayerThreads[i].Terminate;
          Servers[Channel].PlayerThreads[i].WaitFor;
          FreeAndNil(Servers[Channel].PlayerThreads[i]);
          Servers[Channel].FPlayerThreads[i] := nil;
        end;
      end;
    except
      on E: Exception do
        Logger.Write(
          'ConnectionsThread.pas:TPlayerThreadGarbage.Execute ' + E.Message,
          TLogType.Error);
    end;
        Sleep(FDelay);
  end;
end;

end.


