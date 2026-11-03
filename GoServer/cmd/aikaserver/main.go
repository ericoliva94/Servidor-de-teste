// Command aikaserver sobe o servidor Aika portado para Go.
//
// Estado: a fatia vertical do login. O servidor abre a porta 8831, atende o
// pacote $81 validando o token contra a tabela accounts e responde o $82.
// As demais portas (canais, token HTTP, console) entram nas fases seguintes;
// ver GoServer/PORTE.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"aika/internal/auth"
	"aika/internal/config"
	"aika/internal/game"
	"aika/internal/gamedata"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/store"
	"aika/internal/tokensrv"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		iniPath  = flag.String("ini", "AikaServer.ini", "caminho do AikaServer.ini")
		dataDir  = flag.String("data", dataDirDefault(), "pasta com as tabelas .bin")
		logLevel = flag.String("log", "info", "nivel de log: debug, info, warn, error")
		skipDB   = flag.Bool("no-db", false, "sobe sem banco (so para conferir a carga)")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	slog.SetDefault(log)

	cfg, err := config.LoadServer(*iniPath)
	if err != nil {
		return err
	}
	log.Info("configuracao lida",
		"ini", *iniPath, "versao", cfg.Version,
		"max_usuarios", cfg.MaxUsers, "canais", cfg.Channels)

	// As tabelas estaticas sobem antes das portas: sem elas o servidor aceita
	// jogador e depois nao sabe responder.
	tables, err := gamedata.Load(*dataDir)
	if err != nil {
		return fmt.Errorf("carregar %s: %w", *dataDir, err)
	}
	for _, t := range tables.Loaded {
		log.Info("tabela carregada", "detalhe", t.String())
	}
	if len(tables.Missing) > 0 {
		// Arquivo ausente e aviso, nao erro: o Delphi tambem seguia em frente.
		log.Warn("tabelas ausentes", "arquivos", tables.Missing)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		accounts auth.Accounts
		chars    game.Characters
		items    game.ItemSource
		numerics game.NumericStore
		tokens   tokensrv.Store
	)
	if *skipDB {
		log.Warn("subindo sem banco: nenhum login sera aceito")
		accounts = noAccounts{}
		chars = noChars{}
		numerics = noChars{}
		items = noItems{}
	} else {
		db, err := store.Open(ctx, store.Config{
			Server:   cfg.MySQLServer,
			Port:     cfg.MySQLPort,
			User:     cfg.MySQLUser,
			Password: cfg.MySQLPassword,
			Database: cfg.MySQLDatabase,
			Log:      log,
		})
		if err != nil {
			return err
		}
		defer db.Close()
		log.Info("mysql conectado",
			"servidor", cfg.MySQLServer, "porta", cfg.MySQLPort, "banco", cfg.MySQLDatabase)
		accs := store.Accounts{DB: db}
		accounts = accs
		tokens = accs
		cs := store.Characters{DB: db}
		chars = cs
		numerics = cs
		items = store.Items{DB: db}
	}

	loginSrv := netsrv.New(netsrv.Config{
		Name:     "login",
		Addr:     cfg.LoginAddr,
		MaxConns: int(cfg.MaxUsers),
		Log:      log,
	}, &auth.Handler{Accounts: accounts})

	// Os mobs vem de dois CSV; ausencia e aviso, nao erro, como em Load.pas.
	mobs, err := mobdata.Load(*dataDir)
	if err != nil {
		if errors.Is(err, mobdata.ErrNotFound) {
			log.Warn("tabelas de mob ausentes: o servidor sobe sem mobs",
				"erro", err)
		} else {
			return fmt.Errorf("carregar mobs: %w", err)
		}
	}
	for _, a := range mobs.Avisos {
		log.Warn("tabelas de mob", "aviso", a)
	}
	mundoMobs := game.NewMobWorld(mobs)
	log.Info("mobs carregados",
		"especies", len(mobs.Species), "pontos", mobs.SpawnCount(),
		"ativos", mundoMobs.Count())

	// O canal guarda quem esta conectado e aloca o ClientID. Sem ele o
	// servidor atende o login mas ninguem ve ninguem.
	canal := game.NewChannel(0, cfg.MaxUsers)

	canalHandler := &game.Handler{
		Channel:        0,
		Chan:           canal,
		Accounts:       accounts,
		Chars:          chars,
		Items:          items,
		Numerics:       numerics,
		Durations:      tables,
		Data:           tables,
		WatchDistance:  cfg.DistanceToWatch,
		ForgetDistance: cfg.DistanceToForget,
		Log:            log,
		ServerVersion:  uint16(cfg.Version),
		Mobs:           mundoMobs,
		Exp:            tables,
		ExpMultiplier:  cfg.ExpMultiplier,
		MobEsquiva:     cfg.MobEsquiva,
		MobCritRes:     cfg.MobCritRes,
		MobDuploRes:    cfg.MobDuploRes,
	}

	channelSrv := netsrv.New(netsrv.Config{
		Name:     "canal",
		Addr:     cfg.ChannelAddr,
		MaxConns: int(cfg.MaxUsers),
		Log:      log,
	}, canalHandler)

	// Abre as portas antes de anunciar: o original so descobria falha de bind
	// pelo log, depois de dizer que havia iniciado.
	for _, srv := range []*netsrv.Server{loginSrv, channelSrv} {
		if err := srv.Listen(); err != nil {
			return err
		}
	}

	// O servidor de token so sobe com banco: ele existe para gravar o token
	// em accounts, e sem escrita nao faz nada util.
	var tokenSrv *tokensrv.Server
	if tokens != nil {
		tokenSrv = tokensrv.New(tokensrv.Config{
			Addr: cfg.TokenAddr,
			Log:  log,
		}, tokens, onlineCounter{
			loginSrv: loginSrv, channelSrv: channelSrv, canal: canal})
		if err := tokenSrv.Listen(); err != nil {
			return err
		}
	}

	campos := []any{
		"login", loginSrv.Addr().String(),
		"canal", channelSrv.Addr().String(),
		"ver", cfg.DistanceToWatch,
		"esquecer", cfg.DistanceToForget,
	}
	if tokenSrv != nil {
		campos = append(campos, "token", tokenSrv.Addr().String())
	}
	log.Info("servidor pronto", campos...)

	// Um erro em qualquer uma das portas encerra as outras.
	ctx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()

	// O respawn de mob roda no servidor, nao dentro da visibilidade de um
	// jogador: na origem o mob so voltava quando alguem passava perto, e sem
	// ninguem por perto ele ficava morto indefinidamente.
	go respawnLoop(ctx, canalHandler, log)
	go mobAILoop(ctx, canalHandler, log)

	tarefas := 2
	errc := make(chan error, 3)
	for _, srv := range []*netsrv.Server{loginSrv, channelSrv} {
		go func(s *netsrv.Server) { errc <- s.Serve(ctx) }(srv)
	}
	if tokenSrv != nil {
		tarefas++
		go func() { errc <- tokenSrv.Serve(ctx) }()
	}

	var firstErr error
	for range tarefas {
		if err := <-errc; err != nil && !errors.Is(err, context.Canceled) {
			if firstErr == nil {
				firstErr = err
			}
			cancelAll()
		}
	}
	log.Info("servidor encerrado")
	return firstErr
}

