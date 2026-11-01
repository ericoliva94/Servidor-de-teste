// Package entity tem o nucleo compartilhado por jogador, mob, NPC e pet.
//
// Porte da parte viva de Server/Src/Mob/BaseMob.pas (TBaseMob).
package entity

import "time"

// MaxBuffs e o limite de buffs simultaneos de AddBuff em BaseMob.pas.
const MaxBuffs = 60

// Durations da a duracao de um buff em segundos. Na origem era
// SkillData[Index].Duration, com SkillData sendo um array global
// ARRAY [0..11998]: indice de buff acima disso lia memoria fora do array.
// Aqui a tabela e injetada e responde 0 para indice invalido.
type Durations interface {
	// BuffSeconds devolve a duracao do buff, ou 0 se o indice nao existe.
	BuffSeconds(index uint16) int
}

// Buffs guarda os buffs ativos de uma entidade e quando cada um comecou.
//
// Equivale ao TDictionary<WORD, TDateTime> _buffs do TBaseMob: a chave e o
// indice do buff e o valor e o instante inicial, ao qual se soma a duracao
// para saber quando expira.
type Buffs struct {
	start map[uint16]time.Time
	max   int
}

// NewBuffs cria a lista com o limite padrao.
func NewBuffs() *Buffs {
	return &Buffs{start: make(map[uint16]time.Time, 8), max: MaxBuffs}
}

// Count devolve quantos buffs estao ativos.
func (b *Buffs) Count() int {
	if b == nil {
		return 0
	}
	return len(b.start)
}

// Full diz se o limite de buffs foi atingido.
//
// Na origem, quando o limite era atingido AddBuff mandava mensagem usando
// Servers[ChannelId].Players[ClientID], com Players sendo ARRAY [1..1000]:
// num mob (ClientID acima de 1000) isso indexava fora do array. Aqui a lista
// so informa que esta cheia e quem chama decide o que fazer.
func (b *Buffs) Full() bool {
	if b == nil {
		return true
	}
	return len(b.start) >= b.max
}

// Has diz se o buff esta ativo.
func (b *Buffs) Has(index uint16) bool {
	if b == nil {
		return false
	}
	_, ok := b.start[index]
	return ok
}

// StartedAt devolve o instante inicial do buff.
func (b *Buffs) StartedAt(index uint16) (time.Time, bool) {
	if b == nil {
		return time.Time{}, false
	}
	t, ok := b.start[index]
	return t, ok
}

// Set grava o inicio do buff, substituindo se ja existia. Devolve false
// quando o buff e novo e a lista esta cheia.
func (b *Buffs) Set(index uint16, startedAt time.Time) bool {
	if b == nil {
		return false
	}
	if _, exists := b.start[index]; !exists && b.Full() {
		return false
	}
	b.start[index] = startedAt
	return true
}

// Remove apaga o buff e informa se ele existia.
func (b *Buffs) Remove(index uint16) bool {
	if b == nil {
		return false
	}
	if _, ok := b.start[index]; !ok {
		return false
	}
	delete(b.start, index)
	return true
}

// Indexes lista os buffs ativos. A ordem nao e definida, como no dicionario
// da origem.
func (b *Buffs) Indexes() []uint16 {
	if b == nil {
		return nil
	}
	out := make([]uint16, 0, len(b.start))
	for i := range b.start {
		out = append(out, i)
	}
	return out
}

// Expired lista os buffs cuja duracao terminou em now, sem removo-los.
func (b *Buffs) Expired(now time.Time, d Durations) []uint16 {
	if b == nil || len(b.start) == 0 {
		return nil
	}
	var out []uint16
	for index, started := range b.start {
		if started.Add(time.Duration(d.BuffSeconds(index)) * time.Second).Before(now) {
			out = append(out, index)
		}
	}
	return out
}

// RemoveExpired apaga os buffs vencidos e devolve quais foram.
//
// A origem (TBaseMob.RefreshBuffs) removia do dicionario dentro do
// "for Index in Self._buffs.Keys": o enumerador do TDictionary do Delphi nao
// valida mutacao, entao a remocao rehashava a tabela no meio da iteracao e
// buffs vencidos podiam ser pulados, ficando ativos para sempre. Aqui a lista
// de vencidos e montada primeiro e removida depois.
func (b *Buffs) RemoveExpired(now time.Time, d Durations) []uint16 {
	expired := b.Expired(now, d)
	for _, i := range expired {
		delete(b.start, i)
	}
	return expired
}

// Clear remove todos os buffs.
func (b *Buffs) Clear() {
	if b == nil {
		return
	}
	clear(b.start)
}
