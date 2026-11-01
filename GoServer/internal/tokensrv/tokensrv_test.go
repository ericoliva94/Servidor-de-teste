package tokensrv

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"aika/internal/auth"
)

var agora = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// memStore e um Store em memoria que registra o que foi gravado.
type memStore struct {
	contas    map[string]auth.Account
	chars     map[uint32]int
	erroBusca error

	tokenGravado string
	tokenConta   uint32
	tokenCriado  time.Time
	banLiberado  uint32
	erroGravar   error
	erroLiberar  error
	erroContar   error
}

func (m *memStore) ByUsername(_ context.Context, u string) (auth.Account, bool, error) {
	if m.erroBusca != nil {
		return auth.Account{}, false, m.erroBusca
	}
	a, ok := m.contas[u]
	return a, ok, nil
}

func (m *memStore) SaveToken(_ context.Context, id uint32, token string, criado time.Time) error {
	if m.erroGravar != nil {
		return m.erroGravar
	}
	m.tokenGravado, m.tokenConta, m.tokenCriado = token, id, criado
	return nil
}

func (m *memStore) ClearBan(_ context.Context, id uint32) error {
	if m.erroLiberar != nil {
		return m.erroLiberar
	}
	m.banLiberado = id
	return nil
}

func (m *memStore) CharacterCount(_ context.Context, id uint32) (int, error) {
	if m.erroContar != nil {
		return 0, m.erroContar
	}
	return m.chars[id], nil
}

type contador int

func (c contador) OnlinePlayers() int { return int(c) }

func quieto() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// O MD5 de "senha123", que e o que o cliente manda.
const hashSenha = "4297f44b13955235245b2497399d7a93"

func loja() *memStore {
	return &memStore{
		contas: map[string]auth.Account{
			"teste": {ID: 7, Username: "teste", PasswordHash: hashSenha},
			"cancelada": {ID: 8, PasswordHash: hashSenha,
				Status: auth.StatusCancelled},
			"banida": {ID: 9, PasswordHash: hashSenha,
				Status: auth.StatusBanned},
			"suspensa": {ID: 10, PasswordHash: hashSenha,
				Status: auth.StatusBanned, BanDays: 7,
				TokenCreated: agora.AddDate(0, 0, -10)}, // prazo vencido
			"suspensa_ativa": {ID: 11, PasswordHash: hashSenha,
				Status: auth.StatusBanned, BanDays: 7,
				TokenCreated: agora.AddDate(0, 0, -2)}, // ainda em vigor
			"naocbt": {ID: 12, PasswordHash: hashSenha,
				Status: auth.StatusNotCBT},
		},
		chars: map[uint32]int{7: 2},
	}
}

// serve sobe o servidor e devolve a base da URL.
func serve(t *testing.T, st Store, p PlayerCounter) (string, func()) {
	t.Helper()
	s := New(Config{
		Addr: "127.0.0.1:0",
		Log:  quieto(),
		Now:  func() time.Time { return agora },
	}, st, p)
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Serve(ctx); close(done) }()
	return "http://" + s.Addr().String(), func() {
		cancel()
		<-done
	}
}

// get faz a requisicao e devolve corpo e codigo.
func get(t *testing.T, base, path string, q url.Values) (string, int) {
	t.Helper()
	u := base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), resp.StatusCode
}

func TestGetTokenSuccess(t *testing.T) {
	st := loja()
	base, stop := serve(t, st, contador(0))
	defer stop()

	body, code := get(t, base, PathGetToken, url.Values{
		"id": {"teste"}, "pw": {hashSenha},
	})
	if code != http.StatusOK {
		t.Fatalf("http %d", code)
	}
	if len(body) != TokenLength {
		t.Fatalf("token com %d caracteres, queria %d: %q", len(body), TokenLength, body)
	}
	if st.tokenGravado != body {
		t.Errorf("gravou %q, devolveu %q", st.tokenGravado, body)
	}
	if st.tokenConta != 7 {
		t.Errorf("gravou na conta %d, queria 7", st.tokenConta)
	}
	if !st.tokenCriado.Equal(agora) {
		t.Errorf("instante gravado = %v, queria %v", st.tokenCriado, agora)
	}
}

