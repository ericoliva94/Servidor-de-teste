-- Tabelas que o porte Go usa e que nao existem no banco original.
--
-- O servidor Delphi guardava guildas em MySQL (guilds, guilds_players) mas o
-- correio em arquivo por personagem, em Data/Mails. A camada de leitura
-- daqueles arquivos estava esvaziada na origem (TMailFunctions.GetMails e so
-- "Result := True"), o que fazia cada carta recebida apagar a caixa inteira e
-- impedia qualquer leitura. Ver GoServer/PORTE.md.
--
-- Rode este arquivo contra o banco do servidor antes de subir o porte:
--   mysql -u root -p aika_db4 < GoServer/SCHEMA.sql

-- Guildas. A origem ja tinha estas duas, mas sem as colunas de aviso, site e
-- permissao, que ela guardava apenas em memoria e perdia a cada reinicio.
CREATE TABLE IF NOT EXISTS guilds (
  id                INT UNSIGNED NOT NULL,
  name              VARCHAR(18)  NOT NULL DEFAULT '',
  nation            TINYINT UNSIGNED NOT NULL DEFAULT 0,
  exp               INT UNSIGNED NOT NULL DEFAULT 0,
  level             TINYINT UNSIGNED NOT NULL DEFAULT 0,
  leader_char_index INT UNSIGNED NOT NULL DEFAULT 0,
  notice1           VARCHAR(34)  NOT NULL DEFAULT '',
  notice2           VARCHAR(34)  NOT NULL DEFAULT '',
  notice3           VARCHAR(34)  NOT NULL DEFAULT '',
  site              VARCHAR(38)  NOT NULL DEFAULT '',
  ranks_config      VARCHAR(32)  NOT NULL DEFAULT '0,0,0,0,0',
  bravure_points    INT UNSIGNED NOT NULL DEFAULT 0,
  skill_points      INT UNSIGNED NOT NULL DEFAULT 0,
  promote           TINYINT UNSIGNED NOT NULL DEFAULT 0,
  storage_gold      BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guilds_players (
  guild_index INT UNSIGNED NOT NULL,
  char_index  INT UNSIGNED NOT NULL,
  name        VARCHAR(20)  NOT NULL DEFAULT '',
  player_rank TINYINT UNSIGNED NOT NULL DEFAULT 4,
  classinfo   TINYINT UNSIGNED NOT NULL DEFAULT 0,
  level       TINYINT UNSIGNED NOT NULL DEFAULT 0,
  logged      TINYINT UNSIGNED NOT NULL DEFAULT 0,
  last_login  INT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (guild_index, char_index),
  KEY idx_char (char_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Correio. Cada carta leva ate cinco anexos, no mesmo formato de TItem.
CREATE TABLE IF NOT EXISTS mails (
  owner_char_index INT UNSIGNED    NOT NULL,
  mail_index       BIGINT UNSIGNED NOT NULL,
  from_char_index  INT UNSIGNED    NOT NULL DEFAULT 0,
  from_nick        VARCHAR(16)     NOT NULL DEFAULT '',
  titulo           VARCHAR(32)     NOT NULL DEFAULT '',
  texto            VARCHAR(512)    NOT NULL DEFAULT '',
  gold             INT UNSIGNED    NOT NULL DEFAULT 0,
  sent_at          BIGINT          NOT NULL DEFAULT 0,
  return_at        BIGINT          NOT NULL DEFAULT 0,
  checked          TINYINT UNSIGNED NOT NULL DEFAULT 0,
  item_return      TINYINT UNSIGNED NOT NULL DEFAULT 0,
  check_item       TINYINT UNSIGNED NOT NULL DEFAULT 0,
  leilao           TINYINT UNSIGNED NOT NULL DEFAULT 0,

  item0_id INT UNSIGNED NOT NULL DEFAULT 0, item0_app INT UNSIGNED NOT NULL DEFAULT 0,
  item0_identific INT NOT NULL DEFAULT 0,
  item0_min INT UNSIGNED NOT NULL DEFAULT 0, item0_max INT UNSIGNED NOT NULL DEFAULT 0,
  item0_refine INT UNSIGNED NOT NULL DEFAULT 0, item0_time INT UNSIGNED NOT NULL DEFAULT 0,

  item1_id INT UNSIGNED NOT NULL DEFAULT 0, item1_app INT UNSIGNED NOT NULL DEFAULT 0,
  item1_identific INT NOT NULL DEFAULT 0,
  item1_min INT UNSIGNED NOT NULL DEFAULT 0, item1_max INT UNSIGNED NOT NULL DEFAULT 0,
  item1_refine INT UNSIGNED NOT NULL DEFAULT 0, item1_time INT UNSIGNED NOT NULL DEFAULT 0,

  item2_id INT UNSIGNED NOT NULL DEFAULT 0, item2_app INT UNSIGNED NOT NULL DEFAULT 0,
  item2_identific INT NOT NULL DEFAULT 0,
  item2_min INT UNSIGNED NOT NULL DEFAULT 0, item2_max INT UNSIGNED NOT NULL DEFAULT 0,
  item2_refine INT UNSIGNED NOT NULL DEFAULT 0, item2_time INT UNSIGNED NOT NULL DEFAULT 0,

  item3_id INT UNSIGNED NOT NULL DEFAULT 0, item3_app INT UNSIGNED NOT NULL DEFAULT 0,
  item3_identific INT NOT NULL DEFAULT 0,
  item3_min INT UNSIGNED NOT NULL DEFAULT 0, item3_max INT UNSIGNED NOT NULL DEFAULT 0,
  item3_refine INT UNSIGNED NOT NULL DEFAULT 0, item3_time INT UNSIGNED NOT NULL DEFAULT 0,

  item4_id INT UNSIGNED NOT NULL DEFAULT 0, item4_app INT UNSIGNED NOT NULL DEFAULT 0,
  item4_identific INT NOT NULL DEFAULT 0,
  item4_min INT UNSIGNED NOT NULL DEFAULT 0, item4_max INT UNSIGNED NOT NULL DEFAULT 0,
  item4_refine INT UNSIGNED NOT NULL DEFAULT 0, item4_time INT UNSIGNED NOT NULL DEFAULT 0,

  PRIMARY KEY (owner_char_index, mail_index),
  KEY idx_index (mail_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Skills. A origem ja tinha esta tabela; as colunas abaixo sao as que o
-- porte le e grava.
CREATE TABLE IF NOT EXISTS skills (
  owner_charid INT UNSIGNED NOT NULL,
  slot         INT UNSIGNED NOT NULL,
  type         TINYINT UNSIGNED NOT NULL,
  item         INT UNSIGNED NOT NULL DEFAULT 0,
  level        INT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (owner_charid, type, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Leilao. A origem guardava as ofertas na tabela `auction` com o item em
-- colunas soltas e consultava com "SELECT max(id)" fora da transacao de
-- insercao. Aqui o indice da oferta vem de um contador no proprio banco e o
-- fechamento e condicional (UPDATE ... WHERE active = 1), para que dois
-- compradores simultaneos nao levem o mesmo item.
CREATE TABLE IF NOT EXISTS auction_offers (
  offer_index       BIGINT UNSIGNED NOT NULL,
  seller_char_index INT UNSIGNED NOT NULL,
  seller_name       VARCHAR(20) NOT NULL DEFAULT '',
  price             INT UNSIGNED NOT NULL DEFAULT 0,
  hours             SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  listed_at         DATETIME NOT NULL,
  active            TINYINT UNSIGNED NOT NULL DEFAULT 1,
  sold_to           INT UNSIGNED NOT NULL DEFAULT 0,

  item_id        INT UNSIGNED NOT NULL DEFAULT 0,
  item_app       INT UNSIGNED NOT NULL DEFAULT 0,
  item_identific INT NOT NULL DEFAULT 0,
  item_min       INT UNSIGNED NOT NULL DEFAULT 0,
  item_max       INT UNSIGNED NOT NULL DEFAULT 0,
  item_refine    INT UNSIGNED NOT NULL DEFAULT 0,
  item_time      INT UNSIGNED NOT NULL DEFAULT 0,

  PRIMARY KEY (offer_index),
  KEY idx_active (active, listed_at),
  KEY idx_seller (seller_char_index, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Ouro do armazem da conta e do bau da guilda.
--
-- A origem ja usava as duas colunas (accounts.storage_gold e
-- guilds.storage_gold) e os itens dos dois ficavam na tabela items, com
-- slot_type 2 (armazem, owner_id = id da conta) e 3 (bau, owner_id = indice
-- da guilda). Rode estes ALTER se o banco vier de uma instalacao que nao as
-- tenha; num banco que ja as tenha o comando falha com "Duplicate column" e
-- pode ser ignorado.
--
--   ALTER TABLE accounts ADD COLUMN storage_gold BIGINT NOT NULL DEFAULT 0;
--   ALTER TABLE guilds   ADD COLUMN storage_gold BIGINT UNSIGNED NOT NULL DEFAULT 0;
--   ALTER TABLE accounts ADD COLUMN cash INT UNSIGNED NOT NULL DEFAULT 0;
--
-- O inventario premium usa a mesma tabela items, com slot_type 10 e owner_id
-- igual ao id da CONTA. O que cada slot guarda e o indice do catalogo premium
-- (PI.bin), nao o de um item.
