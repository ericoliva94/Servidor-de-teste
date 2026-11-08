-- O leilão do Go (store/auction.go) guarda as datas como segundos unix e os três
-- efeitos do item. O banco criado antes tinha outra forma de auction_offers
-- (listed_at, sem efeitos). Só acrescenta colunas; nada é apagado.
ALTER TABLE auction_offers
  MODIFY COLUMN listed_at DATETIME NULL DEFAULT NULL,
  MODIFY COLUMN price BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN registered_at BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN expires_at    BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN item_e1i INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN item_e2i INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN item_e3i INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN item_e1v INT NOT NULL DEFAULT 0,
  ADD COLUMN item_e2v INT NOT NULL DEFAULT 0,
  ADD COLUMN item_e3v INT NOT NULL DEFAULT 0;
