# Aika Lore Community — servidor

Servidor de Aika em **Delphi 12 (Object Pascal)**: VCL, FireDAC (MySQL 8),
Winsock2 e Indy. Cliente compatível: **Aika Global** com o `Client\SL.bin`.

| Pasta | Conteúdo |
|---|---|
| `Server\Src` | Código-fonte (`AikaServer.dproj` abre no RAD Studio 12) |
| `Server\Bin` | Executáveis e dados que o servidor carrega (`Data\`) |
| `Server\Database` | Schema do banco `aika_db4` e importação de contas |
| `Server\Tools\AikaProxy` | Proxy (.NET 10) que evita perda de pacotes no binário atual |
| `Client` | `SL.bin` para o cliente Aika Global |
| `Scripts` | Instalador auxiliar (Python) |

O que foi corrigido e por quê: [`Server/ALTERACOES.md`](Server/ALTERACOES.md).

## Configurar numa máquina nova

Senhas, tokens e certificados **não estão no repositório**. Recrie-os a partir
dos modelos:

1. `Server\Bin\AikaServer.ini.example` → `AikaServer.ini` (preencha `Password`/`PasswordGM`
   do MySQL e, se usar pagamentos, `[ASAAS_TOKEN]`).
2. `Server\Bin\config.ini.example` → `config.ini`.
3. `Server\Bin\WebPainel\.env.example` → `.env` (só se usar o painel web).
4. Certificado do HTTPS admin (porta 9571), na pasta `Server\Bin`:
   ```
   openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=localhost" -keyout mycert.pem -out mycert.pem
   ```
5. Banco: `mysql -uroot -p < Server\Database\aika_db4.sql`
   (e, se quiser as contas do AikaEmu, `import_aikaemu_accounts.sql`).
   Use `sql_mode = NO_ENGINE_SUBSTITUTION` (veja `ALTERACOES.md`).
6. Proxy: `dotnet publish Server\Tools\AikaProxy -c Release -o Server\Bin\AikaProxy`.
7. Inicie com `Server\Bin\Iniciar_Servidor.bat`.

## Fora do repositório (grandes ou gerados)

Logs, `Server\Bin\PacotesItemList.bin.txt` (227 MB, acima do limite do GitHub),
saída de compilação (`Win64`, `DCUs`, `lib`, `*.rsm`) e histórico do Delphi
(`__history`, `__recovery`). Lista completa em `.gitignore`.

## Compilar

O servidor roda no Windows. Para gerar um `AikaServer.exe` com as correções de
`Src`, compile `Server\AikaServer.dproj` (Win64, Release) no RAD Studio 12; o
código pode ser editado em qualquer lugar, mas compilar exige o Delphi.
