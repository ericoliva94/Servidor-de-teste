// Package combat tem o calculo de dano.
//
// Porte da parte de combate de Server/Src/Mob/BaseMob.pas.
package combat

import "aika/internal/num"

// DamageType e TDamageType de MiscData.pas, na mesma ordem: o valor numerico
// vai no pacote $102 e o cliente desenha a animacao por ele, entao a ordem nao
// pode mudar.
type DamageType uint8

const (
	Normal DamageType = iota
	Critical
	Double
	DoubleCritical
	Immune
	ImmuneCritical
	ImmuneDouble
	ImmuneDoubleCritical
	Miss
	MissCritical
	MissDouble
	MissCritical2
	Miss2
	Miss2Critical
	Miss2Double
	Miss2Critical2
	Block
	BlockCritical
	BlockDouble
	BlockCritical2
	Immune2
	Immune2Critical
	Immune2Double
	Immune2Critical2
	Miss3
	Miss3Critical
	Miss3Double
	Miss3Critical2
	Miss4
	Miss4Critical
	Miss4Double
	Miss4Critical2
	None
)

var damageTypeNames = [...]string{
	"Normal", "Critical", "Double", "DoubleCritical",
	"Immune", "ImmuneCritical", "ImmuneDouble", "ImmuneDoubleCritical",
	"Miss", "MissCritical", "MissDouble", "MissCritical2",
	"Miss2", "Miss2Critical", "Miss2Double", "Miss2Critical2",
	"Block", "BlockCritical", "BlockDouble", "BlockCritical2",
	"Immune2", "Immune2Critical", "Immune2Double", "Immune2Critical2",
	"Miss3", "Miss3Critical", "Miss3Double", "Miss3Critical2",
	"Miss4", "Miss4Critical", "Miss4Double", "Miss4Critical2",
	"None",
}

func (d DamageType) String() string {
	if int(d) < len(damageTypeNames) {
		return damageTypeNames[d]
	}
	return "DamageType(?)"
}

// Attacker reune os atributos do atacante que entram no sorteio.
//
// Na origem vinham de Self.PlayerCharacter; aqui entram por parametro para o
// calculo poder ser testado sem montar um TBaseMob inteiro.
type Attacker struct {
	Critical uint16 // CurrentScore.Critical
	DuploAtk uint16 // PlayerCharacter.DuploAtk
	Acerto   uint16 // CurrentScore.Acerto
	// ForceCrit e o BuffExistsByID(6347) de GetDamageType3: todo golpe que nao
	// errou sai critico. So Roll3 le este campo.
	ForceCrit bool
}

// Defender reune os atributos do alvo.
//
// CritRes e DuploRes ja devem vir somados com as habilidades de mob
// (EF_RESISTANCE6 e EF_RESISTANCE7), como GetDamageType fazia antes de usar.
type Defender struct {
	CritRes  uint16 // GetMobAbility(EF_RESISTANCE6) + PlayerCharacter.CritRes
	DuploRes uint16 // GetMobAbility(EF_RESISTANCE7) + PlayerCharacter.DuploRes
	Esquiva  uint16 // CurrentScore.Esquiva
	// IsPlayer diz se o alvo e jogador. So Roll3 le: contra mob o acerto
	// ganha 20..39 e o atacante com Critico >= 100 critica de vez em quando.
	IsPlayer bool
}

// slotCount e o tamanho da roleta de sorteio (RamdomArray[0..999]).
const slotCount = 1000

// maxRoll e o teto que a origem aplica a soma das chances antes de sortear.
const maxRoll = 998

// Roller monta a roleta de tipos de dano e sorteia nela.
//
// Porte de TBaseMob.GetDamageType. O desenho e o mesmo: um vetor de 1000
// posicoes onde cada chance pinta as primeiras posicoes ainda vazias com o seu
// tipo, e o resultado e a posicao sorteada.
type Roller struct {
	// missCount e o TBaseMob.MissCount de GetDamageType3: depois de dois erros
	// seguidos o terceiro golpe nao pode errar.
	missCount uint16
	slots     [slotCount]DamageType
	total     int
}

