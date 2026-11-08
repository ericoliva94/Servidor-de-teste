-- Aliança de guildas (incremento 27). Bancos criados antes dessa data não têm
-- estas colunas; o schema novo (Server/Database/aika_db4.sql) já as traz.
ALTER TABLE guilds
  ADD COLUMN ally_leader        INT NOT NULL DEFAULT 0,
  ADD COLUMN guild_ally1_index  INT NOT NULL DEFAULT 0,
  ADD COLUMN guild_ally1_name   VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN guild_ally2_index  INT NOT NULL DEFAULT 0,
  ADD COLUMN guild_ally2_name   VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN guild_ally3_index  INT NOT NULL DEFAULT 0,
  ADD COLUMN guild_ally3_name   VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN guild_ally4_index  INT NOT NULL DEFAULT 0,
  ADD COLUMN guild_ally4_name   VARCHAR(32) NOT NULL DEFAULT '';
