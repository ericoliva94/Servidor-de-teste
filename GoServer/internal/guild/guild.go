// Package guild tem o estado das guildas de um canal.
//
// Porte de Server/Src/Guild/GuildData.pas. Bau, alianca, skill de guilda e
// cerco de castelo nao entram aqui: dependem de sistemas ainda nao portados.
// Ver GoServer/PORTE.md.
package guild

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Limites dos vetores de GuildData.pas.
const (
	// MaxMembers e "Members: ARRAY [0..127]".
	MaxMembers = 128
	// MaxRanks e "RanksConfig: ARRAY [0..4]".
	MaxRanks = 5
	// MaxNotices e "Notices: ARRAY [0..2]".
	MaxNotices = 3
	// MaxLevel e o maior indice de MaxGuildPlayersOfLevel, ARRAY [0..9].
	MaxLevel = 9
	// NameLen e "Name: ARRAY [0..18] OF AnsiChar" da guilda.
	NameLen = 18
	// MemberNameLen e "Name: ARRAY [0..19]" do membro.
	MemberNameLen = 20
	// NoticeLen e "Text: ARRAY [0..33]".
	NoticeLen = 34
	// SiteLen e "Site: ARRAY [0..37]".
	SiteLen = 38
)

// MaxPlayersOfLevel e o MaxGuildPlayersOfLevel de GuildData.pas: quantos
// membros a guilda aguenta em cada nivel.
var MaxPlayersOfLevel = [MaxLevel + 1]uint8{32, 48, 64, 80, 96, 112, 128, 128, 128, 128}

// RankLeader e o rank do mestre.
const RankLeader = 0

// Bits de permissao de rank, de TGuild.GetRankConfig.
const (
	PermInvite        = 0x01
	PermKick          = 0x02
	PermUseGuildSkill = 0x04
	PermEditNotices   = 0x10
	PermOpenChest     = 0x40
	PermUseChest      = 0x80
)

// Erros da guilda.
var (
	ErrNoGuild        = errors.New("guild: personagem nao esta em guilda")
	ErrGuildFull      = errors.New("guild: guilda sem espaco")
	ErrAlreadyInGuild = errors.New("guild: personagem ja esta em uma guilda")
	ErrNotLeader      = errors.New("guild: so o mestre pode fazer isso")
	ErrNoPermission   = errors.New("guild: sem permissao")
	ErrNameTaken      = errors.New("guild: nome de guilda em uso")
	ErrInvalidName    = errors.New("guild: nome de guilda invalido")
	ErrMemberNotFound = errors.New("guild: membro nao encontrado")
)

// Member e um membro da guilda. Porte de TPlayerFromGuild.
type Member struct {
	CharIndex uint32
	Name      string
	Rank      uint8
	ClassInfo uint8
	Level     uint8
	Logged    bool
	LastLogin time.Time
}

// RankConfig sao as permissoes de um rank. Porte de TGuildRankConfig.
type RankConfig struct {
	Invite        bool
	Kick          bool
	UseGuildSkill bool
	EditNotices   bool
	OpenChest     bool
	UseChest      bool
}

// Guild e uma guilda. Porte de TGuild, sem bau nem alianca.
type Guild struct {
	Index    uint16
	Slot     int
	Nation   uint8
	Exp      uint32
	Level    uint8
	Name     string
	Notices  [MaxNotices]string
	Site     string
	LeaderID uint32

	BravurePoints uint32
	SkillPoints   uint32
	Promote       bool

	// Members esta na ordem dos slots, com buracos: o slot de um membro que
	// saiu fica vazio, como no vetor da origem.
	Members [MaxMembers]Member
	// RanksConfig guarda os bits de permissao de cada rank.
	RanksConfig [MaxRanks]uint8

	// Chest e o bau da guilda. E ponteiro para o mutex de dentro dele nao
	// tornar a guilda inteira nao-copiavel; Registry.Create e Registry.Put
	// garantem que toda guilda do registro tem o seu.
	Chest *Chest
}

// Capacity devolve quantos membros a guilda aguenta no nivel atual.
//
// A origem fazia "MaxGuildPlayersOfLevel[Self.Level]" com Level sendo DWORD e
// o vetor ARRAY [0..9]: guilda de nivel acima de 9 lia fora do vetor, e o
// limite de membros saia de um byte qualquer da memoria.
func (g *Guild) Capacity() int {
	if g == nil {
		return 0
	}
	nivel := g.Level
	if nivel > MaxLevel {
		nivel = MaxLevel
	}
	return int(MaxPlayersOfLevel[nivel])
}

