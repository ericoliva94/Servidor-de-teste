unit SQL;
interface
uses Data.DB, FireDAC.Stan.Intf, FireDAC.Stan.Option, FireDAC.Stan.Error,
  FireDAC.UI.Intf, FireDAC.Phys.Intf, FireDAC.Stan.Def, FireDAC.Stan.Pool,
  FireDAC.Stan.Async, FireDAC.Phys, FireDAC.Stan.Param, FireDAC.DatS,
  FireDAC.DApt.Intf, FireDAC.DApt, FireDAC.Comp.DataSet, FireDAC.Comp.Client,
  FireDAC.Phys.MySQLDef, FireDAC.Phys.MySQL, Log, System.SysUtils;
type
  TQuery = class(TObject)
    //MySQL: TFDConnection;
    Query: TFDQuery;
    constructor Create(Server: AnsiString; Port: Integer;
      Login, Senha, DB: AnsiString; OnlyTransaction: Boolean = False);
    destructor Destroy; override;
    procedure SetQuery(Query: String);
    procedure AddParameter(Param, Value: AnsiString);
    procedure AddParameter2(Param: AnsiString; Value: Variant);
    procedure Run(Consult: Boolean = True);
    //class procedure UpdateLevel(CharIndex: Int64; Level: Integer);
  private
    procedure ConfigureConnection(Server: AnsiString; Port: Integer;
      Login, Senha, DB: AnsiString; OnlyTransaction: Boolean);
    function Reconnect: Boolean;
   end;
implementation
uses GlobalDefs;
const
  // tentativas de uma query quando a conexao com o MySQL caiu
  SQL_MAX_TRIES = 3;
  // pausa entre tentativas de conectar (antes era um loop sem pausa)
  SQL_CONNECT_RETRY_DELAY = 2000;

procedure TQuery.ConfigureConnection(Server: AnsiString; Port: Integer;
  Login, Senha, DB: AnsiString; OnlyTransaction: Boolean);
begin
  Query.Connection.Params.Clear;
  Query.Connection.Params.Add('DriverID=MySQL');
  Query.Connection.Params.Add('Server=' + String(Server));
  Query.Connection.Params.Add('Port=' + IntToStr(Port));
  Query.Connection.Params.Add('Database=' + String(DB));
  Query.Connection.Params.Add('User_Name=' + String(Login));
  Query.Connection.Params.Add('Password=' + String(Senha));
  Query.Connection.Params.Add('Charset=utf8mb4');
  Query.Connection.ResourceOptions.AutoReconnect := True;
  Query.Connection.TxOptions.AutoCommit := not OnlyTransaction;
  Query.FetchOptions.Mode := fmAll;
end;

constructor TQuery.Create(Server: AnsiString; Port: Integer; Login: AnsiString;
  Senha: AnsiString; DB: AnsiString; OnlyTransaction: Boolean = False);
var
  Connected: Boolean;
begin
  Query := TFDQuery.Create(nil);
  Query.Connection := TFDConnection.Create(nil);
  ConfigureConnection(Server, Port, Login, Senha, DB, OnlyTransaction);

  Connected := False;
  while not(Connected) do
  begin
    try
      Query.Connection.Open();
      Connected := Query.Connection.Connected;
    except
      on E: Exception do
      begin
        Logger.Write(E.ClassName + ': ' + E.Message, TLogType.Error);
        if (Query.Connection.Connected) then
          Query.Connection.Close();
        Sleep(SQL_CONNECT_RETRY_DELAY);
      end;
    end;
  end;

  // O codigo monta varios valores como texto ('' em coluna numerica, datas
  // '1899-12-30', posicoes com virgula decimal). No modo estrito do MySQL 8
  // esses INSERT/UPDATE falham e o dado do jogador nao e salvo.
  try
    Query.Connection.ExecSQL('SET SESSION sql_mode = ''NO_ENGINE_SUBSTITUTION''');
  except
    on E: Exception do
      Logger.Write('SET sql_mode: ' + E.Message, TLogType.Warnings);
  end;
end;
destructor TQuery.Destroy;
begin
  try
    Query.Close;
    Query.Connection.Close;
    FreeAndNil(Query.Connection);
    FreeAndNil(Query);
  except
    on E: Exception do
    begin
      Logger.Write(E.ClassName + ': ' + E.Message, TLogType.Warnings);
    end;
  end;
end;
procedure TQuery.SetQuery(Query: String);
begin
  Self.Query.Close;
  Self.Query.SQL.Clear;
  Self.Query.SQL.Add(Query);
end;
procedure TQuery.AddParameter(Param: AnsiString; Value: AnsiString);
begin
  Self.Query.ParamByName(String(Param)).Value := Value;
end;
procedure TQuery.AddParameter2(Param: AnsiString; Value: Variant);
begin
  Self.Query.ParamByName(String(Param)).Value := Value;
end;

function TQuery.Reconnect: Boolean;
begin
  // reabre so a conexao: a query, o SQL e os parametros continuam os mesmos.
  // Antes a TFDQuery era recriada so com o texto do SQL e a nova tentativa
  // rodava sem os parametros (gravando NULL ou falhando).
  Result := False;
  try
    Query.Close;
    Query.Connection.Close;
    Query.Connection.Open;
    Result := Query.Connection.Connected;
  except
    on E: Exception do
      Logger.Write('MySQL reconnect: ' + E.Message, TLogType.Error);
  end;
end;

procedure TQuery.Run(Consult: Boolean = True);
var
  Tries: Integer;
  ServerGone: Boolean;
begin
  for Tries := 1 to SQL_MAX_TRIES do
  begin
    try
      if (Consult) then
        Query.Open()
      else
      begin
        if not(Query.Connection.InTransaction) and
          not(Query.Connection.TxOptions.AutoCommit) then
          Query.Connection.StartTransaction;
        Query.ExecSQL();
      end;
      Exit;
    except
      on E: Exception do
      begin
        ServerGone := (E is EFDDBEngineException) and
          (EFDDBEngineException(E).Kind = ekServerGone);

        // Erros de escrita eram engolidos sem log: o dado sumia sem aviso.
        Logger.Write('SQL ' + E.ClassName + ': ' + E.Message + ' | ' +
          Copy(Query.SQL.Text, 1, 300), TLogType.Error);

        // erro do proprio comando (sintaxe, chave duplicada...): repetir nao adianta
        if not(ServerGone) then
          Exit;

        if not(Self.Reconnect) then
          Sleep(SQL_CONNECT_RETRY_DELAY);
      end;
    end;
  end;
end;
end.
