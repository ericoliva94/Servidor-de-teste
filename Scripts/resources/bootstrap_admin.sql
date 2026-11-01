-- AIKA admin bootstrap.
-- Evidencia: Server/Src/PacketHandlers/AuthHandlers.pas insere em `accounts`;
-- Server/Src/Data/PlayerData.pas define Admin como o enum de valor 5.
-- A senha normal e "admin"; password_hash recebe o MD5 minusculo exigido pelo servidor.

INSERT INTO accounts (
    username,
    password,
    password_hash,
    last_token_creation_time,
    account_type
) VALUES (
    'admin',
    'admin',
    MD5('admin'),
    0,
    5
)
ON DUPLICATE KEY UPDATE
    password = VALUES(password),
    password_hash = VALUES(password_hash),
    account_type = 5;

-- PERSONAGEM NIVEL 90
-- Nao ha dump SQL nem definicao CREATE TABLE de personagem neste checkout.
-- Por isso nao invente uma INSERT aqui: complete esta secao somente apos conferir
-- a tabela do dump realmente usado. O personagem deve pertencer ao id da conta
-- admin, ter nivel 90, e a permissao GM vem da coluna accounts.account_type = 5.
