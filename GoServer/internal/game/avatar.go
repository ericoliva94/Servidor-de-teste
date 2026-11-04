package game

import (
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/stats"
	"aika/internal/store"
)

// EquipSlots e o tamanho do vetor de equipamento que o pacote de spawn leva.
const EquipSlots = 8

// Avatar e o personagem em jogo: o que os outros clientes precisam para
// desenhar o boneco.
//
// Na origem isso estava espalhado entre TBaseMob.Character (ponteiro para o
// registro do personagem), TBaseMob.PlayerCharacter e campos soltos de
// TPlayer. Aqui fica num lugar so, porque e exatamente o conjunto que
// GetCreateMob le.
type Avatar struct {
	Name      string
	ClassInfo uint16
	Level     uint16
	Nation    uint16
	// GuildIndex e o indice da guilda; zero e sem guilda.
	GuildIndex uint16
	// GuildName vai no campo Title do pacote de spawn.
	GuildName string

	Str     uint16
	Agility uint16
	Int     uint16
	Cons    uint16
	Luck    uint16

	Altura uint8
	Tronco uint8
	Perna  uint8
	Corpo  uint8

	CurHP uint32
	CurMP uint32
	MaxHP uint32
	MaxMP uint32

	Exp          int64
	Gold         int64
	Honor        uint32
	KillPoint    uint32
	Infamia      uint32
	SkillPoint   uint16
	StatusPoints uint16

	Rotation  uint32
	SpeedMove uint16

	// Atributos derivados, o que a origem recalculava em GetCurrentScore.
	// Ficam zerados enquanto o calculo completo (equipamento, buff, skill)
	// nao estiver portado; ver GoServer/PORTE.md.
	AtkFis         uint16
	DefFis         uint16
	AtkMag         uint16
	DefMag         uint16
	Critical       uint16
	DamageCritical uint16
	Esquiva        uint8
	Acerto         uint8
	DuploAtk       uint16
	Resistence     uint16
	CritRes        uint16
	ResDamageCrit  uint16
	DuploRes       uint16
	FisPenetration uint16
	MagPenetration uint16
	HabAtk         uint16
	CureTax        uint16
	ReduceCooldown uint16

	// Equip leva o indice visivel de cada slot. A origem usava o campo APP
	// (aparencia) quando ele era diferente de zero nos slots 2..7.
	Equip    [EquipSlots]uint16
	EquipAPP [EquipSlots]uint16
	// RefineSlot6 e "Equip[6].Refi div 16", que a origem punha em ItemEff[7].
	RefineSlot6 uint8

	TitleID    uint8
	TitleLevel uint8

	// EquipHPMPBonus e o GetEquipedItensHPMPInc da origem: o bonus de refino
	// do equipamento vestido, que ali era somado tanto ao HP quanto ao MP.
	EquipHPMPBonus int

	// PlayerKill marca quem esta com o nome vermelho: a origem somava $80 ao
	// SpawnType nesse caso.
	PlayerKill bool
	// ShopName, quando preenchido, substitui o Title e muda o EffectType,
	// como a loja pessoal da origem.
	ShopName string
}

// AvatarFromCharacter monta o Avatar a partir da linha do banco.
//
// MaxHP e MaxMP saem da conta de stats; a origem os recalculava a cada spawn
// chamando GetCurrentHP/GetCurrentMP, e aqui ficam guardados e sao
// recalculados por RecalcMax quando algo que entra na conta mudar.
func AvatarFromCharacter(src store.Character, nation uint16) Avatar {
	a := Avatar{
		Name:         src.Name,
		ClassInfo:    src.ClassInfo,
		Level:        src.Level,
		Nation:       nation,
		GuildIndex:   src.GuildIndex,
		Str:          src.Strength,
		Agility:      src.Agility,
		Int:          src.Intelligence,
		Cons:         src.Constitution,
		Luck:         src.Luck,
		Altura:       src.Altura,
		Tronco:       src.Tronco,
		Perna:        src.Perna,
		Corpo:        src.Corpo,
		CurHP:        src.CurHP,
		CurMP:        src.CurMP,
		Exp:          src.Exp,
		Gold:         src.Gold,
		Honor:        src.Honor,
		KillPoint:    src.KillPoint,
		Infamia:      src.Infamia,
		SkillPoint:   src.SkillPoint,
		StatusPoints: src.Status,
		Rotation:     src.Rotation,
		SpeedMove:    src.SpeedMove,
		PlayerKill:   src.PlayerKill,
		TitleID:      uint8(min(src.ActiveTitle, 255)),
	}
	if a.SpeedMove == 0 {
		// Personagem gravado com velocidade zero nao andaria. O original
		// mandava o zero ao cliente e o boneco ficava preso no lugar.
		a.SpeedMove = DefaultSpeedMove
	}
	a.Recalc(stats.Bonus{})

	// A origem enviava CurHP como estava e so o limitava ao maximo dentro de
	// GetCreateMob. Um personagem gravado com CurHP acima do maximo (o que
	// acontece ao trocar equipamento com bonus de HP) aparecia com a barra
	// cheia e morria no primeiro golpe porque o servidor usava o valor cru em
	// outros lugares. Aqui o teto vale a partir da entrada.
	if a.CurHP > a.MaxHP {
		a.CurHP = a.MaxHP
	}
	if a.CurMP > a.MaxMP {
		a.CurMP = a.MaxMP
	}
	// Personagem gravado com zero de vida entraria morto e sem poder fazer
	// nada: a origem tratava isso no login dando 1 de HP.
	if a.CurHP == 0 {
		a.CurHP = 1
	}
	return a
}

