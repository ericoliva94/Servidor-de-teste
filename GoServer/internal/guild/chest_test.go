package guild

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
)

var agora = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

// guildaComBau devolve uma guilda com dois membros e o bau pronto.
func guildaComBau(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	g, err := r.Create("Alfa", 1, Member{CharIndex: 10, Name: "Lider"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddMember(Member{CharIndex: 20, Name: "Membro", Rank: 1}); err != nil {
		t.Fatal(err)
	}
	r.Put(g)
	// Rank 1 pode abrir e usar o bau.
	g.RanksConfig[1] = PermOpenChest | PermUseChest
	return r
}

// A origem gravava "MemberInChest := FindMemberFromCharIndex(CharIndex)" sem
// conferir o retorno: para quem nao era membro isso e -1, e dali em diante
// qualquer outro nao-membro passava no teste de posse (-1 <> -1 e falso).
// Aqui o dono e o CharIndex e zero nao e dono de nada.
func TestBauNaoAbreParaPersonagemZero(t *testing.T) {
	c := NewChest()
	if err := c.Open(0, "", agora); !errors.Is(err, ErrChestNotAMember) {
		t.Fatalf("Open(0) = %v, queria ErrChestNotAMember", err)
	}
	if c.IsOpenFor(0) {
		t.Error("bau aberto para o personagem zero")
	}
}

func TestBauRecusaSegundoMembroComNomeDoPrimeiro(t *testing.T) {
	c := NewChest()
	if err := c.Open(10, "Lider", agora); err != nil {
		t.Fatal(err)
	}
	err := c.Open(20, "Membro", agora.Add(time.Second))
	if !errors.Is(err, ErrChestBusy) {
		t.Fatalf("segundo Open = %v, queria ErrChestBusy", err)
	}
	if !strings.Contains(err.Error(), "Lider") {
		t.Errorf("erro %q deveria citar quem esta usando", err)
	}
	if !c.IsOpenFor(10) || c.IsOpenFor(20) {
		t.Error("o bau trocou de dono numa abertura recusada")
	}
}

// A thread da origem, ao vencer o prazo, mandava o sinal $F2F e saia SEM
// chamar CloseChest: se o cliente nao respondesse, ou se o jogador caisse, o
// bau ficava trancado para todos ate o servidor reiniciar.
func TestBauVencidoEhAssumidoPeloProximo(t *testing.T) {
	c := NewChest()
	if err := c.Open(10, "Lider", agora); err != nil {
		t.Fatal(err)
	}
	depois := agora.Add(ChestTimeout + time.Second)
	if err := c.Open(20, "Membro", depois); err != nil {
		t.Fatalf("Open depois do prazo = %v, queria sucesso", err)
	}
	if !c.IsOpenFor(20) || c.IsOpenFor(10) {
		t.Error("o bau deveria estar com o segundo membro")
	}
}

func TestTouchRenovaOPrazo(t *testing.T) {
	c := NewChest()
	if err := c.Open(10, "Lider", agora); err != nil {
		t.Fatal(err)
	}
	// Um minuto depois ele mexe no bau: o prazo recomeca.
	c.Touch(agora.Add(time.Minute))
	// No instante em que o prazo original venceria, ainda e dele.
	if _, venceu := c.expire(agora.Add(ChestTimeout + time.Second)); venceu {
		t.Error("o prazo venceu apesar do Touch")
	}
	if _, venceu := c.expire(agora.Add(time.Minute + ChestTimeout + time.Second)); !venceu {
		t.Error("o prazo deveria ter vencido depois do Touch")
	}
}

func TestFecharSoVindoDoDono(t *testing.T) {
	c := NewChest()
	if err := c.Open(10, "Lider", agora); err != nil {
		t.Fatal(err)
	}
	if c.Close(20) {
		t.Error("um nao-dono fechou o bau")
	}
	if !c.Close(10) {
		t.Error("o dono nao conseguiu fechar")
	}
	if c.Close(10) {
		t.Error("fechar duas vezes devolveu verdadeiro")
	}
}

// A origem fazia "Chest.Gold := Chest.Gold + UInt64(Packet.Value)" com a
// guarda de saldo comparando um UInt64 com um Int64. Retirar mais do que o
// bau tem nao pode dar a volta.
func TestOuroDoBauNaoDaAVoltaNaRetirada(t *testing.T) {
	c := NewChest()
	c.SetGold(100)
	if _, err := c.MoveGold(-500); !errors.Is(err, ErrChestNoGold) {
		t.Fatalf("retirada acima do saldo = %v, queria ErrChestNoGold", err)
	}
	if got := c.Gold(); got != 100 {
		t.Errorf("ouro = %d, queria 100 intacto", got)
	}
	if _, err := c.MoveGold(-100); err != nil {
		t.Fatalf("retirada exata = %v", err)
	}
	if got := c.Gold(); got != 0 {
		t.Errorf("ouro = %d, queria 0", got)
	}
}

// No deposito a origem nao conferia teto nenhum: o campo e UInt64 e crescia
// ate dar a volta em 1,8e19, momento em que o bau ficava quase zerado.
func TestOuroDoBauRespeitaOTetoNoDeposito(t *testing.T) {
	c := NewChest()
	c.SetGold(MaxChestGold - 10)
	if _, err := c.MoveGold(100); !errors.Is(err, ErrChestGoldCap) {
		t.Fatalf("deposito acima do teto = %v, queria ErrChestGoldCap", err)
	}
	if got := c.Gold(); got != MaxChestGold-10 {
		t.Errorf("ouro = %d, queria intacto", got)
	}
	if _, err := c.MoveGold(10); err != nil {
		t.Fatalf("deposito ate o teto = %v", err)
	}
	if got := c.Gold(); got != MaxChestGold {
		t.Errorf("ouro = %d, queria o teto", got)
	}
}

func TestSetGoldCortaNoTeto(t *testing.T) {
	c := NewChest()
	c.SetGold(MaxChestGold * 3)
	if got := c.Gold(); got != MaxChestGold {
		t.Errorf("ouro = %d, queria o teto %d", got, MaxChestGold)
	}
}

// A origem fazia "Chest.Items[ItemSlot]" com ItemSlot vindo do banco num
// vetor ARRAY [0..49]: um slot de 50 a 255 escrevia por cima de Chest.Gold,
// de MemberInChest, do ponteiro da thread e da guilda seguinte.
func TestLoadRecusaSlotForaDoVetor(t *testing.T) {
	c := NewChest()
	itens := make([]model.Item, 60)
	itens[0] = model.Item{Index: 111}
	itens[49] = model.Item{Index: 222}
	itens[50] = model.Item{Index: 333} // fora do vetor da origem
	itens[59] = model.Item{Index: 444}

	c.SetGold(500)
	avisos := c.LoadItems(itens)
	if len(avisos) != 2 {
		t.Fatalf("avisos = %d (%v), queria 2", len(avisos), avisos)
	}
	if !c.IsLoaded() {
		t.Error("LoadItems nao marcou o bau como carregado")
	}
	if it, _ := c.At(0); it.Index != 111 {
		t.Errorf("slot 0 = %d", it.Index)
	}
	if it, _ := c.At(49); it.Index != 222 {
		t.Errorf("slot 49 = %d", it.Index)
	}
	if _, err := c.At(50); !errors.Is(err, ErrChestSlotRange) {
		t.Errorf("At(50) = %v, queria ErrChestSlotRange", err)
	}
	if got := c.Gold(); got != 500 {
		t.Errorf("ouro = %d: o item fora da faixa passou por cima dele", got)
	}
}

func TestSetGoldAvisaOuroAcimaDoTeto(t *testing.T) {
	c := NewChest()
	if !c.SetGold(MaxChestGold + 1) {
		t.Error("SetGold nao avisou que cortou")
	}
	if got := c.Gold(); got != MaxChestGold {
		t.Errorf("ouro = %d, queria o teto", got)
	}
	if c.SetGold(10) {
		t.Error("SetGold avisou corte num valor dentro do teto")
	}
}

func TestSnapshotCopiaEnaoCompartilha(t *testing.T) {
	c := NewChest()
	c.Do(func(itens []model.Item) { itens[3] = model.Item{Index: 777} })
	c.SetGold(42)

	snap := c.Snapshot()
	if snap.Items[3].Index != 777 || snap.Gold != 42 {
		t.Fatalf("snapshot errado: %+v", snap.Items[3])
	}
	snap.Items[3] = model.Item{Index: 1}
	if it, _ := c.At(3); it.Index != 777 {
		t.Error("mexer no snapshot mexeu no bau")
	}
}

// Prova central da trava: dois membros tirando o mesmo slot ao mesmo tempo.
// Na origem os dois lados eram ponteiros crus para a memoria global e o item
// era duplicado.
func TestBauNaoDuplicaItemComMuitosMembros(t *testing.T) {
	c := NewChest()
	c.Do(func(itens []model.Item) { itens[0] = model.Item{Index: 500} })

	const n = 32
	var wg sync.WaitGroup
	levaram := make([]bool, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Do(func(itens []model.Item) {
				if itens[0].IsEmpty() {
					return
				}
				itens[0] = model.Item{}
				levaram[i] = true
			})
		}(i)
	}
	wg.Wait()

	total := 0
	for _, l := range levaram {
		if l {
			total++
		}
	}
	if total != 1 {
		t.Errorf("%d membros levaram o item, queria exatamente 1", total)
	}
}

func TestMoveGoldConcorrenteFechaAConta(t *testing.T) {
	c := NewChest()
	const n = 100
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.MoveGold(10); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := c.Gold(); got != n*10 {
		t.Errorf("ouro = %d, queria %d", got, n*10)
	}
}

// O prazo da origem avisava mas nao fechava. Aqui a varredura fecha de fato e
// devolve quem precisa ser avisado.
func TestSweepChestsFechaEAvisa(t *testing.T) {
	r := guildaComBau(t)
	g := r.Of(10)
	if err := g.Chest.Open(10, "Lider", agora); err != nil {
		t.Fatal(err)
	}

	// Antes do prazo nada acontece.
	if avisos := r.SweepChests(agora.Add(time.Minute)); len(avisos) != 0 {
		t.Fatalf("varredura antes do prazo devolveu %v", avisos)
	}
	if !g.Chest.IsOpenFor(10) {
		t.Fatal("o bau fechou antes do prazo")
	}

	avisos := r.SweepChests(agora.Add(ChestTimeout + time.Second))
	if len(avisos) != 1 {
		t.Fatalf("avisos = %v, queria um", avisos)
	}
	if avisos[0].GuildIndex != g.Index || avisos[0].CharIndex != 10 {
		t.Errorf("aviso = %+v", avisos[0])
	}
	if g.Chest.Holder() != 0 {
		t.Error("o bau continuou trancado depois do prazo")
	}
	// Uma segunda varredura nao repete o aviso.
	if avisos := r.SweepChests(agora.Add(time.Hour)); len(avisos) != 0 {
		t.Errorf("segunda varredura devolveu %v", avisos)
	}
}

// Na origem nada no caminho de desconexao chamava CloseChest.
func TestCloseChestOfSoltaOBauDoPersonagem(t *testing.T) {
	r := guildaComBau(t)
	g := r.Of(20)
	if err := g.Chest.Open(20, "Membro", agora); err != nil {
		t.Fatal(err)
	}
	if !r.CloseChestOf(20) {
		t.Fatal("CloseChestOf nao soltou o bau")
	}
	if g.Chest.Holder() != 0 {
		t.Error("o bau continuou trancado")
	}
	// Personagem sem guilda nao quebra nada.
	if r.CloseChestOf(999) {
		t.Error("CloseChestOf devolveu verdadeiro para quem nao tem guilda")
	}
}

func TestRegistryDaBauATodaGuilda(t *testing.T) {
	r := NewRegistry()
	g, err := r.Create("Beta", 1, Member{CharIndex: 1, Name: "L"})
	if err != nil {
		t.Fatal(err)
	}
	if g.Chest == nil {
		t.Error("Create nao criou o bau")
	}
	// Put completa o bau de uma guilda vinda do banco sem ele.
	semBau := &Guild{Index: 7, Name: "Gama"}
	r.Put(semBau)
	if semBau.Chest == nil {
		t.Error("Put nao criou o bau")
	}
}

// Metodos de ponteiro nil nao entram em panico: a guilda pode vir de um
// caminho que nao passou pelo registro.
func TestBauNiloNaoQuebra(t *testing.T) {
	var c *Chest
	if got := c.Gold(); got != 0 {
		t.Errorf("Gold = %d", got)
	}
	if c.IsOpenFor(1) || c.Close(1) || c.Holder() != 0 || c.HolderName() != "" {
		t.Error("bau nil respondeu como aberto")
	}
	if c.SetGold(5) {
		t.Error("SetGold num bau nil avisou corte")
	}
	c.Touch(agora)
	c.Do(func([]model.Item) { t.Error("Do chamou f num bau nil") })
	if _, err := c.MoveGold(1); err == nil {
		t.Error("MoveGold num bau nil deveria falhar")
	}
	if _, err := c.At(0); err == nil {
		t.Error("At num bau nil deveria falhar")
	}
	if avisos := c.LoadItems(nil); avisos != nil {
		t.Errorf("LoadItems num bau nil = %v", avisos)
	}
	if c.IsLoaded() {
		t.Error("bau nil disse estar carregado")
	}
	if snap := c.Snapshot(); snap.Gold != 0 {
		t.Error("Snapshot num bau nil trouxe ouro")
	}
}
