// Command aikaserver sobe o servidor Aika portado para Go.
//
// Estado: a fatia vertical do login. O servidor abre a porta 8831, atende o
// pacote $81 validando o token contra a tabela accounts e responde o $82.
// As demais portas (canais, token HTTP, console) entram nas fases seguintes;
// ver GoServer/PORTE.md.
package main

import (
	"aika/internal/charcreate"
	"aika/internal/devir"
	nationpkg "aika/internal/nation"
	pranpkg "aika/internal/pran"
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
	"aika/internal/console"
	"aika/internal/dropdata"
	"aika/internal/dungeon"
	"aika/internal/dungeondata"
	"aika/internal/game"
	"aika/internal/gamedata"
	"aika/internal/guild"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/serverlist"
	"aika/internal/skillfn"
	"aika/internal/store"
	"aika/internal/titledata"
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
		iniPath       = flag.String("ini", "AikaServer.ini", "caminho do AikaServer.ini")
		dataDir       = flag.String("data", dataDirDefault(), "pasta com as tabelas .bin")
		logLevel      = flag.String("log", "info", "nivel de log: debug, info, warn, error")
		skipDB        = flag.Bool("no-db", false, "sobe sem banco (so para conferir a carga)")
		nation        = flag.Int("nation", 1, "nacao deste servidor (define os 5 templos de reliquia: nacao 1 = devir 1..5)")
		testeMutantes = flag.Bool("teste-mutantes", false, "MODO DE TESTE: mutantes do nivel 60 (Planta Carnivora, Croshu, Buto) sempre deixam um acessorio e ficam com defesa reduzida")
		testeDefMax   = flag.Uint("teste-def-max", 700, "no modo de teste, teto da defesa fisica e magica desses mutantes")
		trace         = flag.Bool("trace", false, "registra todo pacote enviado e recebido (volume alto; para achar o pacote que trava o cliente)")
		multiCanal    = flag.Bool("multicanal", false, "sobe todos os canais do Channels do .ini, cada um no IP do SL.bin (porta 8822); sem esta opcao sobe um canal so, em :8822")
		slPath        = flag.String("sl", "", "caminho do SL.bin (padrao: ao lado da pasta de dados)")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	netsrv.SetTrace(*trace)
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
	titleTable, err := titledata.Load(*dataDir)
	if err != nil {
		log.Warn("titulos indisponiveis", "erro", err)
	}
	for _, w := range tables.MakeWarnings {
		log.Warn("fabricacao: linha ignorada", "detalhe", w)
	}
	log.Info("receitas de fabricacao", "receitas", len(tables.MakeItems), "ingredientes", len(tables.MakeIngredients))
	if len(tables.Missing) > 0 {
		// Arquivo ausente e aviso, nao erro: o Delphi tambem seguia em frente.
		log.Warn("tabelas ausentes", "arquivos", tables.Missing)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		accounts      auth.Accounts
		chars         game.Characters
		items         game.ItemSource
		skillDB       game.SkillStore
		guildDB       game.GuildStore
		mailDB        game.MailStore
		auctionDB     game.AuctionStore
		friendDB      game.FriendStore
		couponDB      game.CouponStore
		pranDB        game.PranStore
		titleDB       game.TitleStore
		eventDB       game.EventStore
		gmDB          game.GMStore
		gmCmds        game.GMCommandStore
		gmFounders    game.GMFounderStore
		accountNation game.AccountNationStore
		charSave      game.CharacterSaver
		charDelete    game.CharacterDeleter
		charCreate    game.CharacterCreator
		devirDB       game.DevirStore
		nationDB      game.NationStore
		itemBarDB     game.ItemBarStore
		teleportDB    game.TeleportStore
		numerics      game.NumericStore
		tokens        tokensrv.Store
		// contaOuro grava o ouro do armazem e contaCash o saldo premium;
		// nil sem banco.
		contaOuro game.StorageGoldStore
		contaCash game.CashStore
	)
	if *skipDB {
		log.Warn("subindo sem banco: nenhum login sera aceito")
		accounts = noAccounts{}
		chars = noChars{}
		numerics = noChars{}
		items = noItems{}
		skillDB = noSkills{}
		guildDB = noGuilds{}
		mailDB = nil    // sem banco nao ha correio
		auctionDB = nil // nem leilao
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
		contaOuro = accs
		contaCash = accs
		cs := store.Characters{DB: db}
		itemBarDB = store.ItemBars{DB: db}
		teleportDB = cs
		charSave = cs
		charDelete = cs
		charCreate = cs
		devirDB = store.Devires{DB: db}
		nationDB = store.Nations{DB: db}
		chars = cs
		numerics = cs
		items = store.Items{DB: db}
		skillDB = store.Skills{DB: db}
		guildDB = store.Guilds{DB: db}
		mailDB = store.Mails{DB: db}
		auctionDB = store.Auctions{DB: db}
		friendDB = store.Friends{DB: db}
		couponDB = store.Coupons{DB: db}
		pranDB = store.Prans{DB: db}
		titleDB = store.Titles{DB: db}
		eventDB = store.Events{DB: db}
		gmDB = store.GMAccounts{DB: db}
		gmCmds = store.GMAccounts{DB: db}
		gmFounders = store.GMAccounts{DB: db}
		accountNation = store.AccountNations{DB: db}
	}

	loginSrv := netsrv.New(netsrv.Config{
		Name:     "login",
		Addr:     cfg.LoginAddr,
		MaxConns: int(cfg.MaxUsers),
		Log:      log,
	}, &auth.Handler{Accounts: accounts})

	// Os mobs vem de dois CSV; ausencia e aviso, nao erro, como em Load.pas.
	templates, avisosModelos := charcreate.Load(filepath.Join(*dataDir, "BaseAccs"))
	for _, w := range avisosModelos {
		log.Warn("modelo de personagem", "detalhe", w)
	}
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

	// As listas de drop vem de treze CSV; ausencia e aviso, como em Load.pas.
	drops, err := dropdata.Load(*dataDir)
	if err != nil {
		return fmt.Errorf("carregar listas de drop: %w", err)
	}
	for _, a := range drops.Avisos {
		log.Warn("listas de drop", "aviso", a)
	}
	if len(drops.Missing) > 0 {
		log.Warn("listas de drop ausentes", "arquivos", drops.Missing)
	}
	log.Info("listas de drop carregadas",
		"categorias", dropdata.CategoryCount, "itens", drops.Total())
	log.Info("mobs carregados",
		"especies", len(mobs.Species), "pontos", mobs.SpawnCount(),
		"ativos", mundoMobs.Count())

	// As dungeons vem do Dungeons.csv mais um par de CSV por dificuldade.
	// A origem tinha a chamada de carga comentada (ServerSocket.pas:493)
	// porque o laco de carga escrevia duas fichas de dungeon fora de um vetor
	// de um elemento; aqui a carga roda e ausencia e aviso, nao erro.
	dungeons, err := dungeondata.Load(*dataDir)
	if err != nil {
		log.Warn("dungeons nao carregadas: o servidor sobe sem dungeon",
			"erro", err)
	} else {
		for _, a := range dungeons.Avisos {
			log.Warn("tabelas de dungeon", "aviso", a)
		}
		total := 0
		for id := range dungeons.Defs {
			for dif := uint32(0); dif < dungeondata.Difficulties; dif++ {
				if d, ok := dungeons.Get(id, dif); ok {
					total += d.SpawnCount()
				}
			}
		}
		log.Info("dungeons carregadas",
			"dungeons", len(dungeons.Defs),
			"dificuldades", dungeondata.Difficulties, "pontos", total)
	}

	// As guildas sobem do banco antes das portas: um jogador entrando antes
	// disso apareceria sem guilda.
	registroGuildas := guild.NewRegistry()

	// Onde cada canal escuta. Por padrao e um canal so, em :8822, como sempre.
	// Com -multicanal sobem os do Channels do .ini, cada um no IP que o SL.bin da
	// a ele, todos na porta 8822 (era assim em TServerSocket.StartServer).
	canaisAddr := []string{cfg.ChannelAddr}
	canaisNacao := []uint8{uint8(*nation)}
	if *multiCanal && cfg.Channels > 1 {
		caminho := *slPath
		if caminho == "" {
			caminho = filepath.Join(filepath.Dir(*dataDir), serverlist.FileName)
		}
		lista, err := serverlist.Load(caminho)
		if err != nil {
			return fmt.Errorf("-multicanal precisa do SL.bin: %w", err)
		}
		canaisAddr = canaisAddr[:0]
		canaisNacao = canaisNacao[:0]
		for i := 0; i < cfg.Channels; i++ {
			if i >= len(lista) || !lista[i].Active() {
				return fmt.Errorf("canal %d sem IP no %s", i, caminho)
			}
			canaisAddr = append(canaisAddr, lista[i].Addr())
			canaisNacao = append(canaisNacao, uint8(lista[i].Nation))
		}
	} else if *multiCanal {
		log.Warn("-multicanal com um canal so no .ini: nada a fazer", "canais", cfg.Channels)
	}

	// Os canais dividem as guildas, as nacoes e a fila de troca de
	// canal; cada um tem o proprio mundo (jogadores, mobs, grupos, itens no chao
	// e dungeons).
	nacoes := nationpkg.New()
	trocas := game.NewHandoffs()
	estadoCanal := func(id uint8) game.ChannelState {
		if int(id) >= len(canaisAddr) {
			return game.ChannelMissing
		}
		return game.ChannelUp
	}

	var mutantTest *game.MutantTest
	if *testeMutantes {
		mutantTest = game.NewMutantTest(mobs, drops, tables, uint32(*testeDefMax))
		log.Warn("MODO DE TESTE LIGADO: mutantes do nivel 60 deixam sempre acessorio e tem defesa reduzida (reinicie sem -teste-mutantes para voltar ao normal)")
		for _, linha := range mutantTest.Report {
			log.Warn("modo de teste", "detalhe", linha)
		}
	}
	novoHandler := func(idx int) *game.Handler {
		mundo := mundoMobs
		if idx > 0 {
			mundo = game.NewMobWorld(mobs)
		}
		return &game.Handler{
			Channel:        uint8(idx),
			Chan:           game.NewChannel(uint8(idx), cfg.MaxUsers),
			Handoffs:       trocas,
			ChannelStatus:  estadoCanal,
			Accounts:       accounts,
			AccountGold:    contaOuro,
			AccountCash:    contaCash,
			Premium:        tables,
			Chars:          chars,
			Items:          items,
			SkillDB:        skillDB,
			GuildDB:        guildDB,
			MailDB:         mailDB,
			AuctionDB:      auctionDB,
			FriendDB:       friendDB,
			CouponDB:       couponDB,
			PranDB:         pranDB,
			PranIDs:        pranpkg.NewIDs(),
			TitleDB:        titleDB,
			EventDB:        eventDB,
			GMDB:           gmDB,
			GMCmds:         gmCmds,
			GMFounders:     gmFounders,
			Nation:         canaisNacao[idx],
			AccountNation:  accountNation,
			TitleTable:     titleTable,
			CharSave:       charSave,
			CharDelete:     charDelete,
			CharCreate:     charCreate,
			Devir:          devir.New(canaisNacao[idx]),
			DevirDB:        devirDB,
			Nations:        nacoes,
			NationDB:       nationDB,
			Templates:      templates,
			ItemBarDB:      itemBarDB,
			TeleportDB:     teleportDB,
			Guilds:         registroGuildas,
			SkillTable:     tables,
			Numerics:       numerics,
			Durations:      skillfn.BuffSeconds{T: tables},
			Data:           tables,
			WatchDistance:  cfg.DistanceToWatch,
			ForgetDistance: cfg.DistanceToForget,
			Log:            log,
			ServerVersion:  uint16(cfg.Version),
			Mobs:           mundo,
			MutantTest:     mutantTest,
			Parties:        game.NewPartyRegistry(),
			Drops:          drops,
			MapObjs:        game.NewMapObjects(),
			Dungeons:       dungeon.NewRegistry(),
			DungeonLobbies: dungeon.NewLobbies(),
			DropRateBonus:  cfg.DropRateBonus,
			Exp:            tables,
			PranExp:        tables.PranExp,
			ExpMultiplier:  cfg.ExpMultiplier,
			RateEffect5:    cfg.RateEffect5,
			MobEsquiva:     cfg.MobEsquiva,
			MobCritRes:     cfg.MobCritRes,
			MobDuploRes:    cfg.MobDuploRes,
		}
	}
	var handlers []*game.Handler
	var canais []*game.Channel
	for i := range canaisAddr {
		h := novoHandler(i)
		handlers = append(handlers, h)
		canais = append(canais, h.Chan)
	}
	canalHandler := handlers[0]
	for _, h := range handlers {
		h.Channels = handlers
	}

	for _, h := range handlers {
		if err := h.LoadDevires(ctx); err != nil {
			log.Error("carregar templos", "canal", h.Channel, "erro", err)
		}
	}
	if err := canalHandler.LoadNations(ctx); err != nil {
		log.Error("carregar nacoes", "erro", err)
	}
	var canalSrvs []*netsrv.Server
	for i, h := range handlers {
		nome := "canal"
		if len(handlers) > 1 {
			nome = fmt.Sprintf("canal %d", i)
		}
		canalSrvs = append(canalSrvs, netsrv.New(netsrv.Config{
			Name:     nome,
			Addr:     canaisAddr[i],
			MaxConns: int(cfg.MaxUsers),
			Log:      log,
		}, h))
	}

	// Carrega as guildas depois de o banco estar de pe.
	if guildDB != nil {
		guildas, avisos, err := guildDB.All(ctx)
		if err != nil {
			// Guilda e estado de jogo, nao infraestrutura: o servidor sobe
			// sem elas e registra o motivo.
			log.Error("carregar guildas", "erro", err)
		}
		for _, a := range avisos {
			log.Warn("guildas", "aviso", a)
		}
		for _, g := range guildas {
			registroGuildas.Put(g)
		}
		log.Info("guildas carregadas", "quantidade", registroGuildas.Count())
	}

	// Abre as portas antes de anunciar: o original so descobria falha de bind
	// pelo log, depois de dizer que havia iniciado.
	for _, srv := range append([]*netsrv.Server{loginSrv}, canalSrvs...) {
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
			loginSrv: loginSrv, channelSrvs: canalSrvs, canais: canais})
		if err := tokenSrv.Listen(); err != nil {
			return err
		}
	}

	// O console so sobe com token. Na origem a porta 5555 aceitava qualquer
	// conexao sem autenticacao e executava o que chegasse com poder de Admin
	// (ConsoleSocket.pas:122-188); subir sem token seria portar a porta
	// aberta.
	consoleSrv := &console.Server{
		Addr:  cfg.ConsoleAddr,
		Token: cfg.ConsoleToken,
		Run:   canalHandler,
		Log:   log,
	}
	if err := consoleSrv.Listen(); err != nil {
		if errors.Is(err, console.ErrNoToken) {
			log.Warn("console desligado: defina AIKA_CONSOLE_TOKEN para abrir " +
				"a porta de administracao")
		} else {
			return fmt.Errorf("abrir console: %w", err)
		}
		consoleSrv = nil
	}

	campos := []any{
		"login", loginSrv.Addr().String(),
		"canal", canalSrvs[0].Addr().String(),
		"ver", cfg.DistanceToWatch,
		"esquecer", cfg.DistanceToForget,
	}
	if tokenSrv != nil {
		campos = append(campos, "token", tokenSrv.Addr().String())
	}
	if consoleSrv != nil {
		campos = append(campos, "console", consoleSrv.Address())
	}
	if len(canalSrvs) > 1 {
		var todos []string
		for _, s := range canalSrvs {
			todos = append(todos, s.Addr().String())
		}
		campos = append(campos, "canais", todos)
	}
	log.Info("servidor pronto", campos...)

	// Um erro em qualquer uma das portas encerra as outras.
	ctx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()

	// O respawn de mob roda no servidor, nao dentro da visibilidade de um
	// jogador: na origem o mob so voltava quando alguem passava perto, e sem
	// ninguem por perto ele ficava morto indefinidamente.
	for _, h := range handlers {
		if dungeons != nil {
			h.DungeonTable = dungeons
		}
		// O que depende do mundo do canal roda uma vez por canal.
		go respawnLoop(ctx, h, log)
		go mobAILoop(ctx, h, log)
		go mapObjectLoop(ctx, h, log)
		go dungeonLoop(ctx, h, log)
		go chestLoop(ctx, h)
		go saveLoop(ctx, h, log)
		go buffLoop(ctx, h)
		go expiryLoop(ctx, h)
	}
	// A fila de leilao e do banco, nao do canal: uma vez so.
	go auctionLoop(ctx, canalHandler, log)

	tarefas := 1 + len(canalSrvs)
	errc := make(chan error, 3+len(canalSrvs))
	for _, srv := range append([]*netsrv.Server{loginSrv}, canalSrvs...) {
		go func(s *netsrv.Server) { errc <- s.Serve(ctx) }(srv)
	}
	if tokenSrv != nil {
		tarefas++
		go func() { errc <- tokenSrv.Serve(ctx) }()
	}
	if consoleSrv != nil {
		tarefas++
		go func() { errc <- consoleSrv.Serve(ctx) }()
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

// mapObjectInterval e o passo com que o servidor confere os objetos no chao.
const mapObjectInterval = 10 * time.Second

// mapObjectLoop tira do chao os itens que passaram do tempo.
//
// A origem nao apagava o objeto de drop por tempo: ele ficava no vetor OBJ
// ate alguem coletar, e com o vetor cheio CreateMapObject passava a recusar
// todo drop novo.
func mapObjectLoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	t := time.NewTicker(mapObjectInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := h.MapObjectTick(); n > 0 {
				log.Debug("itens do chao expirados", "quantidade", n)
			}
		}
	}
}