// TotalMembers conta os membros ocupados.
//
// A origem guardava um contador TotalMembers em paralelo com o vetor, mantido
// a mao em cada entrada e saida. Contar o vetor nao pode divergir dele.
func (g *Guild) TotalMembers() int {
	if g == nil {
		return 0
	}
	n := 0
	for i := range g.Members {
		if g.Members[i].CharIndex != 0 {
			n++
		}
	}
	return n
}

// FindMember devolve o slot de um personagem, ou -1.
func (g *Guild) FindMember(charIndex uint32) int {
	if g == nil || charIndex == 0 {
		return -1
	}
	for i := range g.Members {
		if g.Members[i].CharIndex == charIndex {
			return i
		}
	}
	return -1
}

// MemberAt devolve o membro de um slot, ou nil fora da faixa.
//
// Existe porque a origem escrevia
// "Self.Members[Self.FindMemberFromCharIndex(CharIndex)].Rank" direto, e
// FindMemberFromCharIndex devolve -1 quando nao acha: aquilo lia
// Members[-1], antes do inicio do vetor.
func (g *Guild) MemberAt(slot int) *Member {
	if g == nil || slot < 0 || slot >= MaxMembers {
		return nil
	}
	if g.Members[slot].CharIndex == 0 {
		return nil
	}
	return &g.Members[slot]
}

// RankConfigOf devolve as permissoes de um rank.
//
// A origem fazia "Self.RanksConfig[Rank]" com Rank sendo Integer e o vetor
// ARRAY [0..4]: um rank fora de 0..4 — e o rank vem do registro do membro,
// que vem do banco — lia fora do vetor e as permissoes saiam de memoria
// qualquer.
func (g *Guild) RankConfigOf(rank uint8) RankConfig {
	if g == nil || int(rank) >= MaxRanks {
		return RankConfig{}
	}
	v := g.RanksConfig[rank]
	return RankConfig{
		Invite:        v&PermInvite != 0,
		Kick:          v&PermKick != 0,
		UseGuildSkill: v&PermUseGuildSkill != 0,
		EditNotices:   v&PermEditNotices != 0,
		OpenChest:     v&PermOpenChest != 0,
		UseChest:      v&PermUseChest != 0,
	}
}

// CanOf devolve as permissoes de um personagem.
func (g *Guild) CanOf(charIndex uint32) RankConfig {
	slot := g.FindMember(charIndex)
	m := g.MemberAt(slot)
	if m == nil {
		return RankConfig{}
	}
	// O mestre pode tudo, independente da configuracao de rank.
	if charIndex == g.LeaderID {
		return RankConfig{true, true, true, true, true, true}
	}
	return g.RankConfigOf(m.Rank)
}

// SetRanksConfig grava a configuracao de permissoes.
//
// A origem fazia "Move(RanksConfig, Self.RanksConfig, 4)": copiava 4 bytes
// num vetor de 5, entao a configuracao do rank 4 NUNCA era atualizada —
// ficava com o que estava na memoria desde a criacao da guilda.
func (g *Guild) SetRanksConfig(cfg []uint8) {
	if g == nil {
		return
	}
	for i := range g.RanksConfig {
		if i < len(cfg) {
			g.RanksConfig[i] = cfg[i]
		}
	}
}

// AddMember poe um membro num slot livre.
func (g *Guild) AddMember(m Member) (slot int, err error) {
	if g == nil {
		return -1, ErrNoGuild
	}
	if m.CharIndex == 0 {
		return -1, ErrMemberNotFound
	}
	if g.FindMember(m.CharIndex) >= 0 {
		return -1, ErrAlreadyInGuild
	}
	if g.TotalMembers() >= g.Capacity() {
		return -1, ErrGuildFull
	}
	for i := range g.Members {
		if g.Members[i].CharIndex == 0 {
			m.Name = truncate(m.Name, MemberNameLen-1)
			g.Members[i] = m
			return i, nil
		}
	}
	// Capacity menor que MaxMembers deixa slots livres; chegar aqui so e
	// possivel com o vetor cheio.
	return -1, ErrGuildFull
}

// RemoveMember tira um membro e diz se ele estava la.
func (g *Guild) RemoveMember(charIndex uint32) bool {
	slot := g.FindMember(charIndex)
	if slot < 0 {
		return false
	}
	g.Members[slot] = Member{}
	return true
}

