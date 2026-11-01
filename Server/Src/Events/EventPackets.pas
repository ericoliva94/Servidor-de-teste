unit EventPackets;

interface

uses
  Windows;

type
  { Packet padrão para enviar mensagens de evento (caso futuro) }
  TEventMessagePacket = packed record
    Header: record
      Size: WORD;
      Code: WORD;
    end;
    R: Byte;
    G: Byte;
    B: Byte;
    Message: array[0..127] of AnsiChar;
  end;

  { Pacote genérico caso você queira um pacote visual no altar futuramente }
  TEventSimplePacket = packed record
    Header: record
      Size: WORD;
      Code: WORD;
    end;
    Param1: DWORD;
    Param2: DWORD;
  end;

implementation

end.