// auctionInterval e o passo com que o servidor confere as ofertas vencidas.
const auctionInterval = time.Minute

// dungeonInterval e o passo da IA de dungeon. A origem usava
// "TDungeonMainThread.Create(1000, ...)", um segundo por instancia.
const dungeonInterval = time.Second

// dungeonLoop roda a IA dos mobs de dungeon e fecha o que venceu.
//
// A origem abria uma TThread por instancia (TDungeonMainThread), que lia o
// ponteiro do grupo uma vez e seguia usando esse ponteiro mesmo depois de o
// fechamento ter posto Party := nil e de o grupo ter sido desfeito.
func dungeonLoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	t := time.NewTicker(dungeonInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := h.DungeonTick(ctx); n > 0 {
				log.Info("instancias de dungeon fechadas", "quantidade", n)
			}
		}
	}
}

// chestInterval e o passo com que o servidor confere os baus de guilda
// abertos. A origem usava "Sleep(50)" dentro de uma thread POR BAU ABERTO,
// o que para um prazo de dois minutos e 2400 voltas de laco por bau.
const chestInterval = 15 * time.Second

// chestLoop fecha os baus de guilda sem acao.
func chestLoop(ctx context.Context, h *game.Handler) {
	t := time.NewTicker(chestInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.ChestTick()
		}
	}
}