// SetRank muda o rank de um membro.
func (g *Guild) SetRank(charIndex uint32, rank uint8) error {
	m := g.MemberAt(g.FindMember(charIndex))
	if m == nil {
		return ErrMemberNotFound
	}
	if int(rank) >= MaxRanks {
		// Rank fora da faixa entraria no registro e dali indexaria
		// RanksConfig fora do vetor na proxima checagem de permissao.
		return ErrNoPermission
	}
	m.Rank = rank
	return nil
}

// SetLeader passa a lideranca a um membro, rebaixando o antigo mestre.
func (g *Guild) SetLeader(charIndex uint32) error {
	novo := g.MemberAt(g.FindMember(charIndex))
	if novo == nil {
		return ErrMemberNotFound
	}
	if antigo := g.MemberAt(g.FindMember(g.LeaderID)); antigo != nil {
		// O antigo mestre fica no rank logo abaixo, nao fora da guilda.
		antigo.Rank = 1
	}
	g.LeaderID = charIndex
	novo.Rank = RankLeader
	return nil
}

// SetNotices grava os avisos, cada um no tamanho do campo.
func (g *Guild) SetNotices(avisos ...string) {
	if g == nil {
		return
	}
	for i := range g.Notices {
		if i < len(avisos) {
			g.Notices[i] = truncate(avisos[i], NoticeLen-1)
		}
	}
}

// SetSite grava o site no tamanho do campo.
func (g *Guild) SetSite(site string) {
	if g == nil {
		return
	}
	g.Site = truncate(site, SiteLen-1)
}

// LoggedMembers lista os CharIndex dos membros marcados como online.
func (g *Guild) LoggedMembers() []uint32 {
	if g == nil {
		return nil
	}
	var out []uint32
	for i := range g.Members {
		if g.Members[i].CharIndex != 0 && g.Members[i].Logged {
			out = append(out, g.Members[i].CharIndex)
		}
	}
	return out
}

// Registry guarda as guildas de um canal.
//
// Substitui o vetor global "Guilds" e o par
// "Character.Base.GuildIndex / Character.GuildSlot" que o original guardava
// no registro do personagem: ali o slot era um indice no vetor global e
// precisava ser mantido em sincronia com o indice da guilda a mao, em todo
// caminho que mexia em guilda.
type Registry struct {
	mu       sync.Mutex
	byIndex  map[uint16]*Guild
	byName   map[string]uint16
	byMember map[uint32]uint16
	nextID   uint16
}

// NewRegistry cria o registro.
func NewRegistry() *Registry {
	return &Registry{
		byIndex:  make(map[uint16]*Guild, 16),
		byName:   make(map[string]uint16, 16),
		byMember: make(map[uint32]uint16, 64),
		nextID:   1,
	}
}

// Count conta as guildas.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byIndex)
}

// Get devolve a guilda de um indice.
func (r *Registry) Get(index uint16) *Guild {
	if r == nil || index == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byIndex[index]
}

// Of devolve a guilda de um personagem.
func (r *Registry) Of(charIndex uint32) *Guild {
	if r == nil || charIndex == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	idx, ok := r.byMember[charIndex]
	if !ok {
		return nil
	}
	return r.byIndex[idx]
}

// ByName acha a guilda pelo nome, ignorando caixa.
func (r *Registry) ByName(nome string) *Guild {
	if r == nil {
		return nil
	}
	n := normalize(nome)
	if n == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	idx, ok := r.byName[n]
	if !ok {
		return nil
	}
	return r.byIndex[idx]
}