// reset limpa a roleta.
//
// A origem nao reiniciava AllChance: a variavel era local e nao inicializada,
// entao a soma das chances comecava num valor de lixo da pilha e o sorteio
// saia de uma faixa aleatoria a cada chamada. Era o bug de GetDamageType.
func (r *Roller) reset() {
	clear(r.slots[:])
	r.total = 0
}

// setChance pinta as primeiras chance posicoes vazias com t.
//
// A soma e acumulada em int, nao em WORD como no original: ali
// "AllChance := AllChance + Chance" estourava silenciosamente com atributos
// muito altos, e um estouro para um valor pequeno encolhia a faixa do sorteio
// em vez de bater no teto de 998.
func (r *Roller) setChance(chance int, t DamageType) {
	if chance <= 0 {
		return
	}
	filled := 0
	for i := range r.slots {
		if filled >= chance {
			break
		}
		if r.slots[i] == Normal {
			r.slots[i] = t
			filled++
		}
	}
	r.total += chance
}

// Rand e a fonte de numeros do sorteio. Receber isto por parametro deixa o
// teste reproduzir uma sequencia.
type Rand interface {
	// IntN devolve um valor em [0, n).
	IntN(n int) int
}

// Roll sorteia o tipo de dano de um golpe.
//
// skill e o indice da skill usada (0 para ataque basico) e isPhysical diz se o
// dano e fisico. Porte de TBaseMob.GetDamageType, incluindo a ordem em que as
// chances sao aplicadas, que importa porque cada uma pinta as posicoes vazias
// que a anterior deixou.
func (r *Roller) Roll(a Attacker, d Defender, isPhysical bool, skill uint32, rnd Rand) DamageType {
	r.reset()

	base := 30
	if isPhysical {
		base = 20
	}
	r.setChance(base, Critical)
	r.setChance(base/2, Miss)

	critHelp := d.CritRes
	var critChance uint16
	if critHelp <= a.Critical {
		critChance = num.DecWord(a.Critical, int(critHelp))
	}
	r.setChance(int(critChance), Critical)

	if isPhysical {
		r.setChance(10, Double)
		r.setChance(20, DoubleCritical)

		var duploChance uint16
		if d.DuploRes <= a.DuploAtk {
			duploChance = num.DecWord(a.DuploAtk, int(d.DuploRes))
		}
		duploHelp := (int(a.DuploAtk) + int(a.Critical)) / 3
		if int(critHelp) >= duploHelp {
			duploHelp = 10
		}
		r.setChance(duploHelp, DoubleCritical)
		r.setChance(int(duploChance), Double)
	} else {
		r.setChance(20, DoubleCritical)
		duploHelp := (int(a.DuploAtk) + int(a.Critical)) / 2
		if duploHelp <= int(critHelp) {
			duploHelp = 20
		}
		r.setChance(duploHelp, DoubleCritical)
	}

	r.setChance(int(num.DecWord(d.Esquiva, int(a.Acerto))), Miss)

	total := r.total
	if total > maxRoll {
		total = maxRoll
	}

	// RandomRange(1, total+1) do Delphi sorteia em [1, total]: a posicao 0 da
	// roleta nunca sai, ainda que seja pintada pela primeira chance. Mantido
	// de proposito — mexer nisso altera a distribuicao que o jogo ja tem.
	res := Normal
	if total > 0 {
		res = r.slots[1+rnd.IntN(total)]
	}

	// Golpe de skill fisica nao conta como duplo.
	if res == Double && skill > 0 && isPhysical {
		res = Normal
	}
	return res
}

// Total devolve a soma das chances da ultima rolagem, antes do teto. Serve
// para teste e diagnostico.
func (r *Roller) Total() int { return r.total }

// SlotsFor conta quantas posicoes da roleta tem o tipo t, na ultima rolagem.
func (r *Roller) SlotsFor(t DamageType) int {
	n := 0
	for _, s := range r.slots {
		if s == t {
			n++
		}
	}
	return n
}