// Dois pedidos seguidos tem de dar tokens diferentes. O original derivava o
// token de MD5(senha)+MD5(data com resolucao de segundo), entao dois pedidos
// no mesmo segundo com a mesma senha davam o mesmo token.
func TestGetTokenIsUniquePerRequest(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()

	vistos := map[string]bool{}
	for i := 0; i < 50; i++ {
		body, _ := get(t, base, PathGetToken, url.Values{
			"id": {"teste"}, "pw": {hashSenha},
		})
		if vistos[body] {
			t.Fatalf("token repetido na tentativa %d: %q", i, body)
		}
		vistos[body] = true
	}
}

func TestGetTokenRejections(t *testing.T) {
	cases := []struct {
		name string
		user string
		pw   string
		want string
	}{
		{"conta inexistente", "ninguem", hashSenha, RespAccountNotFound},
		{"usuario vazio", "", hashSenha, RespAccountNotFound},
		{"senha errada", "teste", "outro", RespWrongPassword},
		{"senha vazia", "teste", "", RespWrongPassword},
		{"conta cancelada", "cancelada", hashSenha, RespCancelled},
		{"conta banida", "banida", hashSenha, RespBanned},
		{"suspensao em vigor", "suspensa_ativa", hashSenha, RespBanned},
		{"nao CBT", "naocbt", hashSenha, RespNotCBT},
	}
	base, stop := serve(t, loja(), contador(0))
	defer stop()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, code := get(t, base, PathGetToken, url.Values{
				"id": {c.user}, "pw": {c.pw},
			})
			if code != http.StatusOK {
				t.Fatalf("http %d", code)
			}
			if body != c.want {
				t.Errorf("resposta = %q, queria %q", body, c.want)
			}
		})
	}
}

// Suspensao com prazo vencido libera a conta e devolve -22.
func TestGetTokenClearsExpiredBan(t *testing.T) {
	st := loja()
	base, stop := serve(t, st, contador(0))
	defer stop()

	body, _ := get(t, base, PathGetToken, url.Values{
		"id": {"suspensa"}, "pw": {hashSenha},
	})
	if body != RespUnbanned {
		t.Errorf("resposta = %q, queria %q", body, RespUnbanned)
	}
	if st.banLiberado != 10 {
		t.Errorf("liberou a conta %d, queria 10", st.banLiberado)
	}
}

// A ordem dos parametros nao pode mudar o resultado: o original lia
// Params[0] como "id=" e Params[1] como "pw=", por posicao.
func TestGetTokenIgnoresParameterOrder(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()

	// pw antes de id, e com um parametro a mais no meio.
	body, _ := get(t, base, PathGetToken, url.Values{
		"pw": {hashSenha}, "extra": {"x"}, "id": {"teste"},
	})
	if len(body) != TokenLength {
		t.Errorf("resposta = %q: a ordem dos parametros mudou o resultado", body)
	}
}

func TestGetTokenNormalizesUsernameCase(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()
	for _, u := range []string{"TESTE", " teste ", "Teste"} {
		body, _ := get(t, base, PathGetToken, url.Values{"id": {u}, "pw": {hashSenha}})
		if len(body) != TokenLength {
			t.Errorf("usuario %q: resposta = %q", u, body)
		}
	}
}

// Erro de banco nao pode virar "conta inexistente": o cliente mostraria a
// mensagem errada e o jogador tentaria criar a conta de novo.
func TestGetTokenStoreErrorIsServerError(t *testing.T) {
	st := loja()
	st.erroBusca = errors.New("banco fora")
	base, stop := serve(t, st, contador(0))
	defer stop()

	body, code := get(t, base, PathGetToken, url.Values{"id": {"teste"}, "pw": {hashSenha}})
	if code != http.StatusInternalServerError {
		t.Errorf("http %d, queria 500; corpo %q", code, body)
	}
	if body == RespAccountNotFound {
		t.Error("erro de banco nao pode virar conta inexistente")
	}
}

