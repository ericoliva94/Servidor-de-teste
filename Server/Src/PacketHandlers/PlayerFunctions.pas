unit PlayerFunctions;

interface

uses Player, SysUtils, GlobalDefs, SQL;


type
  TPlayerFunctions = class
  public
    class function FindByPixNumber(const Pix: string): PPlayer; static;
    class function FindByName(const Name: string): PPlayer; static;
    class function FindBySQLName(const Name: string): PPlayer; static;

  end;

implementation

{ TPlayerFunctions }

class function TPlayerFunctions.FindByPixNumber(const Pix: string): PPlayer;
var
  S: Integer;
  FoundPlayer: PPlayer;
begin
  FoundPlayer := nil;

  for S := Low(Servers) to High(Servers) do
  begin
    if not Servers[S].IsActive then
      Continue;

    TPlayer.ForEach(
      procedure(P: PPlayer)
      begin
        if (P <> nil) and (P.IsInstantiated) and (P.PixNumber = Pix) then
          FoundPlayer := P;
      end,
      S
    );

    if FoundPlayer <> nil then
      Break;
  end;

  Result := FoundPlayer;
end;



class function TPlayerFunctions.FindByName(const Name: string): PPlayer;
var
  S: Integer;
  FoundPlayer: PPlayer;
  SearchName, PlayerName: string;
begin
  Result := nil;
  FoundPlayer := nil;

  // normaliza o nome que veio do comando
  SearchName := Trim(LowerCase(Name));
  if SearchName = '' then
    Exit;

  for S := Low(Servers) to High(Servers) do
  begin
    if not Servers[S].IsActive then
      Continue;

    TPlayer.ForEach(
      procedure(P: PPlayer)
      begin
        if (P = nil) or (not P.IsInstantiated) then
          Exit;

        // nome do personagem no servidor
        PlayerName := Trim(LowerCase(string(P.Base.Character.Name)));

        // comparação direta, já normalizada
        if PlayerName = SearchName then
          FoundPlayer := P;
      end,
      S
    );

    if FoundPlayer <> nil then
      Break;
  end;

  Result := FoundPlayer;
end;

class function TPlayerFunctions.FindBySQLName(const Name: string): PPlayer;
var
  SQL: TQuery;
  OwnerID: Integer;
  SearchName: string;
  S, I: Integer;
  P: PPlayer;
begin
  Result := nil;

  SearchName := Trim(LowerCase(Name));
  if SearchName = '' then
    Exit;

  // -------------------------------
  // 1) Buscar personagem no MySQL
  // -------------------------------
  SQL := TQuery.Create(
            AnsiString(MYSQL_SERVER),
            MYSQL_PORT,
            AnsiString(MYSQL_USERNAME),
            AnsiString(MYSQL_PASSWORD),
            AnsiString(MYSQL_DATABASE)
         );
  try
    SQL.SetQuery(
      'SELECT owner_accid ' +
      'FROM characters ' +
      'WHERE LOWER(name) = :NAME LIMIT 1'
    );

    SQL.AddParameter('NAME', SearchName);
    SQL.Query.Open;

    if SQL.Query.EOF then
    begin
      SQL.Free;
      Exit;  // personagem não existe
    end;

    OwnerID := SQL.Query.FieldByName('owner_accid').AsInteger;
  finally
    SQL.Free;
  end;

  // --------------------------------
  // 2) Buscar player online manual
  // --------------------------------
  for S := Low(Servers) to High(Servers) do
  begin
    if not Servers[S].IsActive then
      Continue;

    // percorre players 1..1000
    for I := Low(Servers[S].Players) to High(Servers[S].Players) do
    begin
      P := @Servers[S].Players[I];

      if (P <> nil) and P^.IsInstantiated then
      begin
        if P^.Account.Header.AccountID = OwnerID then
        begin
          Result := P;
          Exit;  // achou!
        end;
      end;
    end;
  end;
end;




end.