// SetEquip copia do conjunto de itens o que o pacote de spawn mostra.
//
// A origem lia Character^.Equip[i].Index e .APP direto do registro. O refino
// do slot 6 ia para ItemEff[7] como "Refi div 16", porque o campo guarda o
// nivel de refino nos 4 bits altos do byte baixo.
func (a *Avatar) SetEquip(set store.ItemSet) {
	for i := 0; i < EquipSlots; i++ {
		it := set.Get(i)
		a.Equip[i] = it.Index
		a.EquipAPP[i] = it.APP
	}
	// ItemEff[7] recebia "Equip[6].Refi div 16": divisao inteira, nao o
	// Round que GetReinforceFromItem usa. Os dois valores diferem para metade
	// dos refinos, e e assim na origem.
	a.RefineSlot6 = uint8(set.Get(6).Refi / 16 % 256)
}

// ApplyEquipBonus recalcula o bonus de refino do equipamento e os maximos.
func (a *Avatar) ApplyEquipBonus(t itemfn.Tabelas, set store.ItemSet) {
	if t == nil {
		return
	}
	a.EquipHPMPBonus = itemfn.EquippedHPMPInc(t, set.Slots)
	a.Recalc(stats.Bonus{})
	// O teto novo pode ser menor que o HP gravado: trocar de equipamento para
	// um sem bonus reduz o maximo, e a origem deixava CurHP acima dele.
	if a.CurHP > a.MaxHP {
		a.CurHP = a.MaxHP
	}
	if a.CurMP > a.MaxMP {
		a.CurMP = a.MaxMP
	}
}

// RecalcDerived refaz os atributos calculados a partir dos pontos.
//
// Porte de TBaseMob.GetCurrentScore. Os bonus de equipamento, buff, skill e
// reliquia entram pelo parametro quando esses sistemas estiverem portados;
// hoje chegam zerados, e o resultado e o do personagem nu.
func (a *Avatar) RecalcDerived(b stats.Bonus) {
	d := stats.Compute(stats.Score{
		Str: a.Str, Agility: a.Agility, Int: a.Int,
		Cons: a.Cons, Luck: a.Luck,
	}, b)

	a.AtkFis = d.AtkFis
	a.AtkMag = d.AtkMag
	a.DefFis = d.DefFis
	a.DefMag = d.DefMag
	a.DuploAtk = d.DuploAtk
	a.Critical = d.Critical
	a.DamageCritical = d.DamageCritical
	a.FisPenetration = d.FisPenetration
	a.MagPenetration = d.MagPenetration
	a.HabAtk = d.HabAtk
	a.CureTax = d.CureTax
	a.CritRes = d.CritRes
	a.ResDamageCrit = d.ResDamageCrit
	a.DuploRes = d.DuploRes
	a.Acerto = d.Acerto
	a.Esquiva = d.Esquiva
	a.Resistence = d.Resistence
	a.ReduceCooldown = d.ReduceCooldown

	// SpeedMove vem da conta, mas a coluna do banco manda quando ela traz um
	// valor: a origem gravava speedmove por personagem e o recalculo o
	// sobrescrevia, entao montaria e buff de velocidade sumiam a cada
	// recalculo. Aqui o valor gravado so cede se for zero.
	if a.SpeedMove == 0 {
		a.SpeedMove = d.SpeedMove
	}
}

// RecalcMax refaz HP e MP maximos.
func (a *Avatar) RecalcMax() {
	in := stats.Input{
		ClassInfo: a.ClassInfo,
		Level:     a.Level,
		Cons:      a.Cons,
		Luck:      a.Luck,
		FlatHP:    a.EquipHPMPBonus,
		FlatMP:    a.EquipHPMPBonus,
	}
	a.MaxHP = stats.MaxHP(in)
	a.MaxMP = stats.MaxMP(in)
}

// Recalc refaz os derivados e os maximos de uma vez.
func (a *Avatar) Recalc(b stats.Bonus) {
	a.RecalcDerived(b)
	a.RecalcMax()
}

// VisibleEquip devolve o indice que vai no pacote para cada slot.
//
// Porte da regra de GetCreateMob: os slots 0 e 1 vao sempre com o indice real
// e os slots 2..7 vao com a aparencia quando ela existe.
func (a *Avatar) VisibleEquip() [EquipSlots]uint16 {
	var out [EquipSlots]uint16
	for i := range out {
		if i >= 2 && a.EquipAPP[i] != 0 {
			out[i] = a.EquipAPP[i]
			continue
		}
		out[i] = a.Equip[i]
	}
	return out
}

// Status monta o bloco de atributos para os pacotes que o pedem.
func (a *Avatar) Status() model.Status {
	return model.Status{
		Str:     a.Str,
		Agility: a.Agility,
		Int:     a.Int,
		Cons:    a.Cons,
		Luck:    a.Luck,
		Sizes: model.Sizes{
			Altura: a.Altura, Tronco: a.Tronco,
			Perna: a.Perna, Corpo: a.Corpo,
		},
		CurHP: a.CurHP,
		CurMp: a.CurMP,
	}
}