// auctionLoop devolve ao vendedor os itens das ofertas que venceram.
//
// A origem tinha TAuctionFunctions.CheckAuctionItems, cujo corpo e so
// "Result := True": a oferta vencida ficava no banco para sempre e o item
// nunca voltava ao vendedor.
func auctionLoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	t := time.NewTicker(auctionInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := h.AuctionExpireTick(ctx); n > 0 {
				log.Info("ofertas vencidas devolvidas", "quantidade", n)
			}
		}
	}
}

// onlineCounter informa quantos jogadores estao conectados, somando as duas
// portas, para a rota de lista de servidores do token.
type onlineCounter struct {
	loginSrv    *netsrv.Server
	channelSrvs []*netsrv.Server
	canais      []*game.Channel
}

func (o onlineCounter) OnlinePlayers() int {
	if len(o.canais) > 0 {
		// Conta quem esta em jogo, nao socket aberto: o original somava
		// conexoes e a lista de servidores mostrava gente que ainda estava
		// na tela de login.
		n := 0
		for _, c := range o.canais {
			n += int(c.Count())
		}
		return n
	}
	n := 0
	for _, s := range o.channelSrvs {
		n += int(s.Conns())
	}
	return n
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

func (noItems) SaveStorage(context.Context, uint32, store.ItemSet) error { return nil }

func (noItems) GuildChest(context.Context, uint32) (store.ItemSet, error) {
	return store.ItemSet{Slots: make([]model.Item, store.GuildChestSlots)}, nil
}

func (noItems) SaveGuildChest(context.Context, uint32, store.ItemSet) error { return nil }

func (noItems) SaveCash(context.Context, uint32, store.ItemSet) error { return nil }

// noSkills devolve arvore vazia. Usado com -no-db.
type noSkills struct{}

func (noSkills) ByCharacter(context.Context, uint32) (model.SkillsList, []string, error) {
	return model.SkillsList{}, nil, nil
}

func (noSkills) Save(context.Context, uint32, model.SkillsList) error { return nil }

// noGuilds devolve nenhuma guilda. Usado com -no-db.
type noGuilds struct{}

func (noGuilds) All(context.Context) ([]*guild.Guild, []string, error) {
	return nil, nil, nil
}

func (noGuilds) Save(context.Context, *guild.Guild) error { return nil }

func (noGuilds) Delete(context.Context, uint16) error { return nil }

func (noGuilds) SaveChestGold(context.Context, uint16, uint64) error { return nil }

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

// saveInterval e o passo da gravacao periodica de quem esta em jogo. A origem
// so gravava em eventos soltos e na saida; uma queda perdia a sessao inteira.
const saveInterval = 2 * time.Minute

// saveLoop grava periodicamente todos os jogadores em jogo.
func saveLoop(ctx context.Context, h *game.Handler, log *slog.Logger) {
	t := time.NewTicker(saveInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := h.SaveAllTick(); n > 0 {
				log.Debug("gravacao periodica", "jogadores", n)
			}
		}
	}
}

// expiryInterval e o passo da conferencia de itens com prazo. A origem usava
// uma thread propria (TTimeItensThread).
const expiryInterval = 30 * time.Second

// expiryLoop apaga ou marca os itens que venceram.
func expiryLoop(ctx context.Context, h *game.Handler) {
	t := time.NewTicker(expiryInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.ExpireItemsTick()
		}
	}
}

// buffInterval e o passo com que os buffs vencidos saem. A origem os
// conferia numa thread (TBaseMob.RefreshBuffs); sem este laco o Go nunca os
// removia.
const buffInterval = time.Second

// buffLoop tira os buffs vencidos.
func buffLoop(ctx context.Context, h *game.Handler) {
	t := time.NewTicker(buffInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.BuffTick()
		}
	}
}