// Create cria uma guilda com o mestre como primeiro membro.
func (r *Registry) Create(nome string, nation uint8, leader Member) (*Guild, error) {
	if r == nil {
		return nil, ErrNoGuild
	}
	nome = strings.TrimSpace(nome)
	if nome == "" || len([]rune(nome)) > NameLen-1 {
		return nil, ErrInvalidName
	}
	if leader.CharIndex == 0 {
		return nil, ErrMemberNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byName[normalize(nome)]; ok {
		return nil, ErrNameTaken
	}
	if _, ok := r.byMember[leader.CharIndex]; ok {
		return nil, ErrAlreadyInGuild
	}
	idx, ok := r.freeIndexLocked()
	if !ok {
		return nil, ErrGuildFull
	}

	g := &Guild{
		Index:    idx,
		Slot:     int(idx),
		Nation:   nation,
		Name:     nome,
		LeaderID: leader.CharIndex,
		Chest:    NewChest(),
	}
	// O mestre entra no rank 0 com todas as permissoes.
	leader.Rank = RankLeader
	leader.Name = truncate(leader.Name, MemberNameLen-1)
	leader.Logged = true
	g.Members[0] = leader
	for i := range g.RanksConfig {
		if i == RankLeader {
			g.RanksConfig[i] = PermInvite | PermKick | PermUseGuildSkill |
				PermEditNotices | PermOpenChest | PermUseChest
		}
	}

	r.byIndex[idx] = g
	r.byName[normalize(nome)] = idx
	r.byMember[leader.CharIndex] = idx
	return g, nil
}

// Put registra uma guilda vinda do banco.
func (r *Registry) Put(g *Guild) {
	if r == nil || g == nil || g.Index == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g.Chest == nil {
		g.Chest = NewChest()
	}
	r.byIndex[g.Index] = g
	if n := normalize(g.Name); n != "" {
		r.byName[n] = g.Index
	}
	for i := range g.Members {
		if g.Members[i].CharIndex != 0 {
			r.byMember[g.Members[i].CharIndex] = g.Index
		}
	}
	if g.Index >= r.nextID {
		r.nextID = g.Index + 1
	}
}

func (r *Registry) freeIndexLocked() (uint16, bool) {
	for i := uint16(0); i < 65534; i++ {
		idx := r.nextID + i
		if idx == 0 {
			continue
		}
		if _, usado := r.byIndex[idx]; !usado {
			r.nextID = idx + 1
			return idx, true
		}
	}
	return 0, false
}

// Join poe um personagem numa guilda.
func (r *Registry) Join(g *Guild, m Member) (int, error) {
	if r == nil || g == nil {
		return -1, ErrNoGuild
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byIndex[g.Index]; !ok {
		return -1, ErrNoGuild
	}
	if _, ok := r.byMember[m.CharIndex]; ok {
		return -1, ErrAlreadyInGuild
	}
	slot, err := g.AddMember(m)
	if err != nil {
		return -1, err
	}
	r.byMember[m.CharIndex] = g.Index
	return slot, nil
}

// Leave tira um personagem da guilda. Devolve a guilda e se ela foi desfeita.
//
// A guilda e desfeita quando o mestre sai e nao ha mais ninguem; com outros
// membros, a lideranca passa ao primeiro da lista.
func (r *Registry) Leave(charIndex uint32) (g *Guild, desfeita bool) {
	if r == nil || charIndex == 0 {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	idx, ok := r.byMember[charIndex]
	if !ok {
		return nil, false
	}
	g = r.byIndex[idx]
	delete(r.byMember, charIndex)
	if g == nil {
		return nil, false
	}
	eraMestre := g.LeaderID == charIndex
	g.RemoveMember(charIndex)

	if g.TotalMembers() == 0 {
		r.destroyLocked(g)
		return g, true
	}
	if eraMestre {
		for i := range g.Members {
			if g.Members[i].CharIndex != 0 {
				g.LeaderID = g.Members[i].CharIndex
				g.Members[i].Rank = RankLeader
				break
			}
		}
	}
	return g, false
}

// Destroy desfaz a guilda e devolve os CharIndex que ficaram sem guilda.
func (r *Registry) Destroy(g *Guild) []uint32 {
	if r == nil || g == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.destroyLocked(g)
}

func (r *Registry) destroyLocked(g *Guild) []uint32 {
	if _, ok := r.byIndex[g.Index]; !ok {
		return nil
	}
	var soltos []uint32
	for i := range g.Members {
		if id := g.Members[i].CharIndex; id != 0 {
			soltos = append(soltos, id)
			if cur, ok := r.byMember[id]; ok && cur == g.Index {
				delete(r.byMember, id)
			}
		}
	}
	delete(r.byIndex, g.Index)
	if n := normalize(g.Name); n != "" {
		if cur, ok := r.byName[n]; ok && cur == g.Index {
			delete(r.byName, n)
		}
	}
	return soltos
}

// All lista as guildas registradas.
func (r *Registry) All() []*Guild {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Guild, 0, len(r.byIndex))
	for _, g := range r.byIndex {
		out = append(out, g)
	}
	return out
}

// truncate corta o texto no numero de runas, sem partir uma no meio.
//
// A origem usava Move com o tamanho do texto de ORIGEM: um nome de mais de 20
// caracteres no banco passava por cima de Rank, ClassInfo, Level e Logged no
// registro do membro.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
