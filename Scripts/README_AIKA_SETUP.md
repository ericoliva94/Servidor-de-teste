# AIKA Setup Station

Execute no Windows:

```powershell
python .\Scripts\aika_installer_gui.py
```

Para gerar o executavel Windows, execute:

```powershell
powershell -ExecutionPolicy Bypass -File .\Scripts\build_aika_setup.ps1
```

O resultado e `Scripts\AikaSetupStation.exe`. Mantenha o executavel dentro de
`Scripts`, pois ele encontra `Scripts\resources` e `Server` a partir dessa
localizacao.

O programa usa somente a biblioteca padrao do Python e deve ser executado com
Python 3.10 ou mais novo. Ele foi criado para o pacote atual, cujo servidor esta
em `Server\Bin` e possui `LoginServer.exe`, `AikaServer.exe` e `WebServer.exe`.

## Preparacao

Coloque os recursos licenciados em `Scripts\resources` ou selecione-os na tela:

- instalador do MySQL compatível com o dump do projeto;
- instalador da mesma versao licenciada do Navicat usada pelo time;
- todos os dumps SQL do projeto, inclusive os bancos auxiliares;
- `mysql.exe`, quando ele nao estiver no `PATH` apos a instalacao.

O repositório atual nao traz instalador MySQL, instalador Navicat nem dump `.sql`.
Por isso a ferramenta nao escolhe uma versao ficticia e nem baixa software de
origem desconhecida.

## O que cada acao faz

- `Instalar MySQL` e `Instalar Navicat`: abre o instalador selecionado. Para MSI,
  chama o Windows Installer; para EXE, abre o instalador original interativo.
- `Importar todos os dumps`: processa cada `.sql` dentro de `Scripts\resources`
  e de suas subpastas. Para dumps de bancos diferentes, cada arquivo deve trazer
  `CREATE DATABASE` e `USE` para preservar o destino correto.
- `Exportar bancos + configs`: encontra todos os valores `Database` das secoes
  `[MySQL]` de cada configuracao do servidor, exporta cada banco com rotinas,
  eventos e triggers, e copia os arquivos de configuracao para um backup com data.
- `Aplicar senha SQL`: usa a senha digitada somente na memoria e substitui
  `Password` e `PasswordGM` dentro de todas as secoes `[MySQL]` encontradas.
  O arquivo `.ini` resultante a contem em texto simples porque esse servidor a le
  dessa forma; mantenha os arquivos e backups privados.
- `Provisionar admin`: executa `resources\bootstrap_admin.sql`. A conta fica com
  usuario `admin`, senha normal `admin`, hash MD5 de `admin` e `account_type = 5`.
- `Criar regras privadas do Firewall`: solicita elevacao e libera as portas TCP
  informadas somente no perfil de rede `Private`. O campo de IP remoto permite
  restringir o acesso a um IP ou rede CIDR. A porta 3306 permanece fechada salvo
  marcacao explicita.
- `Iniciar Login + Game + Website`: inicia os tres executaveis em `Server\Bin`.
- `Abrir website local`: abre `http://127.0.0.1:8090`.

## Personagem GM nivel 90

O codigo Pascal confirma que a permissao de administrador e `account_type = 5`.
Entretanto este checkout nao contem o dump ou o `CREATE TABLE` da tabela de
personagens. Os campos obrigatorios para criar um personagem nao podem ser
deduzidos com seguranca. O template SQL deixa a conta GM pronta e marca o local
onde inserir o comando verificado para o personagem nivel 90 depois de conferir
o dump real.

## Roteador

O programa fornece instrucoes e o IP local atual. Abra manualmente no roteador
as portas TCP `8831`, `8822` e `8090` para o IP local reservado. Abra `3306`
somente se acesso remoto ao MySQL for essencial. Caso o teste externo falhe mesmo
com a regra correta, confirme se o link usa CGNAT com a operadora.
