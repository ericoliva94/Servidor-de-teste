-- =====================================================================
-- AIKA Lore Community - schema do banco aika_db4
--
-- O pacote original nao traz dump. Este schema foi reconstruido a partir
-- de todas as queries do codigo Pascal (SetQuery/FieldByName), entao cada
-- tabela/coluna abaixo existe porque o servidor le ou grava nela.
--
-- Uso:  mysql -uroot -p < aika_db4.sql
-- Pode ser executado de novo: nao apaga dados existentes.
-- =====================================================================

CREATE DATABASE IF NOT EXISTS aika_db4
  DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE aika_db4;

-- ---------------------------------------------------------------------
-- Contas
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS accounts (
  id                        INT UNSIGNED NOT NULL AUTO_INCREMENT,
  forum_id                  INT          NOT NULL DEFAULT 0,    -- usado pelo WebPage.py
  username                  VARCHAR(32)  NOT NULL,
  password                  VARCHAR(64)  NOT NULL DEFAULT '',
  password_hash             VARCHAR(64)  NOT NULL DEFAULT '',   -- MD5 minusculo (o cliente envia o MD5)
  mail                      VARCHAR(128) NOT NULL DEFAULT '',
  last_token                VARCHAR(64)  NOT NULL DEFAULT '',
  last_token_creation_time  BIGINT       NOT NULL DEFAULT 0,    -- unix
  nation                    INT          NOT NULL DEFAULT 0,
  isactive                  TINYINT      NOT NULL DEFAULT 0,
  account_status            INT          NOT NULL DEFAULT 0,    -- 2 cancelada, 8 banida, 10 nao-CBT
  account_type              INT          NOT NULL DEFAULT 0,    -- 5 = Admin/GM
  storage_gold              BIGINT       NOT NULL DEFAULT 0,
  cash                      INT          NOT NULL DEFAULT 0,
  premium_time              BIGINT       NOT NULL DEFAULT 0,    -- unix
  ban_days                  INT          NOT NULL DEFAULT 0,
  isfounder                 INT          NOT NULL DEFAULT 0,
  ip_created                VARCHAR(64)  NOT NULL DEFAULT '',
  time_created              BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY ux_accounts_username (username),
  KEY ix_accounts_mail (mail)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS account_validate (
  id        INT UNSIGNED NOT NULL AUTO_INCREMENT,
  email     VARCHAR(128) NOT NULL DEFAULT '',
  referrer  VARCHAR(64)  NULL,
  PRIMARY KEY (id),
  KEY ix_account_validate_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Personagens
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS characters (
  id                INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_accid       INT UNSIGNED NOT NULL,
  slot              INT          NOT NULL DEFAULT 0,
  name              VARCHAR(32)  NOT NULL,
  clientid          INT          NOT NULL DEFAULT 0,
  classinfo         INT          NOT NULL DEFAULT 0,
  strength          INT          NOT NULL DEFAULT 0,
  agility           INT          NOT NULL DEFAULT 0,
  intelligence      INT          NOT NULL DEFAULT 0,
  constitution      INT          NOT NULL DEFAULT 0,
  luck              INT          NOT NULL DEFAULT 0,
  status            INT          NOT NULL DEFAULT 0,
  altura            INT          NOT NULL DEFAULT 0,
  tronco            INT          NOT NULL DEFAULT 0,
  perna             INT          NOT NULL DEFAULT 0,
  corpo             INT          NOT NULL DEFAULT 0,
  curhp             INT          NOT NULL DEFAULT 0,
  curmp             INT          NOT NULL DEFAULT 0,
  honor             INT          NOT NULL DEFAULT 0,
  killpoint         INT          NOT NULL DEFAULT 0,
  infamia           INT          NOT NULL DEFAULT 0,
  skillpoint        INT          NOT NULL DEFAULT 0,
  experience        BIGINT       NOT NULL DEFAULT 0,
  level             INT          NOT NULL DEFAULT 1,
  guildindex        INT          NOT NULL DEFAULT 0,
  gold              BIGINT       NOT NULL DEFAULT 0,
  creationtime      BIGINT       NOT NULL DEFAULT 0,
  numeric_token     VARCHAR(16)  NOT NULL DEFAULT '',
  numeric_errors    INT          NOT NULL DEFAULT 0,
  deleted           TINYINT      NOT NULL DEFAULT 0,
  delete_time       VARCHAR(32)  NOT NULL DEFAULT '',
  logintime         INT          NOT NULL DEFAULT 0,
  speedmove         INT          NOT NULL DEFAULT 0,
  rotation          INT          NOT NULL DEFAULT 0,
  lastlogin         BIGINT       NOT NULL DEFAULT 0,
  loggedtime        INT          NOT NULL DEFAULT 0,
  playerkill        TINYINT      NOT NULL DEFAULT 0,
  posx              FLOAT        NOT NULL DEFAULT 0,
  posy              FLOAT        NOT NULL DEFAULT 0,
  saved_posx        INT          NOT NULL DEFAULT 0,
  saved_posy        INT          NOT NULL DEFAULT 0,
  active_title      INT          NOT NULL DEFAULT 0,
  pranevcnt         INT          NOT NULL DEFAULT 0,
  last_diary_event  VARCHAR(32)  NOT NULL DEFAULT '0',
  tp_positions      TEXT         NULL,
  PRIMARY KEY (id),
  UNIQUE KEY ux_characters_name (name),
  KEY ix_characters_owner (owner_accid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- slot_type: 0 equip, 1 inventario, 2 bau, 3 bau da guild, 10 cash,
--            17 itens de evento, PRAN_EQUIP_TYPE / PRAN_INV_TYPE
CREATE TABLE IF NOT EXISTS items (
  id               INT UNSIGNED NOT NULL AUTO_INCREMENT,
  slot_type        INT          NOT NULL DEFAULT 0,
  owner_id         INT UNSIGNED NOT NULL DEFAULT 0,
  slot             INT          NOT NULL DEFAULT 0,
  item_id          INT          NOT NULL DEFAULT 0,
  app              INT          NOT NULL DEFAULT 0,
  identific        INT          NOT NULL DEFAULT 0,
  effect1_index    INT          NOT NULL DEFAULT 0,
  effect1_value    INT          NOT NULL DEFAULT 0,
  effect2_index    INT          NOT NULL DEFAULT 0,
  effect2_value    INT          NOT NULL DEFAULT 0,
  effect3_index    INT          NOT NULL DEFAULT 0,
  effect3_value    INT          NOT NULL DEFAULT 0,
  min              INT          NOT NULL DEFAULT 0,
  max              INT          NOT NULL DEFAULT 0,
  refine           INT          NOT NULL DEFAULT 0,
  time             INT          NOT NULL DEFAULT 0,
  owner_mail_slot  INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_items_owner (owner_id, slot_type, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- type: 1 basicas, 2 outras, 3 pran
CREATE TABLE IF NOT EXISTS skills (
  id            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_charid  INT UNSIGNED NOT NULL,
  slot          INT NOT NULL DEFAULT 0,
  item          INT NOT NULL DEFAULT 0,
  level         INT NOT NULL DEFAULT 0,
  type          INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_skills_owner (owner_charid, type, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- barras de pran usam owner_charid = pran_id + 1024000
CREATE TABLE IF NOT EXISTS itembars (
  id            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_charid  INT UNSIGNED NOT NULL,
  slot          INT NOT NULL DEFAULT 0,
  item          INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_itembars_owner (owner_charid, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS buffs (
  id            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_charid  INT UNSIGNED NOT NULL,
  buff_index    INT NOT NULL DEFAULT 0,
  buff_time     VARCHAR(32) NOT NULL DEFAULT '0',   -- unix (ou data antiga em texto)
  PRIMARY KEY (id),
  KEY ix_buffs_owner (owner_charid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS quests (
  id          INT UNSIGNED NOT NULL AUTO_INCREMENT,
  charid      INT UNSIGNED NOT NULL,
  questid     INT NOT NULL DEFAULT 0,
  req1        INT NOT NULL DEFAULT 0,
  req2        INT NOT NULL DEFAULT 0,
  req3        INT NOT NULL DEFAULT 0,
  req4        INT NOT NULL DEFAULT 0,
  req5        INT NOT NULL DEFAULT 0,
  isdone      TINYINT NOT NULL DEFAULT 0,
  updated_at  BIGINT NOT NULL DEFAULT 0,
  created_at  BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_quests_char (charid, questid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS titles (
  id              INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_charid    INT UNSIGNED NOT NULL,
  title_index     INT NOT NULL DEFAULT 0,
  title_level     INT NOT NULL DEFAULT 0,
  title_progress  INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_titles_owner (owner_charid, title_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS prans (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  acc_id       INT UNSIGNED NOT NULL,
  char_id      INT UNSIGNED NOT NULL DEFAULT 0,
  item_id      INT NOT NULL DEFAULT 0,
  name         VARCHAR(32) NOT NULL DEFAULT '',
  food         INT NOT NULL DEFAULT 0,
  devotion     INT NOT NULL DEFAULT 0,
  p_cute       INT NOT NULL DEFAULT 0,
  p_smart      INT NOT NULL DEFAULT 0,
  p_sexy       INT NOT NULL DEFAULT 0,
  p_energetic  INT NOT NULL DEFAULT 0,
  p_tough      INT NOT NULL DEFAULT 0,
  p_corrupt    INT NOT NULL DEFAULT 0,
  level        INT NOT NULL DEFAULT 0,
  class        INT NOT NULL DEFAULT 0,
  hp           INT NOT NULL DEFAULT 0,
  max_hp       INT NOT NULL DEFAULT 0,
  mp           INT NOT NULL DEFAULT 0,
  max_mp       INT NOT NULL DEFAULT 0,
  xp           BIGINT NOT NULL DEFAULT 0,
  def_p        INT NOT NULL DEFAULT 0,
  def_m        INT NOT NULL DEFAULT 0,
  width        INT NOT NULL DEFAULT 0,
  chest        INT NOT NULL DEFAULT 0,
  leg          INT NOT NULL DEFAULT 0,
  created_at   BIGINT NOT NULL DEFAULT 0,
  updated_at   BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_prans_acc (acc_id),
  KEY ix_prans_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS friend_list (
  id                  INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_characterId   INT UNSIGNED NOT NULL,
  friend_characterId  INT UNSIGNED NOT NULL,
  active              TINYINT NOT NULL DEFAULT 1,
  registerDate        DATETIME NULL,
  lastUpdateDate      DATETIME NULL,
  PRIMARY KEY (id),
  KEY ix_friend_owner (owner_characterId, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Correio
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mails (
  id                 INT UNSIGNED NOT NULL AUTO_INCREMENT,
  characterId        INT UNSIGNED NOT NULL,
  sentCharacterId    INT UNSIGNED NOT NULL DEFAULT 0,
  sentCharacterName  VARCHAR(32)  NOT NULL DEFAULT '',
  title              VARCHAR(64)  NOT NULL DEFAULT '',
  textBody           VARCHAR(1024) NOT NULL DEFAULT '',
  slot               INT NOT NULL DEFAULT 0,
  sentGold           BIGINT NOT NULL DEFAULT 0,
  gold               BIGINT NOT NULL DEFAULT 0,
  returnDate         DATETIME NULL,
  sentDate           DATETIME NULL,
  checked            TINYINT NOT NULL DEFAULT 0,
  canReturn          TINYINT NOT NULL DEFAULT 0,
  hasItems           TINYINT NOT NULL DEFAULT 0,
  isFromAuction      TINYINT NOT NULL DEFAULT 0,
  mailReturned       TINYINT NOT NULL DEFAULT 0,
  active             TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  KEY ix_mails_char (characterId, active, checked)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS mails_items (
  id             INT UNSIGNED NOT NULL AUTO_INCREMENT,
  mail_id        INT UNSIGNED NOT NULL,
  slot           INT NOT NULL DEFAULT 0,
  item_id        INT NOT NULL DEFAULT 0,
  app            INT NOT NULL DEFAULT 0,
  identific      INT NOT NULL DEFAULT 0,
  effect1_index  INT NOT NULL DEFAULT 0,
  effect1_value  INT NOT NULL DEFAULT 0,
  effect2_index  INT NOT NULL DEFAULT 0,
  effect2_value  INT NOT NULL DEFAULT 0,
  effect3_index  INT NOT NULL DEFAULT 0,
  effect3_value  INT NOT NULL DEFAULT 0,
  min            INT NOT NULL DEFAULT 0,
  max            INT NOT NULL DEFAULT 0,
  refine         INT NOT NULL DEFAULT 0,
  time           INT NOT NULL DEFAULT 0,
  active         TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  KEY ix_mails_items_mail (mail_id, active, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Leilao
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS auction_items (
  id             INT UNSIGNED NOT NULL AUTO_INCREMENT,
  item_id        INT NOT NULL DEFAULT 0,
  app            INT NOT NULL DEFAULT 0,
  identific      INT NOT NULL DEFAULT 0,
  effect1_index  INT NOT NULL DEFAULT 0,
  effect2_index  INT NOT NULL DEFAULT 0,
  effect3_index  INT NOT NULL DEFAULT 0,
  effect1_value  INT NOT NULL DEFAULT 0,
  effect2_value  INT NOT NULL DEFAULT 0,
  effect3_value  INT NOT NULL DEFAULT 0,
  min            INT NOT NULL DEFAULT 0,
  max            INT NOT NULL DEFAULT 0,
  refine         INT NOT NULL DEFAULT 0,
  time           INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS auction (
  AuctionId        INT UNSIGNED NOT NULL AUTO_INCREMENT,
  CharacterId      INT UNSIGNED NOT NULL,
  ItemType         INT NOT NULL DEFAULT 0,
  ItemLevel        INT NOT NULL DEFAULT 0,
  ReinforceLevel   INT NOT NULL DEFAULT 0,
  RegisterDate     DATETIME NOT NULL,
  RegisterTime     INT NOT NULL DEFAULT 0,      -- horas
  SellingPrice     BIGINT NOT NULL DEFAULT 0,
  auction_itemsId  INT UNSIGNED NOT NULL,
  Active           TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (AuctionId),
  KEY ix_auction_active (Active, ItemType),
  KEY ix_auction_char (CharacterId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE OR REPLACE VIEW vwauction_getactiveoffers AS
SELECT
    a.AuctionId,
    a.CharacterId,
    c.name AS CharacterName,
    DATE_ADD(a.RegisterDate, INTERVAL a.RegisterTime HOUR) AS ExpireDate,
    a.SellingPrice,
    i.item_id AS ItemId,
    i.app AS ItemLookId,
    i.identific AS IdentificableAddOns,
    i.effect1_index AS EffectId_1,
    i.effect2_index AS EffectId_2,
    i.effect3_index AS EffectId_3,
    i.effect1_value AS EffectValue_1,
    i.effect2_value AS EffectValue_2,
    i.effect3_value AS EffectValue_3,
    i.min AS DurabilityMin,
    i.max AS DurabilityMax,
    i.refine AS Amount_Reinforce,
    i.time AS ItemTime,
    a.ItemType,
    a.ItemLevel,
    a.ReinforceLevel,
    a.Active
FROM auction a
JOIN auction_items i ON i.id = a.auction_itemsId
JOIN characters c ON c.id = a.CharacterId
WHERE a.Active = 1;

-- ---------------------------------------------------------------------
-- Guildas
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS guilds (
  id                 INT UNSIGNED NOT NULL AUTO_INCREMENT,
  slot               INT NOT NULL DEFAULT 0,
  name               VARCHAR(32) NOT NULL,
  nation             INT NOT NULL DEFAULT 0,
  experience         BIGINT NOT NULL DEFAULT 0,
  level              INT NOT NULL DEFAULT 1,
  totalmembers       INT NOT NULL DEFAULT 0,
  bravurepoints      INT NOT NULL DEFAULT 0,
  skillpoints        INT NOT NULL DEFAULT 0,
  promote            TINYINT NOT NULL DEFAULT 0,
  notice1            VARCHAR(128) NOT NULL DEFAULT '',
  notice2            VARCHAR(128) NOT NULL DEFAULT '',
  notice3            VARCHAR(128) NOT NULL DEFAULT '',
  site               VARCHAR(128) NOT NULL DEFAULT '',
  rank1              INT NOT NULL DEFAULT 0,
  rank2              INT NOT NULL DEFAULT 0,
  rank3              INT NOT NULL DEFAULT 0,
  rank4              INT NOT NULL DEFAULT 0,
  rank5              INT NOT NULL DEFAULT 0,
  ally_leader        INT NOT NULL DEFAULT 0,
  guild_ally1_index  INT NOT NULL DEFAULT 0,
  guild_ally1_name   VARCHAR(32) NOT NULL DEFAULT '',
  guild_ally2_index  INT NOT NULL DEFAULT 0,
  guild_ally2_name   VARCHAR(32) NOT NULL DEFAULT '',
  guild_ally3_index  INT NOT NULL DEFAULT 0,
  guild_ally3_name   VARCHAR(32) NOT NULL DEFAULT '',
  guild_ally4_index  INT NOT NULL DEFAULT 0,
  guild_ally4_name   VARCHAR(32) NOT NULL DEFAULT '',
  storage_gold       BIGINT NOT NULL DEFAULT 0,
  leader_char_index  INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY ux_guilds_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS guilds_players (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  guild_index  INT UNSIGNED NOT NULL,
  char_index   INT UNSIGNED NOT NULL,
  name         VARCHAR(32) NOT NULL DEFAULT '',
  player_rank  INT NOT NULL DEFAULT 0,
  classinfo    INT NOT NULL DEFAULT 0,
  level        INT NOT NULL DEFAULT 0,
  logged       TINYINT NOT NULL DEFAULT 0,
  last_login   INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY ix_guilds_players_guild (guild_index),
  KEY ix_guilds_players_char (char_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Nacoes e templos (devires)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS nations (
  nation_id                INT NOT NULL,
  channel_id               INT NOT NULL,
  nation_name              VARCHAR(32) NOT NULL DEFAULT '',
  nation_rank              INT NOT NULL DEFAULT 0,
  guild_id_marshal         INT NOT NULL DEFAULT 0,
  guild_id_tactician       INT NOT NULL DEFAULT 0,
  guild_id_judge           INT NOT NULL DEFAULT 0,
  guild_id_treasurer       INT NOT NULL DEFAULT 0,
  citizen_tax              INT NOT NULL DEFAULT 0,
  visitor_tax              INT NOT NULL DEFAULT 0,
  settlement               INT NOT NULL DEFAULT 0,
  nation_ally              INT NOT NULL DEFAULT 0,
  marechal_ally            VARCHAR(32) NOT NULL DEFAULT '',
  ally_date                INT NOT NULL DEFAULT 0,
  nation_gold              BIGINT NOT NULL DEFAULT 0,
  cerco_guildid_attack_A1  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_A2  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_A3  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_A4  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_B1  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_B2  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_B3  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_B4  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_C1  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_C2  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_C3  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_C4  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_D1  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_D2  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_D3  INT NOT NULL DEFAULT 0,
  cerco_guildid_attack_D4  INT NOT NULL DEFAULT 0,
  PRIMARY KEY (nation_id),
  UNIQUE KEY ux_nations_channel (channel_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- devir_id: nacao 1 -> 1..5, nacao 2 -> 6..10, ... (Nation.pas)
CREATE TABLE IF NOT EXISTS devires (
  devir_id       INT NOT NULL,
  nation_id      INT NOT NULL,
  slot1_itemid   INT NOT NULL DEFAULT 0,
  slot1_name     VARCHAR(32) NOT NULL DEFAULT '',
  slot1_timecap  BIGINT NOT NULL DEFAULT 0,
  slot1_able     TINYINT NOT NULL DEFAULT 1,
  slot2_itemid   INT NOT NULL DEFAULT 0,
  slot2_name     VARCHAR(32) NOT NULL DEFAULT '',
  slot2_timecap  BIGINT NOT NULL DEFAULT 0,
  slot2_able     TINYINT NOT NULL DEFAULT 1,
  slot3_itemid   INT NOT NULL DEFAULT 0,
  slot3_name     VARCHAR(32) NOT NULL DEFAULT '',
  slot3_timecap  BIGINT NOT NULL DEFAULT 0,
  slot3_able     TINYINT NOT NULL DEFAULT 1,
  slot4_itemid   INT NOT NULL DEFAULT 0,
  slot4_name     VARCHAR(32) NOT NULL DEFAULT '',
  slot4_timecap  BIGINT NOT NULL DEFAULT 0,
  slot4_able     TINYINT NOT NULL DEFAULT 1,
  slot5_itemid   INT NOT NULL DEFAULT 0,
  slot5_name     VARCHAR(32) NOT NULL DEFAULT '',
  slot5_timecap  BIGINT NOT NULL DEFAULT 0,
  slot5_able     TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (devir_id),
  KEY ix_devires_nation (nation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Ferramentas de GM / loja
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS gm_accounts (
  id               INT UNSIGNED NOT NULL AUTO_INCREMENT,
  username         VARCHAR(32) NOT NULL,
  password         VARCHAR(64) NOT NULL DEFAULT '',
  account_status   INT NOT NULL DEFAULT 1,
  password_errors  INT NOT NULL DEFAULT 0,
  master_priv      INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY ux_gm_accounts_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS gm_commands (
  id              INT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner_gmid      INT NOT NULL DEFAULT 0,
  command_type    INT NOT NULL DEFAULT 0,
  runned          TINYINT NOT NULL DEFAULT 0,
  command         TEXT NULL,
  created_at      DATETIME NULL,
  runned_at       DATETIME NULL,
  runned_by       VARCHAR(32) NOT NULL DEFAULT '',
  target_name     VARCHAR(32) NOT NULL DEFAULT '',
  target_itemid   INT NOT NULL DEFAULT 0,
  target_itemcnt  INT NOT NULL DEFAULT 0,
  refused         TINYINT NOT NULL DEFAULT 0,
  refused_at      DATETIME NULL,
  reason_run      VARCHAR(255) NOT NULL DEFAULT '',
  reason_refuse   VARCHAR(255) NOT NULL DEFAULT '',
  reason_create   VARCHAR(255) NOT NULL DEFAULT '',
  coupom          VARCHAR(64) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY ix_gm_commands_coupom (coupom)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS founders (
  id                 INT UNSIGNED NOT NULL AUTO_INCREMENT,
  idtransaction      VARCHAR(64) NOT NULL DEFAULT '',
  name               VARCHAR(128) NOT NULL DEFAULT '',
  dateofcomprovant   DATE NULL,
  valueofcomprovant  DECIMAL(12,2) NOT NULL DEFAULT 0,
  coupom             VARCHAR(64) NOT NULL DEFAULT '',
  validated          TINYINT NOT NULL DEFAULT 0,
  validated_gmid     INT NOT NULL DEFAULT 0,
  created_at         DATETIME NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY ix_founders_transaction (idtransaction)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS web_store_queue (
  id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
  char_name  VARCHAR(32) NOT NULL,
  item_id    INT NOT NULL,
  quantity   INT NOT NULL DEFAULT 1,
  delivered  TINYINT NOT NULL DEFAULT 0,
  created_at DATETIME NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY ix_web_store_queue_delivered (delivered)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- Diversos
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS server_info (
  server_id        INT NOT NULL,
  pran_count       INT NOT NULL DEFAULT 0,
  character_count  INT NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- usadas apenas por rotinas de exportacao (comentadas) em Functions.pas
CREATE TABLE IF NOT EXISTS data_exp (
  id     INT NOT NULL AUTO_INCREMENT,
  level  INT NOT NULL DEFAULT 0,
  exp    BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS data_make_item_ingredients (
  id        INT NOT NULL,
  item_id   INT NOT NULL DEFAULT 0,
  quantity  INT NOT NULL DEFAULT 0,
  KEY ix_make_item (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================================
-- Dados iniciais
-- =====================================================================
INSERT IGNORE INTO server_info (server_id, pran_count, character_count) VALUES (1, 0, 0);

-- um registro de nacao por canal do SL.bin (canal 0 = nacao 1, canal 1 = nacao 2)
INSERT IGNORE INTO nations (nation_id, channel_id, nation_name) VALUES
  (1, 0, 'Yggdrasil'),
  (2, 1, 'Mjolnir');

INSERT IGNORE INTO devires (devir_id, nation_id) VALUES
  (1, 1), (2, 1), (3, 1), (4, 1), (5, 1),
  (6, 2), (7, 2), (8, 2), (9, 2), (10, 2);

-- limpa timecaps invalidos (ver "Docs bugs/Relics SQL.txt")
UPDATE devires SET
  slot1_timecap = IF(slot1_timecap < 1000000000, 0, slot1_timecap),
  slot2_timecap = IF(slot2_timecap < 1000000000, 0, slot2_timecap),
  slot3_timecap = IF(slot3_timecap < 1000000000, 0, slot3_timecap),
  slot4_timecap = IF(slot4_timecap < 1000000000, 0, slot4_timecap),
  slot5_timecap = IF(slot5_timecap < 1000000000, 0, slot5_timecap);
