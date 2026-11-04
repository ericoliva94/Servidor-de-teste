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
| `GoServer` | **Porte do servidor para Go** (em andamento) — ver abaixo |

O que foi corrigido e por quê: [`Server/ALTERACOES.md`](Server/ALTERACOES.md).

## Porte para Go (`GoServer/`)

O servidor está sendo reescrito em Go, no mesmo repositório, mantendo
compatibilidade binária com o cliente Aika Global: mesma cifra, mesmo
enquadramento de pacote e mesmo layout de registro (há teste comparando
`binary.Size` com o `sizeof()` de cada record do Delphi).

Hoje funcionam login, canal, movimento, visibilidade, combate entre jogadores,
mobs com IA, skills, grupo, guilda, correio, leilão, dungeon, drop, inventário,
armazém, baú de guilda, usar item, inventário premium e um console de
administração. O estado completo, fase por fase, está em
[`GoServer/PORTE.md`](GoServer/PORTE.md) — que também documenta **cada
divergência em relação ao Delphi e o motivo**, incluindo os defeitos da origem
que o porte corrige.

```bash
cd GoServer
go build ./...
go test ./... -race
```

Para subir sem banco, só para ver o servidor de pé:

```bash
cp ../Server/Bin/AikaServer.ini.example /tmp/AikaServer.ini
go run ./cmd/aikaserver -no-db -ini /tmp/AikaServer.ini -data ../Server/Bin/Data
```

Com banco, rode antes o schema das tabelas que o porte acrescenta:

```bash
mysql -u root -p aika_db4 < GoServer/SCHEMA.sql
```

O console de administração (porta 5555) só abre se a variável
`AIKA_CONSOLE_TOKEN` estiver definida; sem ela o servidor avisa e segue sem o
console.

## Contribuir

O repositório é público e aceita contribuição por *fork* e *pull request*:

1. Clique em **Fork** no GitHub e clone o seu fork.
2. Crie uma branch para a sua mudança.
3. No `GoServer`, antes de abrir o PR: `gofmt -l .` (sem saída),
   `go build ./...`, `go vet ./...` e `go test ./... -race`.
4. Abra o pull request descrevendo o que muda e por quê. Se a mudança
   diverge do comportamento do Delphi, diga isso no PR e em
   `GoServer/PORTE.md`.

Comentário em código e mensagem de commit em português, como o resto do
repositório.

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