// Falha ao gravar o token tambem nao pode devolver um token que nao foi
// salvo: o cliente usaria um token que o servidor de canal nao conhece.
func TestGetTokenSaveFailureIsServerError(t *testing.T) {
	st := loja()
	st.erroGravar = errors.New("sem escrita")
	base, stop := serve(t, st, contador(0))
	defer stop()

	_, code := get(t, base, PathGetToken, url.Values{"id": {"teste"}, "pw": {hashSenha}})
	if code != http.StatusInternalServerError {
		t.Errorf("http %d, queria 500", code)
	}
}

func TestCharCount(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()

	body, _ := get(t, base, PathCharCount, url.Values{"id": {"teste"}, "pw": {hashSenha}})
	if body != "2" {
		t.Errorf("resposta = %q, queria 2", body)
	}
	body, _ = get(t, base, PathCharCount, url.Values{"id": {"ninguem"}, "pw": {hashSenha}})
	if body != RespAccountNotFound {
		t.Errorf("conta inexistente = %q", body)
	}
	body, _ = get(t, base, PathCharCount, url.Values{"id": {"teste"}, "pw": {"errada"}})
	if body != RespWrongPassword {
		t.Errorf("senha errada = %q", body)
	}
}

func TestServerList(t *testing.T) {
	base, stop := serve(t, loja(), contador(37))
	defer stop()
	body, _ := get(t, base, PathServerList, nil)
	if body != "37" {
		t.Errorf("resposta = %q, queria 37", body)
	}
}

// Sem contador ligado a rota nao pode entrar em panico.
func TestServerListWithoutCounter(t *testing.T) {
	base, stop := serve(t, loja(), nil)
	defer stop()
	body, code := get(t, base, PathServerList, nil)
	if code != http.StatusOK || body != "0" {
		t.Errorf("http %d corpo %q", code, body)
	}
}

func TestResetFlag(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()
	body, _ := get(t, base, PathResetFlag, url.Values{"id": {"teste"}})
	if body != "1" {
		t.Errorf("resposta = %q, queria 1", body)
	}
	body, _ = get(t, base, PathResetFlag, url.Values{"id": {"ninguem"}})
	if body != RespAccountNotFound {
		t.Errorf("conta inexistente = %q", body)
	}
	body, _ = get(t, base, PathResetFlag, nil)
	if body != RespAccountNotFound {
		t.Errorf("sem usuario = %q", body)
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()
	_, code := get(t, base, "/rota/que/nao/existe.asp", nil)
	if code != http.StatusNotFound {
		t.Errorf("http %d, queria 404", code)
	}
}

func TestNewTokenShape(t *testing.T) {
	for i := 0; i < 100; i++ {
		tok, err := NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != TokenLength {
			t.Fatalf("token com %d caracteres: %q", len(tok), tok)
		}
		for _, c := range tok {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("caractere invalido %q em %q", c, tok)
			}
		}
	}
}

// Requisicoes tortas nao podem derrubar o servidor.
func TestServerSurvivesOddRequests(t *testing.T) {
	base, stop := serve(t, loja(), contador(0))
	defer stop()

	caminhos := []string{
		PathGetToken + "?id=%ZZ",
		PathGetToken + "?id=%00", // NUL codificado chega ao servidor
		PathGetToken + "?id=" + strings.Repeat("a", 5000),
		PathGetToken + "?" + "id=a&id=b&pw=c&pw=d",
		PathCharCount,
		PathResetFlag + "?id=",
		"/",
	}
	for _, p := range caminhos {
		resp, err := http.Get(base + p)
		if err != nil {
			t.Errorf("%q: %v", p, err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// Continua atendendo depois de tudo isso.
	body, _ := get(t, base, PathGetToken, url.Values{"id": {"teste"}, "pw": {hashSenha}})
	if len(body) != TokenLength {
		t.Errorf("servidor parou de atender: %q", body)
	}
}