// respawnInterval e o passo do laco de respawn. O tempo de cada mob vem da
// coluna do CSV; este e so o passo em que o servidor confere.
const respawnInterval = time.Second

// respawnLoop devolve ao mundo os mobs cujo tempo de respawn passou.
func respawnLoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	t := time.NewTicker(respawnInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := h.RespawnTick(); n > 0 {
				log.Debug("mobs de volta ao mapa", "quantidade", n)
			}
		}
	}
}

// mobAILoop faz os mobs andarem e atacarem.
//
// Na origem eram nove threads por canal: tres de spawn e tres de ataque a
// cada 1000 ms e tres de movimento a cada 600 ms, cada uma varrendo um terco
// do vetor de 22.500 posicoes mesmo com o mapa vazio. Aqui sao dois tickers
// com os mesmos tempos, e a varredura e so sobre os mobs que algum jogador
// esta vendo.
func mobAILoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	atacar := time.NewTicker(game.MobAttackTick)
	defer atacar.Stop()
	andar := time.NewTicker(game.MobMoveTick)
	defer andar.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-atacar.C:
			if n, _ := h.MobTick(t); n > 0 {
				log.Debug("mobs atacaram", "quantidade", n)
			}
		case t := <-andar.C:
			if n := h.MobMoveTickRun(t); n > 0 {
				log.Debug("mobs andaram", "quantidade", n)
			}
		}
	}
}

// onlineCounter informa quantos jogadores estao conectados, somando as duas
// portas, para a rota de lista de servidores do token.
type onlineCounter struct {
	loginSrv   *netsrv.Server
	channelSrv *netsrv.Server
	canal      *game.Channel
}

func (o onlineCounter) OnlinePlayers() int {
	if o.canal != nil {
		// Conta quem esta em jogo, nao socket aberto: o original somava
		// conexoes e a lista de servidores mostrava gente que ainda estava
		// na tela de login.
		return o.canal.Count()
	}
	return int(o.channelSrv.Conns())
}

// noChars devolve lista vazia. Usado com -no-db.
type noChars struct{}

func (noChars) ByAccount(context.Context, uint32) ([store.MaxCharacterSlots]store.Character, []string, error) {
	var zero [store.MaxCharacterSlots]store.Character
	return zero, nil, nil
}

func (noChars) SaveNumeric(context.Context, uint32, string) error { return nil }

func (noChars) SaveNumericErrors(context.Context, uint32, int) error { return nil }

// noItems devolve conjuntos vazios. Usado com -no-db.
type noItems struct{}

func (noItems) Equip(context.Context, uint32) (store.ItemSet, error) {
	return store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}, nil
}

func (noItems) Inventory(context.Context, uint32) (store.ItemSet, error) {
	return store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}, nil
}

func (noItems) Storage(context.Context, uint32) (store.ItemSet, error) {
	return store.ItemSet{Slots: make([]model.Item, store.StorageSlots)}, nil
}

func (noItems) Cash(context.Context, uint32) (store.ItemSet, error) {
	return store.ItemSet{Slots: make([]model.Item, store.CashSlots)}, nil
}

func (noItems) SaveEquip(context.Context, uint32, store.ItemSet) error { return nil }

func (noItems) SaveInventory(context.Context, uint32, store.ItemSet) error { return nil }

// noAccounts recusa todo login. Usado com -no-db.
type noAccounts struct{}

func (noAccounts) ByUsername(context.Context, string) (auth.Account, bool, error) {
	return auth.Account{}, false, nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: l,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(time.Now().Format("15:04:05"))
			}
			return a
		},
	}))
}

// dataDirDefault acha a pasta Data: quem roda de dentro de GoServer a
// encontra em ../Server/Bin/Data, e quem roda de Server/Bin a tem ao lado.
func dataDirDefault() string {
	for _, p := range []string{
		"Data",
		filepath.Join("..", "Server", "Bin", "Data"),
		filepath.Join("..", "..", "Server", "Bin", "Data"),
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "Data"
}
