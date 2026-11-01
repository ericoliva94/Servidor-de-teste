-- Copia as contas do servidor anterior (AikaEmu, banco aikaemu_auth) para o
-- aika_db4, mantendo o mesmo usuario e a mesma senha (o AikaEmu guarda o MD5,
-- que e exatamente o que este servidor compara em password_hash).
-- Pode ser executado de novo: contas existentes so tem a senha atualizada.

USE aika_db4;

INSERT INTO accounts (username, password, password_hash, account_type, time_created)
SELECT a.user,
       -- a senha em texto so e conhecida quando ela e igual ao usuario (admin/admin, teste/teste)
       IF(MD5(a.user) = LOWER(a.pass), a.user, ''),
       LOWER(a.pass),
       IF(a.user = 'admin', 5, 0),
       UNIX_TIMESTAMP()
FROM aikaemu_auth.accounts a
ON DUPLICATE KEY UPDATE password_hash = VALUES(password_hash);
