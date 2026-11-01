ARQUIVOS PARA O AIKA SETUP STATION

Copie para esta pasta os arquivos oficiais ou licenciados do projeto:

- mysql-installer.msi ou mysql-installer.exe
- navicat-installer.msi ou navicat-installer.exe
- todos os dumps .sql necessarios para o servidor

Os nomes podem variar, desde que contenham mysql, navicat e aika/database/dump,
respectivamente. Tambem e possivel selecionar os arquivos pela interface.

Todos os dumps .sql desta pasta e de suas subpastas serao importados pelo botao
"Importar todos os dumps". Cada dump multi-banco deve ter CREATE DATABASE e USE;
isso garante que cada banco seja restaurado no destino correto.

O banco e os instaladores nao foram incluidos aqui para nao redistribuir binarios
ou dados que nao existem no checkout.

bootstrap_admin.sql cria/atualiza a conta admin com MD5 de admin e account_type 5.
Antes de inserir um personagem nivel 90, confira os campos obrigatorios na tabela
de personagens do dump e complete a secao marcada no template.
