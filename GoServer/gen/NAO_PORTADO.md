# Records nao gerados automaticamente

Gerado por `gen/emit.py`. Cada um precisa de tipo Go escrito a
mao porque nao tem layout binario fixo (lista, ponteiro, classe)
ou tem parte variante.

| Record | Unit | Motivo |
|---|---|---|
| `TChangeChannelToken` | `PlayerData.pas` | tlist |
| `TCharName` | `PartyData.pas` | string sem tamanho (ponteiro) |
| `TDevir` | `MiscData.pas` | class |
| `TParty` | `PartyData.pas` | tlist |
| `TPlayerCharacter` | `PlayerData.pas` | tlist |
