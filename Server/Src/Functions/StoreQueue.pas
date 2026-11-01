unit StoreQueue;

interface

uses
  Windows, SysUtils, Classes, DateUtils, AnsiStrings,
  SQL, GlobalDefs, MailFunctions, ItemFunctions, CharacterMail;


type
  TStoreQueueThread = class(TThread)
  protected
    procedure Execute; override;
  end;

implementation

procedure TStoreQueueThread.Execute;
var
  Q: TQuery;
  id, item_id, qty: Integer;
  char_name: string;
  Mail: TMail;
  slot: Integer;
  sendNow: WORD;
begin
  while not Terminated do
  begin
    Sleep(5000);

    Q := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
      AnsiString(MYSQL_USERNAME), AnsiString(MYSQL_PASSWORD),
      AnsiString(MYSQL_DATABASE));
        try
      Q := TQuery.Create(AnsiString(MYSQL_SERVER), MYSQL_PORT,
                         AnsiString(MYSQL_USERNAME),
                         AnsiString(MYSQL_PASSWORD),
                         AnsiString(MYSQL_DATABASE));

      Q.SetQuery('SELECT id, char_name, item_id, quantity FROM web_store_queue WHERE delivered = 0 LIMIT 1');
      Q.Run;

      if Q.Query.RecordCount = 0 then Exit;

      id       := Q.Query.FieldByName('id').AsInteger;
      char_name := string(Q.Query.FieldByName('char_name').AsString);
      item_id  := Q.Query.FieldByName('item_id').AsInteger;
      qty      := Q.Query.FieldByName('quantity').AsInteger;

      { ... monta carta ... }

      TMailFunctions.AddMail(char_name, Mail);

      Q.SetQuery('UPDATE web_store_queue SET delivered = 1 WHERE id = :id');
     // Q.AddParameter('id', id);
      //Q.ExecSQL; // ← CORRETO

    finally
      Q.Free; // NUNCA Q.Destroy
    end;

  end;
end;

end.

