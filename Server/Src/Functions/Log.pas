unit Log;

interface

uses
  Windows, SysUtils, Classes;

type
  TLogType = (Packets, ConnectionsTraffic, Warnings, ServerStatus, Error, Painel);

type
  TLog = class(TObject)
  private
    function FormatMessage(const str: string; logType: TLogType): string;
  public
    procedure Write(str: string; logType: TLogType); overload;
    procedure Space;
  end;

procedure LogTxt(str: String);

implementation

function TimeStamp: string;
begin
  Result := FormatDateTime('dd/mm/yyyy HH:nn:ss', Now);
end;

function TLog.FormatMessage(const str: string; logType: TLogType): string;
const
  LogPrefix: array[TLogType] of string = (
    '[PACKET]',
    '[TRAFFIC]',
    '[WARNING]',
    '[STATUS]',
    '[ERROR]',
    '[PAINEL]'
  );
begin
  Result := Format('%s %s [TID: %d] %s',
    [TimeStamp, LogPrefix[logType], GetCurrentThreadId, str]);
end;

procedure LogTxt(str: String);
var
  NomeDoLog: string;
  Arquivo: TextFile;
begin
  NomeDoLog := GetCurrentDir + '\Logs\StatusLog.txt';

  if not DirectoryExists(GetCurrentDir + '\Logs') then
    ForceDirectories(GetCurrentDir + '\Logs');

  AssignFile(Arquivo, NomeDoLog);
  if FileExists(NomeDoLog) then
    Append(Arquivo)
  else
    ReWrite(Arquivo);

  try
    WriteLn(Arquivo, str);
  finally
    CloseFile(Arquivo)
  end;
end;

procedure LogError(str: String);
var
  NomeDoLog: string;
  Arquivo: TextFile;
begin
  NomeDoLog := GetCurrentDir + '\Logs\ErrorLog.txt';

  if not DirectoryExists(GetCurrentDir + '\Logs') then
    ForceDirectories(GetCurrentDir + '\Logs');

  AssignFile(Arquivo, NomeDoLog);
  if FileExists(NomeDoLog) then
    Append(Arquivo)
  else
    ReWrite(Arquivo);

  try
    WriteLn(Arquivo, str);
  finally
    CloseFile(Arquivo)
  end;
end;

procedure LogPainel(str: String);
var
  NomeDoLog: string;
  Arquivo: TextFile;
begin
  NomeDoLog := GetCurrentDir + '\Logs\PainelLog.txt';

  if not DirectoryExists(GetCurrentDir + '\Logs') then
    ForceDirectories(GetCurrentDir + '\Logs');

  AssignFile(Arquivo, NomeDoLog);
  if FileExists(NomeDoLog) then
    Append(Arquivo)
  else
    ReWrite(Arquivo);

  try
    WriteLn(Arquivo, str);
  finally
    CloseFile(Arquivo)
  end;
end;

procedure TLog.Write(str: string; logType: TLogType);
var
  msg: string;
begin
  msg := FormatMessage(str, logType);

  case logType of
    Packets:
      WriteLn(msg);

    ConnectionsTraffic:
      begin
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_GREEN OR FOREGROUND_INTENSITY);
        WriteLn(msg);
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_RED OR FOREGROUND_GREEN OR FOREGROUND_BLUE);
      end;

    Warnings:
      begin
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_RED OR FOREGROUND_INTENSITY);
        WriteLn(msg);
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_RED OR FOREGROUND_GREEN OR FOREGROUND_BLUE);
      end;

    ServerStatus:
      begin
        WriteLn(msg);
        LogTxt(msg);
      end;

    Error:
      begin
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_RED OR FOREGROUND_INTENSITY);
        WriteLn(msg);
        SetConsoleTextAttribute(GetStdHandle(STD_OUTPUT_HANDLE), FOREGROUND_RED OR FOREGROUND_GREEN OR FOREGROUND_BLUE);
        LogError(msg);
      end;

    Painel:
      begin
        msg := msg + ' [ORIGEM: Painel Web]';
        WriteLn(msg);
        LogPainel(msg);
      end;
  end;
end;

procedure TLog.Space;
begin
  WriteLn('--------------------------------------------------------------------------------');
end;

end.

